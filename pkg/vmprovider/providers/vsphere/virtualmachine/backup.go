// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8syaml "sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

// BackupVirtualMachineOptions contains the options for BackupVirtualMachine.
type BackupVirtualMachineOptions struct {
	VMCtx               context.VirtualMachineContext
	VcVM                *object.VirtualMachine
	DiskUUIDToPVC       map[string]corev1.PersistentVolumeClaim
	AdditionalResources []client.Object
}

// PVCDiskData contains the data of a disk attached to VM backed by a PVC.
type PVCDiskData struct {
	// Filename contains the datastore path to the virtual disk.
	FileName string
	// PVCName is the name of the PVC backed by the virtual disk.
	PVCName string
	// AccessMode is the access modes of the PVC backed by the virtual disk.
	AccessModes []corev1.PersistentVolumeAccessMode
}

// BackupVirtualMachine backs up the required data of a VM into its ExtraConfig.
// Currently, the following data is backed up:
// - VM Kubernetes resource in YAMl.
// - Additional VM-relevant Kubernetes resources in YAML, separated by "---".
// - PVC disk data in JSON format (if DiskUUIDToPVC is not empty).
func BackupVirtualMachine(opts BackupVirtualMachineOptions) error {
	resVM := res.NewVMFromObject(opts.VcVM)
	moVM, err := resVM.GetProperties(opts.VMCtx, []string{"config.extraConfig"})
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get VM properties for backup")
		return err
	}

	curEcMap := util.ExtraConfigToMap(moVM.Config.ExtraConfig)
	ecToUpdate := []types.BaseOptionValue{}

	vmYAML, err := getDesiredVMResourceYAMLForBackup(opts.VMCtx.VM, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get VM resource yaml for backup")
		return err
	}
	if vmYAML == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping VM resource yaml backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.VMResourceYAMLExtraConfigKey,
			Value: vmYAML,
		})
	}

	additionalYAML, err := getDesiredAdditionalResourceYAMLForBackup(
		opts.AdditionalResources,
		curEcMap,
	)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get additional resources yaml for backup")
		return err
	}

	if additionalYAML == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping additional resources yaml backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.AdditionalResourcesYAMLExtraConfigKey,
			Value: additionalYAML,
		})
	}

	pvcDiskData, err := getDesiredPVCDiskDataForBackup(opts, curEcMap)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get PVC disk data for backup")
		return err
	}

	if pvcDiskData == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping PVC disk data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &types.OptionValue{
			Key:   vmopv1.PVCDiskDataExtraConfigKey,
			Value: pvcDiskData,
		})
	}

	if len(ecToUpdate) != 0 {
		opts.VMCtx.Logger.Info("Updating VM ExtraConfig with latest backup data",
			"ExtraConfigToUpdate", ecToUpdate)
		configSpec := &types.VirtualMachineConfigSpec{
			ExtraConfig: ecToUpdate,
		}
		if err := resVM.Reconfigure(opts.VMCtx, configSpec); err != nil {
			opts.VMCtx.Logger.Error(err, "failed to update VM ExtraConfig with latest backup data")
			return err
		}

		opts.VMCtx.Logger.Info("Successfully updated VM ExtraConfig with latest backup data")
	}

	return nil
}

// getDesiredVMResourceYAMLForBackup returns the encoded and gzipped YAML of the
// given VM, or an empty string if the existing backup is already up-to-date.
func getDesiredVMResourceYAMLForBackup(
	vm *vmopv1.VirtualMachine,
	ecMap map[string]string) (string, error) {
	curBackup := ecMap[vmopv1.VMResourceYAMLExtraConfigKey]
	isUpToDate, err := isVMBackupUpToDate(vm, curBackup)
	if err != nil || isUpToDate {
		return "", err
	}

	// Backup the updated VM's YAML with encoding and compression.
	vmYAML, err := k8syaml.Marshal(vm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal VM into YAML %+v: %v", vm, err)
	}

	return util.EncodeGzipBase64(string(vmYAML))
}

// isVMBackupUpToDate returns true if none of the following fields of the VM are
// changed compared to the existing backup:
// - generation (spec changes); annotations; labels.
func isVMBackupUpToDate(vm *vmopv1.VirtualMachine, backup string) (bool, error) {
	if backup == "" {
		return false, nil
	}

	backupYAML, err := util.TryToDecodeBase64Gzip([]byte(backup))
	if err != nil {
		return false, err
	}

	backupUnstructured := &unstructured.Unstructured{}
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	if _, _, err := decUnstructured.Decode([]byte(backupYAML), nil, backupUnstructured); err != nil {
		return false, err
	}

	if vm.GetGeneration() != backupUnstructured.GetGeneration() ||
		!equality.Semantic.DeepEqual(vm.Labels, backupUnstructured.GetLabels()) ||
		!equality.Semantic.DeepEqual(vm.Annotations, backupUnstructured.GetAnnotations()) {
		return false, nil
	}

	return true, nil
}

// getDesiredAdditionalResourceYAMLForBackup returns the encoded and gzipped
// YAML of the given resources, or an empty string if the existing backup is
// already up-to-date (checked by comparing the resource versions).
func getDesiredAdditionalResourceYAMLForBackup(
	resources []client.Object,
	ecMap map[string]string) (string, error) {
	curBackup := ecMap[vmopv1.AdditionalResourcesYAMLExtraConfigKey]
	backupVers, err := getBackupResourceVersions(curBackup)
	if err != nil {
		return "", err
	}

	isLatestBackup := true
	for _, curRes := range resources {
		if backupVers[string(curRes.GetUID())] != curRes.GetResourceVersion() {
			isLatestBackup = false
			break
		}
	}

	// All resources are up-to-date, return an empty string to skip the backup.
	if isLatestBackup {
		return "", nil
	}

	// Backup the given resources YAML with encoding and compression.
	// Use "---" as the separator if more than one resource is passed.
	marshaledStrs := []string{}
	for _, res := range resources {
		marshaledYaml, err := k8syaml.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to marshal object %q: %v", res.GetName(), err)
		}
		marshaledStrs = append(marshaledStrs, string(marshaledYaml))
	}

	resourcesYAML := strings.Join(marshaledStrs, "\n---\n")
	return util.EncodeGzipBase64(resourcesYAML)
}

// getBackupResourceVersions gets the resource version of each object in
// the given encoded and gzipped data. Returns a map of resource UID to version.
func getBackupResourceVersions(ecResourceData string) (map[string]string, error) {
	if ecResourceData == "" {
		return nil, nil
	}

	decoded, err := util.TryToDecodeBase64Gzip([]byte(ecResourceData))
	if err != nil {
		return nil, err
	}

	resourceVersions := map[string]string{}
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	resourcesYAML := strings.Split(decoded, "\n---\n")
	for _, resYAML := range resourcesYAML {
		if resYAML != "" {
			resYAML = strings.TrimSpace(resYAML)
			res := &unstructured.Unstructured{}
			if _, _, err := decUnstructured.Decode([]byte(resYAML), nil, res); err != nil {
				continue
			}
			resourceVersions[string(res.GetUID())] = res.GetResourceVersion()
		}
	}

	return resourceVersions, nil
}

func getDesiredPVCDiskDataForBackup(
	opts BackupVirtualMachineOptions,
	ecMap map[string]string) (string, error) {
	// Return an empty string to skip backup if no disk uuid to PVC is specified.
	if len(opts.DiskUUIDToPVC) == 0 {
		return "", nil
	}

	deviceList, err := opts.VcVM.Device(opts.VMCtx)
	if err != nil {
		return "", err
	}

	var diskData []PVCDiskData
	for _, device := range deviceList.SelectByType((*types.VirtualDisk)(nil)) {
		if disk, ok := device.(*types.VirtualDisk); ok {
			if b, ok := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo); ok {
				if pvc, ok := opts.DiskUUIDToPVC[b.Uuid]; ok {
					diskData = append(diskData, PVCDiskData{
						FileName:    b.FileName,
						PVCName:     pvc.Name,
						AccessModes: pvc.Spec.AccessModes,
					})
				}
			}
		}
	}

	diskDataJSON, err := json.Marshal(diskData)
	if err != nil {
		return "", err
	}
	diskDataBackup, err := util.EncodeGzipBase64(string(diskDataJSON))
	if err != nil {
		return "", err
	}

	// Return an empty string to skip the backup if the data is unchanged.
	if diskDataBackup == ecMap[vmopv1.PVCDiskDataExtraConfigKey] {
		return "", nil
	}

	return diskDataBackup, nil
}
