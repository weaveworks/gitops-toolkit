package unstructuredevent

import (
	"context"
	"fmt"
	gosync "sync"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/event"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/fileevents"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/fileevents/inotify"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured"
	"github.com/weaveworks/libgitops/pkg/util/sync"
)

// Storage is a union of unstructured.Storage and fileevents.Storage.
//
// When the Sync() function is run; the ObjectEvents that are emitted to the
// listening channels with have ObjectEvent.Type == ObjectEventSync.
type Storage interface {
	unstructured.Storage
	fileevents.Storage
}

const defaultEventsBufferSize = 4096

// NewManifest is a high-level constructor for a generic
// unstructured.FileFinder and filesystem.Storage, together with a
// inotify FileWatcher; all combined into an unstructuredevent.Storage.
func NewManifest(
	dir string,
	contentTyper filesystem.ContentTyper,
	namespacer storage.Namespacer,
	recognizer unstructured.ObjectRecognizer,
	pathExcluder filesystem.PathExcluder,
) (Storage, error) {
	fs := filesystem.NewOSFilesystem(dir)
	fileFinder := unstructured.NewGenericFileFinder(contentTyper, fs)
	fsRaw, err := filesystem.NewGeneric(fileFinder, namespacer)
	if err != nil {
		return nil, err
	}
	emitter, err := inotify.NewFileWatcher(dir, &inotify.FileWatcherOptions{
		PathExcluder: pathExcluder,
	})
	if err != nil {
		return nil, err
	}
	unstructuredRaw, err := unstructured.NewGeneric(fsRaw, recognizer, pathExcluder, serializer.NewFrameReaderFactory())
	if err != nil {
		return nil, err
	}
	return NewGeneric(unstructuredRaw, emitter, GenericStorageOptions{
		SyncAtStart:   true,
		EmitSyncEvent: true,
	})
}

// NewGeneric is an extended Storage implementation, which
// together with the provided ObjectRecognizer and FileEventsEmitter listens for
// file events, keeps the mappings of the unstructured.Storage's unstructured.FileFinder
// in sync, and sends high-level ObjectEvents upstream.
func NewGeneric(
	s unstructured.Storage,
	emitter fileevents.Emitter,
	opts GenericStorageOptions,
) (Storage, error) {
	return &Generic{
		Storage: s,
		emitter: emitter,

		inbound: make(fileevents.FileEventStream, defaultEventsBufferSize),
		// outbound set by WatchForObjectEvents
		outboundMu: &gosync.Mutex{},

		// monitor set by WatchForObjectEvents, guarded by outboundMu

		opts: opts,
	}, nil
}

type GenericStorageOptions struct {
	// When Sync(ctx) is run, emit a "SYNC" event to the listening channel
	// Default: false
	EmitSyncEvent bool
	// Do a full re-sync at startup of the watcher
	// Default: true
	SyncAtStart bool
}

// Generic implements unstructuredevent.Storage.
var _ Storage = &Generic{}

// Generic is an extended Storage implementation, which
// together with the provided ObjectRecognizer and FileEventsEmitter listens for
// file events, keeps the mappings of the unstructured.Storage's unstructured.FileFinder
// in sync, and sends high-level ObjectEvents upstream.
//
// This implementation does not support different VersionRefs, but always stays on
// the "zero value" "" branch.
type Generic struct {
	unstructured.Storage
	// the filesystem events emitter
	emitter fileevents.Emitter

	// channels
	inbound    fileevents.FileEventStream
	outbound   event.ObjectEventStream
	outboundMu *gosync.Mutex

	// goroutine
	monitor *sync.Monitor

	// opts
	opts GenericStorageOptions
}

func (s *Generic) FileEventsEmitter() fileevents.Emitter {
	return s.emitter
}

func (s *Generic) WatchForObjectEvents(ctx context.Context, into event.ObjectEventStream) error {
	s.outboundMu.Lock()
	defer s.outboundMu.Unlock()
	// We don't support more than one listener
	// TODO: maybe support many listeners in the future?
	if s.outbound != nil {
		return fmt.Errorf("WatchStorage: not more than one watch supported: %w", fileevents.ErrTooManyWatches)
	}
	// Hook up our inbound channel to the emitter, to make the pipeline functional
	if err := s.emitter.WatchForFileEvents(ctx, s.inbound); err != nil {
		return err
	}
	// Set outbound at this stage so Sync possibly can send events.
	s.outbound = into
	// Start the backing goroutines
	s.monitor = sync.RunMonitor(s.monitorFunc)

	// Do a full sync in the beginning only if asked. Be aware that without running a Sync
	// at all before events start happening, the reporting might not work as it should
	if s.opts.SyncAtStart {
		// Disregard the changed files at Sync.
		if _, _, err := s.Sync(ctx); err != nil {
			return err
		}
	}
	return nil // all ok
}

// Sync extends the underlying unstructured.Storage.Sync(), but optionally also
// sends special "SYNC" and "ERROR" events to the returned "successful" and "duplicates"
// sets, respectively.
func (s *Generic) Sync(ctx context.Context) (successful, duplicates core.UnversionedObjectIDSet, err error) {
	// Sync the underlying UnstructuredStorage, and see what files had changed since last sync
	successful, duplicates, err = s.Storage.Sync(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Send special "sync" or "error" events for each of the changed objects, if configured
	if s.opts.EmitSyncEvent {
		_ = successful.ForEach(func(id core.UnversionedObjectID) error {
			// Send a special "sync" event for this ObjectID to the events channel
			s.sendEvent(event.ObjectEventSync, id)
			return nil
		})
		_ = duplicates.ForEach(func(id core.UnversionedObjectID) error {
			// Send an error upstream for the duplicate
			s.sendError(id, fmt.Errorf("%w: %s", unstructured.ErrTrackingDuplicate, id))
			return nil
		})
	}

	return
}

// Write writes the given content to the resource indicated by the ID.
// Error returns are implementation-specific.
func (s *Generic) Write(ctx context.Context, id core.UnversionedObjectID, content []byte) error {
	// Get the path and verify namespacing info
	p, err := s.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Suspend the write event
	s.emitter.Suspend(ctx, p)
	// Call the underlying filesystem.Storage
	return s.Storage.Write(ctx, id, content)
}

// Delete deletes the resource indicated by the ID.
// If the resource does not exist, it returns ErrNotFound.
func (s *Generic) Delete(ctx context.Context, id core.UnversionedObjectID) error {
	// Get the path and verify namespacing info
	p, err := s.getPath(ctx, id)
	if err != nil {
		return err
	}
	// Suspend the write event
	s.emitter.Suspend(ctx, p)
	// Call the underlying filesystem.Storage
	return s.Storage.Delete(ctx, id)
}

func (s *Generic) getPath(ctx context.Context, id core.UnversionedObjectID) (string, error) {
	// Verify namespacing info
	if err := storage.VerifyNamespaced(s.Namespacer(), id.GroupKind(), id.ObjectKey().Namespace); err != nil {
		return "", err
	}
	// Get the path
	return s.FileFinder().ObjectPath(ctx, id)
}

func (s *Generic) Close() error {
	err := s.emitter.Close()
	// No need to check the error here
	_ = s.monitor.Wait()
	return err
}

func (s *Generic) monitorFunc() error {
	logrus.Debug("WatchStorage: Monitoring thread started")
	defer logrus.Debug("WatchStorage: Monitoring thread stopped")

	ctx := context.Background()

	for {
		// TODO: handle context cancellations, i.e. ctx.Done()
		ev, ok := <-s.inbound
		if !ok {
			logrus.Error("WatchStorage: Fatal: Got non-ok response from watcher.GetFileEventStream()")
			return nil
		}

		logrus.Tracef("WatchStorage: Processing event: %s", ev.Type)

		// Skip the file if it has an invalid path
		if !filesystem.IsValidFileInFilesystem(
			ctx,
			s.FileFinder().Filesystem(),
			s.FileFinder().ContentTyper(),
			s.PathExcluder(),
			ev.Path) {
			logrus.Tracef("WatchStorage: Skipping file %q as it is ignored by the ContentTyper/PathExcluder", ev.Path)
			continue
		}

		var err error
		switch ev.Type {
		// FileEventModify is also sent for newly-created files
		case fileevents.FileEventModify, fileevents.FileEventMove:
			err = s.handleModifyMove(ctx, ev)
		case fileevents.FileEventDelete:
			err = s.handleDelete(ctx, ev)
		default:
			err = fmt.Errorf("cannot handle update of type %v for path %q", ev.Type, ev.Path)
		}
		if err != nil {
			logrus.Errorf("WatchStorage: %v", err)
		}
	}
}

func (s *Generic) handleDelete(ctx context.Context, ev *fileevents.FileEvent) error {
	// Delete the given path from the FileFinder; loop through the deleted objects
	return s.UnstructuredFileFinder().DeleteMapping(ctx, ev.Path).ForEach(func(id core.UnversionedObjectID) error {
		// Send the delete event to the channel
		s.sendEvent(event.ObjectEventDelete, id)
		return nil
	})
}

func (s *Generic) handleModifyMove(ctx context.Context, ev *fileevents.FileEvent) error {
	fileFinder := s.UnstructuredFileFinder()

	// If the file was moved, move the cached mapping(s) too
	if ev.Type == fileevents.FileEventMove {
		// There's no need to check if this move actually was performed; as
		// if OldPath did not exist previously, the code below will just treat
		// it as a Create.
		_ = fileFinder.MoveFile(ctx, ev.OldPath, ev.Path)
	}

	// Recognize the contents of the file
	idSet, cp, alreadyCached, err := unstructured.RecognizeIDsInFile(
		ctx,
		fileFinder,
		s.ObjectRecognizer(),
		s.FrameReaderFactory(),
		ev.Path,
	)
	if err != nil {
		return err
	}
	// If the file is already up-to-date as per the checksum, we're all fine
	if alreadyCached {
		return nil
	}

	// Store this new mapping in the cache
	added, duplicates, removed := fileFinder.SetMapping(ctx, *cp, idSet)

	// Send added events
	_ = added.ForEach(func(id core.UnversionedObjectID) error {
		// Send a create event to the channel
		s.sendEvent(event.ObjectEventCreate, id)
		return nil
	})
	// Send modify events. Do not mutate idSet unnecessarily.
	_ = idSet.Copy().
		DeleteSet(added).
		DeleteSet(removed).
		DeleteSet(duplicates).
		ForEach(func(id core.UnversionedObjectID) error {
			// Send a update event to the channel
			s.sendEvent(event.ObjectEventUpdate, id)
			return nil
		})
	// Send removed events
	_ = removed.ForEach(func(id core.UnversionedObjectID) error {
		// Send a delete event to the channel
		s.sendEvent(event.ObjectEventDelete, id)
		return nil
	})
	// Send duplicate error events
	_ = duplicates.ForEach(func(id core.UnversionedObjectID) error {
		// Send an error event to the channel
		s.sendError(id, fmt.Errorf("%w: %q, %s", unstructured.ErrTrackingDuplicate, ev.Path, id))
		return nil
	})

	return nil
}

func (s *Generic) sendEvent(eventType event.ObjectEventType, id core.UnversionedObjectID) {
	logrus.Tracef("Generic: Sending event: %v", eventType)
	s.outbound <- &event.ObjectEvent{
		ID:   id,
		Type: eventType,
	}
}

func (s *Generic) sendError(id core.UnversionedObjectID, err error) {
	logrus.Tracef("Generic: Sending error event for %s: %v", id, err)
	s.outbound <- &event.ObjectEvent{
		ID:    id,
		Type:  event.ObjectEventError,
		Error: err,
	}
}
