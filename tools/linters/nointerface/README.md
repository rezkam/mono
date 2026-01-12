# nointerface - Go Linter for interface{} → any

A custom Go linter that detects usage of `interface{}` and suggests replacing it with the more idiomatic `any` keyword (available since Go 1.18).

## Why This Matters

Since Go 1.18, `any` is a predeclared identifier that is an alias for `interface{}`. Using `any` provides several benefits:

- **More readable**: `any` is clearer and more concise than `interface{}`
- **More idiomatic**: Modern Go code uses `any` consistently
- **Less verbose**: Reduces visual noise in function signatures and type declarations

## Features

✅ **Detects all interface{} usage patterns:**
- Type declarations: `type T interface{}`
- Function parameters: `func f(v interface{})`
- Function returns: `func f() interface{}`
- Struct fields: `type S struct { F interface{} }`
- Variables: `var v interface{}`
- Maps: `map[string]interface{}`
- Slices: `[]interface{}`
- Channels: `chan interface{}`

✅ **Automatic fixes** via `-fix` flag

✅ **Nolint comment support:**
- General suppression: `//nolint`
- Specific suppression: `//nolint:nointerface`

✅ **Ignores non-empty interfaces:**
- Correctly distinguishes between `interface{}` and `interface { Method() }`

## Installation

The linter is built as part of the project:

```bash
make build-nointerface-linter
```

## Usage

### Run the linter

```bash
./nointerface ./...
```

### Automatically fix violations

```bash
./nointerface -fix ./...
```

### Run as part of lint suite

```bash
make lint
```

## Examples

### Before (violations)

```go
// Function parameters
func Process(data interface{}) error {
    // ...
}

// Return types
func GetConfig() interface{} {
    return config
}

// Struct fields
type Response struct {
    Data interface{} `json:"data"`
}

// Variables
var result interface{}
```

### After (fixed)

```go
// Function parameters
func Process(data any) error {
    // ...
}

// Return types
func GetConfig() any {
    return config
}

// Struct fields
type Response struct {
    Data any `json:"data"`
}

// Variables
var result any
```

### Suppressing Warnings

When you legitimately need to keep `interface{}` (e.g., for backward compatibility), use nolint comments:

```go
// General nolint (suppresses all linters)
//nolint
var legacy interface{}

// Specific nolint (only suppresses nointerface)
var legacy interface{} //nolint:nointerface
```

## Technical Details

- **Framework**: Built using `golang.org/x/tools/go/analysis`
- **AST-based**: Analyzes the Abstract Syntax Tree to detect empty interface types
- **Fix support**: Provides `SuggestedFixes` for automatic replacement
- **Comment-aware**: Respects nolint comments on the same line or line above

## Testing

Run the comprehensive test suite:

```bash
cd tools/linters/nointerface
go test -v
```

The test suite validates:
- All interface{} patterns are detected
- Non-empty interfaces are ignored
- Nolint comments work correctly
- Automatic fixes replace correctly

## Integration

The linter is integrated into:

1. **Make targets**: `make lint` runs this linter automatically
2. **Pre-commit hooks**: Included in `.githooks/pre-commit`
3. **CI/CD**: Runs automatically in GitHub Actions

## Architecture

```
nointerface/
├── README.md              # This file
├── nointerface.go         # Analyzer implementation
├── nointerface_test.go    # Test suite
├── cmd/
│   └── nointerface/
│       └── main.go        # CLI entry point
└── testdata/
    └── src/
        └── a/
            └── a.go       # Test fixtures
```

## See Also

- [Go 1.18 Release Notes](https://go.dev/doc/go1.18) - Introduction of `any` predeclared identifier
- [Effective Go](https://go.dev/doc/effective_go) - Go style guidelines
- [golang.org/x/tools/go/analysis](https://pkg.go.dev/golang.org/x/tools/go/analysis) - Analysis framework
