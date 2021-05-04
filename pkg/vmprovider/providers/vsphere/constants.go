// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import "github.com/vmware-tanzu/vm-operator/pkg"

const (
	// Annotation placed on the VM
	VCVMAnnotation = "Virtual Machine managed by the vSphere Virtual Machine service"

	// TODO: Rename and move to vmoperator-api
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

	// Only allow guestinfo for user-supplied ExtraConfig key/values.
	ExtraConfigGuestInfoPrefix = "guestinfo."

	// ExtraConfig key if GOSC is pending.
	GOSCPendingExtraConfigKey = "tools.deployPkg.fileName"

	// ExtraConfig key to mark vm for DRS to power off the vm as part of its maintenance cycle
	MMPowerOffVMExtraConfigKey = "maintenance.vm.evacuation.poweroff"

	// VirtualMachineImage annotation to cache the last fetched version.
	VMImageCLVersionAnnotation = "vmoperator.vmware.com/content-library-version"
)
