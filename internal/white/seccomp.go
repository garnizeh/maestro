package white

import (
	"encoding/json"
	"fmt"
	"os"
)

// Seccomp represents the OCI seccomp configuration.
type Seccomp struct {
	DefaultAction string           `json:"defaultAction"`
	Architectures []string         `json:"architectures,omitempty"`
	Syscalls      []SeccompSyscall `json:"syscalls,omitempty"`
}

// SeccompSyscall represents a syscall and its action in the seccomp filter.
type SeccompSyscall struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
}

// LoadSeccompProfile reads a JSON seccomp profile from the given path.
func LoadSeccompProfile(path string) (*Seccomp, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read seccomp profile %s: %w", path, err)
	}

	var s Seccomp
	if errUnmarshal := json.Unmarshal(data, &s); errUnmarshal != nil {
		return nil, fmt.Errorf("failed to unmarshal seccomp profile %s: %w", path, errUnmarshal)
	}

	return &s, nil
}
