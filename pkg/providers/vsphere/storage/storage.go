// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

type VMStorageData struct {
	StorageClasses         map[string]storagev1.StorageClass
	StorageClassToPolicyID map[string]string
	PVCs                   []corev1.PersistentVolumeClaim
}

func getStorageClassAndPolicyID(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	storageClassName string) (*storagev1.StorageClass, string, error) {

	sc := storagev1.StorageClass{}
	if err := client.Get(vmCtx, ctrlclient.ObjectKey{Name: storageClassName}, &sc); err != nil {
		vmCtx.Logger.Error(err, "Failed to get StorageClass", "storageClass", storageClassName)
		return nil, "", err
	}
	policyID, err := kubeutil.GetStoragePolicyID(sc)
	return &sc, policyID, err
}

func GetVMStorageData(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client) (VMStorageData, error) {

	storageClassNames, pvcs, err := getVMStorageClassNamesAndPVCs(vmCtx, vmCtx.VM, client)
	if err != nil {
		return VMStorageData{}, err
	}

	data := VMStorageData{
		StorageClasses:         map[string]storagev1.StorageClass{},
		StorageClassToPolicyID: map[string]string{},
		PVCs:                   pvcs,
	}

	for _, name := range storageClassNames {
		if _, ok := data.StorageClassToPolicyID[name]; !ok {
			storageClass, policyID, err := getStorageClassAndPolicyID(vmCtx, client, name)
			if err != nil {
				return VMStorageData{}, err
			}

			data.StorageClasses[name] = *storageClass
			data.StorageClassToPolicyID[name] = policyID
		}
	}

	return data, nil
}

func getVMStorageClassNamesAndPVCs(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	client ctrlclient.Client) ([]string, []corev1.PersistentVolumeClaim, error) {

	var scNames []string
	var pvcs []corev1.PersistentVolumeClaim

	if vm.Spec.StorageClass != "" {
		scNames = append(scNames, vm.Spec.StorageClass)
	}

	for _, vol := range vm.Spec.Volumes {
		claim := vol.PersistentVolumeClaim
		if claim == nil {
			continue
		}

		var storageClass string
		if isClaim := claim.InstanceVolumeClaim; isClaim != nil {
			storageClass = isClaim.StorageClass
		} else {
			pvc := &corev1.PersistentVolumeClaim{}
			pvcKey := ctrlclient.ObjectKey{Name: claim.ClaimName, Namespace: vm.Namespace}
			if err := client.Get(ctx, pvcKey, pvc); err != nil {
				return nil, nil, err
			}

			if n := pvc.Spec.StorageClassName; n != nil {
				storageClass = *n
			}
			pvcs = append(pvcs, *pvc)
		}

		if storageClass != "" {
			scNames = append(scNames, storageClass)
		}
	}

	return scNames, pvcs, nil
}
