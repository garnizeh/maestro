package beam

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
)

// dummyResult is a minimal implementation of types.Result to satisfy the nilnil linter.
type dummyResult struct{}

func (d *dummyResult) Print() error                                { return nil }
func (d *dummyResult) PrintTo(_ io.Writer) error                   { return nil }
func (d *dummyResult) String() string                              { return "dummy" }
func (d *dummyResult) Version() string                             { return "1.1.0" }
func (d *dummyResult) GetAsVersion(_ string) (types.Result, error) { return d, nil }
func (d *dummyResult) GetExitCode() int                            { return 0 }
func (d *dummyResult) MarshalJSON() ([]byte, error)                { return []byte(`{}`), nil }

// ── Mock implementations ───────────────────────────────────────────────────────

type mockNamespaceManager struct {
	newNSFn    func(id string, mount *MountRequest) (string, string, error)
	deleteNSFn func(id string) error
	nsPathFn   func(id string) string
	rootless   bool
}

func (m *mockNamespaceManager) NewNS(id string, mount *MountRequest) (string, string, error) {
	if m.newNSFn != nil {
		return m.newNSFn(id, mount)
	}
	return "/tmp/netns/" + id, "", nil
}

func (m *mockNamespaceManager) DeleteNS(id string) error {
	if m.deleteNSFn != nil {
		return m.deleteNSFn(id)
	}
	return nil
}

func (m *mockNamespaceManager) NSPath(id string) string {
	if m.nsPathFn != nil {
		return m.nsPathFn(id)
	}
	return "/tmp/netns/" + id
}

func (m *mockNamespaceManager) WithRootless(rootless bool) namespaceManager {
	m.rootless = rootless
	return m
}

type mockCNIExecutor struct {
	loadConfigListFn func(confBytes []byte) (*libcni.NetworkConfigList, error)
	invokeADDFn      func(ctx context.Context, netConfList *libcni.NetworkConfigList,
		containerID, netnsPath, ifName string, portMappings []PortMapping) (types.Result, error)
	invokeDELFn func(ctx context.Context, netConfList *libcni.NetworkConfigList,
		containerID, netnsPath, ifName string, portMappings []PortMapping) error
	invokeCHECKFn func(ctx context.Context, netConfList *libcni.NetworkConfigList,
		containerID, netnsPath, ifName string) error
}

func (m *mockCNIExecutor) LoadConfigList(confBytes []byte) (*libcni.NetworkConfigList, error) {
	if m.loadConfigListFn != nil {
		return m.loadConfigListFn(confBytes)
	}
	return libcni.ConfListFromBytes(confBytes)
}

func (m *mockCNIExecutor) InvokeADD(ctx context.Context, netConfList *libcni.NetworkConfigList,
	containerID, netnsPath, ifName string, portMappings []PortMapping) (types.Result, error) {
	if m.invokeADDFn != nil {
		return m.invokeADDFn(ctx, netConfList, containerID, netnsPath, ifName, portMappings)
	}
	return &dummyResult{}, nil
}

func (m *mockCNIExecutor) InvokeDEL(ctx context.Context, netConfList *libcni.NetworkConfigList,
	containerID, netnsPath, ifName string, portMappings []PortMapping) error {
	if m.invokeDELFn != nil {
		return m.invokeDELFn(ctx, netConfList, containerID, netnsPath, ifName, portMappings)
	}
	return nil
}

func (m *mockCNIExecutor) InvokeCHECK(ctx context.Context, netConfList *libcni.NetworkConfigList,
	containerID, netnsPath, ifName string) error {
	if m.invokeCHECKFn != nil {
		return m.invokeCHECKFn(ctx, netConfList, containerID, netnsPath, ifName)
	}
	return nil
}

func newTestBeam(t *testing.T, ns namespaceManager, cni cniExecutor) *Beam {
	t.Helper()
	confDir := t.TempDir()
	binDir := t.TempDir()

	// Create a fake "bridge" binary so DownloadCNIPlugins short-circuits
	if err := os.WriteFile(filepath.Join(binDir, "bridge"), []byte("fake"), 0750); err != nil {
		t.Fatalf("setup: write fake bridge: %v", err)
	}

	b := NewBeam(confDir, binDir, "/tmp/netns").
		WithTodash(ns).
		WithGuardian(cni)
	return b
}

// ── LoadDefaultConfig tests ────────────────────────────────────────────────────

func TestBeam_LoadDefaultConfig_CreatesFromEmbedded(t *testing.T) {
	t.Parallel()
	b := newTestBeam(t, &mockNamespaceManager{}, &mockCNIExecutor{})

	cfg, err := b.LoadDefaultConfig()
	if err != nil {
		t.Fatalf("LoadDefaultConfig() unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config list")
	}
	if cfg.Name != "beam0" {
		t.Errorf("config name: got %q, want %q", cfg.Name, "beam0")
	}
}

func TestBeam_LoadDefaultConfig_ReadsExistingFile(t *testing.T) {
	t.Parallel()
	b := newTestBeam(t, &mockNamespaceManager{}, &mockCNIExecutor{})

	// Write a custom conflist to the confDir
	customJSON := `{"cniVersion":"1.1.0","name":"custom","plugins":[{"type":"loopback"}]}`
	confPath := filepath.Join(b.confDir, "cni-beam0.conflist")
	if err := os.WriteFile(confPath, []byte(customJSON), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := b.LoadDefaultConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "custom" {
		t.Errorf("expected to load %q from file, got %q", "custom", cfg.Name)
	}
}

func TestBeam_LoadDefaultConfig_UnreadableFile(t *testing.T) {
	t.Parallel()
	b := newTestBeam(t, &mockNamespaceManager{}, &mockCNIExecutor{})

	confPath := filepath.Join(b.confDir, "cni-beam0.conflist")
	// Create file then chmod 000 to make unreadable
	if err := os.WriteFile(confPath, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(confPath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if chmodErr := os.Chmod(confPath, 0600); chmodErr != nil {
			t.Fatalf("failed to restore file permissions: %v", chmodErr)
		}
	})

	_, err := b.LoadDefaultConfig()
	if err == nil {
		t.Fatal("expected error reading chmod-000 file, got nil")
	}
}

func TestBeam_LoadDefaultConfig_MkdirAllFailure(t *testing.T) {
	t.Parallel()
	confDir := "/non/existent/path"
	fs := &mockFS{
		MkdirAllFn: func(_ string, _ os.FileMode) error {
			return errors.New("mkdir-fail")
		},
		ReadFileFn:   func(_ string) ([]byte, error) { return nil, os.ErrNotExist },
		IsNotExistFn: func(_ error) bool { return true },
	}

	b := NewBeam(confDir, t.TempDir(), t.TempDir()).WithFS(fs)

	_, err := b.LoadDefaultConfig()
	if err == nil {
		t.Fatal("expected MkdirAll failure for read-only parent, got nil")
	}
}

func TestBeam_LoadDefaultConfig_MkdirFailure(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		MkdirAllFn: func(_ string, _ os.FileMode) error {
			return errors.New("mkdir-fail")
		},
		ReadFileFn:   func(_ string) ([]byte, error) { return nil, os.ErrNotExist },
		IsNotExistFn: func(_ error) bool { return true },
	}

	b := NewBeam("/tmp/some-file", t.TempDir(), t.TempDir()).WithFS(fs)

	_, err := b.LoadDefaultConfig()
	if err == nil {
		t.Fatal("expected error when confDir is a file, got nil")
	}
}

// ── Attach tests ───────────────────────────────────────────────────────────────

func TestBeam_Attach_Success(t *testing.T) {
	t.Parallel()

	nsMgr := &mockNamespaceManager{
		newNSFn: func(id string, _ *MountRequest) (string, string, error) {
			return "/tmp/netns/" + id, "", nil
		},
	}
	cniExec := &mockCNIExecutor{
		invokeADDFn: func(_ context.Context, _ *libcni.NetworkConfigList,
			_, _, _ string, _ []PortMapping) (types.Result, error) {
			return &dummyResult{}, nil
		},
	}
	b := newTestBeam(t, nsMgr, cniExec)

	attRes, err := b.Attach(context.Background(), "ctr-1", nil, nil)
	if err != nil {
		t.Fatalf("Attach() unexpected error: %v", err)
	}
	if attRes.NetNSPath == "" {
		t.Error("expected non-empty nsPath")
	}
	if attRes.Result == nil {
		t.Error("expected non-nil result")
	}
}

func TestBeam_Attach_NSCreationFailure(t *testing.T) {
	t.Parallel()

	nsMgr := &mockNamespaceManager{
		newNSFn: func(_ string, _ *MountRequest) (string, string, error) {
			return "", "", errors.New("netns error")
		},
	}
	b := newTestBeam(t, nsMgr, &mockCNIExecutor{})

	_, err := b.Attach(context.Background(), "ctr-fail", nil, nil)
	if err == nil {
		t.Fatal("expected error on NS creation failure, got nil")
	}
}

func TestBeam_Attach_CNIAddFailure_Rollback(t *testing.T) {
	t.Parallel()

	deleteCalled := false
	nsMgr := &mockNamespaceManager{
		newNSFn: func(_ string, _ *MountRequest) (string, string, error) {
			return "/tmp/netns/wrong", "", nil
		},
		deleteNSFn: func(_ string) error {
			deleteCalled = true
			return nil
		},
	}
	cniExec := &mockCNIExecutor{
		invokeADDFn: func(_ context.Context, _ *libcni.NetworkConfigList,
			_, _, _ string, _ []PortMapping) (types.Result, error) {
			return nil, errors.New("cni bridge failed")
		},
	}
	b := newTestBeam(t, nsMgr, cniExec)

	_, err := b.Attach(context.Background(), "ctr-rollback", nil, nil)
	if err == nil {
		t.Fatal("expected error on CNI ADD failure, got nil")
	}
	if !deleteCalled {
		t.Error("expected namespace rollback (DeleteNS) to be called on CNI failure")
	}
}

func TestBeam_Attach_ConfigLoadFailure(t *testing.T) {
	t.Parallel()

	cniExec := &mockCNIExecutor{
		loadConfigListFn: func(_ []byte) (*libcni.NetworkConfigList, error) {
			return nil, errors.New("bad config")
		},
	}
	b := newTestBeam(t, &mockNamespaceManager{}, cniExec)

	_, err := b.Attach(context.Background(), "ctr-badconf", nil, nil)
	if err == nil {
		t.Fatal("expected config load failure error, got nil")
	}
}

// ── Detach tests ───────────────────────────────────────────────────────────────

func TestBeam_Detach_Success(t *testing.T) {
	t.Parallel()

	b := newTestBeam(t, &mockNamespaceManager{}, &mockCNIExecutor{})
	if err := b.Detach(context.Background(), "ctr-1", nil); err != nil {
		t.Errorf("Detach() unexpected error: %v", err)
	}
}

func TestBeam_Detach_ConfigLoadFailure(t *testing.T) {
	t.Parallel()

	cniExec := &mockCNIExecutor{
		loadConfigListFn: func(_ []byte) (*libcni.NetworkConfigList, error) {
			return nil, errors.New("bad config on detach")
		},
	}
	b := newTestBeam(t, &mockNamespaceManager{}, cniExec)

	err := b.Detach(context.Background(), "ctr-confail", nil)
	if err == nil {
		t.Fatal("expected error on detach config failure, got nil")
	}
}

func TestBeam_Detach_NSDeleteFailure(t *testing.T) {
	t.Parallel()

	nsMgr := &mockNamespaceManager{
		deleteNSFn: func(_ string) error { return errors.New("umount failed") },
	}
	b := newTestBeam(t, nsMgr, &mockCNIExecutor{})

	err := b.Detach(context.Background(), "ctr-delfail", nil)
	if err == nil {
		t.Fatal("expected error on namespace deletion failure, got nil")
	}
}

// ── NewBeam constructor test ───────────────────────────────────────────────────

func TestNewBeam_DefaultBinDir(t *testing.T) {
	t.Parallel()
	b := NewBeam(t.TempDir(), "", t.TempDir())
	if b.binDir != "/opt/cni/bin" {
		t.Errorf("expected default binDir /opt/cni/bin, got %q", b.binDir)
	}
}

func TestNewBeam_CustomBinDir(t *testing.T) {
	t.Parallel()
	b := NewBeam(t.TempDir(), "/usr/lib/cni", t.TempDir())
	if b.binDir != "/usr/lib/cni" {
		t.Errorf("expected /usr/lib/cni, got %q", b.binDir)
	}
}

func TestBeam_LoadDefaultConfig_WriteFailure(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		MkdirAllFn:   func(_ string, _ os.FileMode) error { return nil },
		ReadFileFn:   func(_ string) ([]byte, error) { return nil, os.ErrNotExist },
		WriteFileFn:  func(_ string, _ []byte, _ os.FileMode) error { return errors.New("write-fail") },
		IsNotExistFn: func(_ error) bool { return true },
	}

	b := NewBeam(t.TempDir(), t.TempDir(), t.TempDir()).WithFS(fs)

	_, err := b.LoadDefaultConfig()
	if err == nil {
		t.Fatal("expected write failure for read-only confDir, got nil")
	}
}

func TestBeam_Attach_DownloadFailure(t *testing.T) {
	t.Parallel()
	dl := &mockDownloader{downloadErr: errors.New("download-fail")}
	b := NewBeam(t.TempDir(), t.TempDir(), t.TempDir()).WithDownloader(dl)

	_, err := b.Attach(context.Background(), "ctr-dl-fail", nil, nil)
	if err == nil {
		t.Fatal("expected download failure error, got nil")
	}
}
