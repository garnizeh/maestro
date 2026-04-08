package prim

// Exported for testing from prim_test package.

func (v *VFS) SnapshotsDir() string          { return v.snapshotsDir() }
func (v *VFS) SnapshotDir(key string) string { return v.snapshotDir(key) }

func (v *VFS) ReadMeta(key string) (VFSMeta, error) { return v.readMeta(key) }
func WriteMeta(dir string, meta VFSMeta) error      { return writeMeta(dir, meta) }

func CopyDir(src, dst string) error  { return copyDir(src, dst) }
func CopyFile(src, dst string) error { return copyFile(src, dst) }

func (v *VFS) HasDependents(key string) (bool, error) { return v.hasDependents(key) }
func (v *VFS) CheckNotExists(key string) error        { return v.checkNotExists(key) }

type VFSMetaExport = VFSMeta
