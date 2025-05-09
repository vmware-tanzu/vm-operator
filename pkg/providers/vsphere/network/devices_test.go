// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
)

var _ = Describe("MapEthernetDevicesToSpecIdx", func() {

	var (
		vmCtx       pkgctx.VirtualMachineContext
		devices     object.VirtualDeviceList
		devKeyToIdx map[int32]int
	)

	BeforeEach(func() {
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "map-eth-dev-test",
				Namespace: "map-eth-dev-test",
			},
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{},
			},
		}

		vmCtx = pkgctx.VirtualMachineContext{
			Context: context.Background(),
			Logger:  suite.GetLogger().WithName("list_interfaces_test"),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		devKeyToIdx = network.MapEthernetDevicesToSpecIdx(vmCtx, devices)
	})

	Context("Zips devices and interfaces together", func() {
		BeforeEach(func() {
			dev1 := &vimtypes.VirtualVmxnet3{}
			dev1.Key = 4000
			dev2 := &vimtypes.VirtualE1000e{}
			dev2.Key = 4001
			devices = append(devices, dev1, dev2)

			vmCtx.VM.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
				{
					Name: "eth0",
				},
				{
					Name: "eth1",
				},
			}
		})

		It("returns expected mapping", func() {
			Expect(devKeyToIdx).To(HaveLen(2))
			Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4000), 0))
			Expect(devKeyToIdx).To(HaveKeyWithValue(int32(4001), 1))
		})
	})
})
