package patch

import (
	"encoding/json"
	"fmt"

	jsonbytepatcher "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// BytePatcherForType returns the right BytePatcher for the given
// patch type.
//
// Note: if patchType is unknown, the return value will be nil, so make
// sure you check the BytePatcher is non-nil before using it!
func BytePatcherForType(patchType types.PatchType) BytePatcher {
	switch patchType {
	case types.JSONPatchType:
		return JSONBytePatcher{}
	case types.MergePatchType:
		return MergeBytePatcher{}
	case types.StrategicMergePatchType:
		return StrategicMergeBytePatcher{}
	default:
		return nil
	}
}

// maximum number of operations a single json patch may contain.
const maxJSONBytePatcherOperations = 10000

type BytePatcher interface {
	// TODO: SupportedType() types.PatchType
	// currentData must be versioned bytes of the same GVK as into and patch.Data() (if merge patch)
	// into must be an empty object
	Apply(currentJSON, patchJSON []byte, schema strategicpatch.LookupPatchMeta) ([]byte, error)
}

type JSONBytePatcher struct{}

func (JSONBytePatcher) Apply(currentJSON, patchJSON []byte, _ strategicpatch.LookupPatchMeta) ([]byte, error) {
	// sanity check potentially abusive patches
	// TODO(liggitt): drop this once golang json parser limits stack depth (https://github.com/golang/go/issues/31789)
	// TODO(luxas): Go v1.15 has the above mentioned patch, what needs changing now?
	if len(patchJSON) > 1024*1024 {
		v := []interface{}{}
		if err := json.Unmarshal(patchJSON, &v); err != nil {
			return nil, fmt.Errorf("error decoding patch: %v", err)
		}
	}

	patchObj, err := jsonbytepatcher.DecodePatch(patchJSON)
	if err != nil {
		return nil, err
	}
	if len(patchObj) > maxJSONBytePatcherOperations {
		return nil, errors.NewRequestEntityTooLargeError(
			fmt.Sprintf("The allowed maximum operations in a JSON patch is %d, got %d",
				maxJSONBytePatcherOperations, len(patchObj)))
	}
	return patchObj.Apply(currentJSON)
}

type MergeBytePatcher struct{}

func (MergeBytePatcher) Apply(currentJSON, patchJSON []byte, _ strategicpatch.LookupPatchMeta) ([]byte, error) {
	// sanity check potentially abusive patches
	// TODO(liggitt): drop this once golang json parser limits stack depth (https://github.com/golang/go/issues/31789)
	// TODO(luxas): Go v1.15 has the above mentioned patch, what needs changing now?
	if len(patchJSON) > 1024*1024 {
		v := map[string]interface{}{}
		if err := json.Unmarshal(patchJSON, &v); err != nil {
			return nil, errors.NewBadRequest(fmt.Sprintf("error decoding patch: %v", err))
		}
	}

	return jsonbytepatcher.MergePatch(currentJSON, patchJSON)
}

type StrategicMergeBytePatcher struct{}

func (StrategicMergeBytePatcher) Apply(currentJSON, patchJSON []byte, schema strategicpatch.LookupPatchMeta) ([]byte, error) {
	// TODO: Also check for overflow here?
	// TODO: What to do when schema is nil? error?
	return strategicpatch.StrategicMergePatchUsingLookupPatchMeta(currentJSON, patchJSON, schema)
}
