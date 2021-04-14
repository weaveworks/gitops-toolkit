package serializer

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/errors"
)

func NewDefaulter(schemeLock LockedScheme) Defaulter {
	// We do not write to the scheme in the defaulter at this time.
	// If we start doing that, we must also make use of the locker
	return &defaulter{schemeLock}
}

type defaulter struct {
	LockedScheme
}

func (d *defaulter) GetLockedScheme() LockedScheme {
	return d.LockedScheme
}

// NewDefaultedObject returns a new, defaulted object. It is essentially scheme.New() and
// scheme.Default(obj), but with extra logic to also cover internal versions.
// Important to note here is that the TypeMeta information is NOT applied automatically.
func (d *defaulter) NewDefaultedObject(gvk schema.GroupVersionKind) (runtime.Object, error) {
	obj, err := d.Scheme().New(gvk)
	if err != nil {
		return nil, err
	}

	// Default the new object, this will take care of internal defaulting automatically
	if err := d.Default(obj); err != nil {
		return nil, err
	}

	return obj, nil
}

func (d *defaulter) Default(objs ...runtime.Object) error {
	errs := []error{}
	for _, obj := range objs {
		errs = append(errs, d.runDefaulting(obj))
	}
	return errors.NewAggregate(errs)
}

func (d *defaulter) runDefaulting(obj runtime.Object) error {
	// First, get the groupversionkind of the object
	gvk, err := GVKForObject(d.Scheme(), obj)
	if err != nil {
		return err
	}

	// If the version is external, just default it and return.
	if gvk.Version != runtime.APIVersionInternal {
		d.Scheme().Default(obj)
		return nil
	}

	// We know that the current object is internal
	// Get the preferred external version...
	gv, err := PreferredVersionForGroup(d.Scheme(), gvk.Group)
	if err != nil {
		return err
	}

	// ...and make a new object of it
	external, err := d.Scheme().New(gv.WithKind(gvk.Kind))
	if err != nil {
		return err
	}
	// Convert the internal object to the external
	if err := d.Scheme().Convert(obj, external, nil); err != nil {
		return err
	}
	// Default the external
	d.Scheme().Default(external)
	// And convert back to internal
	return d.Scheme().Convert(external, obj, nil)

}
