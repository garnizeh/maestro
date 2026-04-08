package shardik

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
)

// testResource is a minimal authn.Resource implementation used in white-box tests.
type testResource struct{ registry string }

func (r testResource) RegistryStr() string { return r.registry }
func (r testResource) String() string      { return r.registry }

// ── authFilePath ──────────────────────────────────────────────────────────────

func TestAuthFilePath_HomeDirError(t *testing.T) {
	orig := userHomeDirFn
	defer func() { userHomeDirFn = orig }()
	userHomeDirFn = func() (string, error) { return "", errors.New("no home directory") }

	_, err := authFilePath("")
	if err == nil {
		t.Error("expected error when home directory lookup fails")
	}
}

// ── resolveFromAuthFile ───────────────────────────────────────────────────────

func TestResolveFromAuthFile_PathError(t *testing.T) {
	// Trigger pathErr by making userHomeDirFn fail when override is empty.
	orig := userHomeDirFn
	defer func() { userHomeDirFn = orig }()
	userHomeDirFn = func() (string, error) { return "", errors.New("no home") }

	_, err := resolveFromAuthFile("", "docker.io")
	if err == nil {
		t.Error("expected error when home directory lookup fails")
	}
}

func TestResolveFromAuthFile_JSONError(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{invalid json`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := resolveFromAuthFile(authPath, "docker.io")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestResolveFromAuthFile_TokenEntry(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	af := authFile{
		Auths: map[string]authEntry{
			"ghcr.io": {Token: "bearer-token-xyz"},
		},
	}
	data, _ := json.Marshal(af)
	if err := os.WriteFile(authPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	auth, err := resolveFromAuthFile(authPath, "ghcr.io")
	if err != nil {
		t.Fatalf("resolveFromAuthFile: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil authenticator")
	}
}

func TestResolveFromAuthFile_PermissionWarning(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	af := authFile{
		Auths: map[string]authEntry{
			"docker.io": {Username: "u", Password: "p"},
		},
	}
	data, _ := json.Marshal(af)
	if err := os.WriteFile(authPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Should resolve successfully despite the warning written to stderr.
	auth, err := resolveFromAuthFile(authPath, "docker.io")
	if err != nil {
		t.Fatalf("resolveFromAuthFile with wide perms: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil authenticator")
	}
}

// ── sigulKeychain.Resolve ─────────────────────────────────────────────────────

func TestSigulResolve_DockerKeychainFallback(t *testing.T) {
	// Set DOCKER_CONFIG to a temp dir with credentials for test-docker.io.
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".docker")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dockerConfig := map[string]any{
		"auths": map[string]any{
			"test-docker.io": map[string]string{"username": "u", "password": "p"},
		},
	}
	cfgData, _ := json.Marshal(dockerConfig)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), cfgData, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", configDir)

	// Maestro auth.json has no entry for test-docker.io.
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	kc := &sigulKeychain{cfg: SigulConfig{AuthFilePath: authPath}}
	auth, err := kc.Resolve(testResource{registry: "test-docker.io"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if auth == authn.Anonymous {
		t.Error("expected non-anonymous auth from docker keychain")
	}
}

// ── sigulKeychain.Resolve — early return paths ───────────────────────────────

func TestResolve_CLIFlagsTakePriority(t *testing.T) {
	kc := &sigulKeychain{cfg: SigulConfig{Username: "admin", Password: "s3cr3t"}}
	auth, err := kc.Resolve(testResource{registry: "docker.io"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil authenticator")
	}
}

func TestResolve_EnvTokenTakesPriority(t *testing.T) {
	t.Setenv("MAESTRO_REGISTRY_TOKEN", "test-bearer-token")

	kc := &sigulKeychain{cfg: SigulConfig{}}
	auth, err := kc.Resolve(testResource{registry: "docker.io"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil authenticator")
	}
}

func TestResolve_AuthFileMatchReturnsCredential(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	af := authFile{
		Auths: map[string]authEntry{
			"registry.example.com": {Username: "u", Password: "p"},
		},
	}
	data, _ := json.Marshal(af)
	if err := os.WriteFile(authPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	kc := &sigulKeychain{cfg: SigulConfig{AuthFilePath: authPath}}
	auth, err := kc.Resolve(testResource{registry: "registry.example.com"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil authenticator from auth file")
	}
}

// ── SaveCredentials / RemoveCredentials edge cases ────────────────────────────

func TestSaveCredentials_AuthFilePathError(t *testing.T) {
	orig := userHomeDirFn
	defer func() { userHomeDirFn = orig }()
	userHomeDirFn = func() (string, error) { return "", errors.New("no home") }

	if err := SaveCredentials("docker.io", "u", "p", ""); err == nil {
		t.Error("expected error when home dir lookup fails")
	}
}

func TestSaveCredentials_MkdirError(t *testing.T) {
	dir := t.TempDir()
	// Make dir read-only so MkdirAll of a nested subdirectory fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	authPath := filepath.Join(dir, "subdir", "auth.json")
	if err := SaveCredentials("docker.io", "u", "p", authPath); err == nil {
		t.Error("expected error when parent directory is not writable")
	}
}

func TestSaveCredentials_WriteError(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	// Create a directory at the file path to make WriteFile fail.
	if err := os.Mkdir(authPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredentials("docker.io", "u", "p", authPath); err == nil {
		t.Error("expected error when auth path is a directory")
	}
}

func TestRemoveCredentials_AuthFilePathError(t *testing.T) {
	orig := userHomeDirFn
	defer func() { userHomeDirFn = orig }()
	userHomeDirFn = func() (string, error) { return "", errors.New("no home") }

	if err := RemoveCredentials("docker.io", ""); err == nil {
		t.Error("expected error when home dir lookup fails")
	}
}

func TestRemoveCredentials_ReadError(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	// Create a directory at the file path to make ReadFile return a non-IsNotExist error.
	if err := os.Mkdir(authPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCredentials("docker.io", authPath); err == nil {
		t.Error("expected error when auth path is a directory")
	}
}

func TestRemoveCredentials_WriteError(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	// Write valid JSON, then make file read-only so WriteFile fails.
	af := authFile{Auths: map[string]authEntry{"docker.io": {Username: "u", Password: "p"}}}
	data, _ := json.Marshal(af)
	if err := os.WriteFile(authPath, data, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCredentials("docker.io", authPath); err == nil {
		t.Error("expected error when auth file is read-only")
	}
}
