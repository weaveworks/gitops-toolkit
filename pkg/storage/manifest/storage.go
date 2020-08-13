package manifest

import (
	"github.com/weaveworks/libgitops/pkg/runtime"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage"
	"github.com/weaveworks/libgitops/pkg/storage/sync"
	"github.com/weaveworks/libgitops/pkg/storage/watch"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NewManifestStorage constructs a new storage that watches unstructured manifests in the specified directory,
// decodable using the given serializer.
func NewManifestStorage(manifestDir string, ser serializer.Serializer) (*ManifestStorage, error) {
	ws, err := watch.NewGenericWatchStorage(
		storage.NewGenericStorage(
			storage.NewGenericMappedRawStorage(manifestDir),
			ser,
			[]runtime.IdentifierFactory{runtime.Metav1NameIdentifier},
		),
	)
	if err != nil {
		return nil, err
	}

	ss := sync.NewSyncStorage(ws)

	return &ManifestStorage{
		Storage: ss,
	}, nil
}

// NewManifestStorage constructs a new storage that watches unstructured manifests in the specified directory,
// decodable using the given serializer. However, all changes in the manifest directory, are also propagated to
// the structured data directory that's backed by the default storage implementation. Writes to this storage are
// propagated to both the manifest directory, and the data directory. The GroupVersion used for the GenericRawStorage
// must be specified. The IdentifierFactories also need to be specified.
// TODO: Maybe move this to Ignite?
func NewTwoWayManifestStorage(manifestDir, dataDir string, ser serializer.Serializer, gv schema.GroupVersion, identifiers []runtime.IdentifierFactory) (*ManifestStorage, error) {
	ws, err := watch.NewGenericWatchStorage(storage.NewGenericStorage(storage.NewGenericMappedRawStorage(manifestDir), ser, identifiers))
	if err != nil {
		return nil, err
	}

	ss := sync.NewSyncStorage(
		storage.NewGenericStorage(
			storage.NewGenericRawStorage(dataDir, gv, serializer.ContentTypeJSON), ser, identifiers),
		ws)

	return &ManifestStorage{
		Storage: ss,
	}, nil
}

// ManifestStorage implements the storage interface for GitOps purposes
type ManifestStorage struct {
	storage.Storage
}

// GetUpdateStream gets the channel with updates
func (s *ManifestStorage) GetUpdateStream() sync.UpdateStream {
	return s.Storage.(*sync.SyncStorage).GetUpdateStream()
}
