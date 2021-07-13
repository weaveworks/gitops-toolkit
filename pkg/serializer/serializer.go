package serializer

import (
	"fmt"

	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sserializer "k8s.io/apimachinery/pkg/runtime/serializer"
)

// Serializer is an interface providing high-level decoding/encoding functionality
// for types registered in a *runtime.Scheme
type Serializer interface {
	// Decoder is a high-level interface for decoding Kubernetes API Machinery objects read from
	// a frame.Writer. The decoder can be customized by passing some options (e.g. WithDecodingOptions)
	// to this call.
	// The decoder supports both "classic" API Machinery objects and controller-runtime CRDs
	Decoder(optsFn ...DecodingOptionsFunc) Decoder

	// Encoder is a high-level interface for encoding Kubernetes API Machinery objects and writing them
	// to a frame.Writer. The encoder can be customized by passing some options (e.g. WithEncodingOptions)
	// to this call.
	// The encoder supports both "classic" API Machinery objects and controller-runtime CRDs
	Encoder(optsFn ...EncodingOptionsFunc) Encoder

	// Converter is a high-level interface for converting objects between different versions
	// The converter supports both "classic" API Machinery objects and controller-runtime CRDs
	Converter() Converter

	// Defaulter is a high-level interface for accessing defaulting functions in a scheme
	Defaulter() Defaulter

	// Scheme provides access to the underlying runtime.Scheme, may be used for low-level access to
	// the "type universe" and advanced conversion/defaulting features
	Scheme() *runtime.Scheme

	// Codecs provides access to the underlying serializer.CodecFactory, may be used if low-level access
	// is needed for encoding and decoding
	Codecs() *k8sserializer.CodecFactory
}

type schemeAndCodec struct {
	scheme *runtime.Scheme
	codecs *k8sserializer.CodecFactory
}

// Encoder is a high-level interface for encoding Kubernetes API Machinery objects and writing them
// to a frame.Writer.
type Encoder interface {
	// Encode encodes the given objects and writes them to the specified frame.Writer.
	// The frame.Writer specifies the frame.FramingType. This encoder will automatically convert any
	// internal object given to the preferred external groupversion. No conversion will happen
	// if the given object is of an external version.
	Encode(fw frame.Writer, obj ...runtime.Object) error

	// EncodeForGroupVersion encodes the given object for the specific groupversion. If the object
	// is not of that version currently it will try to convert. The output bytes are written to the
	// frame.Writer. The frame.Writer specifies the frame.FramingType.
	EncodeForGroupVersion(fw frame.Writer, obj runtime.Object, gv schema.GroupVersion) error
}

// Decoder is a high-level interface for decoding Kubernetes API Machinery objects read from
// a frame.Writer. The decoder can be customized by passing some options (e.g. WithDecodingOptions)
// to this call.
type Decoder interface {
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
	// If opts.ConvertToHub is true, the decoded external object will be converted into its internal representation.
	// 	Otherwise, the decoded object will be left in the external representation.
	// If opts.DecodeUnknown is true, any type with an unrecognized apiVersion/kind will be returned as a
	// 	*runtime.Unknown object instead of returning a UnrecognizedTypeError.
	// opts.DecodeListElements is not applicable in this call.
	Decode(fr frame.Reader) (runtime.Object, error)

	// DecodeInto decodes the next document in the frame.Reader stream into obj if the types are matching.
	// If there are multiple documents in the underlying stream, this call will read one
	// 	document and return it. Decode might be invoked for getting new documents until it
	// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
	// The decoded object will automatically be converted into the target one (i.e. one can supply an
	// 	internal object to this function).
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
	DecodeInto(fr frame.Reader, obj runtime.Object) error

	// DecodeAll returns the decoded objects from all documents in the frame.Reader stream. The underlying
	// stream is automatically closed on io.EOF. io.EOF is never returned from this function.
	// If any decoded object is for an unrecognized group, or version, UnrecognizedGroupError
	// 	or UnrecognizedVersionError might be returned.
	// If opts.Default is true, the decoded objects will be defaulted.
	// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
	// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
	// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
	// If opts.ConvertToHub is true, the decoded external object will be converted into their internal representation.
	// 	Otherwise, the decoded objects will be left in their external representation.
	// If opts.DecodeListElements is true and the underlying data contains a v1.List,
	// 	the items of the list will be traversed and decoded into their respective types, which are
	// 	added into the returning slice. The v1.List will in this case not be returned.
	// If opts.DecodeUnknown is true, any type with an unrecognized apiVersion/kind will be returned as a
	// 	*runtime.Unknown object instead of returning a UnrecognizedTypeError.
	DecodeAll(fr frame.Reader) ([]runtime.Object, error)
}

// Converter is an interface that allows access to object conversion capabilities
type Converter interface {
	// Convert converts in directly into out. out should be an empty object of the destination type.
	// Both objects must be of the same kind and either have autogenerated conversions registered, or
	// be controller-runtime CRD-style implementers of the sigs.k8s.io/controller-runtime/pkg/conversion.Hub
	// and Convertible interfaces. In the case of CRD Convertibles and Hubs, there must be one Convertible and
	// one Hub given in the in and out arguments. No defaulting is performed.
	Convert(in, out runtime.Object) error

	// ConvertIntoNew creates a new object for the specified groupversionkind, uses Convert(in, out)
	// under the hood, and returns the new object. No defaulting is performed.
	ConvertIntoNew(in runtime.Object, gvk schema.GroupVersionKind) (runtime.Object, error)

	// ConvertToHub converts the given in object to either the internal version (for API machinery "classic")
	// or the sigs.k8s.io/controller-runtime/pkg/conversion.Hub for the given conversion.Convertible object in
	// the "in" argument. No defaulting is performed.
	ConvertToHub(in runtime.Object) (runtime.Object, error)
}

// Defaulter is a high-level interface for accessing defaulting functions in a scheme
type Defaulter interface {
	// Default runs the registered defaulting functions in the scheme on the given objects, one-by-one.
	// If the given object is internal, it will be automatically defaulted using the preferred external
	// version's defaults (i.e. converted to the preferred external version, defaulted there, and converted
	// back to internal).
	// Important to note here is that the TypeMeta information is NOT applied automatically.
	Default(objs ...runtime.Object) error

	// NewDefaultedObject returns a new, defaulted object. It is essentially scheme.New() and
	// scheme.Default(obj), but with extra logic to cover also internal versions.
	// Important to note here is that the TypeMeta information is NOT applied automatically.
	NewDefaultedObject(gvk schema.GroupVersionKind) (runtime.Object, error)
}

// NewSerializer constructs a new serializer based on a scheme, and optionally a codecfactory
func NewSerializer(scheme *runtime.Scheme, codecs *k8sserializer.CodecFactory) Serializer {
	if scheme == nil {
		panic("scheme must not be nil")
	}

	// Ensure codecs is never nil
	if codecs == nil {
		codecs = &k8sserializer.CodecFactory{}
		*codecs = k8sserializer.NewCodecFactory(scheme)
	}

	return &serializer{
		schemeAndCodec: &schemeAndCodec{
			scheme: scheme,
			codecs: codecs,
		},
		converter: newConverter(scheme),
		defaulter: newDefaulter(scheme),
	}
}

// serializer implements the Serializer interface
type serializer struct {
	*schemeAndCodec
	converter *converter
	defaulter *defaulter
}

// Scheme provides access to the underlying runtime.Scheme, may be used for low-level access to
// the "type universe" and advanced conversion/defaulting features
func (s *serializer) Scheme() *runtime.Scheme {
	return s.scheme
}

// Codecs provides access to the underlying serializer.CodecFactory, may be used if low-level access
// is needed for encoding and decoding
func (s *serializer) Codecs() *k8sserializer.CodecFactory {
	return s.codecs
}

func (s *serializer) Decoder(optFns ...DecodingOptionsFunc) Decoder {
	opts := newDecodeOpts(optFns...)
	return newDecoder(s.schemeAndCodec, *opts)
}

func (s *serializer) Encoder(optFns ...EncodingOptionsFunc) Encoder {
	opts := newEncodeOpts(optFns...)
	return newEncoder(s.schemeAndCodec, *opts)
}

func (s *serializer) Converter() Converter {
	return s.converter
}

func (s *serializer) Defaulter() Defaulter {
	return s.defaulter
}

func prioritizedVersionForGroup(scheme *runtime.Scheme, groupName string) (schema.GroupVersion, error) {
	// Get the prioritized versions for the given group
	gvs := scheme.PrioritizedVersionsForGroup(groupName)
	if len(gvs) < 1 {
		return schema.GroupVersion{}, fmt.Errorf("expected some version to be registered for group %s", groupName)
	}
	// Use the first, preferred, (external) version
	return gvs[0], nil
}

func GVKForObject(scheme *runtime.Scheme, obj runtime.Object) (schema.GroupVersionKind, error) {
	// If we already have TypeMeta filled in here, just use it
	// TODO: This is probably not needed
	gvk := obj.GetObjectKind().GroupVersionKind()
	if !gvk.Empty() {
		return gvk, nil
	}

	// TODO: If there are two GVKs returned, it's probably a misconfiguration in the scheme
	// It might be expected though, and we can tolerate setting the GVK manually IFF there are more than
	// one ObjectKind AND the given GVK is one of them.

	// Get the possible kinds for the object
	gvks, unversioned, err := scheme.ObjectKinds(obj)
	if unversioned || err != nil || len(gvks) != 1 {
		return schema.GroupVersionKind{}, fmt.Errorf("unversioned %t or err %v or invalid gvks %v", unversioned, err, gvks)
	}
	return gvks[0], nil
}
