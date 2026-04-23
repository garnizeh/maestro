package maturin_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

// ── ListImages ────────────────────────────────────────────────────────────────

func TestStore_ListImages_EmptyStore(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	imgs, err := s.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages on empty store: %v", err)
	}
	if len(imgs) != 0 {
		t.Errorf("expected 0 images, got %d", len(imgs))
	}
}

func TestStore_ListImages_AfterPull(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 2)

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	imgs, err := s.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(imgs) != 1 {
		t.Fatalf("expected 1 image, got %d", len(imgs))
	}

	got := imgs[0]
	if !strings.Contains(got.Repository, "nginx") {
		t.Errorf("Repository = %q, want contains 'nginx'", got.Repository)
	}
	if got.Tag != "latest" {
		t.Errorf("Tag = %q, want 'latest'", got.Tag)
	}
	if len(got.ShortID) != 12 {
		t.Errorf("ShortID = %q, want 12 chars", got.ShortID)
	}
	if got.Size <= 0 {
		t.Errorf("Size = %d, want > 0", got.Size)
	}
}

func TestStore_ListImages_MultipleTags(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw nginx:latest: %v", err)
	}
	if err := s.Draw(context.Background(), imageClient(img), "nginx:1.25", maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw nginx:1.25: %v", err)
	}

	imgs, err := s.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(imgs) != 2 {
		t.Errorf("expected 2 images, got %d", len(imgs))
	}
}

// ── InspectImage ──────────────────────────────────────────────────────────────

func TestStore_InspectImage_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	result, err := s.InspectImage("nginx:latest")
	if err != nil {
		t.Fatalf("InspectImage: %v", err)
	}
	if result == nil {
		t.Fatal("InspectImage returned nil result")
	}
	if len(result.ID) != 12 {
		t.Errorf("ID = %q, want 12 chars", result.ID)
	}
	if result.Manifest == nil {
		t.Error("Manifest is nil")
	}
	if result.Config == nil {
		t.Error("Config is nil")
	}
}

func TestStore_InspectImage_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.InspectImage("nonexistent:tag")
	if err == nil {
		t.Fatal("expected error for unknown image, got nil")
	}
}

func TestStore_InspectImage_InvalidRef(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.InspectImage("::invalid::")
	if err == nil {
		t.Fatal("expected error for invalid reference, got nil")
	}
}

// ── ImageHistory ──────────────────────────────────────────────────────────────

func TestStore_ImageHistory_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 2)

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	entries, err := s.ImageHistory("nginx:latest")
	if err != nil {
		t.Fatalf("ImageHistory: %v", err)
	}
	// random images have history entries equal to number of layers
	if len(entries) == 0 {
		t.Error("expected at least one history entry")
	}
}

func TestStore_ImageHistory_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.ImageHistory("nonexistent:tag")
	if err == nil {
		t.Fatal("expected error for unknown image, got nil")
	}
}

func TestStore_ImageHistory_InvalidRef(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.ImageHistory("::invalid::")
	if err == nil {
		t.Fatal("expected error for invalid reference, got nil")
	}
}

// ── RemoveImage ───────────────────────────────────────────────────────────────

func TestStore_RemoveImage_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	if err := s.RemoveImage(context.Background(), "nginx:latest"); err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}

	// Image should no longer appear in listing.
	imgs, err := s.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages after remove: %v", err)
	}
	if len(imgs) != 0 {
		t.Errorf("expected 0 images after remove, got %d", len(imgs))
	}
}

func TestStore_RemoveImage_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	err := s.RemoveImage(context.Background(), "nonexistent:tag")
	if err == nil {
		t.Fatal("expected error for unknown image, got nil")
	}
}

func TestStore_RemoveImage_InvalidRef(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	err := s.RemoveImage(context.Background(), "::invalid::")
	if err == nil {
		t.Fatal("expected error for invalid reference, got nil")
	}
}

func TestStore_RemoveImage_DigestRefUnsupported(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	imgDigest, err := img.Digest()
	if err != nil {
		t.Fatalf("failed to get image digest: %v", err)
	}
	refStr := "nginx@" + imgDigest.String()

	if drawErr := s.Draw(context.Background(), imageClient(img), refStr, maturin.DrawOptions{}); drawErr != nil {
		t.Fatalf("Draw: %v", drawErr)
	}

	err = s.RemoveImage(context.Background(), refStr)
	if err == nil {
		t.Fatal("expected error for digest reference, got nil")
	}
}

// ── resolveRef / InspectImage for digest refs ─────────────────────────────────

func TestStore_InspectImage_DigestRefUnsupported(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	imgDigest, err := img.Digest()
	if err != nil {
		t.Fatalf("failed to get image digest: %v", err)
	}
	refStr := "nginx@" + imgDigest.String()

	if drawErr := s.Draw(context.Background(), imageClient(img), refStr, maturin.DrawOptions{}); drawErr != nil {
		t.Fatalf("Draw: %v", drawErr)
	}

	_, err = s.InspectImage(refStr)
	if err == nil {
		t.Fatal("expected error for digest reference, got nil")
	}
}
