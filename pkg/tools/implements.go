package tools

import (
	"context"
	"fmt"
	"go/types"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/tools/go/packages"
)

// implementor describes a named type implementing a target interface.
type implementor struct {
	Package string `json:"package"`
	Type    string `json:"type"`
	FileURI string `json:"file_uri"`
	Line    int    `json:"line"`
	Char    int    `json:"character"`
}

func (t *LSPTools) registerImplementsTools(s *server.MCPServer) {
	t.registerFindImplementations(s)
}

func (t *LSPTools) registerFindImplementations(s *server.MCPServer) {
	tool := mcp.NewTool("find_implementations",
		mcp.WithDescription("Find all named types in the workspace that implement a given interface"),
		mcp.WithTitleAnnotation("Find Implementations"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("interface_name",
			mcp.Required(),
			mcp.Description("Name of the interface type (e.g. \"LSPClient\")"),
		),
		mcp.WithString("package_path",
			mcp.Description("Optional import path to narrow the search (e.g. \"github.com/ForeverSRC/mcp-gopls-plus/pkg/lsp/client\"). If omitted, searches all packages loaded by the workspace."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := getArguments(request)
		if err != nil {
			return nil, err
		}

		ifaceName, err := getStringArg(args, "interface_name")
		if err != nil {
			return nil, err
		}

		pkgPath, _ := getStringArg(args, "package_path")

		results, err := t.findImplementations(ctx, ifaceName, pkgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to find implementations: %w", err)
		}

		payload := map[string]any{
			"interface_name": ifaceName,
			"implementors":   results,
			"count":          len(results),
		}
		return mcp.NewToolResultJSON(payload)
	})
}

func (t *LSPTools) findImplementations(ctx context.Context, ifaceName, pkgPath string) ([]implementor, error) {
	mode := packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo

	cfg := &packages.Config{
		Context: ctx,
		Mode:    mode,
		Dir:     t.workspaceDir,
		Tests:   false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}

	if packages.PrintErrors(pkgs) > 0 {
		// Non-fatal: continue with packages that loaded successfully
	}

	// Locate the target interface and its package.
	var targetIface *types.Interface
	var targetPkg *types.Package

	if pkgPath != "" {
		// User specified a package — only look there.
		for _, p := range pkgs {
			if p.PkgPath == pkgPath {
				targetPkg = p.Types
				if targetPkg == nil {
					return nil, fmt.Errorf("package %q loaded without type information", pkgPath)
				}
				obj := targetPkg.Scope().Lookup(ifaceName)
				if obj == nil {
					return nil, fmt.Errorf("interface %q not found in package %q", ifaceName, pkgPath)
				}
				named, ok := obj.Type().(*types.Named)
				if !ok {
					return nil, fmt.Errorf("%q is not a named type in package %q", ifaceName, pkgPath)
				}
				targetIface, ok = named.Underlying().(*types.Interface)
				if !ok {
					return nil, fmt.Errorf("%q is not an interface in package %q", ifaceName, pkgPath)
				}
				break
			}
		}
		if targetIface == nil {
			return nil, fmt.Errorf("package %q not found in workspace", pkgPath)
		}
	} else {
		// Scan all packages to find the interface.
		for _, p := range pkgs {
			if p.Types == nil {
				continue
			}
			obj := p.Types.Scope().Lookup(ifaceName)
			if obj == nil {
				continue
			}
			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}
			iface, ok := named.Underlying().(*types.Interface)
			if !ok {
				continue
			}
			if targetIface != nil {
				return nil, fmt.Errorf("found %q in multiple packages; please specify --package_path to disambiguate", ifaceName)
			}
			targetIface = iface
			targetPkg = p.Types
		}
		if targetIface == nil {
			return nil, fmt.Errorf("interface %q not found in workspace", ifaceName)
		}
	}

	if targetIface.NumExplicitMethods() == 0 && targetIface.NumEmbeddeds() == 0 {
		return nil, fmt.Errorf("%q is an empty interface (all types implement it)", ifaceName)
	}

	// Walk every loaded package and check concrete named types.
	var results []implementor
	seen := make(map[string]bool) // dedup by "pkg.type"

	for _, p := range pkgs {
		if p.Types == nil || p.Types.Scope() == nil {
			continue
		}

		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			// Skip unexported types from other packages.
			if !obj.Exported() && p.Types != targetPkg {
				continue
			}
			// Must be a type name.
			tname, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			// Skip the interface itself.
			if p.Types == targetPkg && name == ifaceName {
				continue
			}

			named, ok := tname.Type().(*types.Named)
			if !ok {
				continue
			}

			// Check the type itself.
			if implementsStrict(named, targetIface) {
				loc := t.locateType(p, tname)
				key := loc.PkgPath + "." + loc.TypeName
				if !seen[key] {
					results = append(results, implementor{
						Package: loc.PkgPath,
						Type:    loc.TypeName,
						FileURI: loc.FileURI,
						Line:    loc.Line,
						Char:    loc.Char,
					})
					seen[key] = true
				}
			}

			// Also check *T (pointer receiver).
			ptr := types.NewPointer(named)
			if implementsStrict(ptr, targetIface) {
				loc := t.locateType(p, tname)
				key := loc.PkgPath + ".*" + loc.TypeName
				if !seen[key] {
					results = append(results, implementor{
						Package: loc.PkgPath,
						Type:    "*" + loc.TypeName,
						FileURI: loc.FileURI,
						Line:    loc.Line,
						Char:    loc.Char,
					})
					seen[key] = true
				}
			}
		}
	}

	return results, nil
}

// implementsStrict checks whether V implements interface T, excluding
// the trivial case where V is itself an interface.
func implementsStrict(V types.Type, T *types.Interface) bool {
	if _, ok := V.Underlying().(*types.Interface); ok {
		return false
	}
	return types.Implements(V, T)
}

type typeLocation struct {
	PkgPath  string
	TypeName string
	FileURI  string
	Line     int
	Char     int
}

// locateType finds the source position of a type name in its package files.
func (t *LSPTools) locateType(pkg *packages.Package, tname *types.TypeName) typeLocation {
	pos := tname.Pos()
	fset := pkg.Fset
	file := fset.File(pos)
	posn := file.Position(pos)
	line := posn.Line - 1   // 0-indexed
	char := posn.Column - 1 // 0-indexed

	uri := file.Name()
	if !strings.HasPrefix(uri, "file://") {
		uri = convertPathToURI(uri)
	}

	return typeLocation{
		PkgPath:  pkg.PkgPath,
		TypeName: tname.Name(),
		FileURI:  uri,
		Line:     line,
		Char:     char,
	}
}
