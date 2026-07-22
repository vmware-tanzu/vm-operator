// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

const (
	// VMSpecVolumesPVCsIndexKey is the field-index key used to efficiently
	// determine PVCs that are referenced in a VM Spec.
	VMSpecVolumesPVCsIndexKey = "spec.volumes.persistentVolumeClaim.claimName"
)

// VMSpecVolumesPVCsIndexerFunc returns the list of the PVC names that are referenced
// in the VM Spec.
func VMSpecVolumesPVCsIndexerFunc(obj client.Object) []string {
	vm, ok := obj.(*vmopv1.VirtualMachine)
	if !ok {
		return nil
	}

	pvcs := make([]string, 0, len(vm.Spec.Volumes))
	for _, volume := range vm.Spec.Volumes {
		if pvc := volume.PersistentVolumeClaim; pvc != nil && pvc.ClaimName != "" {
			pvcs = append(pvcs, pvc.ClaimName)
		}
	}
	return pvcs
}

// IndexVMSpecVolumesPVCs registers a field indexer on the VirtualMachine keyed by
// VMSpecVolumesPVCsIndexKey to efficiently list the VirtualMachines that reference
// a PVC.
func IndexVMSpecVolumesPVCs(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&vmopv1.VirtualMachine{},
		VMSpecVolumesPVCsIndexKey,
		VMSpecVolumesPVCsIndexerFunc)
}
