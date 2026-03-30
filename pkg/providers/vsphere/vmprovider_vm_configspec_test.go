// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmConfigSpecTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm                   *vmopv1.VirtualMachine
		vmClass              *vmopv1.VirtualMachineClass
		skipCreateOrUpdateVM bool

		vcVM       *object.VirtualMachine
		configSpec *vimtypes.VirtualMachineConfigSpec
		ethCard    vimtypes.VirtualEthernetCard
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
		})
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm", "")

		// Reduce diff from old tests: by default don't create an NIC.
		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true

		testConfig.WithNetworkEnv = builder.NetworkEnvNamed

		ethCard = vimtypes.VirtualEthernetCard{
			VirtualDevice: vimtypes.VirtualDevice{
				Key: 4000,
				DeviceInfo: &vimtypes.Description{
					Label:   "test-configspec-nic-label",
					Summary: "VM Network",
				},
				SlotInfo: &vimtypes.VirtualDevicePciBusSlotInfo{
					VirtualDeviceBusSlotInfo: vimtypes.VirtualDeviceBusSlotInfo{},
					PciSlotNumber:            32,
				},
				ControllerKey: 100,
			},
			AddressType: string(vimtypes.VirtualEthernetCardMacTypeManual),
			MacAddress:  "00:0c:29:93:d7:27",
			ResourceAllocation: &vimtypes.VirtualEthernetCardResourceAllocation{
				Reservation: ptr.To[int64](42),
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMI1 := &vmopv1.ClusterVirtualMachineImage{}

		if testConfig.WithContentLibrary {
			Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name}, clusterVMI1)).To(Succeed())

		} else {
			// BMV: VM creation without CL is broken - and has been for a long while - since we assume
			// the VM Image will always point to a ContentLibrary item.
			// Hack around that with this knob so we can continue to test the VM clone path.
			vsphere.SkipVMImageCLProviderCheck = true

			// Use the default VM created by vcsim as the source.
			clusterVMI1 = builder.DummyClusterVirtualMachineImage("DC0_C0_RP0_VM0")
			Expect(ctx.Client.Create(ctx, clusterVMI1)).To(Succeed())
			conditions.MarkTrue(clusterVMI1, vmopv1.ReadyConditionType)
			Expect(ctx.Client.Status().Update(ctx, clusterVMI1)).To(Succeed())
		}

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI1.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI1.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		if configSpec != nil {
			var w bytes.Buffer
			enc := vimtypes.NewJSONEncoder(&w)
			Expect(enc.Encode(configSpec)).To(Succeed())

			// Update the VM Class with the XML.
			vmClass.Spec.ConfigSpec = w.Bytes()
			Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())
		}

		vm.Spec.Network.Disabled = false
		vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
			{
				Name:    "eth0",
				Network: &vmopv1common.PartialObjectRef{Name: dvpgName},
			},
		}

		if !skipCreateOrUpdateVM {
			var err error
			vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		vcVM = nil
		configSpec = nil

		vsphere.SkipVMImageCLProviderCheck = false

		if vm != nil && !pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
			By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
				Expect(vm.Status.Crypto).To(BeNil())
			})
		}

		vmClass = nil
		vm = nil
		skipCreateOrUpdateVM = false

		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	Context("FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET", func() {
		var instanceUUID string

		BeforeEach(func() {
			skipCreateOrUpdateVM = true
		})

		JustBeforeEach(func() {
			vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
			Expect(err).ToNot(HaveOccurred())
			Expect(vmList).ToNot(BeEmpty())

			vcVM = vmList[0]
			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
			instanceUUID = o.Config.InstanceUuid
			vm.Spec.InstanceUUID = instanceUUID

			powerState, err := vcVM.PowerState(ctx)
			Expect(err).ToNot(HaveOccurred())
			if powerState == vimtypes.VirtualMachinePowerStatePoweredOn {
				tsk, err := vcVM.PowerOff(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(tsk.Wait(ctx)).To(Succeed())
			}
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		})

		When("fss is disabled", func() {
			JustBeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMImportNewNet = false
				})
			})

			assertClassNotFound := func(
				ctx *builder.TestContextForVCSim,
				vmProvider providers.VirtualMachineProviderInterface,
				vm *vmopv1.VirtualMachine,
				className string) {

				vcVM, err := createOrUpdateAndGetVcVM(
					ctx, vmProvider, vm)
				ExpectWithOffset(1, err).ToNot(BeNil())
				ExpectWithOffset(1, err.Error()).To(ContainSubstring(
					fmt.Sprintf(
						"virtualmachineclasses.vmoperator.vmware.com %q not found",
						className)))
				ExpectWithOffset(1, vcVM).To(BeNil())
			}

			When("spec.className is empty", func() {
				JustBeforeEach(func() {
					vm.Spec.ClassName = ""
				})
				When("spec.instanceUUID matches existing VM", func() {
					JustBeforeEach(func() {
						vm.Spec.InstanceUUID = instanceUUID
					})
					It("should error when getting class", func() {
						assertClassNotFound(
							ctx,
							vmProvider,
							vm,
							"")
					})
				})
				When("spec.instanceUUID does not match existing VM", func() {
					JustBeforeEach(func() {
						vm.Spec.InstanceUUID = uuid.NewString()
					})
					It("should error when getting class", func() {
						assertClassNotFound(
							ctx,
							vmProvider,
							vm,
							"")
					})
				})
			})

		})

		When("fss is enabled", func() {

			assertPoweredOnNoVMClassCondition := func() {
				var err error
				vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				ExpectWithOffset(1, vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
				powerState, err := vcVM.PowerState(ctx)
				ExpectWithOffset(1, err).ToNot(HaveOccurred())
				ExpectWithOffset(1, powerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
				ExpectWithOffset(1, conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeFalse())
			}

			JustBeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.VMImportNewNet = true
				})
			})

			When("spec.className is empty", func() {
				JustBeforeEach(func() {
					vm.Spec.ClassName = ""
				})
				When("spec.instanceUUID matches existing VM", func() {
					JustBeforeEach(func() {
						vm.Spec.InstanceUUID = instanceUUID
					})
					It("should synthesize class from vSphere VM and power it on", func() {
						assertPoweredOnNoVMClassCondition()
					})
				})
				When("spec.instanceUUID does not match existing VM", func() {
					JustBeforeEach(func() {
						vm.Spec.InstanceUUID = uuid.NewString()
					})
					It("should return an error", func() {
						var err error
						vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).To(MatchError("cannot synthesize class from nil ConfigInfo"))
						Expect(vcVM).To(BeNil())
					})
				})
			})
		})
	})

	Context("GetVirtualMachineProperties", func() {
		const (
			propName              = "config.name"
			propPowerState        = "runtime.powerState"
			propExtraConfig       = "config.extraConfig"
			propPathName          = "config.files.vmPathName"
			propExtraConfigKeyKey = "vmservice.example"
			propExtraConfigKey    = `config.extraConfig["` + propExtraConfigKeyKey + `"]`
		)
		var (
			err           error
			result        map[string]any
			propertyPaths []string
		)
		AfterEach(func() {
			propertyPaths = nil
		})
		JustBeforeEach(func() {
			if len(propertyPaths) > 0 {
				result, err = vmProvider.GetVirtualMachineProperties(ctx, vm, propertyPaths)
			}
		})
		When("getting "+propExtraConfig, func() {
			BeforeEach(func() {
				propertyPaths = []string{propExtraConfig}
			})
			It("should retrieve a non-zero number of properties", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(HaveLen(0))
			})
		})
		DescribeTable("getting "+propExtraConfigKey,
			func(val any) {
				t, err := vcVM.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
					ExtraConfig: []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   propExtraConfigKeyKey,
							Value: val,
						},
					},
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(t.Wait(ctx)).To(Succeed())

				result, err := vmProvider.GetVirtualMachineProperties(
					ctx,
					vm,
					[]string{propExtraConfigKey})

				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(HaveKeyWithValue(
					propExtraConfigKey,
					vimtypes.OptionValue{
						Key:   propExtraConfigKeyKey,
						Value: val,
					}))
			},
			Entry("value is a string", "Hello, world."),
			Entry("value is a uint8", uint8(8)),
			Entry("value is an int32", int32(32)),
			Entry("value is a float64", float64(64)),
			Entry("value is a bool", true),
		)
		When("getting "+propName, func() {
			BeforeEach(func() {
				propertyPaths = []string{propName}
			})
			It("should retrieve a single property", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(HaveLen(1))
				Expect(result[propName]).To(Equal(vm.Name))
			})
		})
		When("getting "+propPowerState, func() {
			BeforeEach(func() {
				propertyPaths = []string{propPowerState}
			})
			It("should retrieve a single property", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(HaveLen(1))
				switch vm.Spec.PowerState {
				case vmopv1.VirtualMachinePowerStateOn:
					Expect(result[propPowerState]).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
				case vmopv1.VirtualMachinePowerStateOff:
					Expect(result[propPowerState]).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
				case vmopv1.VirtualMachinePowerStateSuspended:
					Expect(result[propPowerState]).To(Equal(vimtypes.VirtualMachinePowerStateSuspended))
				default:
					panic(fmt.Sprintf("invalid power state: %s", vm.Spec.PowerState))
				}
			})
		})
		When("getting "+propPathName, func() {
			BeforeEach(func() {
				propertyPaths = []string{propPathName}
			})
			It("should not be set", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(HaveLen(1))
				Expect(result[propName]).To(BeNil()) // should only be set if cdrom is present
			})
		})
	})

	Context("VM Class has no ConfigSpec", func() {
		BeforeEach(func() {
			configSpec = nil
		})

		It("creates VM", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
			Expect(o.Config.Annotation).To(Equal(constants.VCVMAnnotation))
			Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
			Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
		})
	})

	Context("ConfigSpec specifies annotation", func() {
		BeforeEach(func() {
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				Annotation: "my-annotation",
			}
		})

		It("VM has class annotation", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
			Expect(o.Config.Annotation).To(Equal("my-annotation"))
		})
	})

	Context("ConfigSpec specifies hardware spec", func() {
		BeforeEach(func() {
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				Name:     "config-spec-name-is-not-used",
				NumCPUs:  7,
				MemoryMB: 5102,
			}
		})

		It("CPU and memory from ConfigSpec are ignored", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
			Expect(o.Summary.Config.Name).To(Equal(vm.Name))
			Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
			Expect(o.Summary.Config.NumCpu).ToNot(BeEquivalentTo(configSpec.NumCPUs))
			Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
			Expect(o.Summary.Config.MemorySizeMB).ToNot(BeEquivalentTo(configSpec.MemoryMB))
		})
	})

	Context("VM Class spec CPU reservation & limits are non-zero and ConfigSpec specifies CPU reservation", func() {
		BeforeEach(func() {
			vmClass.Spec.Policies.Resources.Requests.Cpu = resource.MustParse("2")
			vmClass.Spec.Policies.Resources.Limits.Cpu = resource.MustParse("3")

			// Specify a CPU reservation via ConfigSpec. This value should not be honored.
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](6),
				},
			}
		})

		It("VM gets CPU reservation from VM Class spec", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			resources := &vmClass.Spec.Policies.Resources

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			reservation := o.Config.CpuAllocation.Reservation
			Expect(reservation).ToNot(BeNil())
			Expect(*reservation).To(Equal(virtualmachine.CPUQuantityToMhz(resources.Requests.Cpu, vcsimCPUFreq)))
			Expect(*reservation).ToNot(Equal(*configSpec.CpuAllocation.Reservation))

			limit := o.Config.CpuAllocation.Limit
			Expect(limit).ToNot(BeNil())
			Expect(*limit).To(Equal(virtualmachine.CPUQuantityToMhz(resources.Limits.Cpu, vcsimCPUFreq)))
		})
	})

	Context("VM Class spec CPU reservation is zero and ConfigSpec specifies CPU reservation", func() {
		BeforeEach(func() {
			vmClass.Spec.Policies.Resources.Requests.Cpu = resource.MustParse("0")
			vmClass.Spec.Policies.Resources.Limits.Cpu = resource.MustParse("0")

			// Specify a CPU reservation via ConfigSpec
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](6),
				},
			}
		})

		It("VM gets CPU reservation from ConfigSpec", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			reservation := o.Config.CpuAllocation.Reservation
			Expect(reservation).ToNot(BeNil())
			Expect(*reservation).ToNot(BeZero())
			Expect(*reservation).To(Equal(*configSpec.CpuAllocation.Reservation))
		})
	})

	Context("VM Class spec Memory reservation & limits are non-zero and ConfigSpec specifies memory reservation", func() {
		BeforeEach(func() {
			vmClass.Spec.Policies.Resources.Requests.Memory = resource.MustParse("4Mi")
			vmClass.Spec.Policies.Resources.Limits.Memory = resource.MustParse("4Mi")

			// Specify a Memory reservation via ConfigSpec
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](5120),
				},
			}
		})

		It("VM gets memory reservation from VM Class spec", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			resources := &vmClass.Spec.Policies.Resources

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			reservation := o.Config.MemoryAllocation.Reservation
			Expect(reservation).ToNot(BeNil())
			Expect(*reservation).To(Equal(virtualmachine.MemoryQuantityToMb(resources.Requests.Memory)))
			Expect(*reservation).ToNot(Equal(*configSpec.MemoryAllocation.Reservation))

			limit := o.Config.MemoryAllocation.Limit
			Expect(limit).ToNot(BeNil())
			Expect(*limit).To(Equal(virtualmachine.MemoryQuantityToMb(resources.Limits.Memory)))
		})
	})

	Context("VM Class spec Memory reservations are zero and ConfigSpec specifies memory reservation", func() {
		BeforeEach(func() {
			vmClass.Spec.Policies.Resources.Requests.Memory = resource.MustParse("0Mi")
			vmClass.Spec.Policies.Resources.Limits.Memory = resource.MustParse("0Mi")

			// Specify a Memory reservation via ConfigSpec
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				MemoryAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To[int64](5120),
				},
			}
		})

		It("VM gets memory reservation from ConfigSpec", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			reservation := o.Config.MemoryAllocation.Reservation
			Expect(reservation).ToNot(BeNil())
			Expect(*reservation).ToNot(BeZero())
			Expect(*reservation).To(Equal(*configSpec.MemoryAllocation.Reservation))
		})
	})

	Context("VM Class ConfigSpec specifies a network interface", func() {

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvNamed

			// Create the ConfigSpec with an ethernet card.
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualE1000{
							VirtualEthernetCard: ethCard,
						},
					},
				},
			}
		})

		It("Reconfigures the VM with the NIC specified in ConfigSpec", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			devList := object.VirtualDeviceList(o.Config.Hardware.Device)
			l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
			Expect(l).To(HaveLen(1))

			dev := l[0].GetVirtualDevice()
			backing, ok := dev.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
			Expect(ok).Should(BeTrue())
			_, dvpg := getDVPG(ctx, dvpgName)
			Expect(backing.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))

			ethDevice, ok := l[0].(*vimtypes.VirtualE1000)
			Expect(ok).To(BeTrue())
			Expect(ethDevice.AddressType).To(Equal(ethCard.AddressType))
			Expect(ethDevice.MacAddress).To(Equal(ethCard.MacAddress))

			Expect(dev.DeviceInfo).To(Equal(ethCard.VirtualDevice.DeviceInfo))
			Expect(dev.DeviceGroupInfo).To(Equal(ethCard.VirtualDevice.DeviceGroupInfo))
			Expect(dev.SlotInfo).To(Equal(ethCard.VirtualDevice.SlotInfo))
			Expect(dev.ControllerKey).To(Equal(ethCard.VirtualDevice.ControllerKey))
			Expect(ethDevice.ResourceAllocation).ToNot(BeNil())
			Expect(ethDevice.ResourceAllocation.Reservation).ToNot(BeNil())
			Expect(*ethDevice.ResourceAllocation.Reservation).To(Equal(*ethCard.ResourceAllocation.Reservation))
		})
	})

	Context("ConfigSpec does not specify any network interfaces", func() {

		BeforeEach(func() {
			testConfig.WithNetworkEnv = builder.NetworkEnvNamed

			configSpec = &vimtypes.VirtualMachineConfigSpec{}
		})

		It("Reconfigures the VM with the default NIC settings from provider", func() {
			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			devList := object.VirtualDeviceList(o.Config.Hardware.Device)
			l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
			Expect(l).To(HaveLen(1))

			dev := l[0].GetVirtualDevice()
			backing, ok := dev.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
			Expect(ok).Should(BeTrue())
			_, dvpg := getDVPG(ctx, dvpgName)
			Expect(backing.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))
		})
	})

	Context("VM Class Spec and ConfigSpec both contain GPU and DirectPath devices", func() {
		BeforeEach(func() {
			vmClass.Spec.Hardware.Devices = vmopv1.VirtualDevices{
				VGPUDevices: []vmopv1.VGPUDevice{
					{
						ProfileName: "profile-from-class",
					},
				},
				DynamicDirectPathIODevices: []vmopv1.DynamicDirectPathIODevice{
					{
						VendorID:    50,
						DeviceID:    51,
						CustomLabel: "label-from-class",
					},
				},
			}

			// Create the ConfigSpec with a GPU and a DDPIO device.
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "profile-from-config-spec",
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
									AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
										{
											VendorId: 52,
											DeviceId: 53,
										},
									},
									CustomLabel: "label-from-config-spec",
								},
							},
						},
					},
				},
			}
		})

		It("GPU and DirectPath devices from VM Class Spec.Devices are ignored", func() {
			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			devList := object.VirtualDeviceList(o.Config.Hardware.Device)
			p := devList.SelectByType(&vimtypes.VirtualPCIPassthrough{})
			Expect(p).To(HaveLen(2))

			pciDev1 := p[0].GetVirtualDevice()
			pciBacking1, ok1 := pciDev1.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
			Expect(ok1).Should(BeTrue())
			Expect(pciBacking1.Vgpu).To(Equal("profile-from-config-spec"))

			pciDev2 := p[1].GetVirtualDevice()
			pciBacking2, ok2 := pciDev2.Backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
			Expect(ok2).Should(BeTrue())
			Expect(pciBacking2.AllowedDevice).To(HaveLen(1))
			Expect(pciBacking2.AllowedDevice[0].VendorId).To(Equal(int32(52)))
			Expect(pciBacking2.AllowedDevice[0].DeviceId).To(Equal(int32(53)))
			Expect(pciBacking2.CustomLabel).To(Equal("label-from-config-spec"))
		})
	})

	Context("VM Class Config specifies an ethCard, a GPU and a DDPIO device", func() {

		BeforeEach(func() {
			// Create the ConfigSpec with an ethernet card, a GPU and a DDPIO device.
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualE1000{
							VirtualEthernetCard: ethCard,
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
									Vgpu: "SampleProfile2",
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualPCIPassthrough{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
									AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
										{
											VendorId: 52,
											DeviceId: 53,
										},
									},
									CustomLabel: "SampleLabel2",
								},
							},
						},
					},
				},
			}
		})

		It("Reconfigures the VM with a NIC, GPU and DDPIO device specified in ConfigSpec", func() {
			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			devList := object.VirtualDeviceList(o.Config.Hardware.Device)
			l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
			Expect(l).To(HaveLen(1))

			dev := l[0].GetVirtualDevice()
			backing, ok := dev.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
			Expect(ok).Should(BeTrue())
			_, dvpg := getDVPG(ctx, dvpgName)
			Expect(backing.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))

			ethDevice, ok := l[0].(*vimtypes.VirtualE1000)
			Expect(ok).To(BeTrue())
			Expect(ethDevice.AddressType).To(Equal(ethCard.AddressType))
			Expect(dev.DeviceInfo).To(Equal(ethCard.VirtualDevice.DeviceInfo))
			Expect(dev.DeviceGroupInfo).To(Equal(ethCard.VirtualDevice.DeviceGroupInfo))
			Expect(dev.SlotInfo).To(Equal(ethCard.VirtualDevice.SlotInfo))
			Expect(dev.ControllerKey).To(Equal(ethCard.VirtualDevice.ControllerKey))
			Expect(ethDevice.MacAddress).To(Equal(ethCard.MacAddress))
			Expect(ethDevice.ResourceAllocation).ToNot(BeNil())
			Expect(ethDevice.ResourceAllocation.Reservation).ToNot(BeNil())
			Expect(*ethDevice.ResourceAllocation.Reservation).To(Equal(*ethCard.ResourceAllocation.Reservation))

			p := devList.SelectByType(&vimtypes.VirtualPCIPassthrough{})
			Expect(p).To(HaveLen(2))
			pciDev1 := p[0].GetVirtualDevice()
			pciBacking1, ok1 := pciDev1.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
			Expect(ok1).Should(BeTrue())
			Expect(pciBacking1.Vgpu).To(Equal("SampleProfile2"))
			pciDev2 := p[1].GetVirtualDevice()
			pciBacking2, ok2 := pciDev2.Backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
			Expect(ok2).Should(BeTrue())
			Expect(pciBacking2.AllowedDevice).To(HaveLen(1))
			Expect(pciBacking2.AllowedDevice[0].VendorId).To(Equal(int32(52)))
			Expect(pciBacking2.AllowedDevice[0].DeviceId).To(Equal(int32(53)))
			Expect(pciBacking2.CustomLabel).To(Equal("SampleLabel2"))

			// CPU and memory should be from vm class
			Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
			Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
		})
	})

	Context("VM Class Config specifies disks, disk controllers, other miscellaneous devices", func() {
		BeforeEach(func() {
			// Create the ConfigSpec with disks, disk controller and some misc devices: pointing device,
			// video card, etc. This works fine with vcsim and helps with testing adding misc devices.
			// The simulator can still reconfigure the VM with default device types like pointing devices,
			// keyboard, video card, etc. But VC has some restrictions with reconfiguring a VM with new
			// default device types via ConfigSpec and are usually ignored.
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualPointingDevice{
							VirtualDevice: vimtypes.VirtualDevice{
								Backing: &vimtypes.VirtualPointingDeviceDeviceBackingInfo{
									HostPointingDevice: "autodetect",
								},
								Key:           700,
								ControllerKey: 300,
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualPS2Controller{
							VirtualController: vimtypes.VirtualController{
								Device: []int32{700},
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 300,
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualMachineVideoCard{
							UseAutoDetect: ptr.To(false),
							NumDisplays:   1,
							VirtualDevice: vimtypes.VirtualDevice{
								Key:           500,
								ControllerKey: 100,
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualPCIController{
							VirtualController: vimtypes.VirtualController{
								Device: []int32{500},
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 100,
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualDisk{
							CapacityInBytes: 1024,
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -42,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									ThinProvisioned: ptr.To(true),
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								Device: []int32{-42},
							},
						},
					},
				},
			}
		})

		// FIXME: vcsim behavior needs to be closer to real VC here so there aren't dupes
		It("Reconfigures the VM with all misc devices in ConfigSpec, including SCSI disk controller", func() {
			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			devList := object.VirtualDeviceList(o.Config.Hardware.Device)

			// VM already has a default pointing device and the spec adds one more
			// info about the default device is unknown to assert on
			pointingDev := devList.SelectByType(&vimtypes.VirtualPointingDevice{})
			Expect(pointingDev).To(HaveLen(2))
			dev := pointingDev[0].GetVirtualDevice()
			backing, ok := dev.Backing.(*vimtypes.VirtualPointingDeviceDeviceBackingInfo)
			Expect(ok).Should(BeTrue())
			Expect(backing.HostPointingDevice).To(Equal("autodetect"))
			Expect(dev.Key).To(Equal(int32(700)))
			Expect(dev.ControllerKey).To(Equal(int32(300)))

			ps2Controllers := devList.SelectByType(&vimtypes.VirtualPS2Controller{})
			Expect(ps2Controllers).To(HaveLen(1))
			dev = ps2Controllers[0].GetVirtualDevice()
			Expect(dev.Key).To(Equal(int32(300)))

			pciControllers := devList.SelectByType(&vimtypes.VirtualPCIController{})
			Expect(pciControllers).To(HaveLen(1))
			dev = pciControllers[0].GetVirtualDevice()
			Expect(dev.Key).To(Equal(int32(100)))

			// VM already has a default video card and the spec adds one more
			// info about the default device is unknown to assert on
			video := devList.SelectByType(&vimtypes.VirtualMachineVideoCard{})
			Expect(video).To(HaveLen(2))
			dev = video[0].GetVirtualDevice()
			Expect(dev.Key).To(Equal(int32(500)))
			Expect(dev.ControllerKey).To(Equal(int32(100)))

			// SCSI disk controllers may remain due to CNS and RDM.
			diskControllers := devList.SelectByType(&vimtypes.VirtualSCSIController{})
			Expect(diskControllers).To(HaveLen(1))

			// Only preexisting disk should be present on VM -- len: 1
			disks := devList.SelectByType(&vimtypes.VirtualDisk{})
			Expect(disks).To(HaveLen(1))
			dev = disks[0].GetVirtualDevice()
			Expect(dev.Key).ToNot(Equal(int32(-42)))
		})
	})

	Context("VM Class Config does not specify a hardware version", func() {

		Context("VM Class has vGPU and/or DDPIO devices", func() {
			BeforeEach(func() {
				// Create the ConfigSpec with a GPU and a DDPIO device.
				configSpec = &vimtypes.VirtualMachineConfigSpec{
					Name: "dummy-VM",
					DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
										Vgpu: "profile-from-configspec",
									},
								},
							},
						},
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
										AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
											{
												VendorId: 52,
												DeviceId: 53,
											},
										},
										CustomLabel: "label-from-configspec",
									},
								},
							},
						},
					},
				}
			})

			It("creates a VM with a hardware version minimum supported for PCI devices", func() {
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.Version).To(Equal(fmt.Sprintf("vmx-%d", pkgconst.MinSupportedHWVersionForPCIPassthruDevices)))
			})
		})

		Context("VM Class has vGPU and/or DDPIO devices and VM spec has a PVC", func() {
			BeforeEach(func() {
				// Need to create the PVC before creating the VM.
				skipCreateOrUpdateVM = true

				// Create the ConfigSpec with a GPU and a DDPIO device.
				configSpec = &vimtypes.VirtualMachineConfigSpec{
					Name: "dummy-VM",
					DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
										Vgpu: "profile-from-configspec",
									},
								},
							},
						},
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
										AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
											{
												VendorId: 52,
												DeviceId: 53,
											},
										},
										CustomLabel: "label-from-configspec",
									},
								},
							},
						},
					},
				}

				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "dummy-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-claim-1",
								},
							},
						},
					},
				}

				vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:     "dummy-vol",
						Attached: true,
					},
				}
			})

			It("creates a VM with a hardware version minimum supported for PCI devices", func() {
				pvc1 := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-claim-1",
						Namespace: vm.Namespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To(ctx.StorageClassName),
					},
				}
				Expect(ctx.Client.Create(ctx, pvc1)).To(Succeed())

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.Version).To(Equal(fmt.Sprintf("vmx-%d", pkgconst.MinSupportedHWVersionForPCIPassthruDevices)))
			})
		})

		Context("VM spec has a PVC", func() {
			BeforeEach(func() {
				// Need to create the PVC before creating the VM.
				skipCreateOrUpdateVM = true

				vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "dummy-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc-claim-1",
								},
							},
						},
					},
				}

				vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:     "dummy-vol",
						Attached: true,
					},
				}
			})

			It("creates a VM with a hardware version minimum supported for PVCs", func() {
				pvc1 := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-claim-1",
						Namespace: vm.Namespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						StorageClassName: ptr.To(ctx.StorageClassName),
					},
				}
				Expect(ctx.Client.Create(ctx, pvc1)).To(Succeed())

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.Version).To(Equal(fmt.Sprintf("vmx-%d", pkgconst.MinSupportedHWVersionForPVC)))
			})

		})
	})

	Context("VM Class Config specifies a hardware version", func() {
		BeforeEach(func() {
			configSpec = &vimtypes.VirtualMachineConfigSpec{Version: "vmx-14"}
		})

		When("The minimum hardware version on the VMSpec is greater than VMClass", func() {
			BeforeEach(func() {
				vm.Spec.MinHardwareVersion = 15
			})

			It("updates the VM to minimum hardware version from the Spec", func() {
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.Version).To(Equal("vmx-15"))
			})
		})

		When("The minimum hardware version on the VMSpec is less than VMClass", func() {
			BeforeEach(func() {
				vm.Spec.MinHardwareVersion = 13
			})

			It("uses the hardware version from the VMClass", func() {
				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				Expect(o.Config.Version).To(Equal("vmx-14"))
			})
		})
	})

	When("configSpec has disk and disk controllers", func() {
		BeforeEach(func() {
			configSpec = &vimtypes.VirtualMachineConfigSpec{
				Name: "dummy-VM",
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualSATAController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 101,
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 103,
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualNVMEController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 104,
								},
							},
						},
					},
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualDisk{
							CapacityInBytes: 1024,
							VirtualDevice: vimtypes.VirtualDevice{
								Key: -42,
								Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
									ThinProvisioned: ptr.To(true),
								},
							},
						},
					},
				},
			}
		})

		It("creates a VM with disk controllers", func() {
			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			devList := object.VirtualDeviceList(o.Config.Hardware.Device)
			satacont := devList.SelectByType(&vimtypes.VirtualSATAController{})
			Expect(satacont).To(HaveLen(1))
			dev := satacont[0].GetVirtualDevice()
			Expect(dev.Key).To(Equal(int32(101)))

			scsicont := devList.SelectByType(&vimtypes.VirtualSCSIController{})
			Expect(scsicont).To(HaveLen(1))
			dev = scsicont[0].GetVirtualDevice()
			Expect(dev.Key).To(Equal(int32(103)))

			nvmecont := devList.SelectByType(&vimtypes.VirtualNVMEController{})
			Expect(nvmecont).To(HaveLen(1))
			dev = nvmecont[0].GetVirtualDevice()
			Expect(dev.Key).To(Equal(int32(104)))

			// only preexisting disk should be present on VM -- len: 1
			disks := devList.SelectByType(&vimtypes.VirtualDisk{})
			Expect(disks).To(HaveLen(1))
			dev1 := disks[0].GetVirtualDevice()
			Expect(dev1.Key).ToNot(Equal(int32(-42)))
		})
	})
}
