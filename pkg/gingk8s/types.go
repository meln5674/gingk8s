package gingk8s

import (
	"context"
	"io"
	"os"
	"reflect"

	"github.com/meln5674/gosh"
	. "github.com/onsi/ginkgo/v2"
)

var (
	// GinkgoErr is a gosh wrapper that forwards stderr to Ginkgo
	GinkgoErr = gosh.WriterErr(GinkgoWriter)
	// GinkgoOut is a gosh wrapper that forwards stdout to Gingko
	GinkgoOut = gosh.WriterOut(GinkgoWriter)
	// GinkgoOut is a gosh wrapper that forwards stdout and stderr to Gingko
	GinkgoOutErr = gosh.SetStreams(GinkgoOut, GinkgoErr)
)

var (
	// DefaultKustomizeCommand is the command used to execute kustomize if none is provided
	DefaultKustomizeCommand = []string{"kustomize"}
	// DefaultBuildahCommand is the command used to execute buildah if none is provided
	DefaultBuildahCommand = []string{"buildah"}
	// DefaultDeleteCommand is the command used to remove directories if none is provided
	DefaultDeleteCommand = []string{"rm", "-rf"}
)

// Value is any valid value, but represents that values should be one of the following
// Scalars (strings, bools, numbers)
// Byte Slice/Arrays (Which will be converted to base64 representations when resolved)
// Arrays/Slices of valid values (See `Array`)
// A function with argument list of (), (context.Context) or (context.Context, Cluster) and return types of (Value) or (Value, error)
// values of these types will be correctly handled and evaluated
type Value interface{}

// NestedValues must be either a `Value`, `Object`, or `Array`
type NestedValue interface{}

// Object is a map from string keys to values
type Object map[string]Value

// DeepCopy returns a copy of this object where pointers and arrays have their values deep copied
func (o Object) DeepCopy() Object {
	return robjectCopy(reflect.ValueOf(o)).Interface().(Object)
}

// MergedFrom returns a deep copy of this object after adding or overwriting fields
func (o Object) MergedFrom(toAdd Object) Object {
	o2 := o.DeepCopy()
	for k, v := range toAdd {
		o2[k] = v
	}
	return o2
}

// With returns a deep copy of this object with an extra field added or overwritten
func (o Object) With(k string, v Value) Object {
	return o.MergedFrom(Object{k: v})
}

// StringObject is a map from string keys to string values
type StringObject = map[string]string

// Array is a slice of values
type Array []Value

// A NestedObject is like an `Object`, but recursive
type NestedObject map[string]NestedValue

// DeepCopy returns a copy of this object where arrays, pointers, and objects have their contents deep copied
func (o NestedObject) DeepCopy() NestedObject {
	return rnestedObjectCopy(reflect.ValueOf(o)).Interface().(NestedObject)
}

// MergedFrom returns a deep copy of this object after adding or overwriting fields
func (o NestedObject) MergedFrom(toAdd NestedObject) NestedObject {
	o2 := o.DeepCopy()
	for k, v := range toAdd {
		o2[k] = v
	}
	return o2
}

// With returns a deep copy of this object with an extra field added or overwritten
func (o NestedObject) With(k string, v NestedValue) NestedObject {
	return o.MergedFrom(NestedObject{k: v})
}

func MkdirAll(path string, mode os.FileMode) gosh.Func {
	return func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error {
		go func() {
			defer GinkgoRecover()
			var err error
			defer func() { done <- err; close(done) }()
			err = os.MkdirAll(path, mode)
		}()
		return nil
	}
}

type WaitFor struct {
	Resource string
	For      StringObject
	Flags    []string
}

type ClusterAction func(context.Context, Cluster) error

type ResourceReference struct {
	Namespace string
	Kind      string
	Name      string
}

// ConfigMap returns a NestedObject which can be used in KubernetesManifests.
// binaryData should be raw []byte's, not base64 encoded
func ConfigMap(name, namespace string, data, binaryData Object) NestedObject {
	metadata := NestedObject{
		"name": name,
	}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	configMap := NestedObject{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   metadata,
	}
	if data != nil {
		configMap["data"] = data
	}
	if binaryData != nil {
		configMap["binaryData"] = binaryData
	}
	return configMap
}

// Secret returns a NestedObject which can be used in KubernetesManifests
// data should be raw []byte's, not base64 encoded
func Secret(name, namespace, typ string, data, stringData Object) NestedObject {
	metadata := NestedObject{
		"name": name,
	}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	secret := NestedObject{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   metadata,
		"type":       typ,
	}
	if data != nil {
		secret["data"] = data
	}
	if stringData != nil {
		secret["stringData"] = stringData
	}
	return secret
}
