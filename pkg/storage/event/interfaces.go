package event

import (
	"context"
	"io"

	"github.com/weaveworks/libgitops/pkg/storage"
)

// EventStorage is the abstract combination of a normal Storage, and
// a possiblility to listen for changes to objects as they change.
// TODO: Maybe we could use some of controller-runtime's built-in functionality
// for watching for changes?
type Storage interface {
	storage.Storage

	// WatchForObjectEvents starts feeding ObjectEvents into the given "into"
	// channel. The caller is responsible for setting a channel buffering
	// limit large enough to not block normal operation. An error might
	// be returned if a maximum amount of watches has been opened already,
	// e.g. ErrTooManyWatches.
	WatchForObjectEvents(ctx context.Context, into ObjectEventStream) error

	// Close closes the EventStorage and underlying resources gracefully.
	io.Closer
}
