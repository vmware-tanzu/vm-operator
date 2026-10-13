// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmtags

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// ReconcileTagCRs computes the label key/value pairs this VM owns — every
// pair its own spec.affinity references, regardless of whether the VM
// currently carries the label — ensures a Tag resource exists for each
// owned pair, and adds or removes only this VM's owner reference on the
// Tags whose ownership changed.
//
// Ownership is driven by the affinity reference alone: a VM can own a Tag
// for a pair it does not carry, which is what lets it establish a
// VmToVmGroupsAntiAffinity relationship against other VMs that carry the
// pair without carrying it itself. Because spec.affinity is immutable
// after create, this set is fixed for
// the VM's lifetime once computed — ownership can no longer be lost by
// relabeling, only by the VM's own deletion (see ReleaseOwnership). Tag
// carriage — whether the VM itself gets the vCenter tag — is a separate
// predicate computed in ReconcileTagSpecs/desiredTagSet and still requires
// carrying the label.
//
// It returns the Tag objects it ensured for the VM's owned pairs.
// ReconcileTagSpecs needs them handed over directly rather than re-read,
// because the manager's cached client cannot yet observe a Create issued
// earlier in the same reconcile.
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

	vmCtx := pkgctx.VirtualMachineContext{
		Context: ctx,
		Logger:  pkglog.FromContextOrDefault(ctx),
		VM:      vm,
	}

	referencedPairs := virtualmachine.AffinityLabelPairs(
		vmCtx,
		virtualmachine.CalculateAffinityConstraints(vmCtx, false))

	ownedTags := make([]vspherepolv1.Tag, 0, len(referencedPairs))
	ownedNames := sets.New[string]()

	for _, pair := range referencedPairs {
		tag, err := ensureOwnedTag(ctx, k8sClient, vm, pair)
		if err != nil {
			return nil, err
		}

		ownedTags = append(ownedTags, *tag)
		ownedNames.Insert(tag.Name)
	}

	err := pruneStaleOwnership(ctx, k8sClient, vm, ownedNames)
	if err != nil {
		return nil, err
	}

	return ownedTags, nil
}

// ReleaseOwnership removes this VM's owner reference from every Tag it
// currently owns, using the same metadata.ownerReferences.uid-indexed branch
// ReconcileTagCRs uses to prune stale ownership. It is called by both
// vSphere provider VM-delete paths before either touches vCenter, so a VM
// that is deleted never leaves a dangling owner reference behind on a Tag.
func ReleaseOwnership(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine) error {

	return pruneStaleOwnership(ctx, k8sClient, vm, sets.New[string]())
}

// ensureOwnedTag ensures a Tag resource exists for the given owned label
// pair, creating it with this VM as its first owner if absent, or adding
// this VM's owner reference if the Tag already exists but does not yet
// list it. It fails rather than adopting a Tag whose spec does not match
// the pair its derived name implies.
func ensureOwnedTag(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	pair virtualmachine.LabelPair) (*vspherepolv1.Tag, error) {
	name := virtualmachine.TagResourceName(vm.Namespace, pair.Key, pair.Value)

	tag := &vspherepolv1.Tag{}
	err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: name},
		tag)

	switch {
	case apierrors.IsNotFound(err):
		tag = &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: vm.Namespace,
				Labels:    map[string]string{pair.Key: pair.Value},
			},
			Spec: vspherepolv1.TagSpec{
				Key:   pair.Key,
				Value: pair.Value,
			},
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

	if tag.Spec.Key != pair.Key || tag.Spec.Value != pair.Value {
		return nil, fmt.Errorf(
			"tag %q has spec key/value %q/%q, expected %q/%q",
			name, tag.Spec.Key, tag.Spec.Value, pair.Key, pair.Value)
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
// currently owns whose name is not in keepNames, using the
// TagOwnerReferencesUIDIndexKey index rather than a namespace scan. Called
// with an empty keepNames it releases every owner reference the VM holds —
// the branch ReleaseOwnership shares with ReconcileTagCRs.
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
		// which would otherwise turn this converged no-op into a hard error
		// and, on the ReleaseOwnership path, stall VM deletion. Skip it.
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
