package gan

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kr/pretty"
)

// ── in-memory store for testing ───────────────────────────────────────────────

type memStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte
	// putErr, if set, is returned by Put.
	putErr error
	// getErr, if set, is returned by Get.
	getErr error
	// putFn, if set, is called by Put.
	putFn func() error
	// deleteErr, if set, is returned by Delete.
	deleteErr error
	// listErr, if set, is returned by List.
	listErr error
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string]map[string][]byte)}
}

func (m *memStore) Put(collection, key string, v any) error {
	if m.putErr != nil {
		return m.putErr
	}
	if m.putFn != nil {
		if err := m.putFn(); err != nil {
			return err
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[collection] == nil {
		m.data[collection] = make(map[string][]byte)
	}
	m.data[collection][key] = data
	return nil
}

func (m *memStore) Get(collection, key string, v any) error {
	if m.getErr != nil {
		return m.getErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.data[collection]
	if !ok {
		return errors.New("not found")
	}
	data, ok := c[key]
	if !ok {
		return errors.New("not found")
	}
	return json.Unmarshal(data, v)
}

func (m *memStore) Delete(collection, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.data[collection]
	if !ok {
		return errors.New("not found")
	}
	if _, exists := c[key]; !exists {
		return errors.New("not found")
	}
	delete(c, key)
	return nil
}

func (m *memStore) List(collection string) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	c := m.data[collection]
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(newMemStore(), t.TempDir())
}

func sampleContainer(id, name string) *Container {
	return &Container{
		ID:          id,
		Name:        name,
		Image:       "nginx:latest",
		Ka:          KaCreated,
		BundlePath:  "/bundle/" + id,
		RootFSPath:  "/rootfs/" + id,
		LogPath:     "/log/" + id + ".log",
		RuntimeName: "crun",
		Created:     time.Now().UTC().Truncate(time.Second),
	}
}

// ── Ka tests ──────────────────────────────────────────────────────────────────

func TestKa_String(t *testing.T) {
	cases := []struct {
		k    Ka
		want string
	}{
		{KaCreated, "created"},
		{KaRunning, "running"},
		{KaStopped, "stopped"},
		{KaDeleted, "deleted"},
		{Ka(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("Ka(%d).String() = %q; want %q", tc.k, got, tc.want)
		}
	}
}

func TestKa_MarshalUnmarshal(t *testing.T) {
	states := []Ka{KaCreated, KaRunning, KaStopped, KaDeleted}
	for _, ka := range states {
		data, err := ka.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%v): %v", ka, err)
		}
		var out Ka
		if unmarshalErr := out.UnmarshalText(data); unmarshalErr != nil {
			t.Fatalf("UnmarshalText(%q): %v", data, unmarshalErr)
		}
		if out != ka {
			t.Errorf("round-trip %v → %q → %v; want same", ka, data, out)
		}
	}
}

func TestKa_UnmarshalText_Unknown(t *testing.T) {
	var k Ka
	if err := k.UnmarshalText([]byte("banished")); err == nil {
		t.Fatal("expected error for unknown Ka state")
	}
}

// ── CanTransition tests ───────────────────────────────────────────────────────

func TestCanTransition_Valid(t *testing.T) {
	valid := []struct {
		from, to Ka
	}{
		{KaCreated, KaRunning},
		{KaCreated, KaDeleted},
		{KaRunning, KaStopped},
		{KaStopped, KaDeleted},
	}
	for _, tc := range valid {
		if !CanTransition(tc.from, tc.to) {
			t.Errorf("CanTransition(%v, %v) = false; want true", tc.from, tc.to)
		}
	}
}

func TestCanTransition_Invalid(t *testing.T) {
	invalid := []struct {
		from, to Ka
	}{
		{KaCreated, KaStopped},
		{KaCreated, KaCreated},
		{KaRunning, KaCreated},
		{KaRunning, KaDeleted},
		{KaStopped, KaRunning},
		{KaDeleted, KaCreated},
		{KaDeleted, KaDeleted},
	}
	for _, tc := range invalid {
		if CanTransition(tc.from, tc.to) {
			t.Errorf("CanTransition(%v, %v) = true; want false", tc.from, tc.to)
		}
	}
}

// ── Manager tests ─────────────────────────────────────────────────────────────

func TestManager_SaveAndLoad(t *testing.T) {
	m := newManager(t)
	c := sampleContainer("aabbccdd11223344556677889900aabbccdd11223344556677889900aabb1234", "web")

	if err := m.SaveContainer(context.Background(), c); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}

	got, err := m.LoadContainer(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("LoadContainer: %v", err)
	}
	if diff := pretty.Diff(c, got); len(diff) > 0 {
		t.Log("SaveContainer/LoadContainer mismatch")
		t.Logf("want: %v", c)
		t.Logf("got: %v", got)
		t.Errorf("\n%s", diff)
	}
}

func TestManager_LoadNotFound(t *testing.T) {
	m := newManager(t)
	_, err := m.LoadContainer(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected ErrContainerNotFound")
	}
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound; got: %v", err)
	}
}

func TestManager_LoadError(t *testing.T) {
	ms := newMemStore()
	ms.getErr = errors.New("disk failure")
	m := NewManager(ms, "/tmp")

	_, err := m.LoadContainer(context.Background(), "ctr1")
	if err == nil {
		t.Fatal("expected error on store failure")
	}
}

func TestManager_DeleteContainer(t *testing.T) {
	m := newManager(t)
	c := sampleContainer("aabb112233445566778899001122334455667788990011223344556677881234", "db")
	if err := m.SaveContainer(context.Background(), c); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}

	if err := m.DeleteContainer(context.Background(), c.ID); err != nil {
		t.Fatalf("DeleteContainer: %v", err)
	}

	_, err := m.LoadContainer(context.Background(), c.ID)
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("after delete, expected ErrContainerNotFound; got: %v", err)
	}
}

func TestManager_DeleteNotFound(t *testing.T) {
	m := newManager(t)
	err := m.DeleteContainer(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected ErrContainerNotFound")
	}
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound; got: %v", err)
	}
}

func TestManager_SaveError(t *testing.T) {
	ms := newMemStore()
	ms.putErr = errors.New("disk full")
	m := NewManager(ms, "/tmp")
	c := sampleContainer("x", "x")

	err := m.SaveContainer(context.Background(), c)
	if err == nil {
		t.Fatal("expected error on put failure")
	}
}

func TestManager_ListContainers_Empty(t *testing.T) {
	m := newManager(t)
	ctrs, err := m.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(ctrs) != 0 {
		t.Errorf("expected 0; got %d", len(ctrs))
	}
}

func TestManager_ListContainers_Multiple(t *testing.T) {
	m := newManager(t)
	id1 := "aabb112233445566778899001122334455667788990011223344556677001234"
	id2 := "ccdd112233445566778899001122334455667788990011223344556677001234"
	if err := m.SaveContainer(context.Background(), sampleContainer(id1, "c1")); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}
	if err := m.SaveContainer(context.Background(), sampleContainer(id2, "c2")); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}

	ctrs, err := m.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(ctrs) != 2 {
		t.Errorf("expected 2 containers; got %d", len(ctrs))
	}
}

func TestManager_Transition_Valid(t *testing.T) {
	m := newManager(t)
	id := "aabb112233445566778899001122334455667788990011223344556677111234"
	if err := m.SaveContainer(context.Background(), sampleContainer(id, "tr-test")); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}

	c, err := m.Transition(context.Background(), id, KaRunning)
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if c.Ka != KaRunning {
		t.Errorf("Ka = %v; want KaRunning", c.Ka)
	}
}

func TestManager_Transition_Invalid(t *testing.T) {
	m := newManager(t)
	id := "aabb112233445566778899001122334455667788990011223344556677221234"
	if err := m.SaveContainer(context.Background(), sampleContainer(id, "inv-tr")); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}

	_, err := m.Transition(context.Background(), id, KaStopped)
	if err == nil {
		t.Fatal("expected error on invalid transition")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition; got: %v", err)
	}
}

func TestManager_Transition_NotFound(t *testing.T) {
	m := newManager(t)
	_, err := m.Transition(context.Background(), "nonexistent", KaRunning)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound; got: %v", err)
	}
}

func TestManager_Transition_SaveError(t *testing.T) {
	ms := newMemStore()
	m := NewManager(ms, "/tmp")
	id := "aabb"
	c := sampleContainer(id, "x")
	c.Ka = KaCreated
	if err := m.SaveContainer(context.Background(), c); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}
	// Now make Put fail.
	ms.putErr = errors.New("disk full")

	_, err := m.Transition(context.Background(), id, KaRunning)
	if err == nil {
		t.Fatal("expected save error during transition")
	}
}

func TestManager_FindByName_Found(t *testing.T) {
	m := newManager(t)
	id := "aabb112233445566778899001122334455667788990011223344556677331234"
	if err := m.SaveContainer(context.Background(), sampleContainer(id, "my-web")); err != nil {
		t.Fatalf("SaveContainer: %v", err)
	}

	found, err := m.FindByName(context.Background(), "my-web")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if found == nil {
		t.Fatal("expected non-nil result")
	}
	if found.Name != "my-web" {
		t.Errorf("Name = %q; want my-web", found.Name)
	}
}

func TestManager_FindByName_NotFound(t *testing.T) {
	m := newManager(t)
	found, err := m.FindByName(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil; got %+v", found)
	}
}

// ── Summarise tests ───────────────────────────────────────────────────────────

func TestSummarise_LongID(t *testing.T) {
	c := sampleContainer("aabb112233445566778899001122334455667788990011223344556677441234", "s1")
	s := Summarise(c)
	if len(s.ShortID) != 12 {
		t.Errorf("ShortID length = %d; want 12", len(s.ShortID))
	}
	if s.Ka != "created" {
		t.Errorf("Ka = %q; want created", s.Ka)
	}
}

func TestSummarise_ShortID(t *testing.T) {
	c := sampleContainer("abc", "s2")
	s := Summarise(c)
	if s.ShortID != "abc" {
		t.Errorf("ShortID = %q; want abc", s.ShortID)
	}
}

// ── isNotFound helper tests ──────────────────────────────────────────────────

func TestManager_DeleteContainer_GenericError(t *testing.T) {
	// Use a store where Delete fails with a non-notFound error.
	ms := &genericDeleteErrStore{data: map[string][]byte{
		"ctr1": []byte(
			`{"id":"ctr1","name":"x","image":"img","ka":"created","runtimeName":"crun","created":"2026-01-01T00:00:00Z"}`,
		),
	}}
	m := NewManager(ms, "/tmp")

	err := m.DeleteContainer(context.Background(), "ctr1")
	if err == nil {
		t.Fatal("expected error from delete")
	}
}

// genericDeleteErrStore always returns a generic error from Delete.
type genericDeleteErrStore struct {
	data map[string][]byte
}

func (s *genericDeleteErrStore) Put(_, _ string, _ any) error { return nil }
func (s *genericDeleteErrStore) Get(_, key string, v any) error {
	data, ok := s.data[key]
	if !ok {
		return errors.New("not found")
	}
	return json.Unmarshal(data, v)
}
func (s *genericDeleteErrStore) Delete(_, _ string) error {
	return errors.New("storage backend failure")
}
func (s *genericDeleteErrStore) List(_ string) ([]string, error) { return nil, nil }

func TestManager_ListContainers_SkipsCorrupt(t *testing.T) {
	ms := newMemStore()
	m := NewManager(ms, "/tmp")
	// Put a valid container.
	id1 := "aaaa112233445566778899001122334455667788990011223344556677001234"
	c1 := sampleContainer(id1, "c1")
	if err := ms.Put("containers", id1, c1); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Inject corrupt data manually.
	ms.InjectCorrupt("containers", "corrupt", []byte("{invalid json"))

	ctrs, err := m.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(ctrs) != 1 {
		t.Errorf("expected 1 valid container; got %d", len(ctrs))
	}
}

func (m *memStore) InjectCorrupt(collection, key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[collection] == nil {
		m.data[collection] = make(map[string][]byte)
	}
	m.data[collection][key] = data
}

func TestManager_ListContainers_ListError(t *testing.T) {
	m := NewManager(&listErrStore{}, "/tmp")
	_, err := m.ListContainers(context.Background())
	if err == nil {
		t.Fatal("expected error from List failure")
	}
}

// listErrStore returns an error from List.
type listErrStore struct{}

func (s *listErrStore) Put(_, _ string, _ any) error { return nil }
func (s *listErrStore) Get(_, _ string, _ any) error { return nil }
func (s *listErrStore) Delete(_, _ string) error     { return nil }
func (s *listErrStore) List(_ string) ([]string, error) {
	return nil, errors.New("storage unavailable")
}

func TestManager_FindByName_ListError(t *testing.T) {
	m := NewManager(&listErrStore{}, "/tmp")
	_, err := m.FindByName(context.Background(), "any-name")
	if err == nil {
		t.Fatal("expected error from FindByName when List fails")
	}
}

func TestIsNotFound(t *testing.T) {
	if isNotFound(nil) {
		t.Error("nil is not found")
	}
	if !isNotFound(errors.New("not found")) {
		t.Error("not found string should be identified")
	}
	if isNotFound(errors.New("other")) {
		t.Error("other error is not found")
	}
}
