// Package ptr provides pointer helper functions.
// Similar to k8s.io/utils/ptr for working with pointer fields in configs.
package ptr

// To returns a pointer to the given value.
func To[T any](v T) *T {
	return &v
}

// Deref dereferences ptr and returns the value it points to if no nil,
// or else returns def.
func Deref[T any](ptr *T, def T) T {
	if ptr != nil {
		return *ptr
	}
	return def
}

// ToString converts a pointer to a string-based type to its string value.
// Returns empty string if the pointer is nil.
// The constraint ~string allows any type with string as underlying type (e.g., domain enums).
func ToString[T ~string](ptr *T) string {
	if ptr == nil {
		return ""
	}
	return string(*ptr)
}
