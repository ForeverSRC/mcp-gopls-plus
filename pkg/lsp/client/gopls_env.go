package client

import (
	"os"
	"strings"

	"github.com/ForeverSRC/mcp-gopls-plus/internal/goenv"
)

func buildGoplsEnv(env []string) []string {
	cloned := append([]string(nil), env...)
	hasGoto := false
	pathIdx := -1

	for i, kv := range cloned {
		if strings.HasPrefix(kv, "GOTOOLCHAIN=") {
			hasGoto = true
		}
		if strings.HasPrefix(kv, "PATH=") {
			pathIdx = i
		}
	}

	if !hasGoto {
		cloned = append(cloned, "GOTOOLCHAIN=local")
	}

	goBin, err := goenv.GoBin()
	if err != nil || goBin == "" {
		return cloned
	}

	var currentPath string
	if pathIdx >= 0 {
		currentPath = strings.TrimPrefix(cloned[pathIdx], "PATH=")
	} else {
		currentPath = os.Getenv("PATH")
	}

	if strings.HasPrefix(currentPath, goBin) {
		return cloned
	}

	newPath := goBin
	if currentPath != "" {
		newPath = goBin + string(os.PathListSeparator) + currentPath
	}

	pathEntry := "PATH=" + newPath
	if pathIdx >= 0 {
		cloned[pathIdx] = pathEntry
	} else {
		cloned = append(cloned, pathEntry)
	}

	return cloned
}
