package inotify

import (
	"context"
	"fmt"
	"path/filepath"
	gosync "sync"
	"time"

	"github.com/rjeczalik/notify"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/fileevents"
	"github.com/weaveworks/libgitops/pkg/util/sync"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/sets"
)

var listenEvents = []notify.Event{notify.InDelete, notify.InCloseWrite, notify.InMovedFrom, notify.InMovedTo}

var eventMap = map[notify.Event]fileevents.FileEventType{
	notify.InDelete:     fileevents.FileEventDelete,
	notify.InCloseWrite: fileevents.FileEventModify,
}

// combinedEvents describes the event combinations to concatenate,
// this is iterated in order, so the longest matches should be first
var combinedEvents = []combinedEvent{
	// DELETE + MODIFY => MODIFY
	{[]notify.Event{notify.InDelete, notify.InCloseWrite}, 1},
	// MODIFY + DELETE => NONE
	{[]notify.Event{notify.InCloseWrite, notify.InDelete}, -1},
	// MOVE + MODIFY => MOVE
	{[]notify.Event{notify.InMovedTo, notify.InCloseWrite}, 0},
	// MODIFY + MOVE => MOVE
	{[]notify.Event{notify.InCloseWrite, notify.InMovedTo}, 1},
}

type notifyEvents []notify.EventInfo
type eventStream chan notify.EventInfo

// FileEvents is a slice of FileEvent pointers
type FileEvents []*fileevents.FileEvent

// NewFileWatcher returns a list of files in the watched directory in
// addition to the generated FileWatcher, it can be used to populate
// MappedRawStorage fileMappings
func NewFileWatcher(dir string, opts ...FileWatcherOption) (fileevents.Emitter, error) {
	o := defaultOptions().ApplyOptions(opts)

	w := &FileWatcher{
		dir: dir,

		inbound: make(eventStream, int(o.EventBufferSize)),
		// outbound is set by WatchForFileEvents
		outboundMu: &gosync.Mutex{},

		suspendFiles:   sets.NewString(),
		suspendFilesMu: &gosync.Mutex{},

		// monitor and dispatcher set by WatchForFileEvents, guarded by outboundMu

		opts: *o,

		batcher: sync.NewBatchWriter(o.BatchTimeout),
	}

	log.Tracef("FileWatcher: Starting recursive watch for %q", dir)
	if err := notify.Watch(filepath.Join(dir, "..."), w.inbound, listenEvents...); err != nil {
		notify.Stop(w.inbound)
		return nil, err
	}

	return w, nil
}

var _ fileevents.Emitter = &FileWatcher{}

// FileWatcher recursively monitors changes in files in the given directory
// and sends out events based on their state changes. Only files conforming
// to validSuffix are monitored. The FileWatcher can be suspended for a single
// event at a time to eliminate updates by WatchStorage causing a loop.
type FileWatcher struct {
	dir string
	// channels
	inbound    eventStream
	outbound   fileevents.FileEventStream
	outboundMu *gosync.Mutex
	// new suspend logic
	suspendFiles   sets.String
	suspendFilesMu *gosync.Mutex
	// goroutines
	monitor    *sync.Monitor
	dispatcher *sync.Monitor

	// opts
	opts FileWatcherOptions
	// the batcher is used for properly sending many concurrent inotify events
	// as a group, after a specified timeout. This fixes the issue of one single
	// file operation being registered as many different inotify events
	batcher *sync.BatchWriter
}

func (w *FileWatcher) WatchForFileEvents(ctx context.Context, into fileevents.FileEventStream) error {
	w.outboundMu.Lock()
	defer w.outboundMu.Unlock()
	// We don't support more than one listener
	// TODO: maybe support many listeners in the future?
	if w.outbound != nil {
		return fmt.Errorf("FileWatcher: not more than one watch supported: %w", fileevents.ErrTooManyWatches)
	}
	w.outbound = into
	// Start the backing goroutines
	w.monitor = sync.RunMonitor(w.monitorFunc)
	w.dispatcher = sync.RunMonitor(w.dispatchFunc)
	return nil // all ok
}

func (w *FileWatcher) monitorFunc() error {
	log.Debug("FileWatcher: Monitoring thread started")
	defer log.Debug("FileWatcher: Monitoring thread stopped")
	defer close(w.outbound) // Close the update stream after the FileWatcher has stopped

	for {
		event, ok := <-w.inbound
		if !ok {
			logrus.Debug("FileWatcher: Got non-ok channel recieve from w.inbound, exiting monitorFunc")
			return nil
		}

		if ievent(event).Mask&unix.IN_ISDIR != 0 {
			continue // Skip directories
		}

		// Get the relative path between the root directory and the changed file
		// Note: This is just used for the PathExcluder, absolute paths are used
		// in the underlying file-change computation system, until in sendUpdate
		// where they are converted into relative paths before sending to the listener.
		relativePath, err := filepath.Rel(w.dir, event.Path())
		if err != nil {
			logrus.Errorf("FileWatcher: Error occurred when computing relative path between: %s and %s: %v", w.dir, event.Path(), err)
			continue
		}

		// The PathExcluder only operates on relative paths.
		if w.opts.PathExcluder.ShouldExcludePath(relativePath) {
			continue // Skip ignored files
		}

		// Get any events registered for the specific file, and append the specified event
		var eventList notifyEvents
		if val, ok := w.batcher.Load(event.Path()); ok {
			eventList = val.(notifyEvents)
		}

		eventList = append(eventList, event)

		// Register the event in the map, and dispatch all the events at once after the timeout
		// Note that event.Path() is just the unique key for the map here, it is not actually
		// used later when computing the changes of the filesystem.
		w.batcher.Store(event.Path(), eventList)
		log.Debugf("FileWatcher: Registered inotify events %v for path %q", eventList, event.Path())
	}
}

func (w *FileWatcher) dispatchFunc() error {
	log.Debug("FileWatcher: Dispatch thread started")
	defer log.Debug("FileWatcher: Dispatch thread stopped")

	for {
		// Wait until we have a batch dispatched to us
		ok := w.batcher.ProcessBatch(func(_, val interface{}) bool {
			// Concatenate all known events, and dispatch them to be handled one by one
			for _, event := range w.concatenateEvents(val.(notifyEvents)) {
				w.sendUpdate(event)
			}

			// Continue traversing the map
			return true
		})
		if !ok {
			logrus.Debug("FileWatcher: Got non-ok channel recieve from w.batcher, exiting dispatchFunc")
			return nil // The BatchWriter channel is closed, stop processing
		}

		log.Debug("FileWatcher: Dispatched events batch and reset the events cache")
	}
}

func (w *FileWatcher) sendUpdate(event *fileevents.FileEvent) {
	// Get the relative path between the root directory and the changed file
	relativePath, err := filepath.Rel(w.dir, event.Path)
	if err != nil {
		logrus.Errorf("FileWatcher: Error occurred when computing relative path between: %s and %s: %v", w.dir, event.Path, err)
		return
	}
	// Replace the full path with the relative path for the signaling upstream
	event.Path = relativePath

	if len(event.OldPath) != 0 {
		// Do the same for event.OldPath
		relativePath, err = filepath.Rel(w.dir, event.OldPath)
		if err != nil {
			logrus.Errorf("FileWatcher: Error occurred when computing relative path between: %s and %s: %v", w.dir, event.OldPath, err)
			return
		}
		// Replace the full path with the relative path for the signaling upstream
		event.OldPath = relativePath
	}

	if w.shouldSuspendEvent(event.Path) {
		log.Debugf("FileWatcher: Skipping suspended event %s for path: %q", event.Type, event.Path)
		return // Skip the suspended event
	}
	if event.Type == fileevents.FileEventMove {
		log.Debugf("FileWatcher: Sending update: %s: %q -> %q", event.Type, event.OldPath, event.Path)
	} else {
		log.Debugf("FileWatcher: Sending update: %s -> %q", event.Type, event.Path)
	}

	w.outbound <- event
}

// Close closes active underlying resources
func (w *FileWatcher) Close() error {
	notify.Stop(w.inbound)
	w.batcher.Close()
	close(w.inbound) // Close the inbound event stream
	// No need to check the error here, as we only return nil above
	_ = w.monitor.Wait()
	_ = w.dispatcher.Wait()
	return nil
}

// Suspend enables a one-time suspend for any event from the given path.
// The path must be relative to the root directory, i.e. computed as
// path = filepath.Rel(<rootDir>, <absFilePath>).
func (w *FileWatcher) Suspend(_ context.Context, path string) {
	w.suspendFilesMu.Lock()
	defer w.suspendFilesMu.Unlock()
	w.suspendFiles.Insert(path)
}

// shouldSuspendEvent checks if an event for the given path
// should be suspended for one time. If it should, true will
// be returned, and the mapping will be removed next time.
func (w *FileWatcher) shouldSuspendEvent(path string) bool {
	w.suspendFilesMu.Lock()
	defer w.suspendFilesMu.Unlock()
	// If the path should not be suspended, just return false and be done
	if !w.suspendFiles.Has(path) {
		return false
	}
	// Otherwise, remove it from the list and mark it as suspended
	w.suspendFiles.Delete(path)
	return true
}

func convertEvent(event notify.Event) fileevents.FileEventType {
	if updateEvent, ok := eventMap[event]; ok {
		return updateEvent
	}

	return fileevents.FileEventNone
}

func convertUpdate(event notify.EventInfo) *fileevents.FileEvent {
	fileEvent := convertEvent(event.Event())
	if fileEvent == fileevents.FileEventNone {
		// This should never happen
		panic(fmt.Sprintf("invalid event for update conversion: %q", event.Event().String()))
	}

	return &fileevents.FileEvent{
		Path: event.Path(),
		Type: fileEvent,
	}
}

// moveCache caches an event during a move operation
// and dispatches a FileUpdate if it's not cancelled
type moveCache struct {
	watcher *FileWatcher
	event   notify.EventInfo
	timer   *time.Timer
}

func (w *FileWatcher) newMoveCache(event notify.EventInfo) *moveCache {
	m := &moveCache{
		watcher: w,
		event:   event,
	}

	// moveCaches wait one second to be cancelled before firing
	m.timer = time.AfterFunc(w.opts.BatchTimeout, m.incomplete)
	return m
}

func (m *moveCache) cookie() uint32 {
	return ievent(m.event).Cookie
}

// If the moveCache isn't cancelled, the move is considered incomplete and this
// method is fired. A complete move consists out of a "from" event and a "to" event,
// if only one is received, the file is moved in/out of a watched directory, which
// is treated as a normal creation/deletion by this method.
func (m *moveCache) incomplete() {
	var evType fileevents.FileEventType

	switch m.event.Event() {
	case notify.InMovedFrom:
		evType = fileevents.FileEventDelete
	case notify.InMovedTo:
		evType = fileevents.FileEventModify
	default:
		// This should never happen
		panic(fmt.Sprintf("moveCache: unrecognized event: %v", m.event.Event()))
	}

	log.Tracef("moveCache: Timer expired for %d, dispatching...", m.cookie())
	m.watcher.sendUpdate(&fileevents.FileEvent{Path: m.event.Path(), Type: evType})

	// Delete the cache after the timer has fired
	moveCachesMu.Lock()
	delete(moveCaches, m.cookie())
	moveCachesMu.Unlock()
}

func (m *moveCache) cancel() {
	m.timer.Stop()
	moveCachesMu.Lock()
	delete(moveCaches, m.cookie())
	moveCachesMu.Unlock()
	log.Tracef("moveCache: Dispatching cancelled for %d", m.cookie())
}

var (
	// moveCaches keeps track of active moves by cookie
	moveCaches   = make(map[uint32]*moveCache)
	moveCachesMu = &gosync.RWMutex{}
)

// move processes InMovedFrom and InMovedTo events in any order
// and dispatches FileUpdates when a move is detected
func (w *FileWatcher) move(event notify.EventInfo) (moveUpdate *fileevents.FileEvent) {
	cookie := ievent(event).Cookie
	moveCachesMu.RLock()
	cache, ok := moveCaches[cookie]
	moveCachesMu.RUnlock()
	if !ok {
		// The cookie is not cached, create a new cache object for it
		moveCachesMu.Lock()
		moveCaches[cookie] = w.newMoveCache(event)
		moveCachesMu.Unlock()
		return
	}

	sourcePath, destPath := cache.event.Path(), event.Path()
	switch event.Event() {
	case notify.InMovedFrom:
		sourcePath, destPath = destPath, sourcePath
		fallthrough
	case notify.InMovedTo:
		cache.cancel()                                                                                          // Cancel dispatching the cache's incomplete move
		moveUpdate = &fileevents.FileEvent{Path: destPath, OldPath: sourcePath, Type: fileevents.FileEventMove} // Register an internal, complete move instead
		log.Tracef("FileWatcher: Detected move: %q -> %q", sourcePath, destPath)
	}

	return
}

// concatenateEvents takes in a slice of events and concatenates
// all events possible based on combinedEvents. It also manages
// file moving and conversion from notifyEvents to FileEvents
func (w *FileWatcher) concatenateEvents(events notifyEvents) FileEvents {
	for _, combinedEvent := range combinedEvents {
		// Test if the prefix of the given events matches combinedEvent.input
		if event, ok := combinedEvent.match(events); ok {
			// If so, replace combinedEvent.input prefix in events with combinedEvent.output and recurse
			concatenated := events[len(combinedEvent.input):]
			if event != nil { // Prepend the concatenation result event if any
				concatenated = append(notifyEvents{event}, concatenated...)
			}

			log.Tracef("FileWatcher: Concatenated events: %v -> %v", events, concatenated)
			return w.concatenateEvents(concatenated)
		}
	}

	// Convert the events to updates
	updates := make(FileEvents, 0, len(events))
	for _, event := range events {
		switch event.Event() {
		case notify.InMovedFrom, notify.InMovedTo:
			// Send move-related events to w.move
			if update := w.move(event); update != nil {
				// Add the update to the list if we get something back
				updates = append(updates, update)
			}
		default:
			updates = append(updates, convertUpdate(event))
		}
	}

	return updates
}

func ievent(event notify.EventInfo) *unix.InotifyEvent {
	return event.Sys().(*unix.InotifyEvent)
}

// combinedEvent describes multiple events that should be concatenated into a single event
type combinedEvent struct {
	input  []notify.Event // input is a slice of events to match (in bytes, it speeds up the comparison)
	output int            // output is the event's index that should be returned, negative values equal nil
}

func (c *combinedEvent) match(events notifyEvents) (notify.EventInfo, bool) {
	if len(c.input) > len(events) {
		return nil, false // Not enough events, cannot match
	}

	for i := 0; i < len(c.input); i++ {
		if events[i].Event() != c.input[i] {
			return nil, false
		}
	}

	if c.output >= 0 {
		return events[c.output], true
	}

	return nil, true
}
