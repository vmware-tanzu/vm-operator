// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	goctx "context"
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

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

		It("noop when placement isn't needed", func() {
			vm := builder.DummyVirtualMachine()
			vm.Namespace = nsInfo.Namespace

			vmConfigArgs := vmprovider.VMConfigArgs{}

			err := vmProvider.PlaceVirtualMachine(ctx, vm, vmConfigArgs)
			Expect(err).ToNot(HaveOccurred())

			Expect(vm.Labels).ToNot(HaveKey(topology.KubernetesTopologyZoneLabelKey))
			Expect(vm.Annotations).ToNot(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
		})

		Context("When fault domains is enabled", func() {
			BeforeEach(func() {
				testConfig.WithFaultDomains = true
			})

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

	Context("Create/Update/Delete/Exists VirtualMachine", func() {
		var (
			vmConfigArgs vmprovider.VMConfigArgs
			vmImage      *vmopv1alpha1.VirtualMachineImage

			vm *vmopv1alpha1.VirtualMachine
		)

		BeforeEach(func() {
			testConfig.WithContentLibrary = true
		})

		AfterEach(func() {
			vmImage = nil
			vmConfigArgs = vmprovider.VMConfigArgs{}
		})

		JustBeforeEach(func() {
			vmClass := builder.DummyVirtualMachineClass()
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())
			vmImage = builder.DummyVirtualMachineImage(ctx.ContentLibraryImageName)
			Expect(ctx.Client.Create(ctx, vmImage)).To(Succeed())

			vmConfigArgs = vmprovider.VMConfigArgs{
				VMClass:            *vmClass,
				VMImage:            vmImage,
				StorageProfileID:   ctx.StorageProfileID,
				ContentLibraryUUID: ctx.ContentLibraryID,
			}

			vm = builder.DummyBasicVirtualMachine("test-vm", nsInfo.Namespace)
			vm.Spec.ImageName = vmImage.Name
			vm.Spec.ClassName = vmClass.Name
		})

		Context("Create VM", func() {

			It("Basic Create VM", func() {
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Status.Phase).To(Equal(vmopv1alpha1.Created))

				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				Expect(vcVM).ToNot(BeNil())

				Expect(vcVM.InventoryPath).To(HaveSuffix(fmt.Sprintf("/%s/%s", nsInfo.Namespace, vm.Name)))

				rp, err := vcVM.ResourcePool(ctx)
				Expect(err).ToNot(HaveOccurred())
				nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "")
				Expect(nsRP).ToNot(BeNil())
				Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))

				// TODO: More assertions!
			})

			Context("When fault domains is enabled", func() {
				BeforeEach(func() {
					testConfig.WithFaultDomains = true
				})

				It("creates VM in assigned zone", func() {
					azName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))] //nolint:gosec
					vm.Labels[topology.KubernetesTopologyZoneLabelKey] = azName

					err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
					Expect(err).ToNot(HaveOccurred())

					vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
					Expect(vcVM).ToNot(BeNil())

					By("VM is created in the zone's ResourcePool", func() {
						rp, err := vcVM.ResourcePool(ctx)
						Expect(err).ToNot(HaveOccurred())
						nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, azName)
						Expect(nsRP).ToNot(BeNil())
						Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
					})
				})
			})
		})

		Context("Does VM Exist", func() {

			It("returns true when VM exists", func() {
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				exists, err := vmProvider.DoesVirtualMachineExist(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())
			})

			It("returns false when VM does not exist", func() {
				Expect(vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)).To(Succeed())
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())

				exists, err := vmProvider.DoesVirtualMachineExist(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})

		Context("Update VM", func() {
			JustBeforeEach(func() {
				// Likely "soon" we'll just fold the Placement, Create and Update VM calls into
				// a single CreateOrUpdateVM() method.
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())

				err = vmProvider.UpdateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Basic VM Status fields", func() {
				Expect(vm.Status.Phase).To(Equal(vmopv1alpha1.Created))
				Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))

				uniqueID := vm.Status.UniqueID
				vcVM := ctx.GetVMFromMoID(uniqueID)
				Expect(vcVM).ToNot(BeNil())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
				Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))
				Expect(vm.Status.Host).ToNot(BeEmpty())

				vmClass := vmConfigArgs.VMClass
				Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
				Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
			})
		})

		Context("Delete VM", func() {
			JustBeforeEach(func() {
				err := vmProvider.CreateVirtualMachine(ctx, vm, vmConfigArgs)
				Expect(err).ToNot(HaveOccurred())
			})

			It("when the VM is off", func() {
				uniqueID := vm.Status.UniqueID
				Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			})

			It("when the VM is on", func() {
				uniqueID := vm.Status.UniqueID

				vcVM := ctx.GetVMFromMoID(uniqueID)
				Expect(vcVM).ToNot(BeNil())
				task, err := vcVM.PowerOn(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())

				// This checks that we power off the VM prior to deletion.
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			})

			It("returns NotFound when VM does not exist", func() {
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				err := vmProvider.DeleteVirtualMachine(ctx, vm)
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
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
