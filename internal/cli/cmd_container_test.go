package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/cli"
	"github.com/rodrigo-baliza/maestro/internal/gan"
)

func TestContainerCmd_Stop(t *testing.T) {
	h := cli.NewHandler()
	h.ContainerOpsFn = func(_ context.Context, _ string) (*gan.Ops, error) {
		return nil, errors.New("mock-stop-called")
	}

	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"container", "stop", "my-ctr"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "mock-stop-called") {
		t.Fatalf("expected mock-stop-called error, got: %v", err)
	}
}

func TestContainerCmd_Rm(t *testing.T) {
	h := cli.NewHandler()
	h.ContainerOpsFn = func(_ context.Context, _ string) (*gan.Ops, error) {
		return nil, errors.New("mock-rm-called")
	}

	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"container", "rm", "to-remove"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "mock-rm-called") {
		t.Fatalf("expected mock-rm-called error, got: %v", err)
	}
}

func TestContainerCmd_Ps(t *testing.T) {
	h := cli.NewHandler()
	h.ContainerOpsFn = func(_ context.Context, _ string) (*gan.Ops, error) {
		return nil, errors.New("mock-ps-called")
	}

	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"container", "ps", "--all"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "mock-ps-called") {
		t.Fatalf("expected mock-ps-called error, got: %v", err)
	}
}

func TestContainerCmd_Port(t *testing.T) {
	h := cli.NewHandler()
	h.ContainerOpsFn = func(_ context.Context, _ string) (*gan.Ops, error) {
		return nil, errors.New("mock-port-called")
	}

	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"container", "port", "my-ctr"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "mock-port-called") {
		t.Fatalf("expected mock-port-called error, got: %v", err)
	}
}

func TestContainerCmd_Logs(t *testing.T) {
	h := cli.NewHandler()
	h.ContainerOpsFn = func(_ context.Context, _ string) (*gan.Ops, error) {
		return nil, errors.New("mock-logs-called")
	}

	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"container", "logs", "my-ctr"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "mock-logs-called") {
		t.Fatalf("expected mock-logs-called error, got: %v", err)
	}
}
