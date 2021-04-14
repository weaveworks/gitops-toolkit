package serializer

import (
	"bytes"
	"encoding/json"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sserializer "k8s.io/apimachinery/pkg/runtime/serializer"
)

func NewEncoder(schemeLock LockedScheme, codecs *k8sserializer.CodecFactory, opts ...EncodeOption) Encoder {
	return &encoder{
		LockedScheme: schemeLock,
		codecs:       codecs,
		opts:         *defaultEncodeOpts().ApplyOptions(opts),
	}
}

type encoder struct {
	LockedScheme
	codecs *k8sserializer.CodecFactory

	opts EncodeOptions
}

func (e *encoder) GetLockedScheme() LockedScheme {
	return e.LockedScheme
}

func (e *encoder) CodecFactory() *k8sserializer.CodecFactory {
	return e.codecs
}

// Encode encodes the given objects and writes them to the specified FrameWriter.
// The FrameWriter specifies the ContentType. This encoder will automatically convert any
// internal object given to the preferred external groupversion. No conversion will happen
// if the given object is of an external version.
// TODO: This should automatically convert to the preferred version
// TODO: Fix that sometimes omitempty fields aren't respected
func (e *encoder) Encode(fw FrameWriter, objs ...runtime.Object) error {
	for _, obj := range objs {
		// Get the kind for the given object
		gvk, err := GVKForObject(e.Scheme(), obj)
		if err != nil {
			return err
		}

		// If the object is internal, convert it to the preferred external one
		if gvk.Version == runtime.APIVersionInternal {
			gv, err := PreferredVersionForGroup(e.Scheme(), gvk.Group)
			if err != nil {
				return err
			}
			gvk.Version = gv.Version
		}

		// Encode it
		if err := e.EncodeForGroupVersion(fw, obj, gvk.GroupVersion()); err != nil {
			return err
		}
	}
	return nil
}

// EncodeForGroupVersion encodes the given object for the specific groupversion. If the object
// is not of that version currently it will try to convert. The output bytes are written to the
// FrameWriter. The FrameWriter specifies the ContentType.
func (e *encoder) EncodeForGroupVersion(fw FrameWriter, obj runtime.Object, gv schema.GroupVersion) error {
	// Get the serializer for the media type
	serializerInfo, ok := runtime.SerializerInfoForMediaType(e.codecs.SupportedMediaTypes(), string(fw.ContentType()))
	if !ok {
		return ErrUnsupportedContentType
	}

	// Choose the default, non-pretty serializer, as we prettify if needed later
	// We technically could use the JSON PrettySerializer here, but it does not catch the
	// cases where the JSON iterator invokes MarshalJSON() on an object, and that object
	// returns non-pretty bytes (e.g. *unstructured.Unstructured). Hence, it is more robust
	// and extensible to always use the non-pretty serializer, and only on request indent
	// a given number of spaces after JSON encoding.
	encoder := serializerInfo.Serializer

	// Get a version-specific encoder for the specified groupversion
	versionEncoder := encoderForVersion(e.Scheme(), encoder, gv)

	// Check if the user requested prettified JSON output.
	// If the ContentType is JSON this is ok, we will intent the encode output on the fly.
	if *e.opts.JSONIndent > 0 && fw.ContentType() == ContentTypeJSON {
		fw = &jsonPrettyFrameWriter{indent: *e.opts.JSONIndent, fw: fw}
	}

	// Cast the object to a metav1.Object to get access to annotations
	metaobj, ok := toMetaObject(obj)
	// For objects without ObjectMeta, the cast will fail. Allow that failure and do "normal" encoding
	if !ok {
		return versionEncoder.Encode(obj, fw)
	}

	// Specialize the encoder for a specific gv and encode the object
	return e.encodeWithCommentSupport(versionEncoder, fw, obj, metaobj)
}

// encoderForVersion is used instead of CodecFactory.EncoderForVersion, as we want to use our own converter
func encoderForVersion(scheme *runtime.Scheme, encoder runtime.Encoder, gv schema.GroupVersion) runtime.Encoder {
	return newConversionCodecForScheme(
		scheme,
		encoder, // what content-type encoder to use
		nil,     // no decoder
		gv,      // specify what the target encode groupversion is
		nil,     // no target decode groupversion
		false,   // no defaulting
		true,    // convert if needed before encode
	)
}

type jsonPrettyFrameWriter struct {
	indent int32
	fw     FrameWriter
}

func (w *jsonPrettyFrameWriter) Write(p []byte) (n int, err error) {
	// Indent the source bytes
	var indented bytes.Buffer
	err = json.Indent(&indented, p, "", strings.Repeat(" ", int(w.indent)))
	if err != nil {
		return
	}
	// Write the pretty bytes to the underlying writer
	n, err = w.fw.Write(indented.Bytes())
	return
}

func (w *jsonPrettyFrameWriter) ContentType() ContentType {
	return w.fw.ContentType()
}
