package cli

import (
	"bytes"
	"testing"
)

func TestInitLoggerInternal(t *testing.T) {
	buf := new(bytes.Buffer)
	err := InitLoggerTo(nil, buf, "debug", true, func(_ uintptr) bool { return true })
	if err != nil {
		t.Fatalf("InitLoggerTo: %v", err)
	}
}

func TestPrintFfInternal(t *testing.T) {
	buf := new(bytes.Buffer)
	printFf(buf, "hello %s", "world")
	if buf.String() != "hello world" {
		t.Errorf("expected hello world, got %q", buf.String())
	}
}
