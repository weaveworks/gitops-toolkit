package patch

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

type Patcher interface {
	Create(new runtime.Object, applyFn func(runtime.Object) error) ([]byte, error)
	Apply(original, patch []byte, gvk schema.GroupVersionKind) ([]byte, error)
	ApplyOnFile(filePath string, patch []byte, gvk schema.GroupVersionKind) error
}

func NewPatcher(s serializer.Serializer) Patcher {
	return &patcher{serializer: s}
}

type patcher struct {
	serializer serializer.Serializer
}

// Create is a helper that creates a patch out of the change made in applyFn
func (p *patcher) Create(new runtime.Object, applyFn func(runtime.Object) error) (patchBytes []byte, err error) {
	var oldBytes, newBytes bytes.Buffer
	encoder := p.serializer.Encoder()
	old := new.DeepCopyObject().(runtime.Object)

	if err = encoder.Encode(frame.NewJSONWriter(&oldBytes), old); err != nil {
		return
	}

	if err = applyFn(new); err != nil {
		return
	}

	if err = encoder.Encode(frame.NewJSONWriter(&newBytes), new); err != nil {
		return
	}

	emptyObj, err := p.serializer.Scheme().New(old.GetObjectKind().GroupVersionKind())
	if err != nil {
		return
	}

	patchBytes, err = strategicpatch.CreateTwoWayMergePatch(oldBytes.Bytes(), newBytes.Bytes(), emptyObj)
	if err != nil {
		return nil, fmt.Errorf("CreateTwoWayMergePatch failed: %v", err)
	}

	return patchBytes, nil
}

func (p *patcher) Apply(original, patch []byte, gvk schema.GroupVersionKind) ([]byte, error) {
	emptyObj, err := p.serializer.Scheme().New(gvk)
	if err != nil {
		return nil, err
	}

	b, err := strategicpatch.StrategicMergePatch(original, patch, emptyObj)
	if err != nil {
		return nil, err
	}

	return p.serializerEncode(b)
}

func (p *patcher) ApplyOnFile(filePath string, patch []byte, gvk schema.GroupVersionKind) error {
	oldContent, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	newContent, err := p.Apply(oldContent, patch, gvk)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, newContent, 0644)
}

// StrategicMergePatch returns an unindented, unorganized JSON byte slice,
// this helper takes that as an input and returns the same JSON re-encoded
// with the serializer so it conforms to a runtime.Object
// TODO: Just use encoding/json.Indent here instead?
func (p *patcher) serializerEncode(input []byte) ([]byte, error) {
	obj, err := p.serializer.Decoder().Decode(frame.NewJSONReader(frame.FromBytes(input)))
	if err != nil {
		return nil, err
	}

	var result bytes.Buffer
	if err := p.serializer.Encoder().Encode(frame.NewJSONWriter(&result), obj); err != nil {
		return nil, err
	}

	return result.Bytes(), err
}
