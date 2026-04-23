package beam

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

type mockSyscallMounter struct {
	unshareFn func(flags int) error
	mountFn   func(source, target, fstype string, flags uintptr, data string) error
	unmountFn func(target string, flags int) error
}

func (m *mockSyscallMounter) Unshare(f int) error {
	if m.unshareFn != nil {
		return m.unshareFn(f)
	}
	return nil
}

func (m *mockSyscallMounter) Mount(s, t, ft string, fl uintptr, d string) error {
	if m.mountFn != nil {
		return m.mountFn(s, t, ft, fl, d)
	}
	return nil
}

func (m *mockSyscallMounter) Unmount(t string, f int) error {
	if m.unmountFn != nil {
		return m.unmountFn(t, f)
	}
	return nil
}

func TestTodash_NewNS_Failures(t *testing.T) {
	base := t.TempDir()
	nsID := "test-ns"

	t.Run("MkdirAllFail", func(st *testing.T) {
		fs := &mockFS{
			MkdirAllFn: func(_ string, _ os.FileMode) error {
				return errors.New("mkdir-fail")
			},
		}

		todash := NewTodash(base).WithFS(fs)
		_, _, err := todash.NewNS(nsID, nil)
		if err == nil || err.Error() != "failed to create netns directory "+base+": mkdir-fail" {
			st.Errorf("got error %v, want mkdir-fail", err)
		}
	})

	t.Run("CreateFileFail", func(st *testing.T) {
		fs := &mockFS{
			CreateFn: func(_ string) (*os.File, error) {
				return nil, errors.New("create-fail")
			},
			IsExistFn: func(_ error) bool { return false },
		}

		todash := NewTodash(base).WithFS(fs)
		_, _, err := todash.NewNS(nsID, nil)
		if err == nil ||
			err.Error() != "failed to create mount point "+todash.NSPath(nsID)+": create-fail" {
			st.Errorf("got error %v, want create-fail", err)
		}
	})

	t.Run("UnshareFail", func(st *testing.T) {
		mysys := &mockSyscallMounter{
			unshareFn: func(_ int) error {
				return errors.New("unshare-fail")
			},
		}

		todash := NewTodash(base)
		if rm, ok := todash.mounter.(*RealMounter); ok {
			rm.sys = mysys
		}
		_, _, err := todash.NewNS(nsID, nil)
		if err == nil || err.Error() != "failed to unshare network namespace: unshare-fail" {
			st.Errorf("got error %v, want unshare-fail", err)
		}
	})

	t.Run("MountFail", func(st *testing.T) {
		mysys := &mockSyscallMounter{
			mountFn: func(_ string, _ string, _ string, _ uintptr, _ string) error {
				return errors.New("mount-fail")
			},
		}

		todash := NewTodash(base)
		if rm, ok := todash.mounter.(*RealMounter); ok {
			rm.sys = mysys
		}
		_, _, err := todash.NewNS(nsID, nil)
		if err == nil ||
			err.Error() != "failed to bind mount network namespace to "+todash.NSPath(
				nsID,
			)+": mount-fail" {
			st.Errorf("got error %v, want mount-fail", err)
		}
	})
}

func TestTodash_DeleteNS_Failures(t *testing.T) {
	base := t.TempDir()
	nsID := "test-ns"

	t.Run("UnmountFail", func(st *testing.T) {
		mysys := &mockSyscallMounter{
			unmountFn: func(_ string, _ int) error {
				return syscall.EPERM
			},
		}

		todash := NewTodash(base)
		if rm, ok := todash.mounter.(*RealMounter); ok {
			rm.sys = mysys
		}
		err := todash.DeleteNS(nsID)
		if err == nil ||
			err.Error() != "failed to unmount network namespace "+todash.NSPath(
				nsID,
			)+": operation not permitted" {
			st.Errorf("got error %v, want EPERM", err)
		}
	})

	t.Run("RemoveFail", func(st *testing.T) {
		fs := &mockFS{
			RemoveFn: func(_ string) error {
				return errors.New("remove-fail")
			},
		}
		// mock unmount to succeed so it hits remove
		mysys := &mockSyscallMounter{
			unmountFn: func(_ string, _ int) error { return nil },
		}

		todash := NewTodash(base).WithFS(fs)
		if rm, ok := todash.mounter.(*RealMounter); ok {
			rm.sys = mysys
		}
		err := todash.DeleteNS(nsID)
		if err == nil ||
			err.Error() != "failed to remove network namespace file "+todash.NSPath(
				nsID,
			)+": remove-fail" {
			st.Errorf("got error %v, want remove-fail", err)
		}
	})
}

func TestTodash_DeleteNS_Success(t *testing.T) {
	base := t.TempDir()
	nsID := "test-ns"
	fs := &mockFS{
		RemoveFn: func(_ string) error { return nil },
	}
	mysys := &mockSyscallMounter{
		unmountFn: func(_ string, _ int) error { return nil },
	}
	todash := NewTodash(base).WithFS(fs)
	if rm, ok := todash.mounter.(*RealMounter); ok {
		rm.sys = mysys
	}

	err := todash.DeleteNS(nsID)
	if err != nil {
		t.Errorf("expected no error on DeleteNS success, got %v", err)
	}
}

func TestTodash_DeleteNS_IgnoredErrors(t *testing.T) {
	base := t.TempDir()
	nsID := "test-ns"

	t.Run("UnmountIgnored", func(_ *testing.T) {
		mysys := &mockSyscallMounter{
			unmountFn: func(_ string, _ int) error { return syscall.EINVAL },
		}
		fs := &mockFS{
			RemoveFn: func(_ string) error { return nil },
		}
		todash := NewTodash(base).WithFS(fs)
		if rm, ok := todash.mounter.(*RealMounter); ok {
			rm.sys = mysys
		}

		err := todash.DeleteNS(nsID)
		if err != nil {
			t.Errorf("expected EINVAL to be ignored in DeleteNS, got %v", err)
		}
	})

	t.Run("RemoveIgnored", func(_ *testing.T) {
		mysys := &mockSyscallMounter{
			unmountFn: func(_ string, _ int) error { return nil },
		}
		fs := &mockFS{
			RemoveFn: func(_ string) error { return os.ErrNotExist },
		}
		todash := NewTodash(base).WithFS(fs)
		if rm, ok := todash.mounter.(*RealMounter); ok {
			rm.sys = mysys
		}

		err := todash.DeleteNS(nsID)
		if err != nil {
			t.Errorf("expected ErrNotExist to be ignored in DeleteNS, got %v", err)
		}
	})
}

func TestTodash_NewNS_ExistingFile(t *testing.T) {
	base := t.TempDir()
	nsID := "test-ns"
	todash := NewTodash(base)
	nsPath := todash.NSPath(nsID)

	// Pre-create the file to hit the os.IsExist(err) path in NewNS
	f, err := os.Create(nsPath)
	if err != nil {
		t.Fatalf("fail to create file: %v", err)
	}
	if closeErr := f.Close(); closeErr != nil {
		t.Fatalf("fail to close file: %v", closeErr)
	}

	mysys := &mockSyscallMounter{
		unshareFn: func(_ int) error { return nil },
		mountFn:   func(_, _, _ string, _ uintptr, _ string) error { return nil },
	}
	if rm, ok := todash.mounter.(*RealMounter); ok {
		rm.sys = mysys
	}

	_, _, err = todash.NewNS(nsID, nil)
	if err != nil {
		t.Errorf("expected no error when file exists, got %v", err)
	}
}
