// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"encoding/json"
	"errors"
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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
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
	ClassicDiskUUIDs    map[string]struct{}
}

var ErrBackingUp = pkgerr.NoRequeueNoErr("backed up vm")

// BackupVirtualMachine backs up the required data of a VM into its ExtraConfig.
// Currently, the following data is backed up:
// - VM Kubernetes resource in YAMl.
// - Additional VM-relevant Kubernetes resources in YAML, separated by "---".
// - PVC disk data in JSON format (if DiskUUIDToPVC is not empty).
//
//nolint:gocyclo
func BackupVirtualMachine(opts BackupVirtualMachineOptions) (result error) {
	defer func() {
		if result != nil && !errors.Is(result, ErrBackingUp) {
			conditions.MarkFalse(
				opts.VMCtx.VM,
				vmopv1.VirtualMachineBackupUpToDateCondition,
				vmopv1.VirtualMachineBackupFailedReason,
				"Failed to backup VM. err: %v", result.Error(),
			)
		}
	}()

	config := opts.VMCtx.MoVM.Config
	if config == nil || config.ExtraConfig == nil || config.Hardware.Device == nil {
		// Fetch the properties we need for backup if either are not present.
		if err := opts.VcVM.Properties(opts.VMCtx, opts.VcVM.Reference(), []string{"config.extraConfig", "config.hardware.device"}, &opts.VMCtx.MoVM); err != nil {
			return err
		}
	}
	curExCfg := pkgutil.OptionValues(opts.VMCtx.MoVM.Config.ExtraConfig)
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
			if backupVersionEcVal, _ := curExCfg.GetString(backupapi.BackupVersionExtraConfigKey); backupVersionEcVal != "" {
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
					conditions.MarkFalse(
						opts.VMCtx.VM,
						vmopv1.VirtualMachineBackupUpToDateCondition,
						vmopv1.VirtualMachineBackupPausedReason,
						"A restore was detected",
					)
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
			Key:   backupapi.AdditionalResourcesYAMLExtraConfigKey,
			Value: additionalYAML,
		})
	}

	pvcDiskData, classicDiskData, err := getDesiredDiskDataForBackup(opts)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to get disk data for backup")
		return err
	}

	curPvcDiskData, _ := curExCfg.GetString(backupapi.PVCDiskDataExtraConfigKey)
	if pvcDiskData == curPvcDiskData {
		opts.VMCtx.Logger.V(4).Info("Skipping PVC disk data backup as unchanged")
	} else {
		ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
			Key:   backupapi.PVCDiskDataExtraConfigKey,
			Value: pvcDiskData,
		})
	}

	if pkgcfg.FromContext(opts.VMCtx).Features.VMIncrementalRestore {
		curClassDiskData, _ := curExCfg.GetString(backupapi.ClassicDiskDataExtraConfigKey)
		if classicDiskData == curClassDiskData {
			opts.VMCtx.Logger.V(4).Info("Skipping classic disk data backup as unchanged")
		} else {
			ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
				Key:   backupapi.ClassicDiskDataExtraConfigKey,
				Value: classicDiskData,
			})
		}
	}

	if pkgcfg.FromContext(opts.VMCtx).Features.VMIncrementalRestore {
		curBackup, _ := curExCfg.GetString(backupapi.VMResourceYAMLExtraConfigKey)
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
				Key:   backupapi.BackupVersionExtraConfigKey,
				Value: opts.BackupVersion,
			})

			// Update the VMResourceYAMLExtraConfigKey to account for the new backup version annotation
			// Use a copy of the VM object since the actual VM obj will be annotated if and once reconfigure succeeds.
			copyVM := opts.VMCtx.VM.DeepCopy()
			setBackupVersionAnnotation(copyVM, opts.BackupVersion)

			// Trim certain fields such as last applied configuration,
			// managed fields and Status subresource from the payload.
			trimBackupFields(copyVM)
			copyVM.Status = vmopv1.VirtualMachineStatus{}

			encodedVMYaml, err := getEncodedVMYaml(copyVM)
			if err != nil {
				opts.VMCtx.Logger.Error(err, "failed to get VM resource yaml for backup")
				return err
			}

			ecToUpdate = append(ecToUpdate, &vimtypes.OptionValue{
				Key:   backupapi.VMResourceYAMLExtraConfigKey,
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
				Key:   backupapi.VMResourceYAMLExtraConfigKey,
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
		ctx := ctxop.WithContext(opts.VMCtx)

		resVM := res.NewVMFromObject(opts.VcVM)
		if _, err := resVM.Reconfigure(ctx, configSpec); err != nil {
			opts.VMCtx.Logger.Error(err, "failed to update VM ExtraConfig with latest backup data")
			return err
		}

		// Set the VirtualMachine's backup version annotation once reconfigure succeeds.
		if pkgcfg.FromContext(opts.VMCtx).Features.VMIncrementalRestore {
			setBackupVersionAnnotation(opts.VMCtx.VM, opts.BackupVersion)
			c := conditions.TrueCondition(vmopv1.VirtualMachineBackupUpToDateCondition)
			c.Message = fmt.Sprintf("Backup version: %s", opts.BackupVersion)
			conditions.Set(opts.VMCtx.VM, c)
		}

		opts.VMCtx.Logger.Info("Successfully updated VM ExtraConfig with latest backup data")
		return ErrBackingUp
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
		opts.VMCtx.Logger.Error(err, "failed to parse backup version ExtraConfig value to int64", "version", backupVersionEcVal)
		return false, err
	}

	currVersion, err := strconv.ParseInt(backupVersionAnnotation, 10, 64)
	if err != nil {
		opts.VMCtx.Logger.Error(err, "failed to parse backup version annotation value to int64", "version", backupVersionAnnotation)
		return false, err
	}

	// If the storedVersion in the backup is older than the annotation, this is a restore.
	if currVersion > storedVersion {
		opts.VMCtx.Logger.Info("Skipping VM backup because restore is detected: current version is greater than backup ExtraConfig version. Wait for VM registration",
			"currentVersion", backupVersionAnnotation, "extraConfigVersion", backupVersionEcVal)
		return false, nil
	}

	return true, nil
}

// getDesiredVMResourceYAMLForBackup returns the encoded and gzipped YAML of the
// given VM, or an empty string if the existing backup is already up-to-date.
func getDesiredVMResourceYAMLForBackup(
	vm *vmopv1.VirtualMachine,
	extraConfig pkgutil.OptionValues) (string, error) {

	curBackup, _ := extraConfig.GetString(backupapi.VMResourceYAMLExtraConfigKey)
	isUpToDate, err := isVMBackupUpToDate(vm, curBackup)
	if err != nil || isUpToDate {
		return "", err
	}

	// Use a copy to avoid modifying the VM object at the end of reconciliation.
	copyVM := vm.DeepCopy()
	trimBackupFields(copyVM)

	// Do not include the status of the VM in the backup.
	copyVM.Status = vmopv1.VirtualMachineStatus{}

	// Backup the updated VM's YAML with encoding and compression.
	return getEncodedVMYaml(copyVM)
}

// trimBackupFields removes the object fields that are not necessary for backup.
func trimBackupFields(obj client.Object) {
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		delete(annotations, corev1.LastAppliedConfigAnnotation)
		obj.SetAnnotations(annotations)
	}

	obj.SetManagedFields(nil)
}

// getEncodedVMYaml returns the encoded and gzipped YAML of the given VM object.
func getEncodedVMYaml(vm *vmopv1.VirtualMachine) (string, error) {
	vmYAML, err := k8syaml.Marshal(vm)
	if err != nil {
		return "", fmt.Errorf("failed to marshal VM into YAML %+v: %w", vm, err)
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

	// Do not compare LastAppliedConfigAnnotation as it's not in the backup.
	curAnnotations := maps.Clone(vm.Annotations)
	delete(curAnnotations, corev1.LastAppliedConfigAnnotation)

	return vm.Generation == backupVM.Generation &&
		maps.Equal(vm.Labels, backupVM.Labels) &&
		maps.Equal(curAnnotations, backupVM.Annotations), nil
}

// getDesiredAdditionalResourceYAMLForBackup returns the encoded and gzipped
// YAML of the given resources, or an empty string if the existing backup is
// already up-to-date (checked by comparing the resource versions).
func getDesiredAdditionalResourceYAMLForBackup(
	resources []client.Object,
	extraConfig pkgutil.OptionValues) (string, error) {

	curBackup, _ := extraConfig.GetString(backupapi.AdditionalResourcesYAMLExtraConfigKey)
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
		trimBackupFields(resource)
		marshaledYaml, err := k8syaml.Marshal(resource)
		if err != nil {
			return "", fmt.Errorf("failed to marshal object %q: %w", resource.GetName(), err)
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

func getDesiredDiskDataForBackup(opts BackupVirtualMachineOptions) (string, string, error) {

	deviceList := object.VirtualDeviceList(opts.VMCtx.MoVM.Config.Hardware.Device)

	var (
		pvcDiskData     []backupapi.PVCDiskData
		classicDiskData []backupapi.ClassicDiskData
	)

	for _, device := range deviceList.SelectByType((*vimtypes.VirtualDisk)(nil)) {
		if disk, ok := device.(*vimtypes.VirtualDisk); ok {
			if b, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
				if pvc, ok := opts.DiskUUIDToPVC[b.Uuid]; ok {
					pvcDiskData = append(pvcDiskData, backupapi.PVCDiskData{
						FileName:    b.FileName,
						PVCName:     pvc.Name,
						AccessModes: backupapi.ToPersistentVolumeAccessModes(pvc.Spec.AccessModes),
						UUID:        b.Uuid,
					})
				} else if _, ok := opts.ClassicDiskUUIDs[b.Uuid]; ok {
					classicDiskData = append(classicDiskData, backupapi.ClassicDiskData{
						FileName: b.FileName,
						UUID:     b.Uuid,
					})
				}
			}
		}
	}

	var pvcDiskDataBackup string
	if len(pvcDiskData) > 0 {
		pvcDiskDataJSON, err := json.Marshal(pvcDiskData)
		if err != nil {
			return "", "", err
		}
		pvcDiskDataBackup, err = pkgutil.EncodeGzipBase64(string(pvcDiskDataJSON))
		if err != nil {
			return "", "", err
		}
	}

	var classicDiskDataBackup string
	if len(classicDiskData) > 0 {
		if pkgcfg.FromContext(opts.VMCtx).Features.VMIncrementalRestore {
			classicDiskDataJSON, err := json.Marshal(classicDiskData)
			if err != nil {
				return "", "", err
			}
			classicDiskDataBackup, err = pkgutil.EncodeGzipBase64(string(classicDiskDataJSON))
			if err != nil {
				return "", "", err
			}
		}
	}

	return pvcDiskDataBackup, classicDiskDataBackup, nil
}

func setBackupVersionAnnotation(vm *vmopv1.VirtualMachine, version string) {
	if vm.Annotations == nil {
		vm.Annotations = map[string]string{}
	}

	vm.Annotations[vmopv1.VirtualMachineBackupVersionAnnotation] = version
}
