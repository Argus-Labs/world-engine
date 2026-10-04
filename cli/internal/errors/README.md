# errors package (CLI)

Minimal helpers for CLI-friendly error handling:
- Silent: never printed
- Priority + Must Show: clean user messages with full chain preserved for verbose
- One-liner printing in main

## Quick use

```go
// main.go
if pkgerrors.ShouldPrint(err) {
    // User-facing (short): highest priority + must-show
    printer.Errorln(pkgerrors.FormatError(err))
}
```

## Marking errors where they happen

```go
// 1) Silent: do not show this error to user
if userCanceled {
    return pkgerrors.NewSilentf("user canceled")
}

// 2) Priority: user-friendly message, preserved chain
if err := connectDB(); err != nil {
    return pkgerrors.SetPriority("Failed to connect to database", err, pkgerrors.PriorityHigh)
}

// 3) Must show: extra context that must be displayed
if err := api.CheckStatus(); err != nil {
    return pkgerrors.SetMustShow("service unavailable", err)
}
```

## What to call (90% use cases)

// Build final user-facing message from the error chain
- `pkgerrors.FormatError(err error) string`

// Silence errors from printing
- `pkgerrors.NewSilent(err error) error`
- `pkgerrors.NewSilentf(format string, args ...any) error`

// Check if a error is silenced
- `pkgerrors.ShouldPrint(err error) bool`

// Priority tagging (highest wins)
- `pkgerrors.SetPriority(msg string, err error, p int) error`
- `pkgerrors.SetPriorityf(format string, err error, p int, args ...any) error`
- `pkgerrors.NewPriority(err error, p int) error`        // tag existing err as priority
- `pkgerrors.NewPriorityf(p int, format string, args ...any) error` // originate new

// Must-show tagging (always appended after highest priority)
- `pkgerrors.SetMustShow(msg string, err error) error`
- `pkgerrors.SetMustShowf(format string, err error, p ...any) error`
- `pkgerrors.NewMustShow(err error) error`        // tag existing err as must-show
- `pkgerrors.NewMustShowf(format string, args ...any) error` // originate new

Priority levels:

```go
const (
    PriorityLow      = 1
    PriorityMedium   = 50
    PriorityHigh     = 100
    PriorityCritical = 200
)
```

## Notes

- `ShouldPrint(err)` checks the whole chain: if any link is silent, nothing is printed.
- `FormatError(err)` walks the chain, picks the highest priority short message, and appends any must-show short messages in order (no duplicates). If no priority exists, it falls back to `err.Error()` (full chain top-level string).



