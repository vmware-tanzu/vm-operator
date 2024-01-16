// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"

	storagev1 "k8s.io/api/storage/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	cnsstoragev1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/storagepolicy/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// GetStoragePolicyID returns Storage Policy ID from Storage Class Name.
func GetStoragePolicyID(
	vmCtx context.VirtualMachineContext,
	client ctrlclient.Client,
	storageClassName string) (string, error) {

	var (
		policyID string
		zones    []topologyv1.AvailabilityZone
	)

	if pkgconfig.FromContext(vmCtx).Features.PodVMOnStretchedSupervisor {
		azs, err := topology.GetAvailabilityZones(vmCtx, client)
		if err != nil {
			return "", err
		}
		zones = azs
	}

	if pkgconfig.FromContext(vmCtx).Features.PodVMOnStretchedSupervisor && len(zones) > 1 {
		storagePolicyQuota := &cnsstoragev1.StoragePolicyQuota{}
		if err := client.Get(vmCtx, ctrlclient.ObjectKey{
			Namespace: vmCtx.VM.Namespace,
			Name:      util.CNSStoragePolicyQuotaName(storageClassName),
		}, storagePolicyQuota); err != nil {
			return "", err
		}

		policyID = storagePolicyQuota.Spec.StoragePolicyId
	} else {
		sc := &storagev1.StorageClass{}
		if err := client.Get(vmCtx, ctrlclient.ObjectKey{Name: storageClassName}, sc); err != nil {
			vmCtx.Logger.Error(err, "Failed to get StorageClass", "storageClass", storageClassName)
			return "", err
		}

		var ok bool
		policyID, ok = sc.Parameters["storagePolicyID"]
		if !ok {
			return "", fmt.Errorf("StorageClass %s does not have 'storagePolicyID' parameter", storageClassName)
		}
	}
	return policyID, nil
}

// GetVMStoragePoliciesIDs returns a map of storage class names to their storage policy IDs.
func GetVMStoragePoliciesIDs(
	vmCtx context.VirtualMachineContext,
	client ctrlclient.Client) (map[string]string, error) {

	storageClassNames := getVMStorageClassNames(vmCtx.VM)
	storageClassesToIDs := map[string]string{}

	for _, name := range storageClassNames {
		if _, ok := storageClassesToIDs[name]; !ok {
			id, err := GetStoragePolicyID(vmCtx, client, name)
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
