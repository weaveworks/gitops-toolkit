package serializer

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"github.com/weaveworks/libgitops/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type EncodingOptions struct {
	// Use pretty printing when writing to the output. (Default: true)
	// TODO: Fix that sometimes omitempty fields aren't respected
	Pretty *bool
	// Whether to preserve YAML comments internally. This only works for objects embedding metav1.ObjectMeta.
	// Only applicable to frame.FramingTypeYAML framers.
	// Using any other framer will be silently ignored. Usage of this option also requires setting
	// the PreserveComments in DecodingOptions, too. (Default: false)
	// TODO: Make this a BestEffort & Strict mode
	PreserveComments *bool

	// TODO: Maybe consider an option to always convert to the preferred version (not just internal)
}

type EncodingOptionsFunc func(*EncodingOptions)

func WithPrettyEncode(pretty bool) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		opts.Pretty = &pretty
	}
}

func WithCommentsEncode(comments bool) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		opts.PreserveComments = &comments
	}
}

func WithEncodingOptions(newOpts EncodingOptions) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		// TODO: Null-check all of these before using them
		*opts = newOpts
	}
}

func defaultEncodeOpts() *EncodingOptions {
	return &EncodingOptions{
		Pretty:           util.BoolPtr(true),
		PreserveComments: util.BoolPtr(false),
	}
}

func newEncodeOpts(fns ...EncodingOptionsFunc) *EncodingOptions {
	opts := defaultEncodeOpts()
	for _, fn := range fns {
		fn(opts)
	}
	return opts
}

type encoder struct {
	*schemeAndCodec

	opts EncodingOptions
}

func newEncoder(schemeAndCodec *schemeAndCodec, opts EncodingOptions) Encoder {
	return &encoder{
		schemeAndCodec,
		opts,
	}
}

// Encode encodes the given objects and writes them to the specified frame.Writer.
// The frame.Writer specifies the frame.FramingType. This encoder will automatically convert any
// internal object given to the preferred external groupversion. No conversion will happen
// if the given object is of an external version.
// TODO: This should automatically convert to the preferred version
func (e *encoder) Encode(fw frame.Writer, objs ...runtime.Object) error {
	for _, obj := range objs {
		// Get the kind for the given object
		gvk, err := GVKForObject(e.scheme, obj)
		if err != nil {
			return err
		}

		// If the object is internal, convert it to the preferred external one
		if gvk.Version == runtime.APIVersionInternal {
			gv, err := prioritizedVersionForGroup(e.scheme, gvk.Group)
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
// frame.Writer. The frame.Writer specifies the frame.FramingType.
func (e *encoder) EncodeForGroupVersion(fw frame.Writer, obj runtime.Object, gv schema.GroupVersion) error {
	// Get the serializer for the media type
	serializerInfo, ok := runtime.SerializerInfoForMediaType(e.codecs.SupportedMediaTypes(), string(fw.FramingType()))
	if !ok {
		return frame.MakeUnsupportedFramingTypeError(fw.FramingType())
	}

	// Choose the pretty or non-pretty one
	encoder := serializerInfo.Serializer

	// Use the pretty serializer if it was asked for and is defined for the content type
	if *e.opts.Pretty {
		// Apparently not all SerializerInfos have this field defined (e.g. YAML)
		// TODO: This could be considered a bug in upstream, create an issue
		if serializerInfo.PrettySerializer != nil {
			encoder = serializerInfo.PrettySerializer
		} else {
			logrus.Debugf("PrettySerializer for frame.FramingType %s is nil, falling back to Serializer.", fw.FramingType())
		}
	}

	// Get a version-specific encoder for the specified groupversion
	versionEncoder := encoderForVersion(e.scheme, encoder, gv)

	// Cast the object to a metav1.Object to get access to annotations
	metaobj, ok := toMetaObject(obj)
	// For objects without ObjectMeta, the cast will fail. Allow that failure and do "normal" encoding
	if !ok {
		ctx := context.TODO()
		return versionEncoder.Encode(obj, frame.ToIoWriteCloser(ctx, fw))
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
