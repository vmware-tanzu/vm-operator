// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	goctx "context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmTests() {

	var (
		initObjects []client.Object
		ctx         *builder.TestContextForVCSim
		nsInfo      builder.WorkloadNamespaceInfo
		testConfig  builder.VCSimTestConfig
		vmProvider  vmprovider.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		testConfig.WithFaultDomains = true
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx.Client, ctx.Recorder)

		nsInfo = ctx.CreateWorkloadNamespace()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
	})

	Context("PlaceVirtualMachine", func() {
		It("places VM in zone", func() {
			vm := builder.DummyVirtualMachine()
			vm.Namespace = nsInfo.Namespace

			vmConfigArgs := vmprovider.VMConfigArgs{}

			err := vmProvider.PlaceVirtualMachine(ctx, vm, vmConfigArgs)
			Expect(err).ToNot(HaveOccurred())

			zoneName, ok := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
			Expect(ok).To(BeTrue())
			Expect(zoneName).To(BeElementOf(ctx.ZoneNames))
		})

		Context("When instance storage is enabled", func() {
			BeforeEach(func() {
				testConfig.WithInstanceStorage = true
			})

			It("places VM on a host", func() {
				storageClass := builder.DummyStorageClass()
				Expect(ctx.Client.Create(ctx, storageClass)).To(Succeed())

				vm := builder.DummyVirtualMachine()
				vm.Namespace = nsInfo.Namespace
				builder.AddDummyInstanceStorageVolume(vm)

				vmConfigArgs := vmprovider.VMConfigArgs{}

				err := vmProvider.PlaceVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
				Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
			})
		})
	})
}

// TODO: ResVmToVirtualMachineImage isn't currently used. Just remove it?
var _ = Describe("ResVmToVirtualMachineImage", func() {

	It("returns a VirtualMachineImage object from an inventory VM with annotations", func() {
		simulator.Test(func(ctx goctx.Context, c *vim25.Client) {
			svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
			obj := object.NewVirtualMachine(c, svm.Reference())

			resVM, err := res.NewVMFromObject(obj)
			Expect(err).To(BeNil())

			// TODO: Need to convert this VM to a vApp (and back).
			// annotations := map[string]string{}
			// annotations[versionKey] = versionVal

			image, err := vsphere.ResVMToVirtualMachineImage(ctx, resVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(image).ToNot(BeNil())
			Expect(image.Name).Should(Equal(obj.Name()))
			// Expect(image.Annotations).ToNot(BeEmpty())
			// Expect(image.Annotations).To(HaveKeyWithValue(versionKey, versionVal))
		})
	})
})
