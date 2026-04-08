package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{int64(1.5 * 1024 * 1024), "1.5 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, tc := range tests {
		got := formatBytes(tc.input)
		if got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPullProgress_OnLayerDone_Pulled(t *testing.T) {
	t.Parallel()
	buf := new(bytes.Buffer)
	p := newPullProgress(buf)

	p.OnLayerDone(maturin.LayerEvent{Digest: "aabbcc112233", Skipped: false, Size: 2048})

	out := buf.String()
	if !strings.Contains(out, "aabbcc112233") {
		t.Errorf("expected digest in output, got: %s", out)
	}
	if !strings.Contains(out, "pulled") {
		t.Errorf("expected 'pulled' in output, got: %s", out)
	}
	if !strings.Contains(out, "2.0 KB") {
		t.Errorf("expected size in output, got: %s", out)
	}
}

func TestPullProgress_OnLayerDone_Skipped(t *testing.T) {
	t.Parallel()
	buf := new(bytes.Buffer)
	p := newPullProgress(buf)

	p.OnLayerDone(maturin.LayerEvent{Digest: "ddeeff445566", Skipped: true, Size: 0})

	out := buf.String()
	if !strings.Contains(out, "ddeeff445566") {
		t.Errorf("expected digest in output, got: %s", out)
	}
	if !strings.Contains(out, "already present") {
		t.Errorf("expected 'already present' in output, got: %s", out)
	}
}

func TestPullProgress_Summary(t *testing.T) {
	t.Parallel()
	buf := new(bytes.Buffer)
	p := newPullProgress(buf)

	p.OnLayerDone(maturin.LayerEvent{Digest: "aaaa11112222", Skipped: false, Size: 1024})
	p.OnLayerDone(maturin.LayerEvent{Digest: "bbbb33334444", Skipped: true})

	p.Summary("nginx:latest")

	out := buf.String()
	if !strings.Contains(out, "Pull complete") {
		t.Errorf("expected 'Pull complete' in output, got: %s", out)
	}
	if !strings.Contains(out, "nginx:latest") {
		t.Errorf("expected ref in output, got: %s", out)
	}
}

func TestPullProgress_ConcurrentOnLayerDone(t *testing.T) {
	t.Parallel()
	buf := new(bytes.Buffer)
	p := newPullProgress(buf)

	done := make(chan struct{})
	for i := range 10 {
		go func(i int) {
			p.OnLayerDone(maturin.LayerEvent{Digest: "aabbccddeeff", Skipped: i%2 == 0, Size: 512})
			done <- struct{}{}
		}(i)
	}
	for range 10 {
		<-done
	}
	// No race, no panic — just verify something was written.
	if buf.Len() == 0 {
		t.Error("expected output from concurrent OnLayerDone calls")
	}
}
