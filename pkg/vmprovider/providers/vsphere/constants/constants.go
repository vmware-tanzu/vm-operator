// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	"github.com/vmware-tanzu/vm-operator/pkg"
)

const (
	ExtraConfigTrue            = "TRUE"
	ExtraConfigFalse           = "FALSE"
	ExtraConfigUnset           = ""
	ExtraConfigGuestInfoPrefix = "guestinfo."

	// VCVMAnnotation Annotation placed on the VM.
	VCVMAnnotation = "Virtual Machine managed by the vSphere Virtual Machine service"

	// VMOperatorImageSupportedCheckKey Annotation key to skip validation checks of GuestOS Type
	// TODO: Rename and move to vmoperator-api.
	VMOperatorImageSupportedCheckKey     = pkg.VMOperatorKey + "/image-supported-check"
	VMOperatorImageSupportedCheckDisable = "disable"

	// VSphereCustomizationBypassKey Annotation to skip applying VMware Tools Guest Customization.
	VSphereCustomizationBypassKey     = pkg.VMOperatorKey + "/vsphere-customization"
	VSphereCustomizationBypassDisable = "disable"

	// VMOperatorV1Alpha1ExtraConfigKey Special ExtraConfig key for v1alpha1 images.
	VMOperatorV1Alpha1ExtraConfigKey = "guestinfo.vmservice.defer-cloud-init"
	VMOperatorV1Alpha1ConfigReady    = "ready"
	VMOperatorV1Alpha1ConfigEnabled  = "enabled"

	// GOSCPendingExtraConfigKey and GOSCIgnoreToolsCheckExtraConfigKey are GOSC Related ExtraConfig keys.
	GOSCPendingExtraConfigKey          = "tools.deployPkg.fileName"
	GOSCIgnoreToolsCheckExtraConfigKey = "vmware.tools.gosc.ignoretoolscheck"

	// EnableDiskUUIDExtraConfigKey Enable UUID ExtraConfig key.
	EnableDiskUUIDExtraConfigKey = "disk.enableUUID"

	// MMPowerOffVMExtraConfigKey ExtraConfig key to enable DRS to powerOff VMs when underlying host enters into
	// maintenance mode. This is to ensure the maintenance mode workflow is consistent for VMs with vGPU/DDPIO devices.
	MMPowerOffVMExtraConfigKey = "maintenance.vm.evacuation.poweroff"

	// NetPlanVersion points to the version used for Network config.
	// For more information, please see https://cloudinit.readthedocs.io/en/latest/topics/network-config-format-v2.html
	NetPlanVersion = 2

	// VMImageCLVersionAnnotation VirtualMachineImage annotation to cache the last fetched version.
	VMImageCLVersionAnnotation = pkg.VMOperatorKey + "/content-library-version"
	// VMImageCLVersionAnnotationVersion is the version of the VMImageCLVersionAnnotation for the VirtualMachineImage.
	VMImageCLVersionAnnotationVersion = 1

	PCIPassthruMMIOOverrideAnnotation = pkg.VMOperatorKey + "/pci-passthru-64bit-mmio-size"
	PCIPassthruMMIOExtraConfigKey     = "pciPassthru.use64bitMMIO"    // nolint:gosec
	PCIPassthruMMIOSizeExtraConfigKey = "pciPassthru.64bitMMIOSizeGB" // nolint:gosec
	PCIPassthruMMIOSizeDefault        = "512"

	// MinSupportedHWVersionForPVC is the supported virtual hardware version for persistent volumes.
	MinSupportedHWVersionForPVC = 13

	// FirmwareOverrideAnnotation is the annotation key used for firmware override.
	FirmwareOverrideAnnotation = pkg.VMOperatorKey + "/firmware"

	CloudInitTypeAnnotation         = pkg.VMOperatorKey + "/cloudinit-type"
	CloudInitTypeValueCloudInitPrep = "cloudinitprep"
	CloudInitTypeValueGuestInfo     = "guestinfo"

	CloudInitGuestInfoMetadata         = "guestinfo.metadata"
	CloudInitGuestInfoMetadataEncoding = "guestinfo.metadata.encoding"
	CloudInitGuestInfoUserdata         = "guestinfo.userdata"
	CloudInitGuestInfoUserdataEncoding = "guestinfo.userdata.encoding"

	// InstanceStoragePVCNamePrefix prefix of auto-generated PVC names.
	InstanceStoragePVCNamePrefix = "instance-pvc-"
	// InstanceStorageLabelKey identifies resources related to instance storage.
	// The primary purpose of this label is to identify instance storage resources such as
	// PVCs and CNSNodeVMAttachments but not for List and kubectl-get of VM resources.
	InstanceStorageLabelKey = "vmoperator.vmware.com/instance-storage-resource"
	// InstanceStoragePVCsBoundAnnotationKey annotation key used to set bound state of all instance storage PVCs.
	InstanceStoragePVCsBoundAnnotationKey = "vmoperator.vmware.com/instance-storage-pvcs-bound"
	// InstanceStoragePVPlacementErrorAnnotationKey annotation key to set PV creation error.
	// CSI reference to this annotation where it is defined:
	// https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/master/pkg/syncer/k8scloudoperator/placement.go
	InstanceStoragePVPlacementErrorAnnotationKey = "failure-domain.beta.vmware.com/storagepool"
	// InstanceStorageSelectedNodeAnnotationKey value corresponds to FQDN of ESXi node that is elected to place instance storage volumes.
	InstanceStorageSelectedNodeAnnotationKey = "vmoperator.vmware.com/instance-storage-selected-node"
	// KubernetesSelectedNodeAnnotationKey annotation key to set selected node on PVC.
	KubernetesSelectedNodeAnnotationKey = "volume.kubernetes.io/selected-node"
	// InstanceStoragePVPlacementErrorPrefix indicates prefix of error value.
	InstanceStoragePVPlacementErrorPrefix = "FAILED_"
	// InstanceStorageNotEnoughResErr is an error constant to indicate not enough resources.
	InstanceStorageNotEnoughResErr = "FAILED_PLACEMENT-NotEnoughResources"
)
