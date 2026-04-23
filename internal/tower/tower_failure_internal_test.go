package tower

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/testutil"
)

type mockFS = testutil.MockFS

type mockTOML struct {
	realTOML

	unmarshalFn func([]byte, any) error
	marshalFn   func(any) ([]byte, error)
}

func (m *mockTOML) Unmarshal(data []byte, v any) error {
	if m.unmarshalFn != nil {
		return m.unmarshalFn(data, v)
	}
	return m.realTOML.Unmarshal(data, v)
}
func (m *mockTOML) Marshal(v any) ([]byte, error) {
	if m.marshalFn != nil {
		return m.marshalFn(v)
	}
	return m.realTOML.Marshal(v)
}

func TestLoader_ConfigPath_Error(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		UserHomeDirFn: func() (string, error) { return "", errors.New("home error") },
	}
	l := NewLoader(fs, &realTOML{})

	// Ensure XDG_CONFIG_HOME is empty for this test to trigger UserHomeDir
	fs.GetenvFn = func(_ string) string { return "" }

	_, err := l.ConfigPath("")
	if err == nil || err.Error() != "cannot determine home directory: home error" {
		t.Errorf("got error %v, want home error", err)
	}
}

func TestLoader_LoadConfig_UnmarshalError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		ReadFileFn: func(string) ([]byte, error) { return []byte("data"), nil },
	}
	toml := &mockTOML{
		unmarshalFn: func(_ []byte, _ any) error { return errors.New("unmarshal error") },
	}
	l := NewLoader(fs, toml)

	_, err := l.LoadConfig("test.toml")
	if err == nil || err.Error() != "parse config test.toml: unmarshal error" {
		t.Errorf("got error %v, want unmarshal error", err)
	}
}

func TestLoader_EnsureDefault_MarshalError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		StatFn: func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
	}
	toml := &mockTOML{
		marshalFn: func(any) ([]byte, error) { return nil, errors.New("marshal error") },
	}
	l := NewLoader(fs, toml)

	_, _, err := l.EnsureDefault("/tmp/maestro/katet.toml")
	if err == nil || err.Error() != "marshal defaults: marshal error" {
		t.Errorf("got error %v, want marshal error", err)
	}
}

func TestLoader_LoadConfig_ConfigPathError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		UserHomeDirFn: func() (string, error) { return "", errors.New("home error") },
	}
	l := NewLoader(fs, &realTOML{})
	fs.GetenvFn = func(_ string) string { return "" }

	_, err := l.LoadConfig("")
	if err == nil || !strings.Contains(err.Error(), "cannot determine home directory") {
		t.Errorf("got error %v, want home error", err)
	}
}

func TestConfig_ToTOML_Error(t *testing.T) {
	// Not using t.Parallel() because we are temporarily modifying defaultLoader
	originalMarshaller := defaultLoader.toml
	defer func() { defaultLoader.toml = originalMarshaller }()

	defaultLoader.toml = &mockTOML{
		marshalFn: func(any) ([]byte, error) { return nil, errors.New("marshal error") },
	}

	cfg := &Config{}
	got := cfg.ToTOML()
	want := "# error serialising config: marshal error\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLoader_LoadConfig_ReadError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		ReadFileFn: func(string) ([]byte, error) { return nil, errors.New("read error") },
	}
	l := NewLoader(fs, &realTOML{})

	_, err := l.LoadConfig("nonexistent.toml")
	if err == nil || err.Error() != "read config nonexistent.toml: read error" {
		t.Errorf("got error %v, want read error", err)
	}
}

func TestLoader_EnsureDefault_ConfigPathError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		UserHomeDirFn: func() (string, error) { return "", errors.New("home error") },
	}
	l := NewLoader(fs, &realTOML{})
	fs.GetenvFn = func(_ string) string { return "" }

	_, _, err := l.EnsureDefault("")
	if err == nil || !strings.Contains(err.Error(), "cannot determine home directory") {
		t.Errorf("got error %v, want home error", err)
	}
}

func TestLoader_EnsureDefault_MkdirError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		StatFn: func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		MkdirAllFn: func(string, os.FileMode) error {
			return errors.New("mkdir error")
		},
	}
	l := NewLoader(fs, &realTOML{})

	_, _, err := l.EnsureDefault("/tmp/maestro/katet.toml")
	if err == nil || err.Error() != "create config dir: mkdir error" {
		t.Errorf("got error %v, want mkdir error", err)
	}
}

func TestLoader_EnsureDefault_WriteError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		StatFn:     func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		MkdirAllFn: func(string, os.FileMode) error { return nil },
		WriteFileFn: func(string, []byte, os.FileMode) error {
			return errors.New("write error")
		},
	}
	l := NewLoader(fs, &realTOML{})

	_, _, err := l.EnsureDefault("/tmp/maestro/katet.toml")
	if err == nil || err.Error() != "write default config: write error" {
		t.Errorf("got error %v, want write error", err)
	}
}

func TestLoader_EnsureDefault_Success(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		StatFn:      func(string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		MkdirAllFn:  func(string, os.FileMode) error { return nil },
		WriteFileFn: func(string, []byte, os.FileMode) error { return nil },
	}
	l := NewLoader(fs, &realTOML{})

	created, path, err := l.EnsureDefault("/tmp/maestro/katet.toml")
	if err != nil || !created || path != "/tmp/maestro/katet.toml" {
		t.Errorf("EnsureDefault failed: created=%v, path=%s, err=%v", created, path, err)
	}
}

func TestLoader_defaults_HomeError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		UserHomeDirFn: func() (string, error) { return "", errors.New("home error") },
	}
	l := NewLoader(fs, &realTOML{})
	_, err := l.defaults()
	if err == nil || err.Error() != "defaults: home error" {
		t.Errorf("got error %v, want home error", err)
	}
}

func TestLoader_ConfigPath_XDG(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		GetenvFn: func(key string) string {
			if key == "XDG_CONFIG_HOME" {
				return "/custom/config"
			}
			return ""
		},
	}
	l := NewLoader(fs, &realTOML{})
	path, err := l.ConfigPath("")
	if err != nil {
		t.Fatalf("got error %v, want no error", err)
	}
	if path != "/custom/config/maestro/katet.toml" {
		t.Errorf("expected /custom/config/maestro/katet.toml, got %s", path)
	}
}
