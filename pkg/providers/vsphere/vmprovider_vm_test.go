// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VirtualMachine", Label(testlabels.VCSim), func() {
	Describe("ChangeBlockTracking", vmCBTTests)
	Describe("CNS", vmCNSTests)
	Describe("Cleanup", vmCleanupTests)
	Describe("ConfigSpec", vmConfigSpecTests)
	Describe("ConnectionState", vmConnectionStateTests)
	Describe("Create", Label(testlabels.Create), vmCreateTests)
	Describe("ExtraConfig", vmExtraConfigTests)
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
	Describe("Location", vmLocationTests)
})

// getVMHomeDisk gets the VM's "home" disk, i.e. disk-0.vmdk in the VM's own
// directory on the datastore.
//
// The directory is read from the VM's VMX path rather than built from the VM's
// name, because the two deploy paths name it differently: the content library
// deploy names the directory after the VM, whereas Fast Deploy names it after
// the VM's UID. See vmCreatePathNameFromDatastoreRecommendation in
// pkg/providers/vsphere/vmprovider_vm.go.
func getVMHomeDisk(
	o mo.VirtualMachine) (*vimtypes.VirtualDisk, *vimtypes.VirtualDiskFlatVer2BackingInfo) {

	ExpectWithOffset(1, o.Config).ToNot(BeNil())
	ExpectWithOffset(1, o.Config.Files).ToNot(BeNil())
	ExpectWithOffset(1, o.Config.Files.VmPathName).ToNot(BeEmpty())

	devList := object.VirtualDeviceList(o.Config.Hardware.Device)
	l := devList.SelectByBackingInfo(&vimtypes.VirtualDiskFlatVer2BackingInfo{
		VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
			FileName: path.Join(
				path.Dir(o.Config.Files.VmPathName), "disk-0.vmdk"),
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
