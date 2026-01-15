// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("VPCPostRestoreBackingFixup", Label(testlabels.VCSim), func() {

	const (
		macAddress1   = "01:02:03:04:05:06"
		macAddress2   = "01:02:03:04:05:07"
		dummySubnetID = "/projects/project-quality/vpcs/foo"
	)

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim

		vmCtx       pkgctx.VirtualMachineContext
		vm          *vmopv1.VirtualMachine
		initObjects []client.Object

		dev1, dev1Restored, dev2 vimtypes.VirtualVmxnet3
		result1, result2         network.NetworkInterfaceResult
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			NumNetworks:    3,
			WithNetworkEnv: builder.NetworkEnvVPC,
		}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vpc-restore-test-vm",
				Namespace: "vpc-restore-test-ns",
				UID:       "vpc-restore-uid",
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		logger := suite.GetLogger().WithName("vpc_restore_test")

		vmCtx = pkgctx.VirtualMachineContext{
			Context: logr.NewContext(ctx, logger),
			Logger:  logger,
			VM:      vm,
		}

		initEthCard := func(idx int) vimtypes.VirtualVmxnet3 {
			dev := vimtypes.VirtualVmxnet3{}
			backing, err := ctx.NetworkRefs[idx].EthernetCardBackingInfo(ctx)
			Expect(err).NotTo(HaveOccurred())
			dev.Backing = backing
			dev.MacAddress = macAddress1
			dev.ExternalId = builder.GetVPCTLogicalSwitchUUID(idx)
			dev.SubnetId = dummySubnetID
			return dev
		}

		dev1 = initEthCard(0)
		dev1Restored = initEthCard(1)
		dev2 = initEthCard(2)
		dev2.MacAddress = macAddress2

		initNetworkResult := func(idx int, dev vimtypes.BaseVirtualEthernetCard) network.NetworkInterfaceResult {
			dvpgMoRef := ctx.NetworkRefs[idx].Reference()

			ethCard := dev.GetVirtualEthernetCard()
			r := network.NetworkInterfaceResult{}
			r.Device = ethCard.GetVirtualDevice()
			r.Backing = object.NewDistributedVirtualPortgroup(ctx.VCClient.Client, dvpgMoRef)
			r.MacAddress = ethCard.MacAddress
			r.ExternalID = ethCard.ExternalId
			return r
		}

		result1 = initNetworkResult(1, dev1Restored.GetVirtualEthernetCard())
		result2 = initNetworkResult(2, dev2.GetVirtualEthernetCard())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	It("returns no operations for no changes", func() {
		currentEthCards := []vimtypes.BaseVirtualDevice{
			&dev2,
		}

		networkResult := network.NetworkInterfaceResults{}
		networkResult.Results = []network.NetworkInterfaceResult{result2}

		devChanges, err := network.VPCPostRestoreBackingFixup(vmCtx, currentEthCards, networkResult)
		Expect(err).NotTo(HaveOccurred())
		Expect(devChanges).To(BeEmpty())
	})

	It("returns edit operation for restored ethernet card", func() {
		currentEthCards := []vimtypes.BaseVirtualDevice{
			&dev1,
		}

		networkResult := network.NetworkInterfaceResults{}
		networkResult.Results = []network.NetworkInterfaceResult{result1, result2}

		devChanges, err := network.VPCPostRestoreBackingFixup(vmCtx, currentEthCards, networkResult)
		Expect(err).NotTo(HaveOccurred())
		Expect(devChanges).To(HaveLen(1))

		change := devChanges[0].GetVirtualDeviceConfigSpec()
		Expect(change.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationEdit))
		ethCard, ok := change.Device.(*vimtypes.VirtualVmxnet3)
		Expect(ok).To(BeTrue())
		Expect(ethCard.ExternalId).To(Equal(dev1Restored.ExternalId))
		Expect(ethCard.MacAddress).To(Equal(dev1Restored.MacAddress))
		Expect(ethCard.SubnetId).To(BeEmpty())
	})
})
