// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package constants

import "github.com/vmware-tanzu/vm-operator/pkg"

const (
	// ExtraConfig constants
	ExtraConfigTrue            = "TRUE"
	ExtraConfigFalse           = "FALSE"
	ExtraConfigUnset           = ""
	ExtraConfigGuestInfoPrefix = "guestinfo."

	// VCVMAnnotation Annotation placed on the VM
	VCVMAnnotation = "Virtual Machine managed by the vSphere Virtual Machine service"

	// VMOperatorImageSupportedCheckKey Annotation key to skip validation checks of GuestOS Type
	// TODO: Rename and move to vmoperator-api
	VMOperatorImageSupportedCheckKey     = pkg.VmOperatorKey + "/image-supported-check"
	VMOperatorImageSupportedCheckDisable = "disable"

	// VSphereCustomizationBypassKey Annotation to skip applying VMware Tools Guest Customization.
	VSphereCustomizationBypassKey     = pkg.VmOperatorKey + "/vsphere-customization"
	VSphereCustomizationBypassDisable = "disable"

	// VMOperatorV1Alpha1ExtraConfigKey Special ExtraConfig key for v1alpha1 images.
	VMOperatorV1Alpha1ExtraConfigKey = "guestinfo.vmservice.defer-cloud-init"
	VMOperatorV1Alpha1ConfigReady    = "ready"
	VMOperatorV1Alpha1ConfigEnabled  = "enabled"

	// GOSC Related ExtraConfig keys
	GOSCPendingExtraConfigKey          = "tools.deployPkg.fileName"
	GOSCIgnoreToolsCheckExtraConfigKey = "vmware.tools.gosc.ignoretoolscheck"

	// EnableDiskUUIDExtraConfigKey Enable UUID ExtraConfig key
	EnableDiskUUIDExtraConfigKey = "disk.enableUUID"

	// MMPowerOffVMExtraConfigKey ExtraConfig key to enable DRS to powerOff VMs when underlying host enters into
	// maintenance mode. This is to ensure the maintenance mode workflow is consistent for VMs with vGPU/DDPIO devices.
	MMPowerOffVMExtraConfigKey = "maintenance.vm.evacuation.poweroff"

	// NetPlanVersion points to the version used for Network config.
	// For more information, please see https://cloudinit.readthedocs.io/en/latest/topics/network-config-format-v2.html
	NetPlanVersion = 2

	// VMImageCLVersionAnnotation VirtualMachineImage annotation to cache the last fetched version.
	VMImageCLVersionAnnotation = pkg.VmOperatorKey + "/content-library-version"
	// Version of the VMImageCLVersionAnnotation for the VirtualMachineImage.
	VMImageCLVersionAnnotationVersion = 1

	// 64bitMMIO for Passthrough Devices
	PCIPassthruMMIOOverrideAnnotation = pkg.VmOperatorKey + "/pci-passthru-64bit-mmio-size"
	PCIPassthruMMIOExtraConfigKey     = "pciPassthru.use64bitMMIO"    // nolint:gosec
	PCIPassthruMMIOSizeExtraConfigKey = "pciPassthru.64bitMMIOSizeGB" // nolint:gosec
	PCIPassthruMMIOSizeDefault        = "512"

	// Minimum supported virtual hardware version for persistent volumes
	MinSupportedHWVersionForPVC = 13

	// Firmware Override
	FirmwareOverrideAnnotation = pkg.VmOperatorKey + "/firmware"
)
