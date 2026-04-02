# Lint Patterns

This project uses golangci-lint v2 with a strict production config (based on
[maratori/golangci-lint-config](https://github.com/maratori/golangci-lint-config)).

Run `make ci-local` before every PR — it mirrors CI exactly, including
`golangci-lint config verify` and `govulncheck`.

## Recurring patterns and fixes

### govet strict shadow (most common)

Any `if err := something; err != nil` inside a function that already has an
outer `err` variable shadows it and is rejected.

**Fix:** rename the inner variable.

```go
// wrong
data, err := os.ReadFile(path)
if err := toml.Unmarshal(data, cfg); err != nil { ... }

// correct
data, err := os.ReadFile(path)
if unmarshalErr := toml.Unmarshal(data, cfg); unmarshalErr != nil { ... }
```

Common rename suffixes used in this codebase: `unmarshalErr`, `marshalErr`,
`writeErr`, `closeErr`, `renameErr`, `statErr`, `mkdirErr`, `flockErr`,
`migrateErr`, `execErr`, `fmtErr`.

---

### nolintlint: directive must be on the flagged line

`//nolint:X` must be on the **same line** as the code golangci-lint flags.
When `golines` reformats a multiline expression, the nolint comment ends up on
the closing `)` line while the issue is reported on the opening call line,
making the directive "unused".

**Fix:** place `//nolint:X` as a standalone comment on the line **immediately
before** the flagged statement (golangci-lint v2 supports this form).

```go
//nolint:gosec // G115: Flock requires int; fd fits in int on 64-bit platforms
err := syscall.Flock(int(f.Fd()), how)
```

---

### golines: 120-char line limit

Long `//coverage:ignore` or `//nolint` comments push lines past 120 chars.

**Fix options:**
- Shorten the comment text.
- Move `//nolint` to a standalone line above the statement.
- `//coverage:ignore` must stay on the same line as the code — shorten the
  text.

---

### gochecknoglobals

All package-level `var` declarations are flagged. Known legitimate globals in
this project and the required nolint reason:

| Variable | Package | Reason |
|---|---|---|
| `Version`, `Commit`, `BuildDate`, `GoVersion` | `cli` | `// ldflags injection: set at link time, read-only at runtime` |
| `globalFlags` | `cli` | `// shared flag state bound to cobra persistent flags` |
| `IsTerminalFn` | `cli` | `// dependency injection point: overridden in tests` |

```go
//nolint:gochecknoglobals // ldflags injection: set at link time, read-only at runtime
var (
    Version = "dev"
    ...
)
```

---

### reassign: zerolog global logger

`log.Logger = ...` (from `github.com/rs/zerolog/log`) triggers `reassign`.

**Fix:** assign to a local variable first, then reassign on a single line with
the nolint directive.

```go
logger := zerolog.New(w).With().Timestamp().Logger()
log.Logger = logger //nolint:reassign // zerolog idiom: global logger is designed to be reconfigured
```

---

### errorlint: use errors.Is for sentinel comparisons

`err == ErrSomething` and `err != ErrSomething` fail for wrapped errors and
are rejected by `errorlint`.

**Fix:** use `errors.Is`.

```go
// wrong
if err == ErrNotFound { ... }

// correct
if errors.Is(err, ErrNotFound) { ... }
```

---

### nonamedreturns

Named return values are banned. `func Foo() (x int, err error)` must become
`func Foo() (int, error)`.

Use local variables inside the function body instead of named returns.

---

### perfsprint: errors.New for static strings

`fmt.Errorf("message with no format verbs")` must be `errors.New(...)`.

**Fix:** replace `fmt.Errorf` with `errors.New` when no `%w` or other verbs
are present. Add `"errors"` to imports.

---

### revive unused-parameter

Unused function parameters must be renamed to `_` (or `_ TypeName` when the
type annotation aids readability).

```go
// wrong
func migrate(s *Store, from, to int) error { ... } // s never used

// correct
func migrate(_ *Store, from, to int) error { ... }
```

---

### godoclint: stdlib doc links

Comments mentioning stdlib identifiers should use `[pkg.Name]` link syntax.

| Text | Correct form |
|---|---|
| `os.Stderr` | `[os.Stderr]` |
| `bytes.Buffer` | `[bytes.Buffer]` |
| `os.Exit` | `[os.Exit]` |
| `os.File` | `[os.File]` |

---

### gosec G115: uintptr → int conversion

`int(someFile.Fd())` triggers G115 because `Fd()` returns `uintptr`.

**Fix:**

```go
//nolint:gosec // G115: Flock requires int; fd fits in int on all supported 64-bit platforms
err := syscall.Flock(int(f.Fd()), how)
```

---

### golangci-lint config verify vs run

`golangci-lint run` silently ignores unknown config keys. The CI action
(`golangci-lint-action@v9`) runs `golangci-lint config verify` first, which
validates the full JSON schema and fails on unknown keys — causing CI to fail
while local `run` passes.

**Always run `make ci-local` before pushing** — it runs `config verify` as its
first step.

Invalid v2 config keys (never use):

| Wrong key | Correct location |
|---|---|
| `formatters-settings` | `formatters.settings` (nested) |
| `issues.exclude-rules` | `linters.exclusions.rules` |
| `linters-settings` | `linters.settings` (nested) |

---

### mnd: magic numbers

Numeric literals other than 0 and 1 are flagged. Define a named constant.

Exception: `os.Chmod`, `os.Mkdir*`, `os.WriteFile`, and `os.OpenFile`
permission mode arguments are excluded by the project config.

---

### goconst: repeated string literals

When a string literal appears 3+ times and a matching constant already exists,
use the constant.

In this codebase use `string(FormatJSON)` instead of `"json"`,
`string(FormatYAML)` instead of `"yaml"`, etc.
