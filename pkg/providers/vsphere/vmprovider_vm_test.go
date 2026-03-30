// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VirtualMachine", Label(testlabels.VCSim), func() {
	Describe("CNS", vmCNSTests)
	Describe("Cleanup", vmCleanupTests)
	Describe("ConfigSpec", vmConfigSpecTests)
	Describe("ConnectionState", vmConnectionStateTests)
	Describe("Create", Label(testlabels.Create), vmCreateTests)
	Describe("Crypto", Label(testlabels.Crypto), vmCryptoTests)
	Describe("Delete", Label(testlabels.Delete), vmDeleteTests)
	Describe("Disks", vmDisksTests)
	Describe("Group", Label(testlabels.Group), vmGroupTests)
	Describe("GuestHeartbeat", vmGuestHeartbeatTests)
	Describe("GuestID", vmGuestIDTests)
	Describe("HardwareVersion", vmHardwareVersionTests)
	Describe("ISO", vmISOTests)
	Describe("InstanceStorage", vmInstanceStorageTests)
	Describe("Metadata", vmMetadataTests)
	Describe("Misc", vmMiscTests)
	Describe("Network", vmNetworkTests)
	Describe("NPE", vmNPETests)
	Describe("PCI", vmPCITests)
	Describe("Policy", vmPolicyTests)
	Describe("Power", vmPowerStateTests)
	Describe("Resize", vmResizeTests)
	Describe("SetResourcePolicy", vmSetResourcePolicyTests)
	Describe("Snapshot", Label(testlabels.Snapshot), vmSnapshotTests)
	Describe("Storage", vmStorageTests)
	Describe("UnmanagedVolumes", vmUnmanagedVolumesTests)
	Describe("Upgrade", vmUpgradeTests)
	Describe("VKS", Label(testlabels.VKS), vmVKSTests)
	Describe("WebConsole", vmWebConsoleTests)
	Describe("Zone", vmZoneTests)
})

// getVMHomeDisk gets the VM's "home" disk. It makes some assumptions about the backing and disk name.
func getVMHomeDisk(
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	o mo.VirtualMachine) (*vimtypes.VirtualDisk, *vimtypes.VirtualDiskFlatVer2BackingInfo) {

	ExpectWithOffset(1, vcVM.Name()).ToNot(BeEmpty())
	ExpectWithOffset(1, o.Datastore).ToNot(BeEmpty())
	var dso mo.Datastore
	ExpectWithOffset(1, vcVM.Properties(ctx, o.Datastore[0], nil, &dso)).To(Succeed())

	devList := object.VirtualDeviceList(o.Config.Hardware.Device)
	l := devList.SelectByBackingInfo(&vimtypes.VirtualDiskFlatVer2BackingInfo{
		VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
			FileName: fmt.Sprintf("[%s] %s/disk-0.vmdk", dso.Name, vcVM.Name()),
		},
	})
	ExpectWithOffset(1, l).To(HaveLen(1))

	disk := l[0].(*vimtypes.VirtualDisk)
	backing := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)

	return disk, backing
}

//nolint:unparam
func getDVPG(
	ctx *builder.TestContextForVCSim,
	path string) (object.NetworkReference, *object.DistributedVirtualPortgroup) {

	network, err := ctx.Finder.Network(ctx, path)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	dvpg, ok := network.(*object.DistributedVirtualPortgroup)
	ExpectWithOffset(1, ok).To(BeTrue())

	return network, dvpg
}
