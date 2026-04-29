// Copyright (c) 2023-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package consts

const (
	NSX = "nsx"
	VDS = "vds"

	NSXNetworkType = "nsx-t"
	VDSNetworkType = "vsphere-distributed"
	VPCNetworkType = "nsx-t-vpc"

	DefaultVDSNetworkName = "primary"

	WCP  = "wcp"
	KIND = "kind"

	VMServiceCLName = "vmservice"

	HTTPProxyEnv          = "HTTP_PROXY"
	DefaultVMUserName     = "vmware"
	DefaultVMPassword     = "Admin!23"
	AllowTCPForwardingKey = "AllowTcpForwarding"
	SshdConfig            = "/etc/ssh/sshd_config"
	CmdRestartSSHD        = "systemctl restart sshd"
	SshPort               = 22

	JumpboxPodVMName = "jumpbox"

	VMGroupsCapabilityName                  = "supports_VM_service_VM_groups"
	VirtualMachineSnapshotCapabilityName    = "supports_VM_service_VM_snapshots"
	VMPlacementPoliciesCapabilityName       = "supports_VM_service_VM_placement_policies"
	VMAffinityDuringExecutionCapabilityName = "supports_VM_service_VM_affinity_during_execution"
	InventoryContentLibraryCapabilityName   = "supports_inventory_content_library"
	SharedDisksCapabilityName               = "supports_shared_disks_with_VM_service_VMs"
	AllDisksArePVCapabilityName             = "supports_vm_service_all_disks_are_pvcs"
	IaaSComputePoliciesCapabilityName       = "supports_iaas_compute_policies"
)
