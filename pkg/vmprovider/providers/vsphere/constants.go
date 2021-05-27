// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import "github.com/vmware-tanzu/vm-operator/pkg"

const (
	// ExtraConfig constants
	ExtraConfigTrue            = "TRUE"
	ExtraConfigFalse           = "FALSE"
	ExtraConfigUnset           = ""
	ExtraConfigGuestInfoPrefix = "guestinfo."

	// Annotation placed on the VM
	VCVMAnnotation = "Virtual Machine managed by the vSphere Virtual Machine service"

	// TODO: VMSVC-386: Rename and move to vmoperator-api
	// Annotation key to skip validation checks of GuestOS Type
	VMOperatorImageSupportedCheckKey     = pkg.VmOperatorKey + "/image-supported-check"
	VMOperatorImageSupportedCheckDisable = "disable"

	// Annotation to skip applying VMware Tools Guest Customization.
	VSphereCustomizationBypassKey     = pkg.VmOperatorKey + "/vsphere-customization"
	VSphereCustomizationBypassDisable = "disable"

	// Special ExtraConfig key for v1alpha1 images.
	VMOperatorV1Alpha1ExtraConfigKey = "guestinfo.vmservice.defer-cloud-init"
	VMOperatorV1Alpha1ConfigReady    = "ready"
	VMOperatorV1Alpha1ConfigEnabled  = "enabled"

	// GOSC Related ExtraConfig keys
	GOSCPendingExtraConfigKey          = "tools.deployPkg.fileName"
	GOSCIgnoreToolsCheckExtraConfigKey = "vmware.tools.gosc.ignoretoolscheck"

	// Enable UUID ExtraConfig key
	EnableDiskUUIDExtraConfigKey = "disk.enableUUID"

	// ExtraConfig key to mark vm for DRS to power off the vm as part of its maintenance cycle
	MMPowerOffVMExtraConfigKey = "maintenance.vm.evacuation.poweroff"

	// VirtualMachineImage annotation to cache the last fetched version.
	VMImageCLVersionAnnotation = pkg.VmOperatorKey + "/content-library-version"

	// 64bitMMIO for Passthrough Devices
	PCIPassthruMMIOOverrideAnnotation = pkg.VmOperatorKey + "/pci-passthru-64bit-mmio-size"
	PCIPassthruMMIOExtraConfigKey     = "pciPassthru.use64bitMMIO"    // nolint:gosec
	PCIPassthruMMIOSizeExtraConfigKey = "pciPassthru.64bitMMIOSizeGB" // nolint:gosec
	PCIPassthruMMIOSizeDefault        = "512"

	// Minimum supported virtual hardware version for persistent volumes
	MinSupportedHWVersionForPVC = 13
)
