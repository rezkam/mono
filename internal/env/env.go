package env

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
)

// Parse parses environment variables into the provided struct pointer.
// It supports `env` and `default` tags.
func Parse(v interface{}) error {
	ptrVal := reflect.ValueOf(v)
	if ptrVal.Kind() != reflect.Ptr || ptrVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("env: validation target must be a struct pointer")
	}

	val := ptrVal.Elem()
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		structField := typ.Field(i)

		envKey := structField.Tag.Get("env")
		if envKey == "" {
			continue
		}

		defaultValue := structField.Tag.Get("default")
		envVal, ok := os.LookupEnv(envKey)

		if !ok {
			if defaultValue == "" {
				// No value and no default, leaving as zero value
				continue
			}
			envVal = defaultValue
		}

		if err := setField(field, envVal); err != nil {
			return fmt.Errorf("env: error parsing field %s: %w", structField.Name, err)
		}
	}

	return nil
}

func setField(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(i)
	default:
		return fmt.Errorf("unsupported type %s", field.Kind())
	}
	return nil
}
