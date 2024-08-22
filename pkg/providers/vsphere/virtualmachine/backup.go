// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8syaml "sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	res "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/resources"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// BackupVirtualMachineOptions contains the options for BackupVirtualMachine.
type BackupVirtualMachineOptions struct {
	VMCtx               pkgctx.VirtualMachineContext
	VcVM                *object.VirtualMachine
	DiskUUIDToPVC       map[string]corev1.PersistentVolumeClaim
	AdditionalResources []client.Object
	BackupVersion       string
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
	curExCfg := pkgutil.OptionValues(moVM.Config.ExtraConfig)
	var ecToUpdate pkgutil.OptionValues

	/*
	 * When VMIncrementalRestore FSS is enabled, perform backups when
	 * - no backup version annotation is present.
	 * - the backup version annotation matches the current backup version in extraConfig.
	 * - the backup version annotation is older than the extraConfig backup version key. This can only happen when reconfigure succeeds to add a new
	 *   extraConfig backup version but the next reconcile failed to update the annotation on the VM resource.
	 */
	if pkgcfg.FromContext(opts.VMCtx).Features.VMIncrementalRestore {
		if backupVersionAnnotation, ok := opts.VMCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]; ok && backupVersionAnnotation != "" {
			if backupVersionEcVal, _ := curExCfg.GetString(vmopv1.BackupVersionExtraConfigKey); backupVersionEcVal != "" {
				canBackup, err := canPerformBackup(opts, backupVersionEcVal, backupVersionAnnotation)
				if err != nil {
					return err
				}

				/* TODO: Improvements to fetch the backupVersion extraConfig again right before doing the backup reconfigure?.
				 *  There is a minor chance for a race if a restore is triggered by a vendor between !canBackup check
				 *  and the backup reconfigure call below. Chances are low since the vm reconcile only does a backup when it detects
				 *  changes to the keys and a restore needs to be triggered around that exact time.
				 */
				// restore is detected if doBackup is false with no errors. wait for vm registration.
				if !canBackup {
					// TODO: add a condition to indicate backup is paused
					return nil
				}
			}
		}
	}

	additionalYAML, err := getDesiredAdditionalResourceYAMLForBackup(
		opts.AdditionalResources,
		curExCfg,
	)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get additional resources yaml for backup")
		return err
	}

	if additionalYAML == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping additional resources yaml backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
			Key:   vmopv1.AdditionalResourcesYAMLExtraConfigKey,
			Value: additionalYAML,
		})
	}

	pvcDiskData, err := getDesiredPVCDiskDataForBackup(opts, curExCfg)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get PVC disk data for backup")
		return err
	}

	if pvcDiskData == "" {
		opts.VMCtx.Logger.V(4).Info("Skipping PVC disk data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
			Key:   vmopv1.PVCDiskDataExtraConfigKey,
			Value: pvcDiskData,
		})
	}

	if pkgcfg.FromContext(opts.VMCtx).Features.VMIncrementalRestore {
		curBackup, _ := curExCfg.GetString(vmopv1.VMResourceYAMLExtraConfigKey)
		vmBackupInSync, err := isVMBackupUpToDate(opts.VMCtx.VM, curBackup)
		if err != nil {
			opts.VMCtx.Logger.Error(err, "failed to check if VM resource is in sync with backup")
			return err
		}

		// When VMIncrementalRestore FSS is enabled,
		// Update extraConfig with new backup version
		// - no last backup version annotation is present on VM resource (or)
		// - the current backup's extraConfig keys are not in sync with existing resource data.
		backupVersionAnnotation, ok := opts.VMCtx.VM.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation]
		if (!ok || backupVersionAnnotation == "") || len(ecToUpdate) != 0 || !vmBackupInSync {
			ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
				Key:   vmopv1.BackupVersionExtraConfigKey,
				Value: opts.BackupVersion,
			})

			// Update the VMResourceYAMLExtraConfigKey to account for the new backup version annotation
			// Use a copy of the VM object since the actual VM obj will be annotated if and once reconfigure succeeds.
			copyVM := opts.VMCtx.VM.DeepCopy()
			setLastBackupVersionAnnotation(copyVM, opts.BackupVersion)
			// Backup the updated VM's YAML with encoding and compression.
			encodedVMYaml, err := getEncodedVMYaml(copyVM)
			if err != nil {
				opts.VMCtx.Logger.Error(err, "failed to get VM resource yaml for backup")
				return err
			}

			ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
				Key:   vmopv1.VMResourceYAMLExtraConfigKey,
				Value: encodedVMYaml,
			})
		}

	} else {
		vmYAML, err := getDesiredVMResourceYAMLForBackup(opts.VMCtx.VM, curExCfg)
		if err != nil {
			opts.VMCtx.Logger.Error(err, "failed to get VM resource yaml for backup")
			return err
		}
		if vmYAML == "" {
			opts.VMCtx.Logger.V(4).Info("Skipping VM resource yaml backup as unchanged")
		} else {
			ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
				Key:   vmopv1.VMResourceYAMLExtraConfigKey,
				Value: vmYAML,
			})
		}
	}

	if len(ecToUpdate) != 0 {
		opts.VMCtx.Logger.Info("Updating VM ExtraConfig with latest backup data",
			"ExtraConfigToUpdate", ecToUpdate)
		configSpec := &vimtypes.VirtualMachineConfigSpec{
			ExtraConfig: ecToUpdate,
		}

		// This ensures that the reconfigure done for storing the backup data
		// does not indicate an update operation was triggered. While this does
		// reconfigure the VM, it is not what "update" means to a user.
		ctx := ctxop.JoinContext(opts.VMCtx, ctxop.WithContext(context.TODO()))

		if _, err := resVM.Reconfigure(ctx, configSpec); err != nil {
			opts.VMCtx.Logger.Error(err, "failed to update VM ExtraConfig with latest backup data")
			return err
		}

		// Set the VirtualMachine's backup version annotation once reconfigure succeeds.
		if pkgcfg.FromContext(opts.VMCtx).Features.VMIncrementalRestore {
			setLastBackupVersionAnnotation(opts.VMCtx.VM, opts.BackupVersion)
			// TODO: add a condition to indicate backup has resumed
		}

		opts.VMCtx.Logger.Info("Successfully updated VM ExtraConfig with latest backup data")
	}

	return nil
}

// canPerformBackup checks if a backup can be performed. The method returns true when
// the backup version annotation matches (or) is older (lesser) than the backup version in extraConfig.
// It returns false when the backup version annotation is newer (greater) than the backup extraConfig version which indicates a vendor
// triggered restore in progress.
func canPerformBackup(opts BackupVirtualMachineOptions, backupVersionEcVal, backupVersionAnnotation string) (bool, error) {
	if backupVersionEcVal == backupVersionAnnotation {
		return true, nil
	}

	storedVersion, err := strconv.ParseInt(backupVersionEcVal, 10, 64)
	if err != nil {
		opts.VMCtx.Logger.Error(err, fmt.Sprintf("failed to parse backup version extraConfig key to int64 : %v", backupVersionEcVal))
		return false, err
	}

	currVersion, err := strconv.ParseInt(backupVersionAnnotation, 10, 64)
	if err != nil {
		opts.VMCtx.Logger.Error(err, fmt.Sprintf("failed to parse backup version annotation to int64: %v", backupVersionAnnotation))
		return false, err
	}

	// if the storedVersion in the backup is older than the annotation, this is a restore.
	if currVersion > storedVersion {
		opts.VMCtx.Logger.Info(fmt.Sprintf("Skipping VM backup as a restore is detected: current version: %s greater than backup extraconfig version: %s, wait for VM registration",
			backupVersionAnnotation, backupVersionEcVal))
		return false, nil
	}

	return true, nil
}

// getDesiredVMResourceYAMLForBackup returns the encoded and gzipped YAML of the
// given VM, or an empty string if the existing backup is already up-to-date.
func getDesiredVMResourceYAMLForBackup(
	vm *vmopv1.VirtualMachine,
	extraConfig pkgutil.OptionValues) (string, error) {

	curBackup, _ := extraConfig.GetString(vmopv1.VMResourceYAMLExtraConfigKey)
	isUpToDate, err := isVMBackupUpToDate(vm, curBackup)
	if err != nil || isUpToDate {
		return "", err
	}

	// Backup the updated VM's YAML with encoding and compression.
	return getEncodedVMYaml(vm)
}

func getEncodedVMYaml(vm *vmopv1.VirtualMachine) (string, error) {
	// Backup the updated VM's YAML with encoding and compression.
	vmYAML, err := k8syaml.Marshal(vm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal VM into YAML %+v: %v", vm, err)
	}

	return pkgutil.EncodeGzipBase64(string(vmYAML))
}

// isVMBackupUpToDate returns true if none of the following fields of the VM are
// changed compared to the existing backup:
// - generation (spec changes); annotations; labels.
func isVMBackupUpToDate(vm *vmopv1.VirtualMachine, backup string) (bool, error) {
	if backup == "" {
		return false, nil
	}

	backupYAML, err := pkgutil.TryToDecodeBase64Gzip([]byte(backup))
	if err != nil {
		return false, err
	}

	backupVM := metav1.PartialObjectMetadata{}
	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	if _, _, err := decUnstructured.Decode([]byte(backupYAML), nil, &backupVM); err != nil {
		return false, err
	}

	return vm.Generation == backupVM.Generation &&
		maps.Equal(vm.Labels, backupVM.Labels) &&
		maps.Equal(vm.Annotations, backupVM.Annotations), nil
}

// getDesiredAdditionalResourceYAMLForBackup returns the encoded and gzipped
// YAML of the given resources, or an empty string if the existing backup is
// already up-to-date (checked by comparing the resource versions).
func getDesiredAdditionalResourceYAMLForBackup(
	resources []client.Object,
	extraConfig pkgutil.OptionValues) (string, error) {

	curBackup, _ := extraConfig.GetString(vmopv1.AdditionalResourcesYAMLExtraConfigKey)
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
	var sb strings.Builder
	for i, resource := range resources {
		marshaledYaml, err := k8syaml.Marshal(resource)
		if err != nil {
			return "", fmt.Errorf("failed to marshal object %q: %v", resource.GetName(), err)
		}
		sb.Write(marshaledYaml)
		if i != len(resources)-1 {
			sb.WriteString("\n---\n")
		}
	}

	return pkgutil.EncodeGzipBase64(sb.String())
}

// getBackupResourceVersions gets the resource version of each object in
// the given encoded and gzipped data. Returns a map of resource UID to version.
func getBackupResourceVersions(ecResourceData string) (map[string]string, error) {
	if ecResourceData == "" {
		return nil, nil
	}

	decoded, err := pkgutil.TryToDecodeBase64Gzip([]byte(ecResourceData))
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
	extraConfig pkgutil.OptionValues) (string, error) {

	// Return an empty string to skip backup if no disk uuid to PVC is specified.
	if len(opts.DiskUUIDToPVC) == 0 {
		return "", nil
	}

	deviceList, err := opts.VcVM.Device(opts.VMCtx)
	if err != nil {
		return "", err
	}

	var diskData []PVCDiskData
	for _, device := range deviceList.SelectByType((*vimtypes.VirtualDisk)(nil)) {
		if disk, ok := device.(*vimtypes.VirtualDisk); ok {
			if b, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
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
	diskDataBackup, err := pkgutil.EncodeGzipBase64(string(diskDataJSON))
	if err != nil {
		return "", err
	}

	// Return an empty string to skip the backup if the data is unchanged.
	curDiskDataBackup, _ := extraConfig.GetString(vmopv1.PVCDiskDataExtraConfigKey)
	if diskDataBackup == curDiskDataBackup {
		return "", nil
	}

	return diskDataBackup, nil
}

func setLastBackupVersionAnnotation(vm *vmopv1.VirtualMachine, version string) {
	if vm.Annotations == nil {
		vm.Annotations = map[string]string{}
	}

	vm.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = version
}
