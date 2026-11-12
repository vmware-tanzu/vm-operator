// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmtags

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// ReconcileTagCRs computes all current concrete identities before ensuring
// ownership and pruning stale references. It returns the owned Tags to cover
// same-reconcile cache lag. Ownership follows declarations, not carried labels.
func ReconcileTagCRs(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine) ([]vspherepolv1.Tag, error) {
	if ctx == nil {
		panic("context is nil")
	}

	if k8sClient == nil {
		panic("k8sClient is nil")
	}

	if vm == nil {
		panic("vm is nil")
	}

	desired, err := virtualmachine.DesiredAffinityTags(ctx, vm)
	if err != nil {
		return nil, err
	}
	ownedTags := make([]vspherepolv1.Tag, 0, len(desired))
	ownedNames := sets.New[string]()

	for _, identity := range desired {
		tag, err := ensureOwnedTag(ctx, k8sClient, vm, identity)
		if err != nil {
			return nil, err
		}

		ownedTags = append(ownedTags, *tag)
		ownedNames.Insert(tag.Name)
	}

	if err := pruneStaleOwnership(ctx, k8sClient, vm, ownedNames); err != nil {
		return nil, err
	}

	return ownedTags, nil
}

// ensureOwnedTag ensures a Tag resource exists for the given concrete identity
// by creating it with this VM as its first owner if absent, or adding
// this VM's owner reference if the Tag already exists but does not yet
// list it. It fails rather than adopting a Tag whose spec does not match
// the requested concrete identity.
func ensureOwnedTag(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	desired vspherepolv1.Tag) (*vspherepolv1.Tag, error) {
	name := desired.Name

	tag := &vspherepolv1.Tag{}
	err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: name},
		tag)

	switch {
	case apierrors.IsNotFound(err):
		tag = desired.DeepCopy()
		if tag.Spec.Membership == vspherepolv1.TagMembershipSourceLabel {
			tag.Labels = map[string]string{tag.Spec.Key: tag.Spec.Value}
		}

		err := controllerutil.SetOwnerReference(vm, tag, k8sClient.Scheme())
		if err != nil {
			return nil, fmt.Errorf(
				"failed to set owner reference on Tag %q: %w", name, err)
		}

		err = k8sClient.Create(ctx, tag)
		switch {
		case apierrors.IsAlreadyExists(err):
			// Another VM in this namespace created the Tag for the same
			// label pair between our Get and our Create (concurrent
			// reconciles, or a cache-sync delay on the Get above). Re-Get
			// and fall through to the adopt path below instead of treating
			// this as a hard error.
			tag = &vspherepolv1.Tag{}
			if err := k8sClient.Get(
				ctx,
				ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: name},
				tag); err != nil {
				return nil, fmt.Errorf("failed to get Tag %q: %w", name, err)
			}
		case err != nil:
			return nil, fmt.Errorf("failed to create Tag %q: %w", name, err)
		default:
			return tag, nil
		}

	case err != nil:
		return nil, fmt.Errorf("failed to get Tag %q: %w", name, err)
	}

	actualSpec, desiredSpec := tag.Spec, desired.Spec
	actualSpec.Type, desiredSpec.Type = vspherepolv1.TagTypeSystem, vspherepolv1.TagTypeSystem
	if !apiequality.Semantic.DeepEqual(actualSpec, desiredSpec) {
		return nil, fmt.Errorf("tag %q has conflicting identity for %q", name, desired.Spec.Name)
	}

	// SetOwnerReference upserts the ref keyed by apiVersion/kind/name with
	// the current UID, so calling it unconditionally (rather than gating on
	// controllerutil.HasOwnerReference, which ignores UID) also corrects a
	// stale ref left by a same-named VM that was deleted and recreated (VKS
	// node churn, GitOps re-apply). patchOwnerReferences below is a no-op
	// when nothing actually changed.
	base := tag.DeepCopy()

	err = controllerutil.SetOwnerReference(vm, tag, k8sClient.Scheme())
	if err != nil {
		return nil, fmt.Errorf(
			"failed to set owner reference on Tag %q: %w", name, err)
	}

	err = patchOwnerReferences(ctx, k8sClient, base, tag)
	if err != nil {
		return nil, err
	}

	return tag, nil
}

// pruneStaleOwnership removes this VM's owner reference from every Tag it
// currently owns whose name is not in keepNames. keepNames is empty when
// VM no longer defines affinity.
// A deleted VM's ownership is deliberately not released here: the garbage
// collector prunes its owner references, or deletes a Tag it solely owned,
// once the VM object is removed. That happens only after vCenter has
// deleted the VM, so the VM's policies never outlive the Tags they target.
func pruneStaleOwnership(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	keepNames sets.Set[string]) error {
	tagList := &vspherepolv1.TagList{}

	err := k8sClient.List(
		ctx,
		tagList,
		ctrlclient.InNamespace(vm.Namespace),
		ctrlclient.MatchingFields{
			kubeutil.TagOwnerReferencesUIDIndexKey: string(vm.GetUID()),
		})
	if err != nil {
		return fmt.Errorf("failed to list Tags owned by VM %q: %w", vm.Name, err)
	}

	for i := range tagList.Items {
		tag := &tagList.Items[i]
		if keepNames.Has(tag.Name) {
			continue
		}

		// The List above is served by the informer cache, so a Tag whose ref
		// was already removed by a prior reconcile (not yet observed by the
		// cache) or already pruned by the GC can still show up here.
		// controllerutil.RemoveOwnerReference errors when the ref is absent,
		// which would otherwise turn this converged no-op into a hard error.
		// Skip it.
		hasOwner, err := controllerutil.HasOwnerReference(tag.OwnerReferences, vm, k8sClient.Scheme())
		if err != nil {
			return fmt.Errorf(
				"failed to check owner reference on Tag %q: %w", tag.Name, err)
		}
		if !hasOwner {
			continue
		}

		base := tag.DeepCopy()

		err = controllerutil.RemoveOwnerReference(vm, tag, k8sClient.Scheme())
		if err != nil {
			return fmt.Errorf(
				"failed to remove owner reference from Tag %q: %w", tag.Name, err)
		}

		err = patchOwnerReferences(ctx, k8sClient, base, tag)
		if apierrors.IsNotFound(err) {
			// The Tag is already gone, for example deleted by the Tag
			// controller once it had no owners, while a stale cache still
			// listed it. There is no ownership left to release.
			continue
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// patchOwnerReferences patches base's owner references to match obj's,
// using an optimistic-lock merge patch so a concurrent writer's entry is
// never silently dropped, and skipping the write entirely when the owner
// references are unchanged so a no-op reconcile does not bump the Tag's
// resourceVersion.
func patchOwnerReferences(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	base, obj *vspherepolv1.Tag) error {
	if apiequality.Semantic.DeepEqual(base.OwnerReferences, obj.OwnerReferences) {
		return nil
	}

	err := k8sClient.Patch(
		ctx,
		obj,
		ctrlclient.MergeFromWithOptions(base, ctrlclient.MergeFromWithOptimisticLock{}))
	if err != nil {
		return fmt.Errorf(
			"failed to patch Tag %q owner references: %w", obj.Name, err)
	}

	return nil
}
