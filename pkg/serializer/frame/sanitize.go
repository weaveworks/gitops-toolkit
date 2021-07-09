package frame

import "bytes"

// Sanitizer is an interface for sanitizing frames. Note that a sanitizer can only do
// its work correctly if frame actually only contains one frame within.
type Sanitizer interface {
	// Sanitize sanitizes the frame in a standardized way for the given
	// FramingType. If the FramingType isn't known, the Sanitizer can choose between
	// returning an ErrUnsupportedFramingType error or just returning frame, nil unmodified.
	// If ErrUnsupportedFramingType is returned, the consumer won't probably be able to handle
	// other framing types than the default ones, which might not be desired.
	//
	// The returned frame should have len == 0 if it's considered empty.
	Sanitize(ct FramingType, frame []byte) ([]byte, error)
}

// DefaultSanitizer implements frame sanitation for JSON and YAML.
//
// For YAML it removes unnecessary "---" separators, whitespace and newlines.
// The YAML frame always ends with a newline, unless the sanitized YAML was an empty string, in which
// case an empty string with len == 0 will be returned.
//
// For JSON it sanitizes the JSON frame by removing unnecessary spaces and newlines around it.
type DefaultSanitizer struct{}

func (DefaultSanitizer) Sanitize(ct FramingType, frame []byte) ([]byte, error) {
	switch ct {
	case FramingTypeYAML:
		return sanitizeYAMLFrame(frame), nil
	case FramingTypeJSON:
		return sanitizeJSONFrame(frame), nil
	default:
		// Just passthrough
		return frame, nil
	}
}

func sanitizeYAMLFrame(frame []byte) []byte {
	prevLen := len(frame)
	for {
		// Trim spaces
		frame = bytes.TrimSpace(frame)
		// Trim leading and trailing "---" (not sometimes removed by the underlying YAMLReader, for some reason)
		frame = bytes.TrimPrefix(frame, []byte("---"))
		frame = bytes.TrimSuffix(frame, []byte("---"))

		// Keep trimming as long as the frame keeps shrinking
		if len(frame) < prevLen {
			prevLen = len(frame)
			continue
		}

		// Append a newline when returning YAML, if non-empty. At this point all newlines, etc. are removed
		if len(frame) != 0 {
			frame = append(frame, '\n')
		}

		// If we got here the frame didn't shrink in size during this round, that means we've
		// trimmed all that is possible, hence we can return
		return frame
	}
}

func sanitizeJSONFrame(frame []byte) []byte {
	return bytes.TrimSpace(frame)
}
