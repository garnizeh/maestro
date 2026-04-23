package gan

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kr/pretty"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/garnizeh/maestro/internal/beam"
	"github.com/garnizeh/maestro/internal/eld"
	"github.com/garnizeh/maestro/internal/prim"
	"github.com/garnizeh/maestro/internal/testutil"
	"github.com/garnizeh/maestro/pkg/archive"
	"github.com/garnizeh/maestro/pkg/specgen"
)

// ── mock implementations ──────────────────────────────────────────────────────

type mockFS = testutil.MockFS
type mockMounter = testutil.MockMounter

type mockSpecGenerator struct {
	genRes   *specgen.Spec
	genErr   error
	writeErr error
}

func (m *mockSpecGenerator) Generate(
	_ imagespec.ImageConfig,
	_ specgen.Opts,
) (*specgen.Spec, error) {
	if m.genRes != nil {
		return m.genRes, nil
	}
	return &specgen.Spec{}, m.genErr
}

func (m *mockSpecGenerator) Write(_ string, _ *specgen.Spec) error { return m.writeErr }

type mockIDGenerator struct {
	idRes string
	idErr error
}

func (m *mockIDGenerator) NewID() (string, error) { return m.idRes, m.idErr }

// ── fake eld.Eld for ops tests ────────────────────────────────────────────────

type opsEld struct {
	createErr error
	startErr  error
	killErr   error
	deleteErr error
	stateFn   func(ctx context.Context, id string) (*eld.State, error)
	callCount int
	features  eld.Features
}

func (f *opsEld) Create(_ context.Context, _, _ string, _ *eld.CreateOpts) error {
	return f.createErr
}
func (f *opsEld) Start(_ context.Context, _ string, _ *eld.StartOpts) error {
	return f.startErr
}
func (f *opsEld) Kill(_ context.Context, _ string, _ syscall.Signal) error {
	return f.killErr
}
func (f *opsEld) Delete(_ context.Context, _ string, _ *eld.DeleteOpts) error {
	return f.deleteErr
}
func (f *opsEld) State(ctx context.Context, id string) (*eld.State, error) {
	f.callCount++
	if f.stateFn != nil {
		return f.stateFn(ctx, id)
	}
	return &eld.State{ID: id, Status: eld.StatusStopped}, nil
}
func (f *opsEld) Features(_ context.Context) (*eld.Features, error) {
	return &f.features, nil
}

// ── fake prim.Prim for ops tests ──────────────────────────────────────────────

type opsPrim struct {
	prepareErr error
}

func (p *opsPrim) Prepare(_ context.Context, key, _ string) ([]prim.Mount, error) {
	if p.prepareErr != nil {
		return nil, p.prepareErr
	}
	return []prim.Mount{{Source: "/tmp/rootfs/" + key, Options: []string{"bind"}}}, nil
}
func (p *opsPrim) View(_ context.Context, _, _ string) ([]prim.Mount, error) {
	return nil, nil
}
func (p *opsPrim) Commit(_ context.Context, _, _ string) error { return nil }
func (p *opsPrim) Remove(_ context.Context, _ string) error    { return nil }
func (p *opsPrim) Walk(_ context.Context, _ func(prim.Info) error) error {
	return nil
}
func (p *opsPrim) Usage(_ context.Context, _ string) (prim.Usage, error) {
	return prim.Usage{}, nil
}
func (p *opsPrim) WritableDir(key string) string          { return "/tmp/rootfs/" + key }
func (p *opsPrim) WhiteoutFormat() archive.WhiteoutFormat { return archive.WhiteoutVFS }

// ── fake beam.Beam for ops tests ─────────────────────────────────────────────

type opsNet struct {
	attachErr    error
	detachErr    error
	attachCalled bool
}

func (n *opsNet) Attach(_ context.Context, id string, _ *beam.MountRequest,
	_ []beam.PortMapping) (*beam.AttachResult, error) {
	n.attachCalled = true
	if n.attachErr != nil {
		return nil, n.attachErr
	}
	return &beam.AttachResult{NetNSPath: "/tmp/netns/" + id}, nil
}

func (n *opsNet) Detach(_ context.Context, _ string, _ []beam.PortMapping) error {
	return n.detachErr
}

// ── fake ImageStore for ops tests ─────────────────────────────────────────────

type mockImageStore struct {
	swellFn     func(ctx context.Context, ref string, p prim.Prim) (string, error)
	getConfigFn func(ctx context.Context, ref string) (imagespec.ImageConfig, string, error)
}

func (m *mockImageStore) Swell(ctx context.Context, ref string, p prim.Prim) (string, error) {
	if m.swellFn != nil {
		return m.swellFn(ctx, ref, p)
	}
	return "layer:parent", nil
}

func (m *mockImageStore) GetConfig(
	ctx context.Context,
	ref string,
) (imagespec.ImageConfig, string, error) {
	if m.getConfigFn != nil {
		return m.getConfigFn(ctx, ref)
	}
	return imagespec.ImageConfig{}, "sha256:digest", nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newOpsSetup(t *testing.T) (*Ops, *opsEld) {
	t.Helper()
	e := &opsEld{features: eld.Features{Seccomp: true}}
	p := &opsPrim{}
	n := &opsNet{}
	store := newMemStore()
	manager := NewManager(store, t.TempDir())
	imageStore := &mockImageStore{}
	rtInfo := eld.RuntimeInfo{Name: "crun", Path: "/usr/bin/crun", Version: "1.0.0"}
	ops := NewOps(manager, e, rtInfo, p, n, imageStore, t.TempDir())
	ops.WithFS(&mockFS{})
	ops.WithMounter(&mockMounter{})
	ops.WithSpecGenerator(&mockSpecGenerator{})
	ops.WithIDGenerator(
		&mockIDGenerator{idRes: "aabb112233445566778899001122334455667788990011223344556677001234"},
	)
	e.stateFn = func(_ context.Context, id string) (*eld.State, error) {
		e.callCount++
		if e.callCount <= 2 {
			return &eld.State{ID: id, Status: eld.StatusRunning, Pid: 42}, nil
		}
		return &eld.State{ID: id, Status: eld.StatusStopped}, nil
	}
	return ops, e
}

// ── Run tests ─────────────────────────────────────────────────────────────────

func TestOps_Run_Detached_Success(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	e.stateFn = func(_ context.Context, id string) (*eld.State, error) {
		return &eld.State{ID: id, Status: eld.StatusRunning, Pid: 99}, nil
	}

	ctr, err := ops.Run(context.Background(), RunOpts{
		CreateOpts: CreateOpts{
			Image: "nginx:latest",
			Ports: []string{"80:80", "443:443"},
		},
		StartOpts: StartOpts{
			Detach: true,
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ctr.Ka != KaRunning {
		t.Errorf("Ka = %v; want KaRunning", ctr.Ka)
	}
}

func TestOps_Run_Foreground_Success(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	callCount := 0
	e.stateFn = func(_ context.Context, id string) (*eld.State, error) {
		callCount++
		if callCount == 1 {
			return &eld.State{ID: id, Status: eld.StatusRunning, Pid: 42}, nil
		}
		return &eld.State{ID: id, Status: eld.StatusStopped}, nil
	}

	ctr, err := ops.Run(
		context.Background(),
		RunOpts{CreateOpts: CreateOpts{Image: "nginx:latest"}},
	)
	if err != nil {
		t.Fatalf("Run foreground: %v", err)
	}
	if ctr.Ka != KaStopped {
		t.Errorf("Ka = %v; want KaStopped", ctr.Ka)
	}
}

func TestOps_Run_SwellFails(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	ops.imageStore.(*mockImageStore).swellFn = func(_ context.Context, _ string, _ prim.Prim) (string, error) {
		return "", errors.New("swell fail")
	}

	_, err := ops.Run(context.Background(), RunOpts{CreateOpts: CreateOpts{Image: "nginx"}})
	if err == nil || !strings.Contains(err.Error(), "swell") {
		t.Errorf("expected swell error, got %v", err)
	}
}

func TestOps_Run_MkdirRootfsFails(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)

	// We need to trigger the second MkdirAll (rootfs)
	count := 0
	ops.WithFS(&mockMkdirFS{errAtCount: 2, count: &count})

	_, err := ops.Run(context.Background(), RunOpts{CreateOpts: CreateOpts{Image: "nginx"}})
	if err == nil || !strings.Contains(err.Error(), "mkdir rootfs") {
		t.Errorf("expected mkdir rootfs error, got %v", err)
	}
}

type mockMkdirFS struct {
	mockFS

	errAtCount int
	count      *int
}

func (m *mockMkdirFS) MkdirAll(_ string, _ os.FileMode) error {
	*m.count++
	if *m.count == m.errAtCount {
		return errors.New("mkdir fail")
	}
	return nil
}

func TestOps_Run_SpecWriteFails(t *testing.T) {
	t.Parallel()
	sg := &mockSpecGenerator{writeErr: errors.New("write fail")}
	ops, _ := newOpsSetup(t)
	ops.WithSpecGenerator(sg)

	_, err := ops.Run(context.Background(), RunOpts{CreateOpts: CreateOpts{Image: "nginx"}})
	if err == nil || !strings.Contains(err.Error(), "write spec") {
		t.Errorf("expected write spec error, got %v", err)
	}
}

func TestOps_Run_SaveContainerFails(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	ms := ops.Manager.store.(*memStore)
	ms.putErr = errors.New("save fail")

	_, err := ops.Run(context.Background(), RunOpts{
		CreateOpts: CreateOpts{Image: "nginx"},
		StartOpts:  StartOpts{Detach: true},
	})
	if err == nil || !strings.Contains(err.Error(), "save container") {
		t.Errorf("expected save container error, got %v", err)
	}
}

func TestOps_Run_MonitorFails(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	e.stateFn = func(_ context.Context, _ string) (*eld.State, error) {
		return nil, eld.ErrContainerNotFound
	}

	_, err := ops.Run(context.Background(), RunOpts{
		CreateOpts: CreateOpts{Image: "nginx"},
		StartOpts:  StartOpts{Timeout: 50 * time.Millisecond},
	})
	if err == nil || !strings.Contains(err.Error(), "monitor") {
		t.Errorf("expected monitor error, got %v", err)
	}
}

// ── Stop tests ────────────────────────────────────────────────────────────────

func TestOps_Stop_KillFailure(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.killErr = errors.New("kill fail")

	err := ops.Stop(context.Background(), id, StopOpts{})
	if err == nil || !strings.Contains(err.Error(), "kill") {
		t.Errorf("expected kill error, got %v", err)
	}
}

func TestOps_Stop_KillAlreadyDead(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.killErr = errors.New("no such process")
	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Stop(context.Background(), id, StopOpts{}); err != nil {
		t.Fatalf("Stop with already-dead kill: %v", err)
	}
}

func TestOps_Stop_WaitTimeout(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusRunning, Pid: 1}, nil
	}

	err := ops.Stop(context.Background(), id, StopOpts{Timeout: 50 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got %v", err)
	}
}

func TestOps_Stop_ByName(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	name := "web"
	ctr := sampleContainer(id, name)
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		// Ensure it uses the REAL ID for the runtime call, not the name
		if rid != id {
			return nil, errors.New("Stop called runtime with name instead of ID")
		}
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Stop(context.Background(), name, StopOpts{}); err != nil {
		t.Fatalf("Stop by name: %v", err)
	}
}

func TestOps_Stop_DeleteRuntimeFail(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}
	e.deleteErr = errors.New("delete fail")

	err := ops.Stop(context.Background(), id, StopOpts{})
	if err == nil || !strings.Contains(err.Error(), "delete runtime") {
		t.Errorf("expected delete runtime error, got %v", err)
	}
}

// ── Rm tests ──────────────────────────────────────────────────────────────────

func TestOps_Rm_RunningWithoutForce(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	err := ops.Rm(context.Background(), id, RmOpts{Force: false})
	if err == nil || !errors.Is(err, ErrContainerRunning) {
		t.Errorf("expected ErrContainerRunning, got %v", err)
	}
}

func TestOps_Rm_ByNameSuccess(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	name := "web"
	ctr := sampleContainer(id, name)
	ctr.Ka = KaStopped
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		if rid != id {
			return nil, errors.New("Rm called runtime with name instead of ID")
		}
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Rm(context.Background(), name, RmOpts{}); err != nil {
		t.Fatalf("Rm by name: %v", err)
	}
}

func TestOps_Rm_RunningWithForceSuccess(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	ctr.NetNSPath = "/run/netns/aabb"
	ctr.Ports = []string{"80:80", "invalid-port"}
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Rm(context.Background(), id, RmOpts{Force: true}); err != nil {
		t.Fatalf("Rm Force: %v", err)
	}
}

func TestOps_Rm_StopFails(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, _ string) (*eld.State, error) {
		return &eld.State{Status: eld.StatusRunning}, nil
	}
	e.killErr = errors.New("kill fail")

	err := ops.Rm(context.Background(), id, RmOpts{Force: true})
	if err == nil || !strings.Contains(err.Error(), "force stop") {
		t.Errorf("expected force stop error, got %v", err)
	}
}

func TestOps_Rm_RemoveAllFail(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaStopped
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	fs := &mockFS{RemoveAllFn: func(string) error { return errors.New("remove fail") }}
	ops.WithFS(fs)

	err := ops.Rm(context.Background(), id, RmOpts{})
	if err == nil || !strings.Contains(err.Error(), "remove data dir") {
		t.Errorf("expected remove error, got %v", err)
	}
}

func TestOps_Rm_DeleteStateFail(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaStopped
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	ms := ops.Manager.store.(*memStore)
	ms.deleteErr = errors.New("delete fail")

	err := ops.Rm(context.Background(), id, RmOpts{})
	if err == nil || !strings.Contains(err.Error(), "delete state") {
		t.Errorf("expected delete state error, got %v", err)
	}
}

// ── Generic/Wrapper tests ─────────────────────────────────────────────────────

func TestOps_ListAndLoad(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	_ = ops.Manager.SaveContainer(context.Background(), sampleContainer(id, "web"))

	list, err := ops.ListContainers(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v, len=%d", err, len(list))
	}

	got, err := ops.LoadContainer(context.Background(), id)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := sampleContainer(id, "web")
	if diff := pretty.Diff(want, got); len(diff) > 0 {
		t.Log("Ops.LoadContainer() mismatch")
		t.Logf("want: %v", want)
		t.Logf("got: %v", got)
		t.Errorf("\n%s", diff)
	}
}

func TestOps_LoadContainer_ByName_Success(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	name := "my-container"
	ctr := sampleContainer(id, name)
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	// Load by NAME
	got, err := ops.LoadContainer(context.Background(), name)
	if err != nil {
		t.Fatalf("Load by name: %v", err)
	}
	if got.ID != id {
		t.Errorf("got ID %q; want %q", got.ID, id)
	}
}

func TestOps_LoadContainer_NotFound(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)

	_, err := ops.LoadContainer(context.Background(), "non-existent")
	if err == nil || !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound; got %v", err)
	}
}

func TestOps_LoadContainer_IDLookupFatalError(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	ms := ops.Manager.store.(*memStore)
	ms.getErr = errors.New("disk failure")

	_, err := ops.LoadContainer(context.Background(), "id")
	if err == nil || !strings.Contains(err.Error(), "disk failure") {
		t.Errorf("expected disk failure; got %v", err)
	}
}

func TestOps_LoadContainer_FindByNameError(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	ms := ops.Manager.store.(*memStore)

	// First Get fails with "not found" (triggering name search)
	// Then List fails (within FindByName)
	ms.listErr = errors.New("list failure")

	_, err := ops.LoadContainer(context.Background(), "name")
	if err == nil || !strings.Contains(err.Error(), "list failure") {
		t.Errorf("expected list failure; got %v", err)
	}
}

// ── helper tests ──────────────────────────────────────────────────────────────

func TestOps_WaitForStop_StateError(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	e.stateFn = func(_ context.Context, _ string) (*eld.State, error) {
		return nil, errors.New("state fail")
	}

	err := ops.waitForStop(context.Background(), "id", time.Second)
	if err == nil || !strings.Contains(err.Error(), "state fail") {
		t.Errorf("expected state fail, got %v", err)
	}
}

func TestOps_WaitForStop_GoneIsStopped(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	e.stateFn = func(_ context.Context, _ string) (*eld.State, error) {
		return nil, errors.New("not found")
	}

	if err := ops.waitForStop(context.Background(), "id", time.Second); err != nil {
		t.Fatalf("expected gone to be stopped, got %v", err)
	}
}

func TestOps_HandleMountError_Generic(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	m := prim.Mount{Source: "/src", Type: "ext4"}
	err := ops.handleMountError(errors.New("generic fail"), m, "/target", 0, "some-data")
	if err == nil || !strings.Contains(err.Error(), "generic fail") {
		t.Errorf("expected generic fail, got %v", err)
	}
}

func TestOps_HandleMountError_PermissionDenied(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	m := prim.Mount{Source: "/src", Type: "ext4"}
	err := ops.handleMountError(syscall.EPERM, m, "/target", 0, "data")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected permission denied error, got %v", err)
	}
}

func TestOps_Run_EvalSymlinksFail(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		EvalSymlinksFn: func(p string) (string, error) { return p, errors.New("eval fail") },
	}
	ops, _ := newOpsSetup(t)
	ops.WithFS(fs)

	_, err := ops.Run(context.Background(), RunOpts{
		CreateOpts: CreateOpts{Image: "nginx"},
		StartOpts:  StartOpts{Detach: true},
	})
	if err != nil {
		t.Fatalf("Run should ignore eval fail: %v", err)
	}
}

func TestOps_Run_MkdirBundleFails(t *testing.T) {
	t.Parallel()
	fs := &mockFS{MkdirAllFn: func(string, os.FileMode) error { return errors.New("mkdir fail") }}
	ops, _ := newOpsSetup(t)
	ops.WithFS(fs)

	_, err := ops.Run(context.Background(), RunOpts{CreateOpts: CreateOpts{Image: "nginx"}})
	if err == nil || !strings.Contains(err.Error(), "mkdir bundle") {
		t.Errorf("expected mkdir bundle error, got %v", err)
	}
}

func TestOps_Run_NameSearchFails(t *testing.T) {
	t.Parallel()
	ops, _ := newOpsSetup(t)
	ms := ops.Manager.store.(*memStore)
	ms.listErr = errors.New("list failure")

	_, err := ops.Run(context.Background(), RunOpts{
		CreateOpts: CreateOpts{
			Image: "nginx",
			Name:  "my-ctr",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "find by name") {
		t.Errorf("expected find by name error, got %v", err)
	}
}

func TestOps_Run_UpdateRunningStateFails(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	e.stateFn = func(_ context.Context, id string) (*eld.State, error) {
		return &eld.State{ID: id, Status: eld.StatusRunning, Pid: 123}, nil
	}

	ms := ops.Manager.store.(*memStore)
	count := 0
	ms.putFn = func() error {
		count++
		if count == 2 {
			return errors.New("update fail")
		}
		return nil
	}

	_, err := ops.Run(context.Background(), RunOpts{
		CreateOpts: CreateOpts{Image: "nginx"},
		StartOpts:  StartOpts{Detach: true},
	})
	if err == nil || !strings.Contains(err.Error(), "update running state") {
		t.Errorf("expected update running state error, got %v", err)
	}
}

func TestOps_Stop_SaveStoppedStateFails(t *testing.T) {
	t.Parallel()
	ops, e := newOpsSetup(t)
	id := "aabb"
	ctr := sampleContainer(id, "web")
	ctr.Ka = KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	ms := ops.Manager.store.(*memStore)
	ms.putErr = errors.New("save stopped fail")

	err := ops.Stop(context.Background(), id, StopOpts{})
	if err == nil || !strings.Contains(err.Error(), "save") {
		t.Errorf("expected save error, got %v", err)
	}
}
