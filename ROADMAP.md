# Roadmap: Code Search Layer for mcp-gopls-plus

> Inspired by [Semble](https://github.com/MinishLab/semble) and [Claude Context](https://github.com/zilliztech/claude-context), grounded in raw-material analysis of production-scale AI coding retrieval systems.

---

## Why this exists

`mcp-gopls-plus` already provides best-in-class LSP navigation for Go — go-to-definition, references, hover, diagnostics, test, coverage. What's missing is a **fuzzy entry point**.

The problem, from the raw materials:

> "AI Coding 的很多成败，不取决于模型会不会写代码，而取决于系统能不能先把**真正相关的代码上下文**送进窗口。用户给的是 issue 描述、重构意图或 Bug 现象，系统要在大仓库里快速找到该读的那几段代码，而不是让模型靠反复猜关键词、反复读无关文件来撞运气。"
> 
> *"Much of AI coding success or failure isn't about whether the model can write code — it's about whether the system can get the **truly relevant code context** into the context window first. Users provide issue descriptions, refactoring intent, or bug symptoms; the system must quickly locate those specific code sections in a large repository — not leave the model guessing keywords and reading irrelevant files blindly."*

Concretely, a real-world benchmark from the Claude Context authors:

| Metric | Pure grep (Claude Code native) | With Claude Context MCP |
|--------|-------------------------------|------------------------|
| Tool calls to find a Django bug | 8–11 | **3** |
| Token consumption | 130K | **9K** (93% less) |
| Hit rate | 0% | **50%+** |

The cost of not having a search layer isn't just "slower" — it's **blind guesswork compounding into context bloat.** 99% of what the agent reads is irrelevant to the task.

This roadmap adds the missing fuzzy-entry capability to `mcp-gopls-plus`, Go-native and zero-dependency.

---

## Hard constraints (learned from the raw materials)

These are not optional — they come from real production-scale failures documented in the raw materials.

### 1. Tool returns metadata, never raw source code

The MCP server for the 180K-line Spring Boot project had this exact failure mode:

```
// WRONG — their first version
tool: get_service_code
return: "public class OrderService { \n  @Autowired\n  PaymentService..."  // 5000+ tokens
```

> "这等于把全量读文件的问题移到了工具调用层面，治标不治本。"
> 
> *"This is just relocating the full-file-read problem to the tool-call layer — treating the symptom, not the cause."*

The fix: three-layer retrieval. The search tool only returns **structured metadata** (file, symbol, line range, score, doc-comment summary). The agent decides whether to navigate deeper via existing LSP tools (`go_to_definition`, `find_references`) or directly `read` the specific lines.

### 2. Tool descriptions must be disposable (≤ 20 tokens)

The same team measured: tool descriptions alone consumed **41% of the context window (82K tokens)** before any conversation even started. Their fix:

```
// Before (87 tokens): "This tool allows you to search for dependencies between
// microservices in our Spring Boot monolithic architecture. Provide a service
// name to get a complete list of all services that depend on it..."

// After (15 tokens): "Query service dependency graph. service_name: target service."
```

Rule: **description tells what the tool does, never how to use it.** The parameter schema handles the "how."

### 3. Tool count is a hard constraint

60 tools → Claude silently drops tools with no error. 12 tools → stable. With LLMs showing "cognitive overload" above 10–20 tools, every addition must justify itself against the existing 14 tools.

### 4. The index must be "cheap enough to always stay on"

Semble's key insight: **270ms indexing, 1.5ms query, pure CPU, zero install steps.** If the search tool requires manual setup, API keys, or visible latency, agents and users will just fall back to `grep`. The index must be:
- Built on startup automatically
- Memory-resident (no disk round-trip per query)
- Zero configuration

---

## Phase 1 — Fuzzy entry + file structure map

**Two new MCP tools.** Two deprecations. Net tool count stays at 14. No external dependencies. Go stdlib only.

### 1a. `code_search` — fuzzy entry into precise LSP navigation

### The agent workflow this enables

```
Agent receives: "这个项目怎么处理鉴权的？(How does this project handle authentication?)"
  │
  ├─[1] code_search("authentication handling")     ← fuzzy entry, ~200 tokens return
  │     → [{file:"pkg/auth/handler.go", symbol:"HandleLogin",
  │         score:0.92, summary:"Handles HTTP login, validates JWT"}]
  │     Agent reads summary, decides HandleLogin is worth investigating
  │
  ├─[2] go_to_definition("pkg/auth/handler.go", 42, 1)  ← precise LSP navigation
  │     → locations [{file:"pkg/auth/handler.go", line:42..89}]
  │
  ├─[3] read("pkg/auth/handler.go", lines=42-89)         ← read only 47 lines
  │
  └─[4] find_references("pkg/auth/handler.go", 42, 1)    ← find all callers
        → middlewares/auth.go, cmd/server.go ...
```

4 tool calls, each carrying minimal context. No blind grep rounds. No full-file reads before the target is locked.

### MCP tool signature

```
code_search(
  query: string,          # natural language or identifier
  top_k?: number,         # default 10
  include_tests?: bool    # default false
)
→ {
  results: [{
    file: string,         # e.g. "pkg/auth/handler.go"
    symbol: string,       # e.g. "HandleLogin"
    kind: string,         # "function" | "method" | "type" | "const" | "var"
    start_line: number,
    end_line: number,
    score: number,        # 0.0-1.0
    summary: string       # from doc comment, or auto-generated from signature
  }]
}
```

Key: **no `content` field.** The tool returns a map, not the territory. The agent uses existing tools (`go_to_definition`, `read`) to fetch actual code only after deciding a result is relevant.

### Tool description

```
"Find code by natural language or identifier query. Returns file, symbol, line range, score, and summary."
```
~18 tokens. Covers the what, not the how.

### New packages

```
pkg/search/
├── chunk.go       # AST-aware chunking via go/parser + go/ast
├── tokenize.go    # Identifier-aware tokenization (camelCase/snake_case splitting)
├── bm25.go        # BM25 scoring engine
├── index.go       # In-memory index build, query, serialization
├── rank.go        # Re-ranking signals
└── search.go      # Public API

pkg/tools/search.go  # MCP tool registration + handler
```

### Design decisions

**AST chunking — `go/parser` + `go/ast`.**
Split at `FuncDecl`, `TypeSpec`, and method boundaries. Each chunk is a complete syntactic unit. Fallback: line-based splitting for non-`.go` files (e.g. `go.mod`). This is strictly better than Semble/Claude Context for Go because `go/ast` is a first-class standard library, not a cross-language Tree-sitter parser with Go as an afterthought.

**Identifier-aware tokenization.**
`ParseConfig` → `["parse", "config"]`. `getUserByID` → `["get", "user", "by", "id"]`. Go identifiers carry strong semantics by convention. Combined with keyword stop-word removal, this closes most of the gap that embedding models fill in multi-language tools — at zero cost.

**BM25 scoring, not vector embeddings.**
BM25 is deterministic, zero-dependency, and performs well on token-matched queries. A user querying `"parse config"` will match `ParseConfig` (after tokenization) even though the exact string is different. Compared to Semble's BM25+Model2Vec hybrid, we lose the pure semantic leg — but gain zero deps, zero install steps, and Go's identifier convention already encodes much of the semantics.

**Summary from doc comments.**
Go convention: `// HandleLogin authenticates the user and returns a JWT.` → `summary: "Handles HTTP login, validates credentials, returns JWT"`. If no doc comment exists, auto-generate from the function signature (`func HandleLogin(w http.ResponseWriter, r *http.Request)` → `summary: "HandleLogin(w http.ResponseWriter, r *http.Request)"`).

**Re-ranking signals:**
- **Definition boost**: chunks defining a matched token (`func Foo`) rank above chunks merely referencing it (`foo.Bar()`).
- **Noise penalty**: `_test.go`, `mock/`, `compat/`, `legacy/`, `vendor/`, `.pb.go` patterns down-weighted.
- **File coherence**: when multiple chunks from the same file match, the file's top result gets a boost.
- **Identifier stem match**: query tokens matched against split identifier stems in the chunk.

### 1b. `file_outline` — file structure without reading the file

**MCP tool.** Uses gopls `textDocument/documentSymbol` (already supported by the LSP client's generic `call()` method — no new client interface needed).

```
file_outline(file_uri: string)
→ {
  symbols: [{
    name: string,         # e.g. "Register", "LSPTools"
    kind: string,         # "function" | "method" | "struct" | "interface" | "variable" | "package"
    start_line: number,
    end_line: number,
    children: [...]       # nested symbols (methods of a struct, etc.)
  }]
}
```

**Why this matters for agents.** Today, when an agent wants to understand what's in a Go file, it calls `read(file)` and gets the entire source — hundreds of lines, 5000+ tokens. With `file_outline`, the agent first sees the file's structure: function names, type names, method lists. It then decides which specific function to dive into via `go_to_definition` or `read(lines=N..M)`.

This is the **intent recognition layer** from the structured-retrieval-three-layer principle. Before reading code, the agent gets a map.

Minimal description: `"List symbols (functions, types, methods) defined in a Go file. Returns name, kind, and line range for each."` (~20 tokens).

**Implementation.** The gopls client already calls arbitrary LSP methods via `invoke(ctx, method, params)`. No new `LSPClient` interface method needed. The tool handler sends `textDocument/documentSymbol` and maps the response into the output schema.

### Tool count after Phase 1

| Action | Count |
|--------|-------|
| Current | 14 |
| + `code_search` | 15 |
| + `file_outline` | 16 |
| − `search_workspace_symbols` (subsumed by `code_search`) | 15 |
| − `get_completion` (low agent utility; LLMs are completion engines) | **14** |

Net: **14 tools** (unchanged). Two in, two out.

### Deprecation rationale

- **`search_workspace_symbols` → `code_search`**. The former only does LSP symbol prefix matching. The latter does full BM25 over AST-aware chunks with identifier-aware tokenization — strictly more powerful, yet returns structured metadata (file + symbol + line range + score + summary) instead of raw symbol info.
- **`get_completion` → removed**. Code completion is an IDE interaction primitive, not an agent tool. An LLM-powered agent can already suggest code. The completion list that gopls returns (label strings) has near-zero information value to an LLM that isn't rendering an autocomplete popup.

**Index lifecycle.**
Built at server startup via a file-system walk. Memory-resident (a `map[string]*IndexEntry`). On the scale of real Go monorepos, even 50K+ functions, the BM25 index is _tiny_ compared to the embedding tensors Semble stores. Serialization to disk deferred to Phase 2.

---

## Phase 2 — Index persistence + incremental update + call hierarchy

**Goal:** Survive server restarts; avoid full re-index on file changes; expose call-graph relationships.

- SHA256 per-file hash snapshot.
- Serialize/deserialize BM25 index to `~/.mcp-gopls-plus/index/` (gob-encoded, < 10MB for typical projects).
- On server start: compute workspace hash → load cached index if unchanged.
- Hook into existing `fs.Watcher` for per-file invalidation (debounced ~500ms).

### MCP tools

#### 2a. Index management (2 tools → 1 via parameterization)

Rather than two separate tools, a single parameterized tool to keep the count down:

```
manage_index(
  action: "status" | "rebuild",  # default: "status"
) → {
  indexed_files: number,
  last_indexed_at: string,
  pending_changes: number,
  index_size_bytes: number
}
```

When `action: "rebuild"`, force-reindexes and returns the updated stats. Tool description: `"Check index status or force rebuild. Use action=rebuild to re-index the workspace."` (~14 tokens).

#### 2b. `call_hierarchy` — incoming/outgoing call graph

**MCP tool.** Uses gopls `textDocument/prepareCallHierarchy` + `callHierarchy/incomingCalls` / `callHierarchy/outgoingCalls`.

```
call_hierarchy(
  file_uri: string,
  line: number,
  character: number,
  direction: "incoming" | "outgoing"  # default: "both"
) → {
  symbol: string,
  incoming: [{ file, symbol, line, character }],   # who calls this
  outgoing: [{ file, symbol, line, character }]     # who this calls
}
```

**Why this matters.** `find_references` returns all references to a symbol — including imports, type annotations, string mentions, and comments. The agent then has to filter out the noise. `call_hierarchy` returns **only call-site relationships**: who actually calls `HandleLogin`, and what `HandleLogin` calls in turn. For multi-hop reasoning ("if I change this return value, what downstream code breaks?"), this is the deterministic graph primitive that vector search alone cannot provide.

Tool description: `"Show callers and callees of a function. direction: incoming, outgoing, or both."` (~14 tokens).

### Tool count after Phase 2

| Action | Count |
|--------|-------|
| After Phase 1 | 14 |
| + `manage_index` | 15 |
| + `call_hierarchy` | 16 |
| − `format_document` (agent can use `gofmt` or write formatted code directly) | **15** |

Net: **15 tools** (Phase 1: 14 → 14; Phase 2: 14 → 15). Well within the safe zone established by the Spring Boot team's finding (≤ 16 for reliable tool discovery).

### Deprecation rationale

- **`format_document` → removed**. The tool returns a list of `TextEdit` objects that the agent would need to apply. In practice, agents write Go code that is already gofmt-compliant, or they can run `gofmt -w file.go` as a shell command. The LSP formatting round-trip adds latency without meaningful gain for an agent context.

A remaining candidate for future deprecation: `run_govulncheck`. Security vulnerability scanning is orthogonal to the code-retrieval mission; agents can achieve the same result with `govulncheck ./...` as a shell command. No MCP wrapper needed. `rename_symbol` stays — it is on the code-modification axis and serves a refactoring purpose that is harder to replicate with a simple shell command.

---

## Phase 3 — Code graph expansion (tentative)

**Goal:** Extend beyond call hierarchy into broader structural queries.

With `call_hierarchy` handling the "who calls who" question in Phase 2, Phase 3 would expand into:

- **Interface implementation graph**: given an interface, show the full tree of implementations → callers of those implementations → callees. A single query that answers "this interface is implemented by X, Y, Z; X is called by A, B; Y is called by C."
- **Import graph reversal**: given a package, show which other packages in the workspace import it.
- **Change impact analysis**: given a file diff, identify which call paths intersect the changed region.

Most of this builds on batch `documentSymbol` + `references` queries through gopls, similar to GitNexus's approach of pre-computing the codebase as a queryable structure graph.

Deferred: significant complexity. Revisit when Phase 1 + Phase 2 prove insufficient for real-world Go codebase tasks.

---

## Non-goals

| Non-goal | Why |
|----------|-----|
| Multi-language support | `gopls` anchors this project to Go. Multi-language code search already exists (Semble, Claude Context). |
| Cloud vector database | No Milvus, no Zilliz Cloud. Zero-external-dependency Go tooling is the value proposition. |
| Semantic embedding models | No Model2Vec, no OpenAI, no Ollama. User can wire Semble alongside mcp-gopls-plus if they need dense-vector search. |
| IDE extension / web UI | MCP is the interface layer. |

---

## Risk: Agent behavior is not controllable

The ideal workflow (`code_search` → `go_to_definition` → `read specific lines`) is a target path, not an enforceable SOP. Agents can and will deviate. Three failure modes observed in production (from the raw materials):

| Failure mode | Example | Root cause |
|-------------|---------|------------|
| Skips your tool entirely | Agent uses `grep "auth"` instead of calling `code_search` | grep is a built-in; MCP tools require conscious selection |
| Uses your tool, then reads entire files | `code_search` → `read("handler.go")` (500 lines) | Agent defaults to full-file reads when it lacks confidence in the summary |
| Your tool disappears silently | Agent says "I don't have a code search tool" | Tool count overflow + context window saturation |

### Defense 1: Tool count discipline (by subtraction, not just addition)

Every new tool increases the probability of silent tool loss. Strategy:

- Phase 1: add `code_search` + `file_outline` (+2). Deprecate `search_workspace_symbols` + `get_completion` (−2). Net: 14 → 14.
- Phase 2: add `manage_index` + `call_hierarchy` (+2). Deprecate `format_document` (−1). Net: 14 → 15.
- Long-term target: ≤ 16 tools. If new tools are needed, old ones must be merged or removed.
- Deprecation candidates on watch: `run_govulncheck` (orthogonal to code retrieval; trivially replaced by shell command).

This directly follows the 180K-line Spring Boot team's finding: 60 tools → silent loss; 12 tools → stable. Our 14–15 tools sit in the safe zone.

### Defense 2: Return format as a path constraint

`code_search` deliberately omits the `content` field. By returning only `{file, symbol, start_line, end_line, score, summary}`, the agent is structurally guided toward the next step being `go_to_definition(file, line)` or `read(file, start_line, end_line)` — both low-token operations. You cannot force the agent *not* to `read(file)` (full file), but you remove the path of least resistance.

This is the same principle the Spring Boot team arrived at: "工具只返回结构化的元信息，原始代码文本按需提供" (*"Tools should only return structured metadata; raw source code is provided on demand"*).

### Defense 3: Curated prompt to seed the workflow

mcp-gopls-plus already has a Prompts mechanism (`summarize_diagnostics`, `refactor_plan`). Add a `search_workflow` prompt that the agent (or user) can invoke at session start:

```
When searching for code in this Go workspace:
1. Start with code_search for fuzzy / natural-language queries.
   - Returns file, symbol, line range, score, and doc-comment summary.
2. Use go_to_definition from search results to confirm exact locations.
3. Use find_references to trace callers / callees when needed.
4. Use read with explicit start/end lines, not entire files.
5. Fall back to grep only for exhaustive literal matches on known strings.
```

This is not enforcement — it's a lightweight "suggested procedure" analogous to `CLAUDE.md`. The prompt is ~100 tokens, invoked once per session or loaded automatically.

### Defense 4: Observable, not assumed

Embed lightweight telemetry in `code_search`:

```
On each call, log: {query, result_count, top_score, duration_ms}
On server shutdown, dump: {total_calls, avg_results, avg_duration}
```

If after real-world use the data shows:
- **Low call rate**: agent isn't discovering the tool → re-evaluate tool name/description.
- **High call rate but followed by full-file reads**: summary quality insufficient → improve doc-comment extraction.
- **Zero calls in some sessions**: possible tool loss → reduce total tool count.

The metric is not "did the agent follow the exact workflow" — it's "did the agent find the target with fewer tool calls and less context."

### Defense 5: Progressive disclosure via workspace resource

mcp-gopls-plus already exposes `resource://workspace/overview`. Extend it (or add a parallel `resource://workspace/search-guide`) so the agent can self-discover the intended search workflow without the user manually invoking a prompt. The resource is loaded once and cached by MCP clients.

---

## Appendix A: Capability mapping to Wiki principles

| Wiki principle | Current (mcp-gopls-plus) | Phase 1 | Phase 2 |
|----------------|--------------------------|---------|---------|
| AST-aware code chunking | None | `go/parser` + `go/ast` at func/type boundaries | — |
| Change awareness | `fs.Watcher` (raw events) | — | SHA256 snapshot + `manage_index` incremental re-index |
| Lightweight local search | LSP symbol search only | `code_search`: BM25 + identifier-aware tokens, < 1ms query | Persisted to disk |
| Structured 3-layer retrieval | Implicit (symbol → def → read) | `code_search` (metadata) + `file_outline` (map) → LSP navigation | — |
| Call graph / deterministic relationships | `find_references` (noisy, includes non-call refs) | — | `call_hierarchy` (call-site only, incoming + outgoing) |
| Code graph (full) | `module_graph` (inter-module) | — | — (Phase 3) |
| LSP precise navigation | Complete | Unchanged | Unchanged |

## Appendix B: Comparison with Semble & Claude Context

| Dimension | Semble | Claude Context | mcp-gopls-plus (post Phase 1) |
|-----------|--------|----------------|-------------------------------|
| Scope | All languages | All languages | **Go only** |
| Chunking | AST (Chonkie) | AST (Tree-sitter) | **AST (go/parser — first-class stdlib)** |
| Search | Model2Vec + BM25 + RRF | Dense vector + BM25 + Milvus | **BM25 + identifier-aware tokens** |
| Result format | Code snippets | Code snippets | **Metadata only (map, not territory)** |
| External deps | Python + uv | Node.js + API keys + cloud DB | **Zero (Go stdlib only)** |
| Index time | ~250ms | Network-bound | **< 100ms** (pure Go, in-memory) |
| Query time | ~1.5ms | Network-bound | **< 1ms** (in-memory map lookup) |
| MCP tools | `search`, `find_related` | 4 tools | `code_search` + 14 LSP tools |
| Unique strength | Fastest all-lang semantic search | Scalable team infra | **End-to-end Go: fuzzy search → LSP navigate → test → refactor** |

## Appendix C: Key raw-material references

- [Claude Code与Gemini放弃代码索引，是一步烂棋](https://mp.weixin.qq.com/s/C1h6QveDrX_-yDxwI1CNUA) — grep vs RAG debate, 40% token savings, Claude Context architecture
- [比grep快还准，3.1k Star的代码搜索工具专门为AI Agent而生](https://mp.weixin.qq.com/s/cYaxRW7hrC5OnanP57DB0w) — Semble: lightweight embedding, 250ms/1.5ms, zero deps
- [别再让 Claude Code 全量读代码了，搭一套 MCP 检索层才是大代码库正解](https://mp.weixin.qq.com/s/J4Af-Nty5e2-RIt75H8ZwQ) — Tool count constraint, description token budget, 3-layer retrieval design
- [主流AI IDE的token成本爆炸？试试登上GitHub日榜的Claude Context！](https://mp.weixin.qq.com/s/tUzyz4qH2KaBz1GMgjE3-A) — Claude Context evaluation, YearLookup/swap_dims case studies
- [Karpathy 的 LLM Wiki，被搬进代码仓库](https://mp.weixin.qq.com/s/oZn2r_jqJSkB1_AVmoL-VA) — GitNexus: code-as-graph, persistent structure vs. temporary context
