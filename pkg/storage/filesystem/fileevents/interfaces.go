package fileevents

import (
	"context"
	"errors"
	"io"

	"github.com/weaveworks/libgitops/pkg/storage/event"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
)

var (
	// ErrTooManyWatches can happen when trying to register too many
	// watching reciever channels to an event emitter.
	ErrTooManyWatches = errors.New("too many watches already opened")
)

// Emitter is an interface that provides high-level inotify-like
// behaviour to consumers. It can be used e.g. by even higher-level
// interfaces like FilesystemEventStorage.
type Emitter interface {
	// WatchForFileEvents starts feeding FileEvents into the given "into"
	// channel. The caller is responsible for setting a channel buffering
	// limit large enough to not block normal operation. An error might
	// be returned if a maximum amount of watches has been opened already,
	// e.g. ErrTooManyWatches.
	//
	// Note that it is the receiver's responsibility to "validate" the
	// file so it matches any user defined policy (e.g. only specific
	// content types, or a PathExcluder has been given).
	WatchForFileEvents(ctx context.Context, into FileEventStream) error

	// Suspend blocks the next event dispatch for this given path. Useful
	// for not sending "your own" modification events into the
	// FileEventStream that is listening. path is relative.
	Suspend(ctx context.Context, path string)

	// Close closes the emitter gracefully.
	io.Closer
}

// Storage is the union of a filesystem.Storage, and event.Storage,
// and the possibility to listen for object updates from a Emitter.
type Storage interface {
	filesystem.Storage
	event.Storage

	// FileEventsEmitter gets the Emitter used internally.
	FileEventsEmitter() Emitter
}
