package core

import (
	"os"
	"path/filepath"
)

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
