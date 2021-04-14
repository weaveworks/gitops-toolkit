package inotify

import (
	"time"

	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
)

// How many inotify events we can buffer before watching is interrupted
const DefaultEventBufferSize int32 = 4096

type FileWatcherOption interface {
	ApplyToFileWatcher(*FileWatcherOptions)
}

var _ FileWatcherOption = &FileWatcherOptions{}

// FileWatcherOptions specifies options for the FileWatcher
type FileWatcherOptions struct {
	// BatchTimeout specifies the duration to wait after last event
	// before dispatching grouped inotify events
	// Default: 1s
	BatchTimeout time.Duration
	// EventBufferSize describes how many inotify events can be buffered
	// before watching is interrupted/delayed.
	// Default: DefaultEventBufferSize
	EventBufferSize int32
	// PathExcluder provides a way to exclude paths.
	// Default: filesystem.DefaultPathExcluders()
	PathExcluder filesystem.PathExcluder
}

func (o *FileWatcherOptions) ApplyToFileWatcher(target *FileWatcherOptions) {
	if o.BatchTimeout != 0 {
		target.BatchTimeout = o.BatchTimeout
	}
	if o.EventBufferSize != 0 {
		target.EventBufferSize = o.EventBufferSize
	}
	if o.PathExcluder != nil {
		target.PathExcluder = o.PathExcluder
	}
}

func (o *FileWatcherOptions) ApplyOptions(opts []FileWatcherOption) *FileWatcherOptions {
	for _, opt := range opts {
		opt.ApplyToFileWatcher(o)
	}
	return o
}

// defaultOptions returns the default options
func defaultOptions() *FileWatcherOptions {
	return &FileWatcherOptions{
		BatchTimeout:    1 * time.Second,
		EventBufferSize: DefaultEventBufferSize,
		PathExcluder:    filesystem.DefaultPathExcluders(),
	}
}
