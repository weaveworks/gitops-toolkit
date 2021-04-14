package serializer

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/serializer/comments"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const preserveCommentsAnnotation = "serializer.libgitops.weave.works/original-data"

var (
	// TODO: Investigate if we can just depend on `metav1.Object` interface compliance instead of needing to explicitly
	//  embed the `metav1.ObjectMeta` struct.
	ErrNoObjectMeta     = errors.New("the given object cannot store comments, it is not metav1.ObjectMeta compliant")
	ErrNoStoredComments = errors.New("the given object does not have stored comments")
)

// tryToPreserveComments tries to save the original file data (base64-encoded) into an annotation.
// This original file data can be used at encoding-time to preserve comments
func (d *decoder) tryToPreserveComments(doc []byte, obj runtime.Object, ct ContentType) {
	// If the user opted into preserving comments and the format is YAML, proceed
	// If they didn't, return directly
	if !(d.opts.PreserveComments == PreserveCommentsStrict && ct == ContentTypeYAML) {
		return
	}

	// Preserve the original file content in the annotation (this requires embedding ObjectMeta).
	if !setCommentSourceBytes(obj, doc) {
		// If the object doesn't have ObjectMeta embedded, just do nothing.
		logrus.Debugf("Couldn't convert object with GVK %q to metav1.Object, although opts.PreserveComments is enabled", obj.GetObjectKind().GroupVersionKind())
	}
}

// tryToPreserveComments tries to locate the possibly-saved original file data in the object's annotation
func (e *encoder) encodeWithCommentSupport(versionEncoder runtime.Encoder, fw FrameWriter, obj runtime.Object, metaObj metav1.Object) error {
	// If the user did not opt into preserving comments, just sanitize ObjectMeta temporarily and and return
	if e.opts.PreserveComments == PreserveCommentsDisable {
		// Normal encoding without the annotation (so it doesn't leak by accident)
		return noAnnotationWrapper(metaObj, e.normalEncodeFunc(versionEncoder, fw, obj))
	}

	// The user requested to preserve comments, but content type is not YAML, so log, sanitize and return
	if fw.ContentType() != ContentTypeYAML {
		logrus.Debugf("Asked to preserve comments, but ContentType is not YAML, so ignoring")

		// Normal encoding without the annotation (so it doesn't leak by accident)
		return noAnnotationWrapper(metaObj, e.normalEncodeFunc(versionEncoder, fw, obj))
	}

	priorNode, err := getCommentSourceMeta(metaObj)
	if errors.Is(err, ErrNoStoredComments) {
		// No need to delete the annotation as we know it doesn't exist, just do a normal encode
		return e.normalEncodeFunc(versionEncoder, fw, obj)()
	} else if err != nil {
		return err
	}

	// Encode the new object into a temporary buffer, it should not be written as the "final result" to the FrameWriter
	buf := new(bytes.Buffer)
	if err := noAnnotationWrapper(metaObj, e.normalEncodeFunc(versionEncoder, NewYAMLFrameWriter(buf), obj)); err != nil {
		// fatal error
		return err
	}

	// Parse the new, upgraded, encoded YAML into *yaml.RNode for addition
	// of comments from prevNode
	afterNode, err := yaml.Parse(buf.String())
	if err != nil {
		// fatal error
		return err
	}

	// Copy over comments from the old to the new schema
	if err := comments.CopyComments(priorNode, afterNode, true); err != nil {
		// fatal error
		return err
	}

	// Print the new schema with the old comments kept to the FrameWriter
	_, err = fmt.Fprint(fw, afterNode.MustString())
	// we're done, exit the encode function
	return err
}

func (e *encoder) normalEncodeFunc(versionEncoder runtime.Encoder, fw FrameWriter, obj runtime.Object) func() error {
	return func() error {
		return versionEncoder.Encode(obj, fw)
	}
}

// noAnnotationWrapper temporarily removes the preserveComments annotation before and after running the function
// one example of this function is e.normalEncodeFunc
func noAnnotationWrapper(metaobj metav1.Object, fn func() error) error {
	// If the annotation exists, delete it and defer add it back.
	if val, ok := getAnnotation(metaobj, preserveCommentsAnnotation); ok {
		defer setAnnotation(metaobj, preserveCommentsAnnotation, val)
		deleteAnnotation(metaobj, preserveCommentsAnnotation)
	}
	// If the annotation isn't present, just run the function
	return fn()
}

// GetCommentSource retrieves the YAML tree used as the source for transferring comments for the given runtime.Object.
// This may be used externally to implement e.g. re-parenting of the comment source tree when moving structs around.
func GetCommentSource(obj runtime.Object) (*yaml.RNode, error) {
	// Cast the object to a metav1.Object to get access to annotations.
	// If this fails, the given object does not support storing comments.
	metaObj, ok := toMetaObject(obj)
	if !ok {
		return nil, ErrNoObjectMeta
	}

	// Use getCommentSourceMeta to retrieve the comments from the metav1.Object.
	return getCommentSourceMeta(metaObj)
}

// getCommentSourceMeta retrieves the YAML tree used as the source for transferring comments for the given metav1.Object.
func getCommentSourceMeta(metaObj metav1.Object) (*yaml.RNode, error) {
	// Fetch the source string for the comments. If this fails, the given object does not have any stored comments.
	sourceStr, ok := getAnnotation(metaObj, preserveCommentsAnnotation)
	if !ok {
		return nil, ErrNoStoredComments
	}

	// Decode the base64-encoded comment source string.
	sourceBytes, err := base64.StdEncoding.DecodeString(sourceStr)
	if err != nil {
		return nil, err
	}

	// Parse the decoded source data into a *yaml.RNode and return it.
	return yaml.Parse(string(sourceBytes))
}

// SetCommentSource sets the given YAML tree as the source for transferring comments for the given runtime.Object.
// This may be used externally to implement e.g. re-parenting of the comment source tree when moving structs around.
func SetCommentSource(obj runtime.Object, source *yaml.RNode) error {
	// Convert the given tree into a string. This also handles the source == nil case.
	str, err := source.String()
	if err != nil {
		return err
	}

	// Convert the string to bytes and pass it to setCommentSourceBytes to be applied.
	if !setCommentSourceBytes(obj, []byte(str)) {
		// If this fails, the passed object is not metav1.ObjectMeta compliant.
		return ErrNoObjectMeta
	}

	return nil
}

// SetCommentSource sets the given bytes as the source for transferring comments for the given runtime.Object.
func setCommentSourceBytes(obj runtime.Object, source []byte) bool {
	// Cast the object to a metav1.Object to get access to annotations.
	// If this fails, the given object does not support storing comments.
	metaObj, ok := toMetaObject(obj)
	if !ok {
		return false
	}

	// base64-encode the comments string.
	encodedStr := base64.StdEncoding.EncodeToString(source)

	// Set the value of the comments annotation to the encoded string.
	setAnnotation(metaObj, preserveCommentsAnnotation, encodedStr)
	return true
}
