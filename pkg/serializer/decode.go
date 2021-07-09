package serializer

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"github.com/weaveworks/libgitops/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/versioning"
	"sigs.k8s.io/yaml"
)

// This is the groupversionkind for the v1.List object
var listGVK = metav1.Unversioned.WithKind("List")

type DecodingOptions struct {
	// Not applicable for Decoder.DecodeInto(). If true, the decoded external object
	// will be converted into its hub (or internal, where applicable) representation. Otherwise, the decoded
	// object will be left in its external representation. (Default: false)
	ConvertToHub *bool

	// Parse the YAML/JSON in strict mode, returning a specific error if the input
	// contains duplicate or unknown fields or formatting errors. (Default: true)
	Strict *bool

	// Automatically default the decoded object. (Default: false)
	Default *bool

	// Only applicable for Decoder.DecodeAll(). If the underlying data contains a v1.List,
	// the items of the list will be traversed, decoded into their respective types, and
	// appended to the returned slice. The v1.List will in this case not be returned.
	// This conversion does NOT support preserving comments. If the given scheme doesn't
	// recognize the v1.List, before using it will be registered automatically. (Default: true)
	DecodeListElements *bool

	// Whether to preserve YAML comments internally. This only works for objects embedding metav1.ObjectMeta.
	// Only applicable to frame.FramingTypeYAML framers.
	// Using any other framer will be silently ignored. Usage of this option also requires setting
	// the PreserveComments in EncodingOptions, too. (Default: false)
	PreserveComments *bool

	// DecodeUnknown specifies whether decode objects with an unknown GroupVersionKind into a
	// *runtime.Unknown object when running Decode(All) (true value) or to return an error when
	// any unrecognized type is found (false value). (Default: false)
	DecodeUnknown *bool
}

type DecodingOptionsFunc func(*DecodingOptions)

func WithConvertToHubDecode(convert bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.ConvertToHub = &convert
	}
}

func WithStrictDecode(strict bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.Strict = &strict
	}
}

func WithDefaultsDecode(defaults bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.Default = &defaults
	}
}

func WithListElementsDecoding(listElements bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.DecodeListElements = &listElements
	}
}

func WithCommentsDecode(comments bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.PreserveComments = &comments
	}
}

func WithUnknownDecode(unknown bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.DecodeUnknown = &unknown
	}
}

func WithDecodingOptions(newOpts DecodingOptions) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		// TODO: Null-check all of these before using them
		*opts = newOpts
	}
}

func defaultDecodeOpts() *DecodingOptions {
	return &DecodingOptions{
		ConvertToHub:       util.BoolPtr(false),
		Strict:             util.BoolPtr(true),
		Default:            util.BoolPtr(false),
		DecodeListElements: util.BoolPtr(true),
		PreserveComments:   util.BoolPtr(false),
		DecodeUnknown:      util.BoolPtr(false),
	}
}

func newDecodeOpts(fns ...DecodingOptionsFunc) *DecodingOptions {
	opts := defaultDecodeOpts()
	for _, fn := range fns {
		fn(opts)
	}
	return opts
}

type decoder struct {
	*schemeAndCodec

	decoder runtime.Decoder
	opts    DecodingOptions
}

// Decode returns the decoded object from the next document in the frame.Reader stream.
// If there are multiple documents in the underlying stream, this call will read one
// 	document and return it. Decode might be invoked for getting new documents until it
// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
// If the decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded object will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// If opts.ConvertToHub is true, the decoded external object will be converted into its hub
// 	(or internal, if applicable) representation.
// 	Otherwise, the decoded object will be left in the external representation.
// If opts.DecodeUnknown is true, any type with an unrecognized apiVersion/kind will be returned as a
// 	*runtime.Unknown object instead of returning a UnrecognizedTypeError.
// opts.DecodeListElements is not applicable in this call.
func (d *decoder) Decode(fr frame.Reader) (runtime.Object, error) {
	// Read a frame from the frame.Reader
	// TODO: Make sure to test the case when doc might contain something, and err is io.EOF
	ctx := context.TODO()
	doc, err := fr.ReadFrame(ctx)
	if err != nil {
		return nil, err
	}
	return d.decode(doc, nil, fr.FramingType())
}

func (d *decoder) decode(doc []byte, into runtime.Object, ct frame.FramingType) (runtime.Object, error) {
	// If the scheme doesn't recognize a v1.List, and we enabled opts.DecodeListElements,
	// make the scheme able to decode the v1.List automatically
	if *d.opts.DecodeListElements && !d.scheme.Recognizes(listGVK) {
		d.scheme.AddKnownTypes(metav1.Unversioned, &metav1.List{})
	}

	// Record if this decode call should have runtime.DecodeInto-functionality
	intoGiven := into != nil

	// Use our own special (e.g. strict, defaulting/non-defaulting) decoder
	// TODO: Make sure any possible strict errors are returned/handled properly
	obj, gvk, err := d.decoder.Decode(doc, nil, into)
	if err != nil {
		// If we asked to decode unknown objects, we are in the Decode(All) (not Into)
		// codepath, and the error returned was due to that the kind was not registered
		// in the scheme, decode the document as a *runtime.Unknown
		if *d.opts.DecodeUnknown && !intoGiven && runtime.IsNotRegisteredError(err) {
			return d.decodeUnknown(doc, ct)
		}
		// Give the user good errors wrt missing group & version
		// TODO: It might be unnecessary to unmarshal twice (as we do in handleDecodeError),
		// as gvk was returned from Decode above.
		return nil, d.handleDecodeError(doc, err)
	}

	// Fail fast if object is nil
	if obj == nil {
		return nil, fmt.Errorf("decoded object is nil! Detected gvk is %v", gvk)
	}

	// This logic is the same as in runtime.DecodeInto, and makes sure that if we requested an
	// "into" object, it actually worked
	if intoGiven && obj != into {
		return nil, fmt.Errorf("unable to decode %s into %v", gvk, reflect.TypeOf(into))
	}

	// Try to preserve comments
	d.tryToPreserveComments(doc, obj, ct)

	// Return the decoded object
	return obj, nil
}

// DecodeInto decodes the next document in the frame.Reader stream into obj if the types are matching.
// If there are multiple documents in the underlying stream, this call will read one
// 	document and return it. Decode might be invoked for getting new documents until it
// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
// The decoded object will automatically be converted into the target one (i.e. one can supply an
// 	ConvertToHub object to this function).
// If the decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded object will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// opts.DecodeListElements is not applicable in this call.
// opts.ConvertToHub is not applicable in this call.
// opts.DecodeUnknown is not applicable in this call. In case you want to decode an object into a
// 	*runtime.Unknown, just create a runtime.Unknown object and pass the pointer as obj into DecodeInto
// 	and it'll work.
func (d *decoder) DecodeInto(fr frame.Reader, into runtime.Object) error {
	// Read a frame from the frame.Reader.
	// TODO: Make sure to test the case when doc might contain something, and err is io.EOF
	ctx := context.TODO()
	doc, err := fr.ReadFrame(ctx)
	if err != nil {
		return err
	}

	// Run the internal decode() and pass the into object
	_, err = d.decode(doc, into, fr.FramingType())
	return err
}

// DecodeAll returns the decoded objects from all documents in the frame.Reader stream. The underlying
// stream is automatically closed on io.EOF. io.EOF is never returned from this function.
// If any decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded objects will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// If opts.ConvertToHub is true, the decoded external object will be converted into its hub
// 	(or internal, if applicable) representation.
// If opts.DecodeListElements is true and the underlying data contains a v1.List,
// 	the items of the list will be traversed and decoded into their respective types, which are
// 	added into the returning slice. The v1.List will in this case not be returned.
// If opts.DecodeUnknown is true, any type with an unrecognized apiVersion/kind will be returned as a
// 	*runtime.Unknown object instead of returning a UnrecognizedTypeError.
func (d *decoder) DecodeAll(fr frame.Reader) ([]runtime.Object, error) {
	objs := []runtime.Object{}
	for {
		obj, err := d.Decode(fr)
		if err == io.EOF {
			// If we encountered io.EOF, we know that all is fine and we can exit the for loop and return
			break
		} else if err != nil {
			return nil, err
		}

		// Extract possibly nested objects within the one we got (e.g. unwrapping lists if asked to),
		// or just no-op and return the object given for addition to the larger list
		nestedObjs, err := d.extractNestedObjects(obj, fr.FramingType())
		if err != nil {
			return nil, err
		}
		objs = append(objs, nestedObjs...)
	}
	return objs, nil
}

// decodeUnknown decodes bytes of a certain content type into a returned *runtime.Unknown object
func (d *decoder) decodeUnknown(doc []byte, ct frame.FramingType) (runtime.Object, error) {
	// Do a DecodeInto the new pointer to the object we've got. The resulting into object is
	// also returned.
	// The content type isn't really used here, as runtime.Unknown will never implement
	// ObjectMeta, but the signature needs it so we'll just forward it
	return d.decode(doc, &runtime.Unknown{}, ct)
}

func (d *decoder) handleDecodeError(doc []byte, origErr error) error {
	// Parse the document's TypeMeta information
	gvk, err := extractYAMLTypeMeta(doc)
	if err != nil {
		return fmt.Errorf("failed to interpret TypeMeta from the given the YAML: %v. Decode error was: %w", err, origErr)
	}

	// TODO: Unit test that typed errors are returned properly

	// Check if the group was known. If not, return that specific error
	if !d.scheme.IsGroupRegistered(gvk.Group) {
		return NewUnrecognizedGroupError(*gvk, origErr)
	}

	// Return a structured error if the group was registered with the scheme but the version was unrecognized
	if !d.scheme.IsVersionRegistered(gvk.GroupVersion()) {
		gvs := d.scheme.PrioritizedVersionsForGroup(gvk.Group)
		return NewUnrecognizedVersionError(gvs, *gvk, origErr)
	}

	// Return a structured error if the kind is not known
	if !d.scheme.Recognizes(*gvk) {
		return NewUnrecognizedKindError(*gvk, origErr)
	}

	// If nothing else, just return the underlying error
	return origErr
}

func (d *decoder) extractNestedObjects(obj runtime.Object, ct frame.FramingType) ([]runtime.Object, error) {
	// If we didn't ask for list-unwrapping functionality, return directly
	if !*d.opts.DecodeListElements {
		return []runtime.Object{obj}, nil
	}

	// Try to cast the object to a v1.List. If the object isn't a list, just return it
	list, ok := obj.(*metav1.List)
	if !ok {
		return []runtime.Object{obj}, nil
	}

	// Loop through the list, and decode every item. Return the final list
	var objs []runtime.Object
	for _, item := range list.Items {
		// Decode each item of the list
		listobj, err := d.decode(item.Raw, nil, ct)
		if err != nil {
			return nil, err
		}
		objs = append(objs, listobj)
	}
	return objs, nil
}

func newDecoder(schemeAndCodec *schemeAndCodec, opts DecodingOptions) Decoder {
	// Allow both YAML and JSON inputs (JSON is a subset of YAML), and deserialize in strict mode
	s := json.NewSerializerWithOptions(json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme, json.SerializerOptions{
		Yaml:   true,
		Strict: *opts.Strict,
	})

	decodeCodec := decoderForVersion(schemeAndCodec.scheme, s, *opts.Default, *opts.ConvertToHub)

	return &decoder{schemeAndCodec, decodeCodec, opts}
}

// decoderForVersion is used instead of CodecFactory.DecoderForVersion, as we want to use our own converter
func decoderForVersion(scheme *runtime.Scheme, decoder *json.Serializer, doDefaulting, doConversion bool) runtime.Decoder {
	return newConversionCodecForScheme(
		scheme,
		nil,                            // no encoder
		decoder,                        // our custom JSON serializer
		nil,                            // no target encode groupversion
		runtime.InternalGroupVersioner, // if conversion should happen for classic types, convert into internal
		doDefaulting,                   // default if specified
		doConversion,                   // convert to the hub type conditionally when decoding
	)
}

// newConversionCodecForScheme is a convenience method for callers that are using a scheme.
// This is very similar to apimachinery/pkg/serializer/versioning.NewDefaultingCodecForScheme
func newConversionCodecForScheme(
	scheme *runtime.Scheme,
	encoder runtime.Encoder,
	decoder runtime.Decoder,
	encodeVersion runtime.GroupVersioner,
	decodeVersion runtime.GroupVersioner,
	performDefaulting bool,
	performConversion bool,
) runtime.Codec {
	var defaulter runtime.ObjectDefaulter
	if performDefaulting {
		defaulter = scheme
	}
	convertor := newObjectConvertor(scheme, performConversion)
	return versioning.NewCodec(encoder, decoder, convertor, scheme, scheme, defaulter, encodeVersion, decodeVersion, scheme.Name())
}

// TODO: Use https://github.com/kubernetes/apimachinery/blob/master/pkg/runtime/serializer/yaml/meta.go
// when we can assume everyone is vendoring k8s v1.19
func extractYAMLTypeMeta(data []byte) (*schema.GroupVersionKind, error) {
	typeMeta := runtime.TypeMeta{}
	if err := yaml.Unmarshal(data, &typeMeta); err != nil {
		return nil, fmt.Errorf("could not interpret GroupVersionKind: %w", err)
	}
	gv, err := schema.ParseGroupVersion(typeMeta.APIVersion)
	if err != nil {
		return nil, err
	}
	gvk := gv.WithKind(typeMeta.Kind)
	return &gvk, nil
}
