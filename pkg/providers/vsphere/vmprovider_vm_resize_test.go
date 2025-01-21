// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
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
			WithContentLibrary: true,
			WithNetworkEnv:     builder.NetworkEnvNamed,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.AsyncSignalDisabled = true
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()
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
		class.Spec.Policies = vmopv1.VirtualMachineClassPolicies{}
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

	updateVMClassPolicies := func(class *vmopv1.VirtualMachineClass, polices vmopv1.VirtualMachineClassPolicies) {
		class.Spec.Policies = polices
		class.Generation++ // Fake client doesn't increment this.
		ExpectWithOffset(1, ctx.Client.Update(ctx, class)).To(Succeed())
	}

	assertExpectedBrownfieldNoResizedClassFields := func(vm *vmopv1.VirtualMachine, class *vmopv1.VirtualMachineClass) {
		_, _, _, exists := vmopv1util.GetLastResizedAnnotation(*vm)
		ExpectWithOffset(1, exists).To(BeFalse(), "LRA not present")

		ExpectWithOffset(1, vm.Status.Class).ToNot(BeNil(), "Status.Class")
		ExpectWithOffset(1, vm.Status.Class.APIVersion).To(Equal(vmopv1.GroupVersion.String()), "Status.Class.APIVersion")
		ExpectWithOffset(1, vm.Status.Class.Kind).To(Equal("VirtualMachineClass"), "Status.Class.Kind")
		ExpectWithOffset(1, vm.Status.Class.Name).To(Equal(class.Name), "Status.Class.Name")
	}

	assertExpectedResizedClassFields := func(vm *vmopv1.VirtualMachine, class *vmopv1.VirtualMachineClass, synced ...bool) {
		inSync := len(synced) == 0 || synced[0]

		name, uid, generation, exists := vmopv1util.GetLastResizedAnnotation(*vm)
		ExpectWithOffset(1, exists).To(BeTrue(), "LRA present")
		ExpectWithOffset(1, name).To(Equal(class.Name), "LRA ClassName")
		ExpectWithOffset(1, uid).To(BeEquivalentTo(class.UID), "LRA UID")
		if inSync {
			ExpectWithOffset(1, generation).To(Equal(class.Generation), "LRA Generation")
		}

		ExpectWithOffset(1, vm.Status.Class).ToNot(BeNil(), "Status.Class")
		ExpectWithOffset(1, vm.Status.Class.APIVersion).To(Equal(vmopv1.GroupVersion.String()), "Status.Class.APIVersion")
		ExpectWithOffset(1, vm.Status.Class.Kind).To(Equal("VirtualMachineClass"), "Status.Class.Kind")
		ExpectWithOffset(1, vm.Status.Class.Name).To(Equal(class.Name), "Status.Class.Name")

		if inSync {
			ExpectWithOffset(1, conditions.IsTrue(vm, vmopv1.VirtualMachineClassConfigurationSynced)).To(BeTrue(), "Synced Condition")
		}
	}

	assertExpectedReservationFields := func(o mo.VirtualMachine, cpuReservation, cpuLimit, memReservation, memLimit int64) {
		cpuAllocation, memAllocation := o.Config.CpuAllocation, o.Config.MemoryAllocation

		ExpectWithOffset(1, cpuAllocation).ToNot(BeNil(), "cpuAllocation")
		ExpectWithOffset(1, cpuAllocation.Reservation).To(HaveValue(BeEquivalentTo(cpuReservation)), "cpu reservation")
		ExpectWithOffset(1, cpuAllocation.Limit).To(HaveValue(BeEquivalentTo(cpuLimit)), "cpu limit")

		ExpectWithOffset(1, memAllocation).ToNot(BeNil(), "memoryAllocation")
		ExpectWithOffset(1, memAllocation.Reservation).To(HaveValue(BeEquivalentTo(memReservation)), "mem reservation")
		ExpectWithOffset(1, memAllocation.Limit).To(HaveValue(BeEquivalentTo(memLimit)), "mem limit")
	}

	assertExpectedNoReservationFields := func(o mo.VirtualMachine) {
		assertExpectedReservationFields(o, 0, -1, 0, -1)
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

				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			})

			Context("Powered off VM", func() {

				Context("Resize NumCPUs/MemoryMB", func() {
					BeforeEach(func() {
						configSpec.NumCPUs = 2
						configSpec.MemoryMB = 1024
					})

					It("Resizes", func() {
						newCS := configSpec
						newCS.NumCPUs = 42
						configSpec.MemoryMB = 8192
						newVMClass := createVMClass(newCS)
						vm.Spec.ClassName = newVMClass.Name

						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))

						assertExpectedResizedClassFields(vm, newVMClass)
					})
				})

				Context("CPU/Memory Reservations", func() {

					Context("No reservations", func() {

						It("Resizes", func() {
							newCS := configSpec
							newVMClass := createVMClass(newCS)
							vm.Spec.ClassName = newVMClass.Name

							vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							var o mo.VirtualMachine
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
							Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
							Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
							assertExpectedNoReservationFields(o)

							assertExpectedResizedClassFields(vm, newVMClass)
						})
					})

					Context("With Reservations", func() {

						It("Resizes", func() {
							newCS := configSpec
							newCS.NumCPUs = 42
							newCS.MemoryMB = 8192
							newVMClass := createVMClass(newCS)
							vm.Spec.ClassName = newVMClass.Name

							polices := vmopv1.VirtualMachineClassPolicies{
								Resources: vmopv1.VirtualMachineClassResources{
									Limits: vmopv1.VirtualMachineResourceSpec{
										Cpu:    resource.MustParse("2"),
										Memory: resource.MustParse("8192Mi"),
									},
									Requests: vmopv1.VirtualMachineResourceSpec{
										Cpu:    resource.MustParse("1"),
										Memory: resource.MustParse("4096Mi"),
									},
								},
							}
							updateVMClassPolicies(newVMClass, polices)

							By("Resize set reservations", func() {
								vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
								Expect(err).ToNot(HaveOccurred())

								var o mo.VirtualMachine
								Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
								Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
								Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))
								assertExpectedReservationFields(o, 1*vcsimCPUFreq, 2*vcsimCPUFreq, 4096, 8192)

								assertExpectedResizedClassFields(vm, newVMClass)
							})

							By("Resize back to initial class removes reservations", func() {
								vm.Spec.ClassName = vmClass.Name
								vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
								Expect(err).ToNot(HaveOccurred())

								var o mo.VirtualMachine
								Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
								Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
								Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
								assertExpectedNoReservationFields(o)

								assertExpectedResizedClassFields(vm, vmClass)
							})
						})
					})
				})
			})

			Context("Powering On VM", func() {
				BeforeEach(func() {
					configSpec.NumCPUs = 2
					configSpec.MemoryMB = 1024
				})

				It("Resizes", func() {
					newCS := configSpec
					newCS.NumCPUs = 42
					newCS.MemoryMB = 8192
					newVMClass := createVMClass(newCS)
					vm.Spec.ClassName = newVMClass.Name

					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
					Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
					Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))
					assertExpectedNoReservationFields(o)

					assertExpectedResizedClassFields(vm, newVMClass)
				})

				Context("With Reservations", func() {

					It("Resizes", func() {
						newCS := configSpec
						newCS.NumCPUs = 42
						newCS.MemoryMB = 8192
						newVMClass := createVMClass(newCS)
						vm.Spec.ClassName = newVMClass.Name

						polices := vmopv1.VirtualMachineClassPolicies{
							Resources: vmopv1.VirtualMachineClassResources{
								Limits: vmopv1.VirtualMachineResourceSpec{
									Cpu:    resource.MustParse("2"),
									Memory: resource.MustParse("8192Mi"),
								},
								Requests: vmopv1.VirtualMachineResourceSpec{
									Cpu:    resource.MustParse("1"),
									Memory: resource.MustParse("4096Mi"),
								},
							},
						}
						updateVMClassPolicies(newVMClass, polices)

						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))
						assertExpectedReservationFields(o, 1*vcsimCPUFreq, 2*vcsimCPUFreq, 4096, 8192)

						assertExpectedResizedClassFields(vm, newVMClass)
					})
				})

				Context("Brownfield VM", func() {

					It("Does not resize when LRA annotation is not present", func() {
						newCS := configSpec
						newCS.NumCPUs = 42
						newCS.MemoryMB = 8192
						updateVMClass(vmClass, newCS)

						// Remove annotation so the VM appears to be from before this feature.
						Expect(vm.Annotations).To(HaveKey(vmopv1util.LastResizedAnnotationKey))
						delete(vm.Annotations, vmopv1util.LastResizedAnnotationKey)

						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
						assertExpectedNoReservationFields(o)

						assertExpectedBrownfieldNoResizedClassFields(vm, vmClass)
					})

					It("Resizes", func() {
						newCS := configSpec
						newCS.NumCPUs = 42
						newCS.MemoryMB = 8192
						newVMClass := createVMClass(newCS)
						vm.Spec.ClassName = newVMClass.Name

						// Simulate what the VM mutation webhook would do by setting the LRA to the prior class name.
						Expect(vm.Annotations).To(HaveKey(vmopv1util.LastResizedAnnotationKey))
						Expect(vmopv1util.SetLastResizedAnnotationClassName(vm, vmClass.Name)).To(Succeed())

						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))
						assertExpectedNoReservationFields(o)

						assertExpectedResizedClassFields(vm, newVMClass)
					})
				})

				Context("Without Same Class Resize Annotation", func() {

					It("Does not resize", func() {
						newCS := configSpec
						newCS.NumCPUs = 42
						newCS.MemoryMB = 8192
						updateVMClass(vmClass, newCS)

						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
						assertExpectedNoReservationFields(o)

						assertExpectedResizedClassFields(vm, vmClass, false)
					})
				})

				Context("With Same Class Resize Annotation", func() {

					It("Resizes", func() {
						newCS := configSpec
						newCS.NumCPUs = 42
						newCS.MemoryMB = 8192
						updateVMClass(vmClass, newCS)

						vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))
						assertExpectedNoReservationFields(o)

						assertExpectedResizedClassFields(vm, vmClass)
					})
				})
			})

			Context("Powered On VM", func() {

				It("Resize Pending", func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

					newCS := configSpec
					newCS.NumCPUs = 42
					newCS.MemoryMB = 8192
					newVMClass := createVMClass(newCS)
					vm.Spec.ClassName = newVMClass.Name

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					By("Does not resize", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
						assertExpectedNoReservationFields(o)
					})

					assertExpectedResizedClassFields(vm, vmClass, false)

					c := conditions.Get(vm, vmopv1.VirtualMachineClassConfigurationSynced)
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("ClassNameChanged"))
				})

				It("Has Same Class Resize Annotation", func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

					newCS := configSpec
					newCS.NumCPUs = 42
					newCS.MemoryMB = 8192
					updateVMClass(vmClass, newCS)

					vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					By("Does not resize", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
						assertExpectedNoReservationFields(o)
					})

					assertExpectedResizedClassFields(vm, vmClass, false)

					c := conditions.Get(vm, vmopv1.VirtualMachineClassConfigurationSynced)
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("ClassUpdated"))
				})
			})

			Context("Same Class Resize Annotation", func() {
				BeforeEach(func() {
					configSpec.NumCPUs = 2
					configSpec.MemoryMB = 1024
				})

				It("Resizes", func() {
					newCS := configSpec
					newCS.NumCPUs = 42
					newCS.MemoryMB = 8192
					updateVMClass(vmClass, newCS)

					By("Does not resize without annotation", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
						assertExpectedNoReservationFields(o)
					})

					vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
					Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))
					assertExpectedNoReservationFields(o)

					assertExpectedResizedClassFields(vm, vmClass)
				})

				It("Resizes brownfield VM", func() {
					newCS := configSpec
					newCS.NumCPUs = 42
					newCS.MemoryMB = 8192
					updateVMClass(vmClass, newCS)

					// Remove annotation so the VM appears to be from before this feature.
					Expect(vm.Annotations).To(HaveKey(vmopv1util.LastResizedAnnotationKey))
					delete(vm.Annotations, vmopv1util.LastResizedAnnotationKey)

					By("Does not resize without same class annotation", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
						assertExpectedNoReservationFields(o)

						Expect(vm.Annotations).ToNot(HaveKey(vmopv1util.LastResizedAnnotationKey))
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineClassConfigurationSynced)).To(BeTrue())
					})

					vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(newCS.NumCPUs))
					Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(newCS.MemoryMB))
					assertExpectedNoReservationFields(o)

					assertExpectedResizedClassFields(vm, vmClass)
				})

				It("Powered On brownfield VM", func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

					// Remove annotation so the VM appears to be from before this feature.
					Expect(vm.Annotations).To(HaveKey(vmopv1util.LastResizedAnnotationKey))
					delete(vm.Annotations, vmopv1util.LastResizedAnnotationKey)

					newCS := configSpec
					newCS.NumCPUs = 42
					newCS.MemoryMB = 8192
					updateVMClass(vmClass, newCS)

					vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation] = ""
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					By("Does not resize powered on VM", func() {
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
						Expect(o.Config.Hardware.NumCPU).To(BeEquivalentTo(configSpec.NumCPUs))
						Expect(o.Config.Hardware.MemoryMB).To(BeEquivalentTo(configSpec.MemoryMB))
						assertExpectedNoReservationFields(o)
					})

					c := conditions.Get(vm, vmopv1.VirtualMachineClassConfigurationSynced)
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("SameClassResize"))
				})
			})
		},

		Entry("Full", true),
		Entry("CPU & Memory", false),
	)

	Context("Devops Overrides", func() {

		var (
			vm         *vmopv1.VirtualMachine
			vmClass    *vmopv1.VirtualMachineClass
			configSpec vimtypes.VirtualMachineConfigSpec
		)

		BeforeEach(func() {
			testConfig.WithVMResize = true

			vm = builder.DummyBasicVirtualMachine("test-vm", "")

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

			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
		})

		Context("ChangeBlockTracking", func() {
			It("Overrides", func() {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
					ChangeBlockTracking: vimtypes.NewBool(true),
				}

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
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

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.ChangeTrackingEnabled).To(HaveValue(BeTrue()))

				// BMV: TBD exactly what we should do in this case.
				// Expect(vm.Status.Class).To(BeNil())

				c := conditions.Get(vm, vmopv1.VirtualMachineClassConfigurationSynced)
				Expect(c).ToNot(BeNil())
				Expect(c.Status).To(Equal(metav1.ConditionUnknown))
				Expect(c.Reason).To(Equal("ClassNotFound"))
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

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.ChangeTrackingEnabled).To(HaveValue(BeTrue()))

				Expect(vm.Status.Class).To(BeNil())
				Expect(conditions.Get(vm, vmopv1.VirtualMachineClassConfigurationSynced)).To(BeNil())
			})
		})
	})
}
