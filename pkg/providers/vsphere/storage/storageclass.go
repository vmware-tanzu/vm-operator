// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"

	storagev1 "k8s.io/api/storage/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// getStoragePolicyID returns Storage Policy ID from Storage Class Name.
func getStoragePolicyID(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	storageClassName string) (string, error) {

	sc := &storagev1.StorageClass{}
	if err := client.Get(vmCtx, ctrlclient.ObjectKey{Name: storageClassName}, sc); err != nil {
		vmCtx.Logger.Error(err, "Failed to get StorageClass", "storageClass", storageClassName)
		return "", err
	}

	policyID, ok := sc.Parameters["storagePolicyID"]
	if !ok {
		return "", fmt.Errorf("StorageClass %s does not have 'storagePolicyID' parameter", storageClassName)
	}

	return policyID, nil
}

// GetVMStoragePoliciesIDs returns a map of storage class names to their storage policy IDs.
func GetVMStoragePoliciesIDs(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client) (map[string]string, error) {

	storageClassNames := getVMStorageClassNames(vmCtx.VM)
	storageClassesToIDs := map[string]string{}

	for _, name := range storageClassNames {
		if _, ok := storageClassesToIDs[name]; !ok {
			id, err := getStoragePolicyID(vmCtx, client, name)
			if err != nil {
				return nil, err
			}

			storageClassesToIDs[name] = id
		}
	}

	return storageClassesToIDs, nil
}

func getVMStorageClassNames(vm *vmopv1.VirtualMachine) []string {
	var names []string

	if vm.Spec.StorageClass != "" {
		names = append(names, vm.Spec.StorageClass)
	}

	for _, vol := range vm.Spec.Volumes {
		var storageClass string

		claim := vol.PersistentVolumeClaim
		if claim != nil {
			if isClaim := claim.InstanceVolumeClaim; isClaim != nil {
				storageClass = isClaim.StorageClass
			} else { //nolint
				// TODO: Fetch claim.ClaimName PVC to get the StorageClass.
			}
		}

		if storageClass != "" {
			names = append(names, storageClass)
		}
	}

	return names
}
