package unstructured

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
)

// NewGeneric creates a new generic unstructured.Storage for the given underlying
// interfaces. storage and recognizer are mandatory, pathExcluder and framingFactory
// are optional (can be nil). framingFactory defaults to serializer.NewFrameReaderFactory().
func NewGeneric(
	storage filesystem.Storage,
	recognizer ObjectRecognizer,
	pathExcluder filesystem.PathExcluder,
	framingFactory serializer.FrameReaderFactory,
) (Storage, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage is mandatory")
	}
	if recognizer == nil {
		return nil, fmt.Errorf("recognizer is mandatory")
	}
	// optional: use YAML/JSON by default.
	if framingFactory == nil {
		framingFactory = serializer.NewFrameReaderFactory()
	}
	fileFinder, ok := storage.FileFinder().(FileFinder)
	if !ok {
		return nil, errors.New("the given filesystem.Storage must use a unstructured.FileFinder")
	}
	return &Generic{
		Storage:        storage,
		recognizer:     recognizer,
		fileFinder:     fileFinder,
		pathExcluder:   pathExcluder,
		framingFactory: framingFactory,
	}, nil
}

type Generic struct {
	filesystem.Storage
	recognizer     ObjectRecognizer
	fileFinder     FileFinder
	pathExcluder   filesystem.PathExcluder
	framingFactory serializer.FrameReaderFactory
}

// Sync synchronizes the current state of the filesystem, and overwrites all
// previously cached mappings in the unstructured.FileFinder. "successful"
// mappings returned are those that are observed to be distinct. "duplicates"
// contains such IDs that weren't distinct; but existed in multiple files.
func (s *Generic) Sync(ctx context.Context) (successful, duplicates core.UnversionedObjectIDSet, err error) {
	fileFinder := s.UnstructuredFileFinder()
	fs := fileFinder.Filesystem()
	contentTyper := fileFinder.ContentTyper()

	// Create pre-made empty sets for the "successful" and "duplicate" IDs
	// This ensures that although errors occur, we don't return a nil set
	successful = core.NewUnversionedObjectIDSet()
	duplicates = core.NewUnversionedObjectIDSet()

	ref := core.GetVersionRef(ctx)
	if !fileFinder.HasVersionRef(ref) {
		if err = fileFinder.RegisterVersionRef(ref, nil); err != nil {
			return
		}
	}

	// List all valid files in the fs
	var files []string
	files, err = filesystem.ListValidFilesInFilesystem(
		ctx,
		fs,
		contentTyper,
		s.PathExcluder(),
	)
	if err != nil {
		return
	}

	// Walk all files and fill the mappings of the unstructured.FileFinder.
	allMappings := make(map[ChecksumPath]core.UnversionedObjectIDSet)
	objectCount := 0

	for _, filePath := range files {
		// Recognize the IDs in all the given file
		idSet, cp, _, err := RecognizeIDsInFile(
			ctx,
			fileFinder,
			s.ObjectRecognizer(),
			s.FrameReaderFactory(),
			filePath,
		)
		if err != nil {
			logrus.Error(err)
			continue
		}
		objectCount += idSet.Len()
		allMappings[*cp] = idSet
	}

	// ResetMappings overwrites all data at once; so these
	// mappings are now the "truth" about what's on disk
	// Duplicate mappings are returned from ResetMappings
	duplicates = fileFinder.ResetMappings(ctx, allMappings)
	// For each set of IDs; add them to the "successful" batch
	for _, set := range allMappings {
		successful.InsertSet(set)
	}
	// Remove the duplicates from the successful bucket
	successful.DeleteSet(duplicates)
	return
}

// ObjectRecognizer returns the underlying ObjectRecognizer used.
func (s *Generic) ObjectRecognizer() ObjectRecognizer {
	return s.recognizer
}

// FrameReaderFactory returns the underlying FrameReaderFactory used.
func (s *Generic) FrameReaderFactory() serializer.FrameReaderFactory {
	return s.framingFactory
}

// PathExcluder specifies what paths to not sync
func (s *Generic) PathExcluder() filesystem.PathExcluder {
	return s.pathExcluder
}

// UnstructuredFileFinder returns the underlying unstructured.FileFinder used.
func (s *Generic) UnstructuredFileFinder() FileFinder {
	return s.fileFinder
}

// RecognizeIDsInFile reads the given file and its content type; and then recognizes it.
// However, if the checksum is already up-to-date, the function returns directly, without
// reading the file. In that case, the bool is true (in all other cases, false). The
// ObjectIDSet and ChecksumPath are returned when err == nil.
func RecognizeIDsInFile(
	ctx context.Context,
	fileFinder FileFinder,
	recognizer ObjectRecognizer,
	framingFactory serializer.FrameReaderFactory,
	filePath string,
) (core.UnversionedObjectIDSet, *ChecksumPath, bool, error) {
	fs := fileFinder.Filesystem()
	contentTyper := fileFinder.ContentTyper()

	// Get the current checksum of the file
	currentChecksum, err := fs.Checksum(ctx, filePath)
	if err != nil {
		return nil, nil, false, fmt.Errorf("Could not get checksum for file %q: %v", filePath, err)
	}
	cp := &ChecksumPath{Path: filePath, Checksum: currentChecksum}

	// Check the cached checksum
	cachedChecksum, ok := fileFinder.ChecksumForPath(ctx, filePath)
	if ok && cachedChecksum == currentChecksum {
		// If the cache is up-to-date, we don't need to do anything
		logrus.Tracef("Checksum for file %q is up-to-date: %q, skipping...", filePath, currentChecksum)
		// Just get the IDs that are cached, and done.
		idSet, err := fileFinder.ObjectsAt(ctx, filePath)
		if err != nil {
			return nil, nil, false, err
		}
		return idSet, cp, true, nil
	}

	// If the file is not known to the FileFinder yet, or if the checksum
	// was empty, read the file, and recognize it.
	content, err := fs.ReadFile(ctx, filePath)
	if err != nil {
		return nil, nil, false, fmt.Errorf("Could not read file %q: %v", filePath, err)
	}
	// Get the content type for this file so that we can read it properly
	ct, err := contentTyper.ContentTypeForPath(ctx, fs, filePath)
	if err != nil {
		return nil, nil, false, fmt.Errorf("Could not get content type for file %q: %v", filePath, err)
	}
	// Create a new FrameReader for the given ContentType and ReadCloser
	fr := framingFactory.NewFrameReader(ct, serializer.FromBytes(content))
	// Recognize all IDs in the file
	versionedIDs, err := recognizer.RecognizeObjectIDs(filePath, fr)
	if err != nil {
		return nil, nil, false, fmt.Errorf("Could not recognize object IDs in %q: %v", filePath, err)
	}
	// Convert to an unversioned set
	return core.UnversionedObjectIDSetFromVersionedSlice(versionedIDs), cp, false, nil
}
