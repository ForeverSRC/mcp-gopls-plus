package client

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func resolveGoplsExecutable(explicit string) (string, error) {
	if candidate := strings.TrimSpace(explicit); candidate != "" {
		return validateGoplsPath(candidate)
	}

	if path, err := exec.LookPath("gopls"); err == nil {
		return path, nil
	}

	if path, ok := searchGoBinDirs(); ok {
		return path, nil
	}

	return "", errors.New("gopls not found; install via `go install golang.org/x/tools/gopls@latest` or set MCP_GOPLS_BIN")
}

func validateGoplsPath(candidate string) (string, error) {
	if path, err := exec.LookPath(candidate); err == nil {
		return path, nil
	}

	abs := candidate
	if !filepath.IsAbs(abs) {
		if cwd, err := os.Getwd(); err == nil {
			abs = filepath.Join(cwd, candidate)
		}
	}
	abs = filepath.Clean(abs)

	if isExecutableFile(abs) {
		return abs, nil
	}

	return "", fmt.Errorf("gopls executable not found at %q", candidate)
}

func searchGoBinDirs() (string, bool) {
	name := goplsBinaryName()
	for _, dir := range goBinDirs() {
		candidate := filepath.Join(dir, name)
		if isExecutableFile(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func goBinDirs() []string {
	var dirs []string
	seen := make(map[string]struct{})

	add := func(dir string) {
		if dir == "" {
			return
		}
		cleaned := filepath.Clean(dir)
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		dirs = append(dirs, cleaned)
	}

	add(os.Getenv("GOBIN"))

	for _, entry := range splitPathList(os.Getenv("GOPATH")) {
		if entry == "" {
			continue
		}
		add(filepath.Join(entry, "bin"))
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add(filepath.Join(home, "go", "bin"))
	}

	return dirs
}

func splitPathList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return strings.Split(value, string(os.PathListSeparator))
}

func goplsBinaryName() string {
	if runtime.GOOS == "windows" {
		return "gopls.exe"
	}
	return "gopls"
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func resolveWorkspace(dir string) (string, string, error) {
	var err error
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("determine working directory: %w", err)
		}
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace path: %w", err)
	}

	dir = findGoRoot(dir)
	if stat, statErr := os.Stat(dir); statErr != nil || !stat.IsDir() {
		return "", "", fmt.Errorf("workspace directory invalid: %w", statErr)
	}

	return dir, pathToURI(dir), nil
}

func findGoRoot(start string) string {
	current := start
	for {
		if fileExists(filepath.Join(current, "go.work")) || fileExists(filepath.Join(current, "go.mod")) {
			return current
		}
		next := filepath.Dir(current)
		if next == current {
			return start
		}
		current = next
	}
}

func pathToURI(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ReplaceAll(path, "\\", "/")
		if len(path) >= 2 && path[1] == ':' {
			drive := strings.ToLower(string(path[0]))
			path = "/" + drive + ":" + path[2:]
		}
	}
	u := url.URL{Scheme: "file", Path: path}
	return u.String()
}

func uriToPath(uri string) (string, error) {
	if !strings.HasPrefix(uri, "file://") {
		return "", fmt.Errorf("unsupported uri: %s", uri)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	path := parsed.Path
	if runtime.GOOS == "windows" {
		path = strings.TrimPrefix(path, "/")
		path = strings.ReplaceAll(path, "/", "\\")
	}
	return path, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
