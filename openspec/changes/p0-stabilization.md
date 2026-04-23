# Change: p0-stabilization

**Phase:** 1 — The Gunslinger (MVP)
**Milestone:** 1.x — Stabilization & Technical Debt
**Status:** Completed

## What

Address high-priority (P0) gaps identified during the functional validation of Milestone 1.4. This includes moving core container lifecycle commands from stubs to functional implementations, fixing the application log capture in the native monitor, and hardening error handling with standardized wrapping and transactional rollback.

## Why

Maestro has successfully passed the "Fire Test" for its core logic, but the user interface (CLI) and resilience layers require stabilization to reach production-grade quality. Completing these P0s ensures that Maestro is not just "functional," but "reliable" and "orchestrative."

## Specs Affected

- `openspec/specs/gan-lifecycle/spec.md` — Container Create, Start, Kill, Inspect logic.
- `openspec/specs/eld-runtime/spec.md` — Log redirection and Monitor resilience.
- `openspec/specs/tower-engine/spec.md` — Error handling and rollback standards.

## Tasks

| # | Task | Complexity | Status | Validation |
|---|------|:----------:|:------:|------------|
| 1 | **Container Create** `[Roadmap #55]` — Spec gen + bundle prep without execution | 3 | ✅ | `internal/gan/create_test.go` |
| 2 | **Container Start** `[Roadmap #56]` — Load bundle + invoke OCI runtime start | 2 | ✅ | `internal/gan/start_test.go` |
| 3 | **Container Kill** `[Roadmap #58]` — Signal propagation via OCIRuntime.Kill | 2 | ✅ | `internal/gan/kill_test.go` |
| 4 | **Container Inspect** `[Roadmap #63]` — Export state + OCI spec to JSON/YAML | 2 | ✅ | `internal/cli/inspect_test.go` |
| 5 | **Log Redirection** `[Roadmap #64]` — Redirect container stdout/stderr to monitor | 4 | ✅ | `internal/eld/monitor_test.go` |
| 6 | **Error Handling Audit** — Standardize wrapping (`%w`) and implement rollback | 3 | ✅ | `make lint` + integration tests |
| 7 | **Verification** — Final validation of lifecycle isolation and logging | 1 | ✅ | `scripts/smoke-test.sh` |

## Acceptance Criteria

- `maestro container create` generates a valid OCI bundle without starting the process.
- `maestro logs` displays the actual application output (stdout/stderr).
- `maestro run` handles failures by cleaning up snapshots and state (atomic rollback).
- Full "Standard OCI UX" achieved for all P0 lifecycle commands.
