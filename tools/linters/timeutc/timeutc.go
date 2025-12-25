// Package timeutc provides a linter that checks for time.Now() calls without .UTC()
package timeutc

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the timeutc analyzer that detects time.Now() calls without .UTC()
var Analyzer = &analysis.Analyzer{
	Name: "timeutc",
	Doc:  "checks for time.Now() calls without .UTC() to ensure timezone consistency",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	// Track which time.Now() calls are part of .UTC() chains
	nowCallsWithUTC := make(map[*ast.CallExpr]bool)

	// First pass: find all time.Now().UTC() patterns
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			// Look for selector expressions like x.UTC()
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check if this is .UTC()
			if sel.Sel.Name != "UTC" {
				return true
			}

			// Check if the receiver (X) is a call to time.Now()
			call, ok := sel.X.(*ast.CallExpr)
			if !ok {
				return true
			}

			if isTimeNow(call) {
				// Mark this time.Now() call as having .UTC()
				nowCallsWithUTC[call] = true
			}

			return true
		})
	}

	// Second pass: find all time.Now() calls and report those without .UTC()
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check if this is a time.Now() call
			if !isTimeNow(call) {
				return true
			}

			// If this call is already marked as having .UTC(), skip it
			if nowCallsWithUTC[call] {
				return true
			}

			// Check for nolint comment
			if hasNolintComment(pass, call) {
				return true
			}

			// Report the issue
			pass.Reportf(call.Pos(), "time.Now() should be followed by .UTC() for timezone consistency")

			return true
		})
	}

	return nil, nil
}

// isTimeNow checks if the call expression is time.Now()
func isTimeNow(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check if method is "Now"
	if sel.Sel.Name != "Now" {
		return false
	}

	// Check if package is "time"
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	return ident.Name == "time"
}

// hasNolintComment checks if there's a nolint comment for this position
func hasNolintComment(pass *analysis.Pass, call *ast.CallExpr) bool {
	pos := pass.Fset.Position(call.Pos())

	// Find the file containing this call
	var astFile *ast.File
	for _, f := range pass.Files {
		filePos := pass.Fset.Position(f.Pos())
		fileEnd := pass.Fset.Position(f.End())
		if filePos.Filename == pos.Filename {
			astFile = f
			break
		}
		_ = fileEnd
	}

	if astFile == nil {
		return false
	}

	// Check all comment groups in the file
	for _, cg := range astFile.Comments {
		for _, comment := range cg.List {
			commentPos := pass.Fset.Position(comment.Pos())

			// Check if comment is on the same line or the line before
			if commentPos.Line == pos.Line || commentPos.Line == pos.Line-1 {
				text := comment.Text
				// Support both //nolint and //nolint:timeutc
				if contains(text, "nolint") {
					// If it's a general nolint or specifically for timeutc
					if !contains(text, ":") || contains(text, "timeutc") {
						return true
					}
				}
			}
		}
	}

	return false
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				indexOfSubstring(s, substr) >= 0))
}

// indexOfSubstring returns the index of substr in s, or -1 if not found
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
