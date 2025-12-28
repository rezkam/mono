package a

// Test cases for interface{} detection and replacement with 'any'

// Bad: interface{} in type declaration
type BadType interface{} // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"

// Good: using any
type GoodType any

// Bad: interface{} in function parameter
func badParam(v interface{}) { // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"
	_ = v
}

// Good: using any in function parameter
func goodParam(v any) {
	_ = v
}

// Bad: interface{} in function return
func badReturn() interface{} { // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"
	return nil
}

// Good: using any in function return
func goodReturn() any {
	return nil
}

// Bad: interface{} in struct field
type BadStruct struct {
	Field interface{} // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"
}

// Good: using any in struct field
type GoodStruct struct {
	Field any
}

// Bad: interface{} in map value
var badMap map[string]interface{} // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"

// Good: using any in map value
var goodMap map[string]any

// Bad: interface{} in slice
var badSlice []interface{} // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"

// Good: using any in slice
var goodSlice []any

// Bad: interface{} in channel
var badChan chan interface{} // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"

// Good: using any in channel
var goodChan chan any

// Bad: nested interface{} in map of slices
var badNested map[string][]interface{} // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"

// Good: nested any in map of slices
var goodNested map[string][]any

// Bad: interface{} in type assertion
func badTypeAssertion() {
	var x interface{} // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"
	_ = x.(string)
}

// Good: any in type assertion
func goodTypeAssertion() {
	var x any
	_ = x.(string)
}

// Bad: multiple interface{} parameters
func badMultipleParams(a interface{}, b interface{}) { // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)" "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"
	_, _ = a, b
}

// Good: multiple any parameters
func goodMultipleParams(a any, b any) {
	_, _ = a, b
}

// Nolint tests - these should NOT be flagged

func nolintGeneral() {
	//nolint
	var x interface{}
	_ = x
}

func nolintSpecific() {
	var x interface{} //nolint:nointerface
	_ = x
}

func nolintOtherLinter() {
	var x interface{} //nolint:otherlinter // want "use 'any' instead of 'interface\\{\\}' \\(available since Go 1.18\\)"
	_ = x
}

// nolint
func nolintFunction(v interface{}) {
	_ = v
}

func nolintFieldAbove() {
	type S struct {
		//nolint
		Field interface{}
	}
	_ = S{}
}

// This is a non-empty interface, should NOT be flagged
type MyInterface interface {
	Method() string
}

func useMyInterface(v MyInterface) {
	_ = v
}

// This is also a non-empty interface, should NOT be flagged
type ComplexInterface interface {
	Method1() string
	Method2() int
}

func useComplexInterface(v ComplexInterface) {
	_ = v
}
