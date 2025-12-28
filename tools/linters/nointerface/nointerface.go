// Package nointerface provides a linter that detects interface{} usage and suggests replacement with 'any'.
//
// Since Go 1.18, the predeclared identifier 'any' is an alias for interface{}.
// Using 'any' is more idiomatic and clearer than the verbose interface{}.
//
// The linter detects all occurrences of empty interface{} types and suggests
// replacing them with 'any'. It provides automatic fixes that can be applied
// via the -fix flag.
//
// Example violations:
//
//	var x interface{}           // Bad: verbose
//	var x any                   // Good: clear and idiomatic
//
//	func Process(v interface{}) // Bad
//	func Process(v any)         // Good
//
// The linter respects //nolint comments to suppress warnings when needed.
package nointerface

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the nointerface analyzer that detects interface{} and suggests using 'any'.
// It provides automatic fixes to replace interface{} with any.
var Analyzer = &analysis.Analyzer{
	Name: "nointerface",
	Doc:  "checks for interface{} usage and suggests using 'any' (available since Go 1.18)",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			// Look for interface type declarations
			ifaceType, ok := n.(*ast.InterfaceType)
			if !ok {
				return true
			}

			// Check if this is an empty interface (interface{})
			// An empty interface has no methods
			if !isEmptyInterface(ifaceType) {
				return true
			}

			// Check for nolint comment
			if hasNolintComment(pass, ifaceType) {
				return true
			}

			// Calculate the exact positions for replacement
			start := ifaceType.Pos()
			end := ifaceType.End()

			// Report the issue with a suggested fix
			pass.Report(analysis.Diagnostic{
				Pos:     start,
				End:     end,
				Message: "use 'any' instead of 'interface{}' (available since Go 1.18)",
				SuggestedFixes: []analysis.SuggestedFix{
					{
						Message: "Replace 'interface{}' with 'any'",
						TextEdits: []analysis.TextEdit{
							{
								Pos:     start,
								End:     end,
								NewText: []byte("any"),
							},
						},
					},
				},
			})

			return true
		})
	}

	return nil, nil
}

// isEmptyInterface checks if an interface type is empty (has no methods).
// An empty interface{} can be replaced with 'any' (Go 1.18+).
func isEmptyInterface(iface *ast.InterfaceType) bool {
	// Empty interface has no methods in the method list
	if iface.Methods == nil || len(iface.Methods.List) == 0 {
		return true
	}
	return false
}

// hasNolintComment checks if there's a nolint comment for this position.
// Supports both general //nolint and specific //nolint:nointerface.
func hasNolintComment(pass *analysis.Pass, node ast.Node) bool {
	pos := pass.Fset.Position(node.Pos())

	// Find the file containing this node
	var astFile *ast.File
	for _, f := range pass.Files {
		filePos := pass.Fset.Position(f.Pos())
		if filePos.Filename == pos.Filename {
			astFile = f
			break
		}
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
				// Support both //nolint and //nolint:nointerface
				if contains(text, "nolint") {
					// If it's a general nolint or specifically for nointerface
					if !contains(text, ":") || contains(text, "nointerface") {
						return true
					}
				}
			}
		}
	}

	return false
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				indexOfSubstring(s, substr) >= 0))
}

// indexOfSubstring returns the index of substr in s, or -1 if not found.
func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
