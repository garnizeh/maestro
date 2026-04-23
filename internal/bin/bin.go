package bin

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	dirPerm  = 0750
	execPerm = 0700
)

// Find returns the absolute path to the requested binary.
// It first checks if the binary is available in the system PATH.
// If not, it falls back to the embedded version, extracting it to a
// persistent location if necessary.
func Find(name string) (string, error) {
	// 1. Try system PATH.
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	// 2. Try embedded.
	return Extract(name)
}

// Extract extracts an embedded binary to ~/.local/share/maestro/bin.
func Extract(name string) (string, error) {
	data, ok := GetEmbedded(name)
	if !ok {
		return "", fmt.Errorf("binary %q not found in embedded store", name)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home dir: %w", err)
	}

	destDir := filepath.Join(home, ".local", "share", "maestro", "bin")
	if errMkdir := os.MkdirAll(destDir, dirPerm); errMkdir != nil {
		return "", fmt.Errorf("create bin dir: %w", errMkdir)
	}

	destPath := filepath.Join(destDir, name)

	// Check if already extracted and matches hash.
	if current, errRead := os.ReadFile(destPath); errRead == nil {
		if sha256.Sum256(current) == sha256.Sum256(data) {
			return destPath, nil
		}
	}

	// Extract.
	// We use execPerm because it's an executable binary.
	if errWrite := os.WriteFile(destPath, data, execPerm); errWrite != nil {
		return "", fmt.Errorf("write embedded binary %q: %w", name, errWrite)
	}

	return destPath, nil
}
