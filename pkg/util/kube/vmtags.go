// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
)

const (
	// TagOwnerReferencesUIDIndexKey is the field-index key used to
	// efficiently list every Tag owned by a given VirtualMachine UID.
	TagOwnerReferencesUIDIndexKey = "metadata.ownerReferences.uid"
)

// TagOwnerReferencesUIDIndexerFunc extracts the value indexed by
// TagOwnerReferencesUIDIndexKey from a Tag object: one entry per
// owner-reference UID.
func TagOwnerReferencesUIDIndexerFunc(obj ctrlclient.Object) []string {
	tag, ok := obj.(*vspherepolv1.Tag)
	if !ok {
		return nil
	}

	ownerRefs := tag.GetOwnerReferences()
	if len(ownerRefs) == 0 {
		return nil
	}

	uids := make([]string, 0, len(ownerRefs))
	for _, ref := range ownerRefs {
		uids = append(uids, string(ref.UID))
	}

	return uids
}

// RegisterVMTagsIndexes registers the field indexes used to query Tag and
// VirtualMachine objects on their label relationships:
// TagOwnerReferencesUIDIndexKey.
//
// It is called once per manager, from the VirtualMachine controller's
// AddToManager under the Features.TaggingAPI gate, because every consumer of
// these indexes is reached through that controller's reconcile path:
// pruneStaleOwnership/ReleaseOwnership (TagOwnerReferencesUIDIndexKey). The
// Tag controller queries none of them, so registration lives with the
// consumer rather than with the reconciler for the indexed type — that keeps
// a manager that wires only the VM controller, such as a per-controller
// envtest suite, from starting with an index it requires missing.
//
// Registering the same object/field pair twice returns an error, so this must
// not also be called from another AddToManager in the same manager.
func RegisterVMTagsIndexes(ctx context.Context, fieldIndexer ctrlclient.FieldIndexer) error {
	return fieldIndexer.IndexField(
		ctx,
		&vspherepolv1.Tag{},
		TagOwnerReferencesUIDIndexKey,
		TagOwnerReferencesUIDIndexerFunc)
}
