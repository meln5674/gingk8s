package gingk8s

import (
	"context"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
)

const (
	badFuncErrFmt = `func() values provided to Set must return a compliant type or a compliant type and an error, and must have one of the following signatures.
(context.Context)
(context.Context, gingk8s.Cluster)
(gingk8s.Gingk8s, context.Context, gingk8s.Cluster)
Instead, got %v`
)

func scalarString(g Gingk8s, ctx context.Context, cluster Cluster, s *strings.Builder, v Value) error {
	s.WriteString(strings.ReplaceAll(fmt.Sprintf("%v", v), ",", `\,`))
	return nil
}

func resolveRArray(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) ([]interface{}, error) {
	var err error
	out := make([]interface{}, val.Len())
	for ix := 0; ix < val.Len(); ix++ {
		out[ix], err = resolveRValue(g, ctx, cluster, val.Index(ix))
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func resolveRNestedArray(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) ([]interface{}, error) {
	var err error
	out := make([]interface{}, val.Len())
	for ix := 0; ix < val.Len(); ix++ {
		out[ix], err = resolveRNestedValue(g, ctx, cluster, val.Index(ix))
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rarrayString(g Gingk8s, ctx context.Context, cluster Cluster, s *strings.Builder, val reflect.Value) error {
	s.WriteString("{")
	defer s.WriteString("}")
	for ix := 0; ix < val.Len(); ix++ {
		if ix != 0 {
			s.WriteString(",")
		}
		err := rvalueString(g, ctx, cluster, s, val.Index(ix))
		if err != nil {
			return err
		}
	}
	return nil
}

func rnestedArrayCopy(val reflect.Value) reflect.Value {
	val2 := reflect.MakeSlice(val.Type(), val.Len(), val.Len())
	for ix := 0; ix < val.Len(); ix++ {
		val2.Index(ix).Set(rnestedValueCopy(val.Index(ix)))
	}
	return val2
}

func rarrayCopy(val reflect.Value) reflect.Value {
	val2 := reflect.MakeSlice(val.Type(), val.Len(), val.Len())
	for ix := 0; ix < val.Len(); ix++ {
		val2.Index(ix).Set(rvalueCopy(val.Index(ix)))
	}
	return val2
}

func resolveRFunc(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	typ := val.Type()
	tooManyArgs := typ.NumIn() > 3
	/*
		firstArgNotContext := typ.NumIn() > 0 && !typ.In(0).AssignableTo(reflect.ValueOf(context.Context(nil)).Type())
		secondArgNotCluster := typ.NumIn() > 1 && !typ.In(1).AssignableTo(reflect.ValueOf(Cluster(nil)).Type())
	*/
	notEnoughReturns := typ.NumOut() == 0
	tooManyReturns := typ.NumOut() > 2
	/*
		secondReturnNotError := typ.NumOut() == 2 && !typ.Out(1).AssignableTo(reflect.ValueOf(error(nil)).Type())

	*/
	badType := tooManyArgs ||
		/*
			firstArgNotContext ||
			secondArgNotCluster ||
		*/
		notEnoughReturns ||
		tooManyReturns /* ||
		secondReturnNotError
		*/
	if badType {
		panic(fmt.Sprintf(badFuncErrFmt, typ))
	}
	// Go reflection is dumb, there doesn't appear to be a way to check if a function argument is an interface, so we just have to not have a nice error message
	var in []reflect.Value
	if typ.NumIn() > 0 {
		in = append(in, reflect.ValueOf(ctx))
	}
	if typ.NumIn() > 1 {
		in = append(in, reflect.ValueOf(cluster))
	}
	if typ.NumIn() > 2 {
		in = append([]reflect.Value{reflect.ValueOf(g)}, in...)
	}
	if len(in) != typ.NumIn() {
		panic(fmt.Errorf("t: %#v, in: %#v", typ, in))
	}
	out := val.Call(in)
	if typ.NumOut() == 2 {
		errV := out[1].Interface()
		if errV != nil {
			return out[0].Interface(), errV.(error)
		}
	}
	return resolveRValue(g, ctx, cluster, out[0])
}
func resolveRNestedFunc(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	typ := val.Type()
	tooManyArgs := typ.NumIn() > 3
	/*
		firstArgNotContext := typ.NumIn() > 0 && !typ.In(0).AssignableTo(reflect.ValueOf(context.Context(nil)).Type())
		secondArgNotCluster := typ.NumIn() > 1 && !typ.In(1).AssignableTo(reflect.ValueOf(Cluster(nil)).Type())
	*/
	notEnoughReturns := typ.NumOut() == 0
	tooManyReturns := typ.NumOut() > 2
	/*
		secondReturnNotError := typ.NumOut() == 2 && !typ.Out(1).AssignableTo(reflect.ValueOf(error(nil)).Type())
	*/
	badType := tooManyArgs ||
		/*
			firstArgNotContext ||
			secondArgNotCluster ||
		*/
		notEnoughReturns ||
		tooManyReturns /*||
		secondReturnNotError
		*/
	if badType {
		panic(fmt.Sprintf(badFuncErrFmt, typ))
	}
	// Go reflection is dumb, there doesn't appear to be a way to check if a function argument is an interface, so we just have to not have a nice error message
	var in []reflect.Value
	if typ.NumIn() > 0 {
		in = append(in, reflect.ValueOf(ctx))
	}
	if typ.NumIn() > 1 {
		in = append(in, reflect.ValueOf(cluster))
	}
	if typ.NumIn() > 2 {
		in = append([]reflect.Value{reflect.ValueOf(g)}, in...)
	}
	if len(in) != typ.NumIn() {
		panic(fmt.Errorf("t: %#v, in: %#v", typ, in))
	}
	out := val.Call(in)
	if typ.NumOut() == 2 {
		errV := out[1].Interface()
		if errV != nil {
			return out[0].Interface(), errV.(error)
		}
	}
	return resolveRNestedValue(g, ctx, cluster, out[0])
}
func rfuncString(g Gingk8s, ctx context.Context, cluster Cluster, s *strings.Builder, val reflect.Value) error {
	out, err := resolveRFunc(g, ctx, cluster, val)
	if err != nil {
		return err
	}
	return valueString(g, ctx, cluster, s, out)
}

func rvalueString(g Gingk8s, ctx context.Context, cluster Cluster, s *strings.Builder, val reflect.Value) error {
	typ := val.Type()
	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		if val.IsNil() {
			return nil
		}
		fmt.Printf("Deref'ing %v (%v) -> %v (%v)\n", val, typ, val.Elem(), typ.Elem())
		return rvalueString(g, ctx, cluster, s, val.Elem())
	case reflect.Array, reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			s.WriteString(base64.StdEncoding.EncodeToString(val.Interface().([]byte)))
			return nil
		}
		return rarrayString(g, ctx, cluster, s, val)
	case reflect.Func:
		fmt.Printf("Calling %v (%v)\n", val, typ)
		return rfuncString(g, ctx, cluster, s, val)
	case reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Map, reflect.Struct, reflect.UnsafePointer:
		panic(fmt.Errorf("Helm substitution does not support %v values (%#v). Values must be typical scalar, arrays/slices of valid types, and functions return return valid types", typ, val))
	}
	return scalarString(g, ctx, cluster, s, val)
}
func resolveRValue(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	typ := val.Type()
	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		return resolveRValue(g, ctx, cluster, val.Elem())
	case reflect.Array, reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return base64.StdEncoding.EncodeToString(val.Interface().([]byte)), nil
		}
		return resolveRArray(g, ctx, cluster, val)
	case reflect.Func:
		return resolveRFunc(g, ctx, cluster, val)
	case reflect.Complex64, reflect.Complex128, reflect.Chan, reflect.Map, reflect.Struct, reflect.UnsafePointer:
		panic(fmt.Errorf("Helm substitution does not support %v values (%#v). Values must be typical scalar, arrays/slices of valid types, and functions return return valid types", typ, val))
		// return nil, fmt.Errorf("Helm substitution does not support %v values (%#v). Values must be typical scalar, arrays/slices of valid types, and functions return return valid types", typ, val)
	}
	return val.Interface(), nil
}

func valueString(g Gingk8s, ctx context.Context, cluster Cluster, s *strings.Builder, v Value) error {
	return rvalueString(g, ctx, cluster, s, reflect.ValueOf(v))
}

func rvalueCopy(val reflect.Value) reflect.Value {
	switch val.Kind() {
	case reflect.Interface:
		if val.IsNil() {
			return val
		}
		val2 := rvalueCopy(val.Elem()).Convert(val.Type())
		fmt.Printf("Copied %v/%v (%v/%v) as %v/%v (%v/%v)\n", val.Type(), val, val.Elem().Type(), val.Elem(), val2.Type(), val2, val2.Elem().Type(), val2.Elem())
		return val2
	case reflect.Pointer:
		if val.IsNil() {
			return val
		}
		val2 := reflect.New(val.Type().Elem())
		val2.Elem().Set(rvalueCopy(val2.Elem()))
		fmt.Printf("Copied %v/%v (%v/%v) as %v/%v (%v/%v)\n", val.Type(), val, val.Elem().Type(), val.Elem(), val2.Type(), val2, val2.Elem().Type(), val2.Elem())
		return val2
	case reflect.Array, reflect.Slice:
		if val.Type().Elem().Kind() == reflect.Uint8 {
			out := make([]byte, val.Len())
			copy(out, val.Interface().([]byte))
			return reflect.ValueOf(out)
		}
		return rarrayCopy(val)
	}
	return val
}

func robjectCopy(val reflect.Value) reflect.Value {
	val2 := reflect.MakeMapWithSize(val.Type(), val.Len())
	iter := val.MapRange()
	for iter.Next() {
		val2.SetMapIndex(iter.Key(), rvalueCopy(iter.Value()))
	}
	return val2
}

func resolveRObject(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	var err error
	out := make(map[string]interface{}, val.Len())
	iter := val.MapRange()
	for iter.Next() {
		out[iter.Key().Interface().(string)], err = resolveRValue(g, ctx, cluster, iter.Value())
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func rnestedObjectCopy(val reflect.Value) reflect.Value {
	val2 := reflect.MakeMapWithSize(val.Type(), val.Len())
	iter := val.MapRange()
	for iter.Next() {
		val2.SetMapIndex(iter.Key(), rnestedValueCopy(iter.Value()))
	}
	return val2
}

func resolveRNestedObject(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	var err error
	out := make(map[string]interface{}, val.Len())
	iter := val.MapRange()
	for iter.Next() {
		out[iter.Key().Interface().(string)], err = resolveRNestedValue(g, ctx, cluster, iter.Value())
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func resolveRNestedValue(g Gingk8s, ctx context.Context, cluster Cluster, val reflect.Value) (interface{}, error) {
	switch val.Kind() {
	case reflect.Interface, reflect.Pointer:
		return resolveRNestedValue(g, ctx, cluster, val.Elem())
	case reflect.Array, reflect.Slice:
		if val.Type().Elem().Kind() == reflect.Uint8 {
			return base64.StdEncoding.EncodeToString(val.Interface().([]byte)), nil
		}
		return resolveRNestedArray(g, ctx, cluster, val)
	case reflect.Func:
		return resolveRNestedFunc(g, ctx, cluster, val)
	case reflect.Map:
		return resolveRNestedObject(g, ctx, cluster, val)
	}
	return resolveRValue(g, ctx, cluster, val)
}
func resolveNestedValue(g Gingk8s, ctx context.Context, cluster Cluster, v NestedValue) (interface{}, error) {
	return resolveRNestedValue(g, ctx, cluster, reflect.ValueOf(v))
}

func rnestedValueCopy(val reflect.Value) reflect.Value {
	switch val.Kind() {
	case reflect.Interface:
		if val.IsNil() {
			return val
		}
		return rvalueCopy(val.Elem()).Convert(val.Type())
	case reflect.Pointer:
		if val.IsNil() {
			return val
		}
		val2 := reflect.New(val.Type().Elem())
		val2.Elem().Set(rnestedValueCopy(val2.Elem()))
	case reflect.Array, reflect.Slice:
		if val.Type().Elem().Kind() == reflect.Uint8 {
			out := make([]byte, val.Len())
			copy(out, val.Interface().([]byte))
			return reflect.ValueOf(out)
		}
		return rnestedArrayCopy(val)
	case reflect.Map:
		return rnestedObjectCopy(val)
	}
	return val
}

func resolveNestedObject(g Gingk8s, ctx context.Context, cluster Cluster, o NestedObject) (interface{}, error) {
	return resolveNestedValue(g, ctx, cluster, o)
}
