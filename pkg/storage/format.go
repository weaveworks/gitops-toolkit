package storage

import "github.com/weaveworks/libgitops/pkg/serializer/frame"

// ContentTypes describes the connection between
// file extensions and a content types.
var ContentTypes = map[string]frame.ContentType{
	".json": frame.ContentTypeJSON,
	".yaml": frame.ContentTypeYAML,
	".yml":  frame.ContentTypeYAML,
}

func extForContentType(wanted frame.ContentType) string {
	for ext, ct := range ContentTypes {
		if ct == wanted {
			return ext
		}
	}
	return ""
}
