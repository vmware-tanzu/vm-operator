// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
)

const (
	// PVCOwnerRefVMUIDIndexKey is the field-index key used to efficiently
	// list PersistentVolumeClaims by the UID of any VirtualMachine
	// OwnerReference they carry. See IndexPVCByVMOwnerRef.
	PVCOwnerRefVMUIDIndexKey = "metadata.ownerReferences.vmUID"

	vmKind = "VirtualMachine"
)

// apiGroupNamePrefix is used when matching just the API group of a VM Op
// object.
const apiGroupNamePrefix = vmopv1.GroupName + "/"

// PVCOwnerRefVMUIDIndexerFunc extracts the UIDs of any VirtualMachine
// OwnerReferences from a PersistentVolumeClaim. It is used to populate the
// field index registered by IndexPVCByVMOwnerRef.
func PVCOwnerRefVMUIDIndexerFunc(obj ctrlclient.Object) []string {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil
	}

	var uids []string
	for _, ref := range pvc.OwnerReferences {

		if ref.Kind == vmKind &&
			strings.HasPrefix(ref.APIVersion, apiGroupNamePrefix) {

			uids = append(uids, string(ref.UID))
		}
	}

	return uids
}

// IndexPVCByVMOwnerRef registers a field index on PersistentVolumeClaim,
// keyed by PVCOwnerRefVMUIDIndexKey, so RemoveStaleVMOwnerRefFromPVCs can
// efficiently list the PVCs owned by a given VirtualMachine instead of
// scanning every PVC in the namespace.
func IndexPVCByVMOwnerRef(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&corev1.PersistentVolumeClaim{},
		PVCOwnerRefVMUIDIndexKey,
		PVCOwnerRefVMUIDIndexerFunc)
}

// RemoveStaleVMOwnerRefFromPVCs finds every PersistentVolumeClaim in vm's
// namespace whose OwnerReferences includes vm, and removes that specific
// OwnerReference entry from any such PVC whose name is no longer present in
// vm.Spec.Volumes[].PersistentVolumeClaim.ClaimName. This prevents
// Kubernetes' garbage collector from cascade-deleting a PVC that was
// detached from the VM before the VM itself was deleted.
//
// A PVC that carries the pkgconst.KeepOwnerRefAnnotationKey annotation is
// skipped regardless of its attachment state, letting a user opt a specific
// PVC back into the old cascade-delete behavior.
//
// The field index registered by IndexPVCByVMOwnerRef must already be present
// on the client/manager for this to find any candidate PVCs.
func RemoveStaleVMOwnerRefFromPVCs(
	ctx context.Context,
	c ctrlclient.Client,
	vm *vmopv1.VirtualMachine) error {

	var pvcList corev1.PersistentVolumeClaimList
	if err := c.List(
		ctx,
		&pvcList,
		ctrlclient.InNamespace(vm.Namespace),
		ctrlclient.MatchingFields{PVCOwnerRefVMUIDIndexKey: string(vm.UID)}); err != nil {

		return fmt.Errorf(
			"failed to list pvcs owned by vm %s/%s: %w",
			vm.Namespace, vm.Name, err)
	}

	if len(pvcList.Items) == 0 {
		return nil
	}

	attachedClaimNames := sets.New[string]()
	for _, vol := range vm.Spec.Volumes {
		if pvc := vol.PersistentVolumeClaim; pvc != nil && pvc.ClaimName != "" {
			attachedClaimNames.Insert(pvc.ClaimName)
		}
	}

	var errs []error
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]

		if attachedClaimNames.Has(pvc.Name) {
			continue
		}

		key := ctrlclient.ObjectKeyFromObject(pvc)
		if err := removeVMOwnerRefFromPVC(ctx, c, key, vm); err != nil {
			errs = append(errs, fmt.Errorf(
				"failed to remove owner ref from pvc %s/%s: %w",
				pvc.Namespace, pvc.Name, err))
		}
	}

	return apierrorsutil.NewAggregate(errs)
}

// removeVMOwnerRefFromPVC removes the OwnerReference matching vm from the
// PersistentVolumeClaim identified by key, retrying on write conflicts. All
// other OwnerReferences on the PVC are left untouched. A PVC that no longer
// exists, or that no longer carries the OwnerReference, is a no-op success.
func removeVMOwnerRefFromPVC(
	ctx context.Context,
	c ctrlclient.Client,
	key ctrlclient.ObjectKey,
	vm *vmopv1.VirtualMachine) error {

	expOwnerRef := metav1.OwnerReference{
		APIVersion: vmopv1.GroupVersion.String(),
		Kind:       vmKind,
		Name:       vm.Name,
		UID:        vm.UID,
	}

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		pvc := &corev1.PersistentVolumeClaim{}
		if err := c.Get(ctx, key, pvc); err != nil {
			return ctrlclient.IgnoreNotFound(err)
		}

		if _, ok := pvc.Annotations[pkgconst.KeepOwnerRefAnnotationKey]; ok {
			return nil
		}

		filteredRefs := make([]metav1.OwnerReference, 0, len(pvc.OwnerReferences))
		changed := false
		for _, ref := range pvc.OwnerReferences {
			if strings.HasPrefix(ref.APIVersion, apiGroupNamePrefix) &&
				ref.Kind == expOwnerRef.Kind &&
				ref.Name == expOwnerRef.Name &&
				ref.UID == expOwnerRef.UID {

				changed = true
				continue
			}
			filteredRefs = append(filteredRefs, ref)
		}

		if !changed {
			return nil
		}

		objPatch := ctrlclient.MergeFrom(pvc.DeepCopy())
		pvc.OwnerReferences = filteredRefs
		return c.Patch(ctx, pvc, objPatch)
	})
}
