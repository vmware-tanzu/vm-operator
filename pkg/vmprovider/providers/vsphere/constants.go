// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

const (
	VCVMAnnotation = "Virtual Machine managed by the vSphere Virtual Machine service"

	// Only allow guestinfo for user-supplied ExtraConfig key/values.
	ExtraConfigGuestInfoPrefix = "guestinfo."

	// ExtraConfig key if GOSC is pending.
	GOSCPendingExtraConfigKey = "tools.deployPkg.fileName"

	// ExtraConfig key to mark vm for DRS to power off the vm as part of its maintenance cycle
	MMPowerOffVMExtraConfigKey = "maintenance.vm.evacuation.poweroff"
)
