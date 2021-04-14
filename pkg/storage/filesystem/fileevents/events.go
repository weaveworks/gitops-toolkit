package fileevents

// FileEventType is an enum describing a change in a file's state
type FileEventType byte

const (
	FileEventNone   FileEventType = iota // 0
	FileEventModify                      // 1
	FileEventDelete                      // 2
	FileEventMove                        // 3
)

func (e FileEventType) String() string {
	switch e {
	case 0:
		return "NONE"
	case 1:
		return "MODIFY"
	case 2:
		return "DELETE"
	case 3:
		return "MOVE"
	}

	return "UNKNOWN"
}

// FileEvent describes a file change of a certain kind at a certain
// (relative) path. Often emitted by FileEventsEmitter.
type FileEvent struct {
	Path string
	Type FileEventType

	// OldPath is non-empty only when Type == FileEventMove.
	OldPath string
}

// FileEventStream is a channel of FileEvents
type FileEventStream chan *FileEvent
