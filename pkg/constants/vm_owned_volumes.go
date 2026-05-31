// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package constants

const (
	// VMOwnedVolumesAnnotation is stamped on VMs created while the
	// VMOwnedVolumes feature gate is enabled. Its presence marks the VM as
	// VM-owned storage path for the new ownership-transfer attach/detach path. The
	// annotation is set once at VM creation and is immutable thereafter.
	VMOwnedVolumesAnnotation = "vmoperator.vmware.com/vm-owned-volumes"

	// PVCVolumeOwnershipLabel is the label key on PersistentVolumeClaims that
	// reflects the volume's current ownership state.
	PVCVolumeOwnershipLabel = "cns.vmware.com/volume-ownership"

	// PVC label values for the PVCVolumeOwnershipLabel key.
	PVCOwnershipVMOwned            = "vm-owned"
	PVCOwnershipRetainedBySnapshot = "retained-by-snapshot"
	PVCOwnershipCSIOwned           = "csi-owned"
)
