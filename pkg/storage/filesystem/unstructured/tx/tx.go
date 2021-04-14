package unstructuredtx

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/client/transactional"
	"github.com/weaveworks/libgitops/pkg/storage/core"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem"
	"github.com/weaveworks/libgitops/pkg/storage/filesystem/unstructured"
)

// NewUnstructuredStorageTxHandler returns a TransactionHook that before the transaction starts
// informs the unstructured.FileFinder (if used) that the new head branch should be created (if
// not already exists) using the base branch as the cow baseline.
func NewUnstructuredStorageTxHandler(c client.Client) transactional.TransactionHook {
	fsStorage, ok := c.BackendReader().Storage().(filesystem.Storage)
	if !ok {
		return nil
	}
	fileFinder, ok := fsStorage.FileFinder().(unstructured.FileFinder)
	if !ok {
		return nil
	}
	return &unstructuredStorageTxHandler{fileFinder}
}

type unstructuredStorageTxHandler struct {
	fileFinder unstructured.FileFinder
}

func (h *unstructuredStorageTxHandler) PreTransactionHook(ctx context.Context, info transactional.TxInfo) error {
	head := core.NewBranchRef(info.Head)
	if h.fileFinder.HasVersionRef(head) {
		return nil // head exists, no-op
	}
	base := core.NewBranchRef(info.Base)
	// If both head and base are the same, and we know that head does not exist in the system, we need to create
	// head "from scratch" as a "root version"
	if info.Head == info.Base {
		base = nil
	}
	return h.fileFinder.RegisterVersionRef(head, base)
}

func (h *unstructuredStorageTxHandler) PostTransactionHook(ctx context.Context, info transactional.TxInfo) error {
	return nil // cleanup?
}
