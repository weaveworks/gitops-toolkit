package serializer

import (
	"k8s.io/utils/pointer"
)

type EncodeOption interface {
	ApplyToEncode(*EncodeOptions)
}

func defaultEncodeOpts() *EncodeOptions {
	return &EncodeOptions{
		// Default to "pretty encoding"
		JSONIndent:       pointer.Int32Ptr(2),
		PreserveComments: PreserveCommentsDisable,
	}
}

type EncodeOptions struct {
	// Indent JSON encoding output with this many spaces.
	// Set this to 0, use PrettyEncode(false) or JSONIndent(0) to disable pretty output.
	// Only applicable to ContentTypeJSON framers.
	//
	// Default: 2, i.e. pretty output
	// TODO: Make this a property of the FrameWriter instead?
	JSONIndent *int32

	// Whether to preserve YAML comments internally.
	// This only works for objects embedding metav1.ObjectMeta.
	//
	// Only applicable to ContentTypeYAML framers. Using any other framer will be silently ignored.
	//
	// Usage of this option also requires setting the PreserveComments in DecodeOptions, too.
	//
	// Default: PreserveCommentsDisable
	PreserveComments PreserveComments

	// TODO: Maybe consider an option to always convert to the preferred version (not just internal)
}

var _ EncodeOption = &EncodeOptions{}

func (o *EncodeOptions) ApplyToEncode(target *EncodeOptions) {
	if o.JSONIndent != nil {
		target.JSONIndent = o.JSONIndent
	}
	if o.PreserveComments != 0 {
		target.PreserveComments = o.PreserveComments
	}
}

func (o *EncodeOptions) ApplyOptions(opts []EncodeOption) *EncodeOptions {
	for _, opt := range opts {
		opt.ApplyToEncode(o)
	}
	// it is guaranteed that all options are non-nil, as defaultEncodeOpts() includes all fields
	return o
}

// Whether to preserve YAML comments internally.
// This only works for objects embedding metav1.ObjectMeta.
//
// Only applicable to ContentTypeYAML framers. Using any other framer will be silently ignored.
// TODO: Add a BestEffort mode
type PreserveComments int

const (
	// PreserveCommentsDisable means do not try to preserve comments
	PreserveCommentsDisable PreserveComments = 1 + iota
	// PreserveCommentsStrict means try to preserve comments, and fail if it does not work
	PreserveCommentsStrict
)

var _ EncodeOption = PreserveComments(0)
var _ DecodeOption = PreserveComments(0)

func (p PreserveComments) ApplyToEncode(target *EncodeOptions) {
	// TODO: Validate?
	target.PreserveComments = p
}

func (p PreserveComments) ApplyToDecode(target *DecodeOptions) {
	// TODO: Validate?
	target.PreserveComments = p
}

// Indent JSON encoding output with this many spaces.
// Use PrettyEncode(false) or JSONIndent(0) to disable pretty output.
// Only applicable to ContentTypeJSON framers.
type JSONIndent int32

var _ EncodeOption = JSONIndent(0)

func (i JSONIndent) ApplyToEncode(target *EncodeOptions) {
	target.JSONIndent = pointer.Int32Ptr(int32(i))
}

// Shorthand for JSONIndent(0) if false, or JSONIndent(2) if true
type PrettyEncode bool

var _ EncodeOption = PrettyEncode(false)

func (pretty PrettyEncode) ApplyToEncode(target *EncodeOptions) {
	if pretty {
		JSONIndent(2).ApplyToEncode(target)
	} else {
		JSONIndent(0).ApplyToEncode(target)
	}
}

// DECODING

type DecodeOption interface {
	ApplyToDecode(*DecodeOptions)
}

func defaultDecodeOpts() *DecodeOptions {
	return &DecodeOptions{
		ConvertToHub:       pointer.BoolPtr(false),
		Strict:             pointer.BoolPtr(true),
		Default:            pointer.BoolPtr(false),
		DecodeListElements: pointer.BoolPtr(true),
		PreserveComments:   PreserveCommentsDisable,
		DecodeUnknown:      pointer.BoolPtr(false),
	}
}

type DecodeOptions struct {
	// Not applicable for Decoder.DecodeInto(). If true, the decoded external object
	// will be converted into its hub (or internal, where applicable) representation.
	// Otherwise, the decoded object will be left in its external representation.
	//
	// Default: false
	ConvertToHub *bool

	// Parse the YAML/JSON in strict mode, returning a specific error if the input
	// contains duplicate or unknown fields or formatting errors.
	//
	// Default: true
	Strict *bool

	// Automatically default the decoded object.
	// Default: false
	Default *bool

	// Only applicable for Decoder.DecodeAll(). If the underlying data contains a v1.List,
	// the items of the list will be traversed, decoded into their respective types, and
	// appended to the returned slice. The v1.List will in this case not be returned.
	// This conversion does NOT support preserving comments. If the given scheme doesn't
	// recognize the v1.List, before using it will be registered automatically.
	//
	// Default: true
	DecodeListElements *bool

	// Whether to preserve YAML comments internally.
	// This only works for objects embedding metav1.ObjectMeta.
	//
	// Only applicable to ContentTypeYAML framers. Using any other framer will be silently ignored.
	//
	// Usage of this option also requires setting the PreserveComments in EncodeOptions, too.
	//
	// Default: PreserveCommentsDisable
	PreserveComments PreserveComments

	// DecodeUnknown specifies whether decode objects with an unknown GroupVersionKind into a
	// *runtime.Unknown object when running Decode(All) (true value) or to return an error when
	// any unrecognized type is found (false value).
	//
	// Default: false
	DecodeUnknown *bool
}

var _ DecodeOption = &DecodeOptions{}

func (o *DecodeOptions) ApplyToDecode(target *DecodeOptions) {
	if o.ConvertToHub != nil {
		target.ConvertToHub = o.ConvertToHub
	}
	if o.Strict != nil {
		target.Strict = o.Strict
	}
	if o.Default != nil {
		target.Default = o.Default
	}
	if o.DecodeListElements != nil {
		target.DecodeListElements = o.DecodeListElements
	}
	if o.PreserveComments != 0 {
		target.PreserveComments = o.PreserveComments
	}
	if o.DecodeUnknown != nil {
		target.DecodeUnknown = o.DecodeUnknown
	}
}

func (o *DecodeOptions) ApplyOptions(opts []DecodeOption) *DecodeOptions {
	for _, opt := range opts {
		opt.ApplyToDecode(o)
	}
	// it is guaranteed that all options are non-nil, as defaultDecodeOpts() includes all fields
	return o
}

// Not applicable for Decoder.DecodeInto(). If true, the decoded external object
// will be converted into its hub (or internal, where applicable) representation.
// Otherwise, the decoded object will be left in its external representation.
type ConvertToHub bool

var _ DecodeOption = ConvertToHub(false)

func (b ConvertToHub) ApplyToDecode(target *DecodeOptions) {
	target.ConvertToHub = pointer.BoolPtr(bool(b))
}

// Parse the YAML/JSON in strict mode, returning a specific error if the input
// contains duplicate or unknown fields or formatting errors.
type DecodeStrict bool

var _ DecodeOption = DecodeStrict(false)

func (b DecodeStrict) ApplyToDecode(target *DecodeOptions) {
	target.Strict = pointer.BoolPtr(bool(b))
}

// Automatically default the decoded object.
type DefaultAtDecode bool

var _ DecodeOption = DefaultAtDecode(false)

func (b DefaultAtDecode) ApplyToDecode(target *DecodeOptions) {
	target.Default = pointer.BoolPtr(bool(b))
}

// Only applicable for Decoder.DecodeAll(). If the underlying data contains a v1.List,
// the items of the list will be traversed, decoded into their respective types, and
// appended to the returned slice. The v1.List will in this case not be returned.
// This conversion does NOT support preserving comments. If the given scheme doesn't
// recognize the v1.List, before using it will be registered automatically.
type DecodeListElements bool

var _ DecodeOption = DecodeListElements(false)

func (b DecodeListElements) ApplyToDecode(target *DecodeOptions) {
	target.DecodeListElements = pointer.BoolPtr(bool(b))
}

// DecodeUnknown specifies whether decode objects with an unknown GroupVersionKind into a
// *runtime.Unknown object when running Decode(All) (true value) or to return an error when
// any unrecognized type is found (false value).
type DecodeUnknown bool

var _ DecodeOption = DecodeUnknown(false)

func (b DecodeUnknown) ApplyToDecode(target *DecodeOptions) {
	target.DecodeUnknown = pointer.BoolPtr(bool(b))
}
