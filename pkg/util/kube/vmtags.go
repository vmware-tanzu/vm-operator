// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

const (
	// TagSourceLabelIndexKey indexes every label-derived Tag by its source pair.
	TagSourceLabelIndexKey = "spec.sourceLabel"

	// TagOwnerReferencesUIDIndexKey is the field-index key used to
	// efficiently list every Tag owned by a given VirtualMachine UID.
	TagOwnerReferencesUIDIndexKey = "metadata.ownerReferences.uid"

	// VMLabelKeyValueIndexKey is the field-index key used to efficiently list
	// every VirtualMachine carrying a given label key/value pair.
	VMLabelKeyValueIndexKey = "metadata.labels.keyValue"
)

// TagSourceLabelIndexerFunc indexes label-derived Tags by source key/value.
// Tags without a source label do not participate in label-based lookup.
func TagSourceLabelIndexerFunc(obj ctrlclient.Object) []string {
	tag, ok := obj.(*vspherepolv1.Tag)
	if !ok || tag.Spec.Membership != vspherepolv1.TagMembershipSourceLabel || tag.Spec.Key == "" {
		return nil
	}
	return []string{TagLabelKeyValue(tag.Spec.Key, tag.Spec.Value)}
}

// TagLabelKeyValue joins a label key and value into the string indexed by
// VMLabelKeyValueIndexKey.
func TagLabelKeyValue(key, value string) string {
	return key + ":" + value
}

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

// VMLabelKeyValueIndexerFunc extracts the value indexed by
// VMLabelKeyValueIndexKey from a VirtualMachine object: one
// "<key>:<value>" entry per label, after RemoveVMOperatorLabels. It does not
// mutate the cached object, since RemoveVMOperatorLabels returns a fresh map.
func VMLabelKeyValueIndexerFunc(obj ctrlclient.Object) []string {
	vm, ok := obj.(*vmopv1.VirtualMachine)
	if !ok {
		return nil
	}

	labels := RemoveVMOperatorLabels(vm.GetLabels())
	if len(labels) == 0 {
		return nil
	}

	values := make([]string, 0, len(labels))
	for k, v := range labels {
		values = append(values, TagLabelKeyValue(k, v))
	}

	return values
}

// RegisterVMTagsIndexes registers Tag source-pair, owner-UID, and VM label indexes.
// The VM controller calls it once per manager under Features.TaggingAPI:
// ownership cleanup uses TagOwnerReferencesUIDIndexKey, and the Tag watch's
// mapper uses VMLabelKeyValueIndexKey. Managers running only the VM controller
// therefore also have every index required by its reconciliation path.
// Registering the same object/field pair twice returns an error, so another
// controller must not register these indexes in the same manager.
func RegisterVMTagsIndexes(ctx context.Context, fieldIndexer ctrlclient.FieldIndexer) error {
	if err := fieldIndexer.IndexField(ctx, &vspherepolv1.Tag{},
		TagSourceLabelIndexKey, TagSourceLabelIndexerFunc); err != nil {
		return err
	}

	if err := fieldIndexer.IndexField(
		ctx,
		&vspherepolv1.Tag{},
		TagOwnerReferencesUIDIndexKey,
		TagOwnerReferencesUIDIndexerFunc); err != nil {
		return err
	}

	return fieldIndexer.IndexField(
		ctx,
		&vmopv1.VirtualMachine{},
		VMLabelKeyValueIndexKey,
		VMLabelKeyValueIndexerFunc)
}

// TagToVirtualMachineMapper returns a mapper function used to enqueue
// reconcile requests for VMs in response to an event on a Tag.
//
// Self tags map directly to valid VM owner references without peer reads.
//
// For source-label Tags, only the first owner being added (Tag created) and
// the last owner being removed (Tag deleted) change which VMs carry the tag.
// Owner membership changes in between do not, so TagFanOutPredicate filters
// those events out. Both the VMs carrying the label and the Tag's owners are
// enqueued: an owner references the label in an affinity term but may not
// carry it, and must recreate or re-adopt a deleted Tag. A deleted Tag still
// carries its owner references, so every owner is re-enqueued, which also
// costs the creating VM one extra reconcile on Tag create.
func TagToVirtualMachineMapper(
	ctx context.Context,
	k8sClient ctrlclient.Client) handler.MapFunc {

	if ctx == nil {
		panic("context is nil")
	}

	if k8sClient == nil {
		panic("k8sClient is nil")
	}

	return func(ctx context.Context, o ctrlclient.Object) []reconcile.Request {
		if ctx == nil {
			panic("context is nil")
		}

		if o == nil {
			panic("object is nil")
		}
		tag, ok := o.(*vspherepolv1.Tag)
		if !ok {
			panic(fmt.Sprintf("object is %T", o))
		}

		ownerRequests := tagOwnerRequests(tag)
		if tag.Spec.Membership == vspherepolv1.TagMembershipPolicyRequesters {
			return ownerRequests
		}
		if tag.Spec.Membership != vspherepolv1.TagMembershipSourceLabel || tag.Spec.Key == "" {
			return nil
		}

		logger := pkglog.FromContextOrDefault(ctx).
			WithValues("name", tag.GetName(), "namespace", tag.GetNamespace())
		logger.V(4).Info("Reconciling all VMs carrying a Tag's label")

		vmList := &vmopv1.VirtualMachineList{}
		if err := k8sClient.List(
			ctx,
			vmList,
			ctrlclient.InNamespace(tag.Namespace),
			ctrlclient.MatchingFields{
				VMLabelKeyValueIndexKey: TagLabelKeyValue(tag.Spec.Key, tag.Spec.Value),
			}); err != nil {

			logger.Error(err,
				"Failed to list VirtualMachines for reconciliation due to Tag watch")
			return ownerRequests
		}

		// Start with the owners, which may not carry the label, then add the
		// label carriers that are not already enqueued.
		requests := make([]reconcile.Request, 0, len(ownerRequests)+len(vmList.Items))
		requests = append(requests, ownerRequests...)
		seen := make(map[string]bool, len(ownerRequests))
		for _, r := range ownerRequests {
			seen[r.Name] = true
		}
		for i := range vmList.Items {
			vm := vmList.Items[i]
			if seen[vm.Name] {
				continue
			}
			requests = append(requests, reconcile.Request{
				NamespacedName: ctrlclient.ObjectKey{
					Namespace: vm.Namespace,
					Name:      vm.Name,
				},
			})
		}

		if len(requests) > 0 {
			logger.V(4).Info("Reconciling VMs due to Tag watch", "requests", requests)
		}

		return requests
	}
}

// tagOwnerRequests returns a reconcile request for each distinct valid
// VirtualMachine owner reference of the Tag.
func tagOwnerRequests(tag *vspherepolv1.Tag) []reconcile.Request {
	var requests []reconcile.Request
	seen := make(map[string]bool)
	for _, ref := range tag.OwnerReferences {
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil || gv.Version == "" || gv.Group != vmopv1.GroupVersion.Group || ref.Kind != "VirtualMachine" ||
			ref.Name == "" || ref.UID == "" || seen[ref.Name] {
			continue
		}
		seen[ref.Name] = true
		requests = append(requests, reconcile.Request{NamespacedName: ctrlclient.ObjectKey{
			Namespace: tag.Namespace, Name: ref.Name,
		}})
	}
	return requests
}

// TagFanOutPredicate returns a predicate that admits only the Tag events
// that can change a VM's desired tag set: create and delete as they
// may result in tag spec reconfigure tasks on the VMs.
//
// Create events for self Tags (TagMembershipPolicyRequesters) are dropped.
// A self Tag is only created by its first owner during that VM's own
// reconcile, and no other VM's desired tag set depends on it, so the event
// would only cause a wasted reconcile of the creator. Self Tag deletes are
// still admitted so that owners recreate a Tag that was removed.
func TagFanOutPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if tag, ok := e.Object.(*vspherepolv1.Tag); ok &&
				tag.Spec.Membership ==
					vspherepolv1.TagMembershipPolicyRequesters {

				return false
			}
			return true
		},
		UpdateFunc: func(event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}
