package crudo

import "reflect"

func GetType(i any) reflect.Type {
	t := reflect.TypeOf(i)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Array || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t
}
