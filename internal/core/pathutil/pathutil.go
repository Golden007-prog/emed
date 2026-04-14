package pathutil

import (
	"path/filepath"
	"strings"
)

func IsWithin(basePath, targetPath string) bool {
	if basePath == "" || targetPath == "" {
		return false
	}

	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return false
	}

	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}

	parentPrefix := ".." + string(filepath.Separator)
	return rel != ".." && !strings.HasPrefix(rel, parentPrefix)
}
