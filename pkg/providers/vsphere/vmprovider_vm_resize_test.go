// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmResizeTests() {

	var (
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary:    true,
			WithNetworkEnv:        builder.NetworkEnvNamed,
			WithWorkloadIsolation: true,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace(testConfig)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	encodedConfigSpec := func(cs vimtypes.VirtualMachineConfigSpec) []byte {
		var w bytes.Buffer
		enc := vimtypes.NewJSONEncoder(&w)
		ExpectWithOffset(2, enc.Encode(cs)).To(Succeed())
		return w.Bytes()
	}

	createOrUpdateAndGetVcVM := func(
		ctx *builder.TestContextForVCSim,
		vm *vmopv1.VirtualMachine) (*object.VirtualMachine, error) {

		err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
		if err != nil {
			return nil, err
		}

		ExpectWithOffset(1, vm.Status.UniqueID).ToNot(BeEmpty())
		vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
		ExpectWithOffset(1, vcVM).ToNot(BeNil())
		return vcVM, nil
	}

	createVMClass := func(cs vimtypes.VirtualMachineConfigSpec, name ...string) *vmopv1.VirtualMachineClass {
		var class *vmopv1.VirtualMachineClass
		if len(name) == 1 {
			class = builder.DummyVirtualMachineClass(name[0])
		} else {
			class = builder.DummyVirtualMachineClassGenName()
		}
		class.Namespace = nsInfo.Namespace
		class.Spec.ConfigSpec = encodedConfigSpec(cs)
		class.Spec.Hardware.Cpus = int64(cs.NumCPUs)
		class.Spec.Hardware.Memory = resource.MustParse(fmt.Sprintf("%dMi", cs.MemoryMB))
		ExpectWithOffset(1, ctx.Client.Create(ctx, class)).To(Succeed())
		return class
	}

	updateVMClass := func(class *vmopv1.VirtualMachineClass, cs vimtypes.VirtualMachineConfigSpec) {
		class.Spec.ConfigSpec = encodedConfigSpec(cs)
		class.Spec.Hardware.Cpus = int64(cs.NumCPUs)
		class.Spec.Hardware.Memory = resource.MustParse(fmt.Sprintf("%dMi", cs.MemoryMB))
		class.Generation++ // Fake client doesn't increment this.
		ExpectWithOffset(1, ctx.Client.Update(ctx, class)).To(Succeed())
	}

	assertExpectedResizedClassFields := func(vm *vmopv1.VirtualMachine, class *vmopv1.VirtualMachineClass) {
		name, uid, generation, exists := vmopv1util.GetLastResizedAnnotation(*vm)
		ExpectWithOffset(1, exists).To(BeTrue())
		ExpectWithOffset(1, name).To(Equal(class.Name))
		ExpectWithOffset(1, uid).To(BeEquivalentTo(class.UID))
		ExpectWithOffset(1, generation).To(Equal(class.Generation))

		ExpectWithOffset(1, vm.Status.Class).ToNot(BeNil())
		ExpectWithOffset(1, vm.Status.Class.APIVersion).To(Equal(vmopv1.GroupVersion.String()))
		ExpectWithOffset(1, vm.Status.Class.Kind).To(Equal("VirtualMachineClass"))
		ExpectWithOffset(1, vm.Status.Class.Name).To(Equal(class.Name))
	}

	DescribeTableSubtree("Resize VM",
		func(fullResize bool) {

			var (
				vm         *vmopv1.VirtualMachine
				vmClass    *vmopv1.VirtualMachineClass
				configSpec vimtypes.VirtualMachineConfigSpec
			)

			BeforeEach(func() {
				vm = builder.DummyBasicVirtualMachine("test-vm", "")

				if fullResize {
					testConfig.WithVMResize = true
				} else {
					testConfig.WithVMResizeCPUMemory = true
				}

				configSpec = vimtypes.VirtualMachineConfigSpec{}
				configSpec.NumCPUs = 1
				configSpec.MemoryMB = 512
			})

			JustBeforeEach(func() {
				vmClass = createVMClass(configSpec, "initial-class")

				clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryImageName}, clusterVMImage)).To(Succeed())

				vm.Namespace = nsInfo.Namespace
				vm.Spec.ClassName = vmClass.Name
				vm.Spec.ImageName = clusterVMImage.Name
				vm.Spec.Image.Kind = cvmiKind
				vm.Spec.Image.Name = clusterVMImage.Name
				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				vm.Spec.StorageClass = ctx.StorageClassName

				_, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("NumCPUs", func() {
				BeforeEach(func() {
					configSpec.NumCPUs = 2
				})

				It("Resizes", func() {
					cs := configSpec
					cs.NumCPUs = 42
					newVMClass := createVMClass(cs)
					vm.Spec.ClassName = newVMClass.Name

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(42))

					assertExpectedResizedClassFields(vm, newVMClass)
				})
			})

			Context("MemoryMB", func() {
				BeforeEach(func() {
					configSpec.MemoryMB = 1024
				})

				It("Resizes", func() {
					cs := configSpec
					cs.MemoryMB = 8192
					newVMClass := createVMClass(cs)
					vm.Spec.ClassName = newVMClass.Name

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(8192))

					assertExpectedResizedClassFields(vm, newVMClass)
				})
			})

			Context("Powering On VM", func() {
				BeforeEach(func() {
					configSpec.NumCPUs = 2
					configSpec.MemoryMB = 1024
				})

				It("Resizes", func() {
					cs := configSpec
					cs.NumCPUs = 42
					cs.MemoryMB = 8192
					newVMClass := createVMClass(cs)
					vm.Spec.ClassName = newVMClass.Name

					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
					Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(42))
					Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(8192))

					assertExpectedResizedClassFields(vm, newVMClass)
				})
			})

			Context("Same Class Resize Annotation", func() {
				BeforeEach(func() {
					configSpec.MemoryMB = 1024
				})

				It("Resizes", func() {
					cs := configSpec
					cs.MemoryMB = 8192
					updateVMClass(vmClass, cs)

					By("Does not resize without annotation", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(1024))
					})

					vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(8192))

					assertExpectedResizedClassFields(vm, vmClass)
				})

				It("Resizes brownfield VM", func() {
					cs := configSpec
					cs.MemoryMB = 8192
					updateVMClass(vmClass, cs)

					// Remove annotation so the VM appears to be from before this feature.
					Expect(vm.Annotations).To(HaveKey(vmopv1util.LastResizedAnnotationKey))
					delete(vm.Annotations, vmopv1util.LastResizedAnnotationKey)

					By("Does not resize without same class annotation", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(1024))

						Expect(vm.Annotations).ToNot(HaveKey(vmopv1util.LastResizedAnnotationKey))
					})

					vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(8192))

					assertExpectedResizedClassFields(vm, vmClass)
				})
			})

			Context("Devops Overrides", func() {
				Context("ChangeBlockTracking", func() {
					It("Overrides", func() {
						vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
							ChangeBlockTracking: vimtypes.NewBool(true),
						}

						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.ChangeTrackingEnabled).To(HaveValue(BeTrue()))

						assertExpectedResizedClassFields(vm, vmClass)
					})
				})

				Context("VM Class does not exist", func() {
					BeforeEach(func() {
						configSpec.ChangeTrackingEnabled = vimtypes.NewBool(false)
					})

					It("Still applies overrides", func() {
						Expect(ctx.Client.Delete(ctx, vmClass)).To(Succeed())

						vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
							ChangeBlockTracking: vimtypes.NewBool(true),
						}

						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.ChangeTrackingEnabled).To(HaveValue(BeTrue()))

						// BMV: TBD exactly what we should do in this case.
						// Expect(vm.Status.Class).To(BeNil())
					})
				})

				Context("VM Classless VMs", func() {
					BeforeEach(func() {
						configSpec.ChangeTrackingEnabled = vimtypes.NewBool(false)
					})

					It("Still applies overrides", func() {
						vm.Spec.ClassName = ""
						vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
							ChangeBlockTracking: vimtypes.NewBool(true),
						}

						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.ChangeTrackingEnabled).To(HaveValue(BeTrue()))

						Expect(vm.Status.Class).To(BeNil())
					})
				})
			})
		},

		Entry("Full", true),
		Entry("CPU & Memory", false),
	)
}
