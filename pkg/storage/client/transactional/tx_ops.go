package transactional

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/storage/client"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// Implement the required "fluent/functional" methods on BranchTx.
// Go doesn't have generics; hence we need to do this twice.

func (tx *txImpl) Get(key core.ObjectKey, obj client.Object) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Get(ctx, key, obj)
	})
}
func (tx *txImpl) List(list client.ObjectList, opts ...client.ListOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.List(ctx, list, opts...)
	})
}

func (tx *txImpl) Create(obj client.Object, opts ...client.CreateOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Create(ctx, obj, opts...)
	})
}
func (tx *txImpl) Update(obj client.Object, opts ...client.UpdateOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Update(ctx, obj, opts...)
	})
}
func (tx *txImpl) Patch(obj client.Object, patch client.Patch, opts ...client.PatchOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Patch(ctx, obj, patch, opts...)
	})
}
func (tx *txImpl) Delete(obj client.Object, opts ...client.DeleteOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Delete(ctx, obj, opts...)
	})
}
func (tx *txImpl) DeleteAllOf(obj client.Object, opts ...client.DeleteAllOfOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.DeleteAllOf(ctx, obj, opts...)
	})
}

func (tx *txImpl) UpdateStatus(obj client.Object, opts ...client.UpdateOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Update(ctx, obj, opts...)
	})
}
func (tx *txImpl) PatchStatus(obj client.Object, patch client.Patch, opts ...client.PatchOption) Tx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Patch(ctx, obj, patch, opts...)
	})
}

// Implement the required "fluent/functional" methods on BranchTx.
// Go doesn't have generics; hence we need to do this twice.

func (tx *txBranchImpl) Get(key core.ObjectKey, obj client.Object) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Get(ctx, key, obj)
	})
}
func (tx *txBranchImpl) List(list client.ObjectList, opts ...client.ListOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.List(ctx, list, opts...)
	})
}

func (tx *txBranchImpl) Create(obj client.Object, opts ...client.CreateOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Create(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) Update(obj client.Object, opts ...client.UpdateOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Update(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) Patch(obj client.Object, patch client.Patch, opts ...client.PatchOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Patch(ctx, obj, patch, opts...)
	})
}
func (tx *txBranchImpl) Delete(obj client.Object, opts ...client.DeleteOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.Delete(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) DeleteAllOf(obj client.Object, opts ...client.DeleteAllOfOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return tx.c.DeleteAllOf(ctx, obj, opts...)
	})
}

func (tx *txBranchImpl) UpdateStatus(obj client.Object, opts ...client.UpdateOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Update(ctx, obj, opts...)
	})
}
func (tx *txBranchImpl) PatchStatus(obj client.Object, patch client.Patch, opts ...client.PatchOption) BranchTx {
	return tx.Custom(func(ctx context.Context) error {
		return nil // TODO tx.c.Status().Patch(ctx, obj, patch, opts...)
	})
}
