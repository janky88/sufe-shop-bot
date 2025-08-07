package httpadmin

import (
	"fmt"
	"reflect"
)

// toFloat64 converts various numeric types to float64
func toFloat64(i interface{}) (float64, error) {
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	default:
		return 0, fmt.Errorf("cannot convert %v to float64", v.Type())
	}
}