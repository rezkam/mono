package config

import (
	"os"
	"strconv"
)

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
