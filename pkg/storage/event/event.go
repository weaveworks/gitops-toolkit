package event

import (
	"fmt"

	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// ObjectEventType is an enum describing a change in an Object's state.
type ObjectEventType byte

var _ fmt.Stringer = ObjectEventType(0)

const (
	ObjectEventNone   ObjectEventType = iota // 0
	ObjectEventCreate                        // 1
	ObjectEventUpdate                        // 2
	ObjectEventDelete                        // 3
	ObjectEventSync                          // 4
	ObjectEventError                         // 5
)

func (o ObjectEventType) String() string {
	switch o {
	case 0:
		return "NONE"
	case 1:
		return "CREATE"
	case 2:
		return "UPDATE"
	case 3:
		return "DELETE"
	case 4:
		return "SYNC"
	case 5:
		return "ERROR"
	}

	// Should never happen
	return "UNKNOWN"
}

// ObjectEvent describes a change that has been observed
// for the given object with the given ID.
type ObjectEvent struct {
	ID   core.UnversionedObjectID
	Type ObjectEventType
	// Error is only non-nil if Type == ObjectEventError. The receiver
	// must check/respect the error if set.
	Error error
}

// ObjectEventStream is a channel of ObjectEvents
type ObjectEventStream chan *ObjectEvent
