package env

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
)

// Validator is implemented by config structs that need validation.
type Validator interface {
	Validate() error
}

// ErrInvalidValue is returned when an environment variable value cannot be parsed.
type ErrInvalidValue struct {
	Field  string
	EnvVar string
	Value  string
	Err    error
}

func (e ErrInvalidValue) Error() string {
	return fmt.Sprintf("invalid value for %s=%q (field: %s): %v", e.EnvVar, e.Value, e.Field, e.Err)
}

func (e ErrInvalidValue) Unwrap() error {
	return e.Err
}

// ErrNotStructPointer is returned when Load is called with a non-pointer or non-struct argument.
type ErrNotStructPointer struct {
	Type string
}

func (e ErrNotStructPointer) Error() string {
	return fmt.Sprintf("env.Load: argument must be a pointer to struct, got %s", e.Type)
}

// ErrUnsupportedType is returned when a field has an unsupported type.
type ErrUnsupportedType struct {
	Kind string
}

func (e ErrUnsupportedType) Error() string {
	return fmt.Sprintf("unsupported type: %s", e.Kind)
}

// Load loads configuration from environment variables into the provided struct pointer.
// After parsing, it automatically validates any nested struct that implements Validator.
//
// Supported struct tags:
//   - env:"VAR_NAME" - maps field to environment variable VAR_NAME
//
// Supported field types:
//   - string
//   - int, int8, int16, int32, int64
//   - bool
//   - time.Duration (parses Go duration strings like "5s", "1m30s")
//
// Nested structs are loaded recursively. If a nested struct implements Validator,
// its Validate() method is called automatically after loading.
//
// Zero values are used for unset fields. Defaults should be handled by the
// consuming code (application/infrastructure layer).
func Load(v any) error {
	ptrVal := reflect.ValueOf(v)
	if ptrVal.Kind() != reflect.Pointer || ptrVal.Elem().Kind() != reflect.Struct {
		return ErrNotStructPointer{Type: fmt.Sprintf("%T", v)}
	}

	if err := parseStruct(ptrVal.Elem()); err != nil {
		return err
	}

	// Validate the root struct if it implements Validator
	if validator, ok := v.(Validator); ok {
		if err := validator.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func parseStruct(val reflect.Value) error {
	typ := val.Type()

	for i := range val.NumField() {
		field := val.Field(i)
		structField := typ.Field(i)

		// Skip unexported fields
		if !field.CanSet() {
			continue
		}

		// Handle nested structs recursively (skip time.Time which is a struct)
		if field.Kind() == reflect.Struct && field.Type() != reflect.TypeOf(time.Time{}) {
			if err := parseStruct(field); err != nil {
				return err
			}

			// After parsing, validate if the nested struct implements Validator
			if field.CanAddr() {
				if validator, ok := field.Addr().Interface().(Validator); ok {
					if err := validator.Validate(); err != nil {
						return err
					}
				}
			}
			continue
		}

		envKey := structField.Tag.Get("env")
		if envKey == "" {
			continue
		}

		envVal, exists := os.LookupEnv(envKey)
		if !exists {
			continue
		}

		if err := setField(field, envVal); err != nil {
			return ErrInvalidValue{
				Field:  structField.Name,
				EnvVar: envKey,
				Value:  envVal,
				Err:    err,
			}
		}
	}

	return nil
}

func setField(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		return nil

	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Special case for time.Duration
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
			return nil
		}

		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(i)
		return nil

	default:
		return ErrUnsupportedType{Kind: field.Kind().String()}
	}
}
