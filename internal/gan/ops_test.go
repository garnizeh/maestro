package gan_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/rodrigo-baliza/maestro/internal/eld"
	"github.com/rodrigo-baliza/maestro/internal/gan"
	"github.com/rodrigo-baliza/maestro/internal/prim"
)

// ── fake eld.Eld for ops tests ────────────────────────────────────────────────

type opsEld struct {
	createErr error
	startErr  error
	killErr   error
	deleteErr error
	stateFn   func(ctx context.Context, id string) (*eld.State, error)
	callCount int
}

func (f *opsEld) Create(_ context.Context, _, _ string, _ *eld.CreateOpts) error {
	return f.createErr
}
func (f *opsEld) Start(_ context.Context, _ string) error { return f.startErr }
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
	return &eld.Features{}, nil
}

// ── fake prim.Prim for ops tests ──────────────────────────────────────────────

type opsPrim struct {
	prepareErr error
}

func (p *opsPrim) Prepare(_ context.Context, key, _ string) ([]prim.Mount, error) {
	if p.prepareErr != nil {
		return nil, p.prepareErr
	}
	return []prim.Mount{{Source: "/tmp/rootfs/" + key}}, nil
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

// ── helpers ───────────────────────────────────────────────────────────────────

func newOpsSetup(t *testing.T) (*gan.Ops, *opsEld) {
	t.Helper()
	e := &opsEld{}
	p := &opsPrim{}
	store := newMemStore()
	manager := gan.NewManager(store, t.TempDir())
	rtInfo := eld.RuntimeInfo{Name: "crun", Path: "/usr/bin/crun", Version: "1.0.0"}
	ops := gan.NewOps(manager, e, rtInfo, p, t.TempDir())
	// Fix state to return Running then Stopped.
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
	ops, e := newOpsSetup(t)
	e.stateFn = func(_ context.Context, id string) (*eld.State, error) {
		return &eld.State{ID: id, Status: eld.StatusRunning, Pid: 99}, nil
	}

	ctr, err := ops.Run(context.Background(), gan.RunOpts{
		Image:  "nginx:latest",
		Detach: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ctr.Ka != gan.KaRunning {
		t.Errorf("Ka = %v; want KaRunning", ctr.Ka)
	}
}

func TestOps_Run_Foreground_Success(t *testing.T) {
	ops, e := newOpsSetup(t)
	callCount := 0
	e.stateFn = func(_ context.Context, id string) (*eld.State, error) {
		callCount++
		if callCount == 1 {
			// waitForPid: returns Running
			return &eld.State{ID: id, Status: eld.StatusRunning, Pid: 42}, nil
		}
		// waitForExit: returns Stopped immediately
		return &eld.State{ID: id, Status: eld.StatusStopped}, nil
	}

	ctr, err := ops.Run(context.Background(), gan.RunOpts{Image: "nginx:latest"})
	if err != nil {
		t.Fatalf("Run foreground: %v", err)
	}
	if ctr.Ka != gan.KaStopped {
		t.Errorf("Ka = %v; want KaStopped", ctr.Ka)
	}
}

func TestOps_Run_NameConflict(t *testing.T) {
	ops, e := newOpsSetup(t)
	e.stateFn = func(_ context.Context, id string) (*eld.State, error) {
		return &eld.State{ID: id, Status: eld.StatusRunning, Pid: 1}, nil
	}

	// First run with name "web".
	_, err := ops.Run(context.Background(), gan.RunOpts{Image: "nginx", Name: "web", Detach: true})
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}

	// Second run with the same name should fail.
	_, err = ops.Run(context.Background(), gan.RunOpts{Image: "nginx", Name: "web"})
	if err == nil {
		t.Fatal("expected ErrNameAlreadyInUse")
	}
	if !errors.Is(err, gan.ErrNameAlreadyInUse) {
		t.Errorf("expected ErrNameAlreadyInUse; got: %v", err)
	}
}

func TestOps_Run_PrepareRootfsFails(t *testing.T) {
	store := newMemStore()
	manager := gan.NewManager(store, t.TempDir())
	e := &opsEld{}
	p := &opsPrim{prepareErr: errors.New("no space left on device")}
	rtInfo := eld.RuntimeInfo{Name: "crun"}
	ops := gan.NewOps(manager, e, rtInfo, p, t.TempDir())

	_, err := ops.Run(context.Background(), gan.RunOpts{Image: "nginx"})
	if err == nil {
		t.Fatal("expected error when prepare rootfs fails")
	}
}

func TestOps_Run_MkdirBundleFails(t *testing.T) {
	// Use a data root that's actually a file to force mkdir to fail.
	store := newMemStore()
	manager := gan.NewManager(store, t.TempDir())
	e := &opsEld{}
	p := &opsPrim{}
	rtInfo := eld.RuntimeInfo{Name: "crun"}

	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "containers")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	ops := gan.NewOps(manager, e, rtInfo, p, tmp)

	_, err := ops.Run(context.Background(), gan.RunOpts{Image: "nginx"})
	if err == nil {
		t.Fatal("expected error when bundle dir creation fails")
	}
}

func TestOps_Run_MonitorFails(t *testing.T) {
	ops, e := newOpsSetup(t)
	// State always returns NotFound → monitor times out.
	e.stateFn = func(_ context.Context, _ string) (*eld.State, error) {
		return nil, eld.ErrContainerNotFound
	}

	// Use a short timeout to make the monitor time out quickly.
	_, err := ops.Run(context.Background(), gan.RunOpts{
		Image:   "nginx",
		Timeout: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error when monitor fails")
	}
}

// ── Stop tests ────────────────────────────────────────────────────────────────

func TestOps_Stop_Success(t *testing.T) {
	ops, e := newOpsSetup(t)
	// Put a running container into the store.
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Stop(context.Background(), id, gan.StopOpts{}); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	updated, _ := ops.Manager.LoadContainer(context.Background(), id)
	if updated.Ka != gan.KaStopped {
		t.Errorf("Ka = %v; want KaStopped", updated.Ka)
	}
}

func TestOps_Stop_NotRunning(t *testing.T) {
	ops, _ := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaStopped
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	err := ops.Stop(context.Background(), id, gan.StopOpts{})
	if err == nil {
		t.Fatal("expected error stopping a non-running container")
	}
}

func TestOps_Stop_NotFound(t *testing.T) {
	ops, _ := newOpsSetup(t)
	err := ops.Stop(context.Background(), "nonexistent", gan.StopOpts{})
	if err == nil {
		t.Fatal("expected ErrContainerNotFound")
	}
}

func TestOps_Stop_KillError_AlreadyDead(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	// Kill returns "no such process" — should be treated as already dead.
	e.killErr = errors.New("no such process")
	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Stop(context.Background(), id, gan.StopOpts{}); err != nil {
		t.Fatalf("Stop with already-dead kill: %v", err)
	}
}

func TestOps_Stop_Force(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Stop(context.Background(), id, gan.StopOpts{Force: true}); err != nil {
		t.Fatalf("Stop Force: %v", err)
	}
}

func TestOps_Stop_WaitTimeout(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	// State always returns Running (never stops).
	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusRunning, Pid: 1}, nil
	}

	err := ops.Stop(context.Background(), id, gan.StopOpts{Timeout: 75 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestOps_Stop_DeleteRuntime_AlreadyGone(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}
	e.deleteErr = errors.New("container not found")

	if err := ops.Stop(context.Background(), id, gan.StopOpts{}); err != nil {
		t.Fatalf("Stop with already-gone delete: %v", err)
	}
}

// ── Rm tests ──────────────────────────────────────────────────────────────────

func TestOps_Rm_StoppedContainer(t *testing.T) {
	ops, _ := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaStopped
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	if err := ops.Rm(context.Background(), id, gan.RmOpts{}); err != nil {
		t.Fatalf("Rm: %v", err)
	}

	_, err := ops.Manager.LoadContainer(context.Background(), id)
	if !errors.Is(err, gan.ErrContainerNotFound) {
		t.Errorf("after Rm, expected ErrContainerNotFound; got: %v", err)
	}
}

func TestOps_Rm_RunningWithoutForce(t *testing.T) {
	ops, _ := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	err := ops.Rm(context.Background(), id, gan.RmOpts{})
	if err == nil {
		t.Fatal("expected error removing running container without force")
	}
	if !errors.Is(err, gan.ErrContainerRunning) {
		t.Errorf("expected ErrContainerRunning; got: %v", err)
	}
}

func TestOps_Rm_RunningWithForce(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677001234"
	ctr := sampleContainer(id, "web")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusStopped}, nil
	}

	if err := ops.Rm(context.Background(), id, gan.RmOpts{Force: true}); err != nil {
		t.Fatalf("Rm Force: %v", err)
	}
}

func TestOps_Rm_NotFound(t *testing.T) {
	ops, _ := newOpsSetup(t)
	err := ops.Rm(context.Background(), "nonexistent", gan.RmOpts{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── helper tests ──────────────────────────────────────────────────────────────

func TestOps_Stop_KillError_Generic(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677991234"
	ctr := sampleContainer(id, "w1")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	// Kill returns a non-dead error.
	e.killErr = errors.New("permission denied")
	err := ops.Stop(context.Background(), id, gan.StopOpts{})
	if err == nil {
		t.Fatal("expected error when kill fails with generic error")
	}
}

func TestOps_Stop_WaitStop_ContextCancelled(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677881234"
	ctr := sampleContainer(id, "w2")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, rid string) (*eld.State, error) {
		return &eld.State{ID: rid, Status: eld.StatusRunning, Pid: 1}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := ops.Stop(ctx, id, gan.StopOpts{Timeout: 5 * time.Second})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestOps_Stop_WaitStop_StateGone(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677771234"
	ctr := sampleContainer(id, "w3")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	// State returns "not found" → container gone → treated as stopped.
	e.stateFn = func(_ context.Context, _ string) (*eld.State, error) {
		return nil, errors.New("not found")
	}

	if err := ops.Stop(context.Background(), id, gan.StopOpts{}); err != nil {
		t.Fatalf("Stop with state-gone: %v", err)
	}
}

func TestOps_Stop_WaitStop_StateError(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677661234"
	ctr := sampleContainer(id, "w4")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	e.stateFn = func(_ context.Context, _ string) (*eld.State, error) {
		return nil, errors.New("kernel panic")
	}

	err := ops.Stop(context.Background(), id, gan.StopOpts{})
	if err == nil {
		t.Fatal("expected error on state failure during wait")
	}
}

func TestOps_Rm_ForceStop_Fails(t *testing.T) {
	ops, e := newOpsSetup(t)
	id := "aabb112233445566778899001122334455667788990011223344556677551234"
	ctr := sampleContainer(id, "w5")
	ctr.Ka = gan.KaRunning
	_ = ops.Manager.SaveContainer(context.Background(), ctr)

	// Kill returns generic error → Stop fails.
	e.killErr = errors.New("operation not permitted")

	err := ops.Rm(context.Background(), id, gan.RmOpts{Force: true})
	if err == nil {
		t.Fatal("expected error when force stop fails")
	}
}

func TestOps_Run_FindByNameError(t *testing.T) {
	// Use a store that errors on List (called by FindByName).
	manager := gan.NewManager(&listErrStore{}, t.TempDir())
	e := &opsEld{}
	p := &opsPrim{}
	rtInfo := eld.RuntimeInfo{Name: "crun"}
	ops := gan.NewOps(manager, e, rtInfo, p, t.TempDir())

	_, err := ops.Run(context.Background(), gan.RunOpts{
		Image: "nginx",
		Name:  "conflict-check",
	})
	if err == nil {
		t.Fatal("expected error when FindByName fails")
	}
}
