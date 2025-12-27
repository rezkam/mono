package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// ErrMissingEnvVar is returned when a required environment variable is not set or invalid.
var ErrMissingEnvVar = errors.New("required environment variable is not set or invalid")

// GetEnv retrieves and parses an environment variable.
// Returns (value, true) if env var is set and successfully parsed.
// Returns (zero, false) if env var is not set or parsing fails.
func GetEnv[T string | int | bool](key string) (T, bool) {
	value := os.Getenv(key)
	var zero T

	if value == "" {
		return zero, false
	}

	var result any
	switch any(zero).(type) {
	case string:
		result = value
	case int:
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return zero, false
		}
		result = intVal
	case bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return zero, false
		}
		result = boolVal
	default:
		return zero, false
	}

	return result.(T), true
}

// MustGetEnv retrieves and parses a required environment variable.
// Returns ErrMissingEnvVar if the variable is not set or cannot be parsed.
// Use this for critical configuration that must be present for the application to function.
func MustGetEnv[T string | int | bool](key string) (T, error) {
	value, ok := GetEnv[T](key)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%w: %s", ErrMissingEnvVar, key)
	}
	return value, nil
}
