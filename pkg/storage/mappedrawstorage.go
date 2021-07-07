package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/serializer/frame"
	"github.com/weaveworks/libgitops/pkg/util"
)

var (
	// ErrNotTracked is returned when the requested resource wasn't found.
	ErrNotTracked = fmt.Errorf("untracked object: %w", ErrNotFound)
)

// MappedRawStorage is an interface for RawStorages which store their
// data in a flat/unordered directory format like manifest directories.
type MappedRawStorage interface {
	RawStorage

	// AddMapping binds a Key's virtual path to a physical file path
	AddMapping(key ObjectKey, path string)
	// RemoveMapping removes the physical file
	// path mapping matching the given Key
	RemoveMapping(key ObjectKey)

	// SetMappings overwrites all known mappings
	SetMappings(m map[ObjectKey]string)
}

func NewGenericMappedRawStorage(dir string) MappedRawStorage {
	return &GenericMappedRawStorage{
		dir:          dir,
		fileMappings: make(map[ObjectKey]string),
		mux:          &sync.Mutex{},
	}
}

// GenericMappedRawStorage is the default implementation of a MappedRawStorage,
// it stores files in the given directory via a path translation map.
type GenericMappedRawStorage struct {
	dir          string
	fileMappings map[ObjectKey]string
	mux          *sync.Mutex
}

func (r *GenericMappedRawStorage) realPath(key ObjectKey) (string, error) {
	r.mux.Lock()
	path, ok := r.fileMappings[key]
	r.mux.Unlock()
	if !ok {
		return "", fmt.Errorf("GenericMappedRawStorage: cannot resolve %q: %w", key, ErrNotTracked)
	}

	return path, nil
}

// If the file doesn't exist, returns ErrNotFound + ErrNotTracked.
func (r *GenericMappedRawStorage) Read(key ObjectKey) ([]byte, error) {
	file, err := r.realPath(key)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadFile(file)
}

func (r *GenericMappedRawStorage) Exists(key ObjectKey) bool {
	file, err := r.realPath(key)
	if err != nil {
		return false
	}

	return util.FileExists(file)
}

func (r *GenericMappedRawStorage) Write(key ObjectKey, content []byte) error {
	// GenericMappedRawStorage isn't going to generate files itself,
	// only write if the file is already known
	file, err := r.realPath(key)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(file, content, 0644)
}

// If the file doesn't exist, returns ErrNotFound + ErrNotTracked.
func (r *GenericMappedRawStorage) Delete(key ObjectKey) (err error) {
	file, err := r.realPath(key)
	if err != nil {
		return
	}

	// GenericMappedRawStorage files can be deleted
	// externally, check that the file exists first
	if util.FileExists(file) {
		err = os.Remove(file)
	}

	if err == nil {
		r.RemoveMapping(key)
	}

	return
}

func (r *GenericMappedRawStorage) List(kind KindKey) ([]ObjectKey, error) {
	result := make([]ObjectKey, 0)

	for key := range r.fileMappings {
		// Include objects with the same kind and group, ignore version mismatches
		if key.EqualsGVK(kind, false) {
			result = append(result, key)
		}
	}

	return result, nil
}

// This returns the modification time as a UnixNano string.
// If the file doesn't exist, returns ErrNotFound + ErrNotTracked.
func (r *GenericMappedRawStorage) Checksum(key ObjectKey) (string, error) {
	path, err := r.realPath(key)
	if err != nil {
		return "", err
	}

	return checksumFromModTime(path)
}

func (r *GenericMappedRawStorage) ContentType(key ObjectKey) (ct frame.ContentType) {
	if file, err := r.realPath(key); err == nil {
		ct = ContentTypes[filepath.Ext(file)] // Retrieve the correct format based on the extension
	}

	return
}

func (r *GenericMappedRawStorage) WatchDir() string {
	return r.dir
}

func (r *GenericMappedRawStorage) GetKey(path string) (ObjectKey, error) {
	for key, p := range r.fileMappings {
		if p == path {
			return key, nil
		}
	}

	return objectKey{}, fmt.Errorf("no mapping found for path %q", path)
}

func (r *GenericMappedRawStorage) AddMapping(key ObjectKey, path string) {
	log.Debugf("GenericMappedRawStorage: AddMapping: %q -> %q", key, path)
	r.mux.Lock()
	r.fileMappings[key] = path
	r.mux.Unlock()
}

func (r *GenericMappedRawStorage) RemoveMapping(key ObjectKey) {
	log.Debugf("GenericMappedRawStorage: RemoveMapping: %q", key)
	r.mux.Lock()
	delete(r.fileMappings, key)
	r.mux.Unlock()
}

func (r *GenericMappedRawStorage) SetMappings(m map[ObjectKey]string) {
	log.Debugf("GenericMappedRawStorage: SetMappings: %v", m)
	r.mux.Lock()
	r.fileMappings = m
	r.mux.Unlock()
}
