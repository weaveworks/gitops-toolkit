package storage

import "github.com/weaveworks/libgitops/pkg/serializer/frame"

// ContentTypes describes the connection between
// file extensions and a content types.
var ContentTypes = map[string]frame.FramingType{
	".json": frame.FramingTypeJSON,
	".yaml": frame.FramingTypeYAML,
	".yml":  frame.FramingTypeYAML,
}

func extForContentType(wanted frame.FramingType) string {
	for ext, ct := range ContentTypes {
		if ct == wanted {
			return ext
		}
	}
	return ""
}
