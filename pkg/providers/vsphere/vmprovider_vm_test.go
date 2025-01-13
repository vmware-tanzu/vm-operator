// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vimcrypto "github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	// Hardcoded vcsim CPU frequency.
	vcsimCPUFreq = 2294
)

//nolint:gocyclo // allowed is 30, this function is 32
func vmTests() {

	const (
		// Default network created for free by vcsim.
		dvpgName = "DC0_DVPG0"
	)

	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo
	)

	BeforeEach(func() {
		parentCtx = ctxop.WithContext(pkgcfg.NewContext())
		parentCtx = ovfcache.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateDisabled = true
			config.AsyncSignalDisabled = true
		})
		testConfig = builder.VCSimTestConfig{
			WithContentLibrary:    true,
			WithWorkloadIsolation: true,
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
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

	Context("Create/Update/Delete VirtualMachine", func() {
		var (
			vm                   *vmopv1.VirtualMachine
			vmClass              *vmopv1.VirtualMachineClass
			skipCreateOrUpdateVM bool
		)

		BeforeEach(func() {
			vmClass = builder.DummyVirtualMachineClassGenName()
			vm = builder.DummyBasicVirtualMachine("test-vm", "")

			// Reduce diff from old tests: by default don't create an NIC.
			if vm.Spec.Network == nil {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
			}
			vm.Spec.Network.Disabled = true
		})

		AfterEach(func() {
			if vm != nil && !pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
				By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
					Expect(vm.Status.Crypto).To(BeNil())
				})
			}

			vmClass = nil
			vm = nil
			skipCreateOrUpdateVM = false
		})

		JustBeforeEach(func() {
			vmClass.Namespace = nsInfo.Namespace
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

			clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}
			if testConfig.WithContentLibrary {
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryImageName}, clusterVMImage)).To(Succeed())
			} else {
				// BMV: VM creation without CL is broken - and has been for a long while - since we assume
				// the VM Image will always point to a ContentLibrary item.
				// Hack around that with this knob so we can continue to test the VM clone path.
				vsphere.SkipVMImageCLProviderCheck = true

				// Use the default VM created by vcsim as the source.
				clusterVMImage = builder.DummyClusterVirtualMachineImage("DC0_C0_RP0_VM0")
				Expect(ctx.Client.Create(ctx, clusterVMImage)).To(Succeed())
				conditions.MarkTrue(clusterVMImage, vmopv1.ReadyConditionType)
				Expect(ctx.Client.Status().Update(ctx, clusterVMImage)).To(Succeed())
			}

			vm.Namespace = nsInfo.Namespace
			vm.Spec.ClassName = vmClass.Name
			vm.Spec.ImageName = clusterVMImage.Name
			vm.Spec.Image.Kind = cvmiKind
			vm.Spec.Image.Name = clusterVMImage.Name
			vm.Spec.StorageClass = ctx.StorageClassName
		})

		AfterEach(func() {
			vsphere.SkipVMImageCLProviderCheck = false
		})

		Context("VM Class and ConfigSpec", func() {

			var (
				vcVM       *object.VirtualMachine
				configSpec *vimtypes.VirtualMachineConfigSpec
				ethCard    vimtypes.VirtualEthernetCard
			)

			BeforeEach(func() {
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
						Network: &common.PartialObjectRef{Name: dvpgName},
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
			})

			Context("FSS_WCP_MOBILITY_VM_IMPORT_NEW_NET", func() {

				BeforeEach(func() {
					skipCreateOrUpdateVM = true
				})

				JustBeforeEach(func() {
					vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
					Expect(err).ToNot(HaveOccurred())
					Expect(vmList).ToNot(BeEmpty())

					vcVM = vmList[0]
					vm.Spec.BiosUUID = vcVM.UUID(ctx)

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

					assertClassNotFound := func(className string) {
						var err error
						vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						ExpectWithOffset(1, err).ToNot(BeNil())
						ExpectWithOffset(1, err.Error()).To(ContainSubstring(
							fmt.Sprintf("virtualmachineclasses.vmoperator.vmware.com %q not found", className)))
						ExpectWithOffset(1, vcVM).To(BeNil())
					}

					When("spec.className is empty", func() {
						JustBeforeEach(func() {
							vm.Spec.ClassName = ""
						})
						When("spec.biosUUID matches existing VM", func() {
							JustBeforeEach(func() {
								vm.Spec.BiosUUID = vcVM.UUID(ctx)
							})
							It("should error when getting class", func() {
								assertClassNotFound("")
							})
						})
						When("spec.biosUUID does not match existing VM", func() {
							JustBeforeEach(func() {
								vm.Spec.BiosUUID = uuid.NewString()
							})
							It("should error when getting class", func() {
								assertClassNotFound("")
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
						When("spec.biosUUID matches existing VM", func() {
							JustBeforeEach(func() {
								vm.Spec.BiosUUID = vcVM.UUID(ctx)
							})
							It("should synthesize class from vSphere VM and power it on", func() {
								assertPoweredOnNoVMClassCondition()
							})
						})
						When("spec.biosUUID does not match existing VM", func() {
							JustBeforeEach(func() {
								vm.Spec.BiosUUID = uuid.NewString()
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
		})

		Context("CreateOrUpdate VM", func() {

			var zoneName string

			JustBeforeEach(func() {
				zoneName = ctx.GetFirstZoneName()
				// Explicitly place the VM into one of the zones that the test context will create.
				vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
			})

			It("Basic VM", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				By("has VC UUID annotation set", func() {
					Expect(vm.Annotations).Should(HaveKeyWithValue(vmopv1.ManagerID, ctx.VCClient.Client.ServiceContent.About.InstanceUuid))
				})

				By("has expected Status values", func() {
					Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))
					Expect(vm.Status.Host).ToNot(BeEmpty())
					Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
					Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))

					Expect(vm.Status.Class).ToNot(BeNil())
					Expect(vm.Status.Class.Name).To(Equal(vm.Spec.ClassName))
					Expect(vm.Status.Class.APIVersion).To(Equal(vmopv1.GroupVersion.String()))

					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionStorageReady)).To(BeTrue())
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

					By("did not have VMSetResourcePool", func() {
						Expect(vm.Spec.Reserved).To(BeNil())
						Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady)).To(BeFalse())
					})
					By("did not have Bootstrap", func() {
						Expect(vm.Spec.Bootstrap).To(BeNil())
						Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
					})
					By("did not have Network", func() {
						Expect(vm.Spec.Network.Disabled).To(BeTrue())
						Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeFalse())
					})
				})

				By("has expected inventory path", func() {
					Expect(vcVM.InventoryPath).To(HaveSuffix(fmt.Sprintf("/%s/%s", nsInfo.Namespace, vm.Name)))
				})

				By("has expected namespace resource pool", func() {
					rp, err := vcVM.ResourcePool(ctx)
					Expect(err).ToNot(HaveOccurred())
					nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")
					Expect(nsRP).ToNot(BeNil())
					Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
				})

				By("has expected power state", func() {
					Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
				})

				vmClassRes := &vmClass.Spec.Policies.Resources

				By("has expected CpuAllocation", func() {
					Expect(o.Config.CpuAllocation).ToNot(BeNil())

					reservation := o.Config.CpuAllocation.Reservation
					Expect(reservation).ToNot(BeNil())
					Expect(*reservation).To(Equal(virtualmachine.CPUQuantityToMhz(vmClassRes.Requests.Cpu, vcsimCPUFreq)))
					limit := o.Config.CpuAllocation.Limit
					Expect(limit).ToNot(BeNil())
					Expect(*limit).To(Equal(virtualmachine.CPUQuantityToMhz(vmClassRes.Limits.Cpu, vcsimCPUFreq)))
				})

				By("has expected MemoryAllocation", func() {
					Expect(o.Config.MemoryAllocation).ToNot(BeNil())

					reservation := o.Config.MemoryAllocation.Reservation
					Expect(reservation).ToNot(BeNil())
					Expect(*reservation).To(Equal(virtualmachine.MemoryQuantityToMb(vmClassRes.Requests.Memory)))
					limit := o.Config.MemoryAllocation.Limit
					Expect(limit).ToNot(BeNil())
					Expect(*limit).To(Equal(virtualmachine.MemoryQuantityToMb(vmClassRes.Limits.Memory)))
				})

				By("has expected hardware config", func() {
					Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
					Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
				})

				By("has expected backup ExtraConfig key", func() {
					Expect(o.Config.ExtraConfig).ToNot(BeNil())

					ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()
					Expect(ecMap).To(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))
				})

				// TODO: More assertions!
			})

			When("using async create", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
						config.AsyncCreateDisabled = false
						config.AsyncSignalDisabled = false
						config.Features.WorkloadDomainIsolation = true
					})
				})
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.MaxDeployThreadsOnProvider = 16
					})
				})

				It("should succeed", func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.UniqueID).ToNot(BeEmpty())
				})

				When("there is an error getting the pre-reqs", func() {
					It("should not prevent a subsequent create attempt from going through", func() {
						imgName := vm.Spec.Image.Name
						vm.Spec.Image.Name = "does-not-exist"
						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(
							"clustervirtualmachineimages.vmoperator.vmware.com \"does-not-exist\" not found: " +
								"clustervirtualmachineimages.vmoperator.vmware.com \"does-not-exist\" not found"))
						vm.Spec.Image.Name = imgName
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.UniqueID).ToNot(BeEmpty())
					})
				})

				// Please note this test uses FlakeAttempts(5) due to the
				// validation of some predictable-over-time behavior.
				When("there is a reconcile in progress", FlakeAttempts(5), func() {
					When("there is a duplicate create", func() {
						It("should return ErrReconcileInProgress", func() {
							var (
								errs   []error
								errsMu sync.Mutex
								done   sync.WaitGroup
								start  = make(chan struct{})
							)

							// Set up five goroutines that race to
							// create the VM first.
							for i := 0; i < 5; i++ {
								done.Add(1)
								go func(copyOfVM *vmopv1.VirtualMachine) {
									defer done.Done()
									<-start
									err := createOrUpdateVM(ctx, vmProvider, copyOfVM)
									if err != nil {
										errsMu.Lock()
										errs = append(errs, err)
										errsMu.Unlock()
									} else {
										vm = copyOfVM
									}
								}(vm.DeepCopy())
							}

							close(start)

							done.Wait()

							Expect(errs).To(HaveLen(4))

							Expect(errs).Should(ConsistOf(
								providers.ErrReconcileInProgress,
								providers.ErrReconcileInProgress,
								providers.ErrReconcileInProgress,
								providers.ErrReconcileInProgress,
							))

							Expect(vm.Status.UniqueID).ToNot(BeEmpty())
						})
					})

					When("there is a delete during async create", func() {
						It("should return ErrReconcileInProgress", func() {
							chanCreateErrs, createErr := vmProvider.CreateOrUpdateVirtualMachineAsync(ctx, vm)
							deleteErr := vmProvider.DeleteVirtualMachine(ctx, vm)

							Expect(createErr).ToNot(HaveOccurred())
							Expect(errors.Is(deleteErr, providers.ErrReconcileInProgress))

							var createErrs []error
							for e := range chanCreateErrs {
								if e != nil {
									createErrs = append(createErrs, e)
								}
							}
							Expect(createErrs).Should(BeEmpty())

							Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
						})
					})
				})
			})

			It("TKG VM", func() {
				if vm.Labels == nil {
					vm.Labels = make(map[string]string)
				}
				vm.Labels[kubeutil.CAPVClusterRoleLabelKey] = ""
				vm.Labels[kubeutil.CAPWClusterRoleLabelKey] = ""

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				By("does not have any backup ExtraConfig key", func() {
					Expect(o.Config.ExtraConfig).ToNot(BeNil())
					ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()
					Expect(ecMap).ToNot(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))
				})
			})
			It("TKG VM that has opt-in annotation gets the backup EC", func() {
				if vm.Labels == nil {
					vm.Labels = make(map[string]string)
				}
				vm.Labels[kubeutil.CAPVClusterRoleLabelKey] = ""
				vm.Labels[kubeutil.CAPWClusterRoleLabelKey] = ""

				vm.Annotations[vmopv1.ForceEnableBackupAnnotation] = "true"

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				By("has backup ExtraConfig key", func() {
					Expect(o.Config.ExtraConfig).ToNot(BeNil())
					ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()
					Expect(ecMap).To(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))
				})
			})

			Context("Crypto", Label(testlabels.Crypto), func() {
				BeforeEach(func() {
					pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
						config.Features.BringYourOwnEncryptionKey = true
					})
					parentCtx = vmconfig.WithContext(parentCtx)
					parentCtx = vmconfig.Register(parentCtx, crypto.New())

					vm.Spec.Crypto = &vmopv1.VirtualMachineCryptoSpec{}
				})
				JustBeforeEach(func() {
					var storageClass storagev1.StorageClass
					Expect(ctx.Client.Get(
						ctx,
						client.ObjectKey{Name: ctx.EncryptedStorageClassName},
						&storageClass)).To(Succeed())
					Expect(kubeutil.MarkEncryptedStorageClass(
						ctx,
						ctx.Client,
						storageClass,
						true)).To(Succeed())
				})

				useExistingVM := func(
					cryptoSpec vimtypes.BaseCryptoSpec, vTPM bool) {

					vmList, err := ctx.Finder.VirtualMachineList(ctx, "*")
					ExpectWithOffset(1, err).ToNot(HaveOccurred())
					ExpectWithOffset(1, vmList).ToNot(BeEmpty())

					vcVM := vmList[0]
					vm.Spec.BiosUUID = vcVM.UUID(ctx)

					powerState, err := vcVM.PowerState(ctx)
					ExpectWithOffset(1, err).ToNot(HaveOccurred())
					if powerState == vimtypes.VirtualMachinePowerStatePoweredOn {
						tsk, err := vcVM.PowerOff(ctx)
						ExpectWithOffset(1, err).ToNot(HaveOccurred())
						ExpectWithOffset(1, tsk.Wait(ctx)).To(Succeed())
					}

					if cryptoSpec != nil || vTPM {
						configSpec := vimtypes.VirtualMachineConfigSpec{
							Crypto: cryptoSpec,
						}
						if vTPM {
							configSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
								&vimtypes.VirtualDeviceConfigSpec{
									Device: &vimtypes.VirtualTPM{
										VirtualDevice: vimtypes.VirtualDevice{
											Key:           -1000,
											ControllerKey: 100,
										},
									},
									Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
								},
							}
						}
						tsk, err := vcVM.Reconfigure(ctx, configSpec)
						ExpectWithOffset(1, err).ToNot(HaveOccurred())
						ExpectWithOffset(1, tsk.Wait(ctx)).To(Succeed())
					}
				}

				When("deploying an encrypted vm", func() {
					JustBeforeEach(func() {
						vm.Spec.StorageClass = ctx.EncryptedStorageClassName
					})

					When("using a default provider", func() {

						When("default provider is native key provider", func() {
							JustBeforeEach(func() {
								m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
								Expect(m.MarkDefault(ctx, ctx.NativeKeyProviderID)).To(Succeed())
							})

							When("using sync create", func() {
								BeforeEach(func() {
									pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
										config.AsyncCreateDisabled = true
										config.AsyncSignalDisabled = false
										config.Features.WorkloadDomainIsolation = true
									})
								})
								It("should succeed", func() {
									Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
									Expect(vm.Status.Crypto).ToNot(BeNil())
									Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
										[]vmopv1.VirtualMachineEncryptionType{
											vmopv1.VirtualMachineEncryptionTypeConfig,
										}))
									Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
									Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
									Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
								})
							})

							When("using async create", func() {
								BeforeEach(func() {
									pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
										config.AsyncCreateDisabled = false
										config.AsyncSignalDisabled = false
										config.Features.WorkloadDomainIsolation = true
									})
								})
								It("should succeed", func() {
									Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
									Expect(vm.Status.Crypto).ToNot(BeNil())
									Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
										[]vmopv1.VirtualMachineEncryptionType{
											vmopv1.VirtualMachineEncryptionTypeConfig,
										}))
									Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
									Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
									Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
								})

								// Please note this test uses FlakeAttempts(5) due to the
								// validation of some predictable-over-time behavior.
								When("there is a duplicate create", FlakeAttempts(5), func() {
									JustBeforeEach(func() {
										pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
											config.MaxDeployThreadsOnProvider = 16
										})
									})
									It("should return ErrReconcileInProgress", func() {
										var (
											errs   []error
											errsMu sync.Mutex
											done   sync.WaitGroup
											start  = make(chan struct{})
										)

										// Set up five goroutines that race to
										// create the VM first.
										for i := 0; i < 5; i++ {
											done.Add(1)
											go func(copyOfVM *vmopv1.VirtualMachine) {
												defer done.Done()
												<-start
												err := createOrUpdateVM(ctx, vmProvider, copyOfVM)
												if err != nil {
													errsMu.Lock()
													errs = append(errs, err)
													errsMu.Unlock()
												} else {
													vm = copyOfVM
												}
											}(vm.DeepCopy())
										}

										close(start)

										done.Wait()

										Expect(errs).To(HaveLen(4))

										Expect(errs).Should(ConsistOf(
											providers.ErrReconcileInProgress,
											providers.ErrReconcileInProgress,
											providers.ErrReconcileInProgress,
											providers.ErrReconcileInProgress,
										))

										Expect(vm.Status.Crypto).ToNot(BeNil())
										Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
											[]vmopv1.VirtualMachineEncryptionType{
												vmopv1.VirtualMachineEncryptionTypeConfig,
											}))
										Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
										Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
										Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
									})
								})
							})
						})

						When("default provider is not native key provider", func() {
							JustBeforeEach(func() {
								m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
								Expect(m.MarkDefault(ctx, ctx.EncryptionClass1ProviderID)).To(Succeed())
							})

							It("should succeed", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).ToNot(BeNil())
								Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
									[]vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
									}))
								Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass1ProviderID))
								Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})
						})
					})

					Context("using an encryption class", func() {

						JustBeforeEach(func() {
							vm.Spec.Crypto.EncryptionClassName = ctx.EncryptionClass1Name
						})

						It("should succeed", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.Crypto).ToNot(BeNil())
							Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
								[]vmopv1.VirtualMachineEncryptionType{
									vmopv1.VirtualMachineEncryptionTypeConfig,
								}))
							Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass1ProviderID))
							Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass1KeyID))
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})
					})
				})

				When("encrypting an existing vm", func() {
					var (
						hasVTPM bool
					)

					BeforeEach(func() {
						hasVTPM = false
					})

					JustBeforeEach(func() {
						useExistingVM(nil, hasVTPM)
						vm.Spec.StorageClass = ctx.EncryptedStorageClassName
					})

					When("using a default provider", func() {

						When("default provider is native key provider", func() {
							JustBeforeEach(func() {
								m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
								Expect(m.MarkDefault(ctx, ctx.NativeKeyProviderID)).To(Succeed())
							})

							It("should succeed", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).To(BeNil())

								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).ToNot(BeNil())

								Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
									[]vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
									}))
								Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
								Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})
						})

						When("default provider is not native key provider", func() {
							JustBeforeEach(func() {
								m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
								Expect(m.MarkDefault(ctx, ctx.EncryptionClass1ProviderID)).To(Succeed())
							})

							It("should succeed", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).To(BeNil())

								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).ToNot(BeNil())

								Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
									[]vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
									}))
								Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass1ProviderID))
								Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})
						})
					})

					Context("using an encryption class", func() {

						JustBeforeEach(func() {
							vm.Spec.Crypto.EncryptionClassName = ctx.EncryptionClass2Name
						})

						It("should succeed", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.Crypto).To(BeNil())

							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.Crypto).ToNot(BeNil())

							Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
								[]vmopv1.VirtualMachineEncryptionType{
									vmopv1.VirtualMachineEncryptionTypeConfig,
								}))
							Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
							Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})

						When("using a non-encryption storage class", func() {
							JustBeforeEach(func() {
								vm.Spec.StorageClass = ctx.StorageClassName
							})

							When("there is no vTPM", func() {
								It("should not error, but have condition", func() {
									Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
									Expect(vm.Status.Crypto).To(BeNil())
									c := conditions.Get(vm, vmopv1.VirtualMachineEncryptionSynced)
									Expect(c).ToNot(BeNil())
									Expect(c.Status).To(Equal(metav1.ConditionFalse))
									Expect(c.Reason).To(Equal("InvalidState"))
									Expect(c.Message).To(Equal("Must use encryption storage class or have vTPM when encrypting vm"))
								})
							})

							When("there is a vTPM", func() {
								BeforeEach(func() {
									hasVTPM = true
								})
								It("should succeed", func() {
									Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
									Expect(vm.Status.Crypto).To(BeNil())

									Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
									Expect(vm.Status.Crypto).ToNot(BeNil())

									Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
										[]vmopv1.VirtualMachineEncryptionType{
											vmopv1.VirtualMachineEncryptionTypeConfig,
										}))
									Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
									Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
									Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
								})
							})
						})
					})
				})

				When("recrypting a vm", func() {
					var (
						hasVTPM bool
					)

					BeforeEach(func() {
						hasVTPM = false
					})

					JustBeforeEach(func() {
						useExistingVM(&vimtypes.CryptoSpecEncrypt{
							CryptoKeyId: vimtypes.CryptoKeyId{
								KeyId: nsInfo.EncryptionClass1KeyID,
								ProviderId: &vimtypes.KeyProviderId{
									Id: ctx.EncryptionClass1ProviderID,
								},
							},
						}, hasVTPM)
						vm.Spec.StorageClass = ctx.EncryptedStorageClassName
					})

					When("using a default provider", func() {

						When("default provider is native key provider", func() {
							JustBeforeEach(func() {
								m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
								Expect(m.MarkDefault(ctx, ctx.NativeKeyProviderID)).To(Succeed())
							})

							It("should succeed", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).To(BeNil())

								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).ToNot(BeNil())

								Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
									[]vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
									}))
								Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.NativeKeyProviderID))
								Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
								Expect(vm.Status.Crypto.KeyID).ToNot(Equal(nsInfo.EncryptionClass1KeyID))
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})
						})

						When("default provider is not native key provider", func() {
							JustBeforeEach(func() {
								m := vimcrypto.NewManagerKmip(ctx.VCClient.Client)
								Expect(m.MarkDefault(ctx, ctx.EncryptionClass2ProviderID)).To(Succeed())
							})

							It("should succeed", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).To(BeNil())

								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).ToNot(BeNil())

								Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
									[]vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
									}))
								Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
								Expect(vm.Status.Crypto.KeyID).ToNot(BeEmpty())
								Expect(vm.Status.Crypto.KeyID).ToNot(Equal(nsInfo.EncryptionClass1KeyID))
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})
						})
					})

					Context("using an encryption class", func() {

						JustBeforeEach(func() {
							vm.Spec.Crypto.EncryptionClassName = ctx.EncryptionClass2Name
						})

						It("should succeed", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.Crypto).To(BeNil())

							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.Crypto).ToNot(BeNil())

							Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
								[]vmopv1.VirtualMachineEncryptionType{
									vmopv1.VirtualMachineEncryptionTypeConfig,
								}))
							Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
							Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
						})

						When("using a non-encryption storage class with a vTPM", func() {
							BeforeEach(func() {
								hasVTPM = true
							})

							JustBeforeEach(func() {
								vm.Spec.StorageClass = ctx.StorageClassName
							})

							It("should succeed", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).To(BeNil())

								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.Crypto).ToNot(BeNil())

								Expect(vm.Status.Crypto.Encrypted).To(HaveExactElements(
									[]vmopv1.VirtualMachineEncryptionType{
										vmopv1.VirtualMachineEncryptionTypeConfig,
									}))
								Expect(vm.Status.Crypto.ProviderID).To(Equal(ctx.EncryptionClass2ProviderID))
								Expect(vm.Status.Crypto.KeyID).To(Equal(nsInfo.EncryptionClass2KeyID))
								Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineEncryptionSynced)).To(BeTrue())
							})
						})
					})
				})
			})

			Context("VM Class with PCI passthrough devices", func() {
				BeforeEach(func() {
					vmClass.Spec.Hardware.Devices = vmopv1.VirtualDevices{
						VGPUDevices: []vmopv1.VGPUDevice{
							{
								ProfileName: "profile-from-class-without-class-as-config-fss",
							},
						},
						DynamicDirectPathIODevices: []vmopv1.DynamicDirectPathIODevice{
							{
								VendorID:    59,
								DeviceID:    60,
								CustomLabel: "label-from-class-without-class-as-config-fss",
							},
						},
					}
				})

				It("VM should not have PCI devices from VM Class", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					p := devList.SelectByType(&vimtypes.VirtualPCIPassthrough{})
					Expect(p).To(BeEmpty())
				})
			})

			Context("Without Storage Class", func() {
				BeforeEach(func() {
					testConfig.WithoutStorageClass = true
				})

				It("Creates VM", func() {
					Expect(vm.Spec.StorageClass).To(BeEmpty())

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					By("has expected datastore", func() {
						datastore, err := ctx.Finder.DefaultDatastore(ctx)
						Expect(err).ToNot(HaveOccurred())

						Expect(o.Datastore).To(HaveLen(1))
						Expect(o.Datastore[0]).To(Equal(datastore.Reference()))
					})
				})
			})

			Context("Without Content Library", func() {
				BeforeEach(func() {
					testConfig.WithContentLibrary = false
				})

				// TODO: Dedupe this with "Basic VM" above
				It("Clones VM", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					By("has expected Status values", func() {
						Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))
						Expect(vm.Status.Host).ToNot(BeEmpty())
						Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
						Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))

						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeTrue())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionStorageReady)).To(BeTrue())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

						By("did not have VMSetResourcePool", func() {
							Expect(vm.Spec.Reserved).To(BeNil())
							Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady)).To(BeFalse())
						})
						By("did not have Bootstrap", func() {
							Expect(vm.Spec.Bootstrap).To(BeNil())
							Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
						})
						By("did not have Network", func() {
							Expect(vm.Spec.Network.Disabled).To(BeTrue())
							Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeFalse())
						})
					})

					By("has expected inventory path", func() {
						Expect(vcVM.InventoryPath).To(HaveSuffix(fmt.Sprintf("/%s/%s", nsInfo.Namespace, vm.Name)))
					})

					By("has expected namespace resource pool", func() {
						rp, err := vcVM.ResourcePool(ctx)
						Expect(err).ToNot(HaveOccurred())
						nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")
						Expect(nsRP).ToNot(BeNil())
						Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
					})

					By("has expected power state", func() {
						Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
					})

					By("has expected hardware config", func() {
						// TODO: Fix vcsim behavior: NumCPU is correct "2" in the CloneSpec.Config but ends up
						// with 1 CPU from source VM. Ditto for MemorySize. These assertions are only working
						// because the state is on so we reconfigure the VM after it is created.

						// TODO: These assertions are excluded right now because
						// of the aforementioned vcsim behavior. The referenced
						// loophole is no longer in place because the FSS for
						// VM Class as Config was removed, and we now rely on
						// the deploy call to set the correct CPU/memory.
						// Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
						// Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
					})

					// TODO: More assertions!
				})
			})

			// BMV: I don't think this is actually supported.
			XIt("Create VM from VMTX in ContentLibrary", func() {
				imageName := "test-vm-vmtx"

				ctx.ContentLibraryItemTemplate("DC0_C0_RP0_VM0", imageName)
				vm.Spec.ImageName = imageName

				_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())
			})

			When("vm has explicit zone", func() {
				JustBeforeEach(func() {
					delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)
				})

				It("creates VM in placement selected zone", func() {
					Expect(vm.Labels).ToNot(HaveKey(topology.KubernetesTopologyZoneLabelKey))
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					azName, ok := vm.Labels[topology.KubernetesTopologyZoneLabelKey]
					Expect(ok).To(BeTrue())
					Expect(azName).To(BeElementOf(ctx.ZoneNames))

					By("VM is created in the zone's ResourcePool", func() {
						rp, err := vcVM.ResourcePool(ctx)
						Expect(err).ToNot(HaveOccurred())
						nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, azName, "")
						Expect(nsRP).ToNot(BeNil())
						Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
					})
				})
			})

			It("creates VM in assigned zone", func() {
				Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
				azName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
				vm.Labels[topology.KubernetesTopologyZoneLabelKey] = azName

				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				By("VM is created in the zone's ResourcePool", func() {
					rp, err := vcVM.ResourcePool(ctx)
					Expect(err).ToNot(HaveOccurred())
					nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, azName, "")
					Expect(nsRP).ToNot(BeNil())
					Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
				})
			})

			When("VM zone is constrained by PVC", func() {
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

				})

				It("creates VM in allowed zone", func() {
					Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
					azName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]

					// Make sure we do placement.
					delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)

					pvc1 := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "pvc-claim-1",
							Namespace: vm.Namespace,
							Annotations: map[string]string{
								"csi.vsphere.volume-accessible-topology": fmt.Sprintf(`[{"topology.kubernetes.io/zone":"%s"}]`, azName),
							},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							StorageClassName: ptr.To(ctx.StorageClassName),
						},
						Status: corev1.PersistentVolumeClaimStatus{
							Phase: corev1.ClaimBound,
						},
					}
					Expect(ctx.Client.Create(ctx, pvc1)).To(Succeed())
					Expect(ctx.Client.Status().Update(ctx, pvc1)).To(Succeed())

					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Status.Zone).To(Equal(azName))
				})
			})

			Context("When Instance Storage FSS is enabled", func() {
				BeforeEach(func() {
					testConfig.WithInstanceStorage = true
				})

				expectInstanceStorageVolumes := func(
					vm *vmopv1.VirtualMachine,
					isStorage vmopv1.InstanceStorage) {

					ExpectWithOffset(1, isStorage.Volumes).ToNot(BeEmpty())
					isVolumes := vmopv1util.FilterInstanceStorageVolumes(vm)
					ExpectWithOffset(1, isVolumes).To(HaveLen(len(isStorage.Volumes)))

					for _, isVol := range isStorage.Volumes {
						found := false

						for idx, vol := range isVolumes {
							claim := vol.PersistentVolumeClaim.InstanceVolumeClaim
							if claim.StorageClass == isStorage.StorageClass && claim.Size == isVol.Size {
								isVolumes = append(isVolumes[:idx], isVolumes[idx+1:]...)
								found = true
								break
							}
						}

						ExpectWithOffset(1, found).To(BeTrue(), "failed to find instance storage volume for %v", isVol)
					}
				}

				It("creates VM without instance storage", func() {
					_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())
				})

				It("create VM with instance storage", func() {
					Expect(vm.Spec.Volumes).To(BeEmpty())

					vmClass.Spec.Hardware.InstanceStorage = vmopv1.InstanceStorage{
						StorageClass: vm.Spec.StorageClass,
						Volumes: []vmopv1.InstanceStorageVolume{
							{
								Size: resource.MustParse("256Gi"),
							},
							{
								Size: resource.MustParse("512Gi"),
							},
						},
					}
					Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())

					_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).To(MatchError("instance storage PVCs are not bound yet"))
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeFalse())

					By("Instance storage volumes should be added to VM", func() {
						Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeTrue())
						expectInstanceStorageVolumes(vm, vmClass.Spec.Hardware.InstanceStorage)
					})

					By("Placement should have been done", func() {
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
						Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
						Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
					})

					isVol0 := vm.Spec.Volumes[0]
					Expect(isVol0.PersistentVolumeClaim.InstanceVolumeClaim).ToNot(BeNil())

					By("simulate volume controller workflow", func() {
						// Simulate what would be set by volume controller.
						vm.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = ""

						_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("status update pending for persistent volume: %s on VM", isVol0.Name)))

						// Simulate what would be set by the volume controller.
						for _, vol := range vm.Spec.Volumes {
							vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
								Name:     vol.Name,
								Attached: true,
							})
						}
					})

					By("VM is now created", func() {
						_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
					})
				})
			})

			It("Powers VM off", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

				vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				state, err := vcVM.PowerState(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
			})

			It("returns error when StorageClass is required but none specified", func() {
				vm.Spec.StorageClass = ""
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(MatchError("StorageClass is required but not specified"))

				c := conditions.Get(vm, vmopv1.VirtualMachineConditionStorageReady)
				Expect(c).ToNot(BeNil())
				expectedCondition := conditions.FalseCondition(
					vmopv1.VirtualMachineConditionStorageReady,
					"StorageClassRequired",
					"StorageClass is required but not specified")
				Expect(*c).To(conditions.MatchCondition(*expectedCondition))
			})

			It("Can be called multiple times", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				modified := o.Config.Modified

				_, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				// Try to assert nothing changed.
				Expect(o.Config.Modified).To(Equal(modified))
			})

			Context("VM Metadata", func() {

				Context("ExtraConfig Transport", func() {
					var ec map[string]interface{}

					JustBeforeEach(func() {
						configMap := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								GenerateName: "md-configmap-",
								Namespace:    vm.Namespace,
							},
							Data: map[string]string{
								"foo.bar":       "should-be-ignored",
								"guestinfo.Foo": "foo",
							},
						}
						Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())

						/*
							vm.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
								ConfigMapName: configMap.Name,
								Transport:     vmopv1.VirtualMachineMetadataExtraConfigTransport,
							}
						*/
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						ec = map[string]interface{}{}
						for _, option := range o.Config.ExtraConfig {
							if val := option.GetOptionValue(); val != nil {
								ec[val.Key] = val.Value.(string)
							}
						}
					})

					AfterEach(func() {
						ec = nil
					})

					// TODO: As is we can't really honor "guestinfo.*" prefix
					XIt("Metadata data is included in ExtraConfig", func() {
						Expect(ec).ToNot(HaveKey("foo.bar"))
						Expect(ec).To(HaveKeyWithValue("guestinfo.Foo", "foo"))

						By("Should include default keys and values", func() {
							Expect(ec).To(HaveKeyWithValue("disk.enableUUID", "TRUE"))
							Expect(ec).To(HaveKeyWithValue("vmware.tools.gosc.ignoretoolscheck", "TRUE"))
						})
					})

					Context("JSON_EXTRA_CONFIG is specified", func() {
						BeforeEach(func() {
							b, err := json.Marshal(
								struct {
									Foo string
									Bar string
								}{
									Foo: "f00",
									Bar: "42",
								},
							)
							Expect(err).ToNot(HaveOccurred())
							testConfig.WithJSONExtraConfig = string(b)
						})

						It("Global config is included in ExtraConfig", func() {
							Expect(ec).To(HaveKeyWithValue("Foo", "f00"))
							Expect(ec).To(HaveKeyWithValue("Bar", "42"))
						})
					})
				})
			})

			Context("Network", func() {

				It("Should not have a nic", func() {
					Expect(vm.Spec.Network.Disabled).To(BeTrue())

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeFalse())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
					Expect(l).To(BeEmpty())
				})

				Context("Multiple NICs are specified", func() {
					BeforeEach(func() {
						testConfig.WithNetworkEnv = builder.NetworkEnvNamed

						vm.Spec.Network.Disabled = false
						vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name:    "eth0",
								Network: &common.PartialObjectRef{Name: "VM Network"},
							},
							{
								Name:    "eth1",
								Network: &common.PartialObjectRef{Name: dvpgName},
							},
						}
					})

					It("Has expected devices", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())
						Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeTrue())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						devList := object.VirtualDeviceList(o.Config.Hardware.Device)
						l := devList.SelectByType(&vimtypes.VirtualEthernetCard{})
						Expect(l).To(HaveLen(2))

						dev1 := l[0].GetVirtualDevice()
						backing1, ok := dev1.Backing.(*vimtypes.VirtualEthernetCardNetworkBackingInfo)
						Expect(ok).Should(BeTrue())
						Expect(backing1.DeviceName).To(Equal("VM Network"))

						dev2 := l[1].GetVirtualDevice()
						backing2, ok := dev2.Backing.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
						Expect(ok).Should(BeTrue())
						_, dvpg := getDVPG(ctx, dvpgName)
						Expect(backing2.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))
					})
				})
			})

			Context("Disks", func() {

				Context("VM has thin provisioning", func() {
					BeforeEach(func() {
						if vm.Spec.Advanced == nil {
							vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
						}
						vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VirtualMachineVolumeProvisioningModeThin
					})

					It("Succeeds", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.ThinProvisioned).To(PointTo(BeTrue()))
					})
				})

				XContext("VM has thick provisioning", func() {
					BeforeEach(func() {
						vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VirtualMachineVolumeProvisioningModeThick
					})

					It("Succeeds", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						/* vcsim CL deploy has "thick" but that isn't reflected for this disk. */
						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.ThinProvisioned).To(PointTo(BeFalse()))
					})
				})

				XContext("VM has eager zero provisioning", func() {
					BeforeEach(func() {
						if vm.Spec.Advanced == nil {
							vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
						}
						vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VirtualMachineVolumeProvisioningModeThickEagerZero
					})

					It("Succeeds", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						/* vcsim CL deploy has "eagerZeroedThick" but that isn't reflected for this disk. */
						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.EagerlyScrub).To(PointTo(BeTrue()))
					})
				})

				Context("Should resize root disk", func() {
					It("Succeeds", func() {
						newSize := resource.MustParse("4242Gi")

						if vm.Spec.Advanced == nil {
							vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
						}
						vm.Spec.Advanced.BootDiskCapacity = &newSize
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						disk, _ := getVMHomeDisk(ctx, vcVM, o)
						Expect(disk.CapacityInBytes).To(BeEquivalentTo(newSize.Value()))
					})
				})
			})

			Context("CNS Volumes", func() {
				cnsVolumeName := "cns-volume-1"

				It("CSI Volumes workflow", func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					By("Add CNS volume to VM", func() {
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: cnsVolumeName,
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: "pvc-volume-1",
										},
									},
								},
							},
						}

						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("status update pending for persistent volume: %s on VM", cnsVolumeName)))
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})

					By("CNS volume is not attached", func() {
						errMsg := "blah blah blah not attached"

						vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:     cnsVolumeName,
								Attached: false,
								Error:    errMsg,
							},
						}

						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("persistent volume: %s not attached to VM", cnsVolumeName)))
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})

					By("CNS volume is attached", func() {
						vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:     cnsVolumeName,
								Attached: true,
							},
						}
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
					})
				})
			})

			It("Reverse lookups existing VM into correct zone", func() {
				_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
				Expect(vm.Status.Zone).To(Equal(zoneName))
				delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)

				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
				Expect(vm.Status.Zone).To(Equal(zoneName))
			})
		})

		Context("VM SetResourcePolicy", func() {
			var resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy

			JustBeforeEach(func() {
				resourcePolicyName := "test-policy"
				resourcePolicy = getVirtualMachineSetResourcePolicy(resourcePolicyName, nsInfo.Namespace)
				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
				Expect(ctx.Client.Create(ctx, resourcePolicy)).To(Succeed())

				vm.Annotations["vsphere-cluster-module-group"] = resourcePolicy.Spec.ClusterModuleGroups[0]
				if vm.Spec.Reserved == nil {
					vm.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
				}
				vm.Spec.Reserved.ResourcePolicyName = resourcePolicy.Name
			})

			AfterEach(func() {
				resourcePolicy = nil
			})

			It("VM is created in child Folder and ResourcePool", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				By("has expected condition", func() {
					Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady)).To(BeTrue())
				})

				By("has expected inventory path", func() {
					Expect(vcVM.InventoryPath).To(HaveSuffix(
						fmt.Sprintf("/%s/%s/%s", nsInfo.Namespace, resourcePolicy.Spec.Folder, vm.Name)))
				})

				By("has expected namespace resource pool", func() {
					rp, err := vcVM.ResourcePool(ctx)
					Expect(err).ToNot(HaveOccurred())
					childRP := ctx.GetResourcePoolForNamespace(
						nsInfo.Namespace,
						vm.Labels[topology.KubernetesTopologyZoneLabelKey],
						resourcePolicy.Spec.ResourcePool.Name)
					Expect(childRP).ToNot(BeNil())
					Expect(rp.Reference().Value).To(Equal(childRP.Reference().Value))
				})
			})

			It("Cluster Modules", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				var members []vimtypes.ManagedObjectReference
				for i := range resourcePolicy.Status.ClusterModules {
					m, err := cluster.NewManager(ctx.RestClient).ListModuleMembers(ctx, resourcePolicy.Status.ClusterModules[i].ModuleUuid)
					Expect(err).ToNot(HaveOccurred())
					members = append(m, members...)
				}

				Expect(members).To(ContainElements(vcVM.Reference()))
			})

			It("Returns error with non-existence cluster module", func() {
				vm.Annotations["vsphere-cluster-module-group"] = "bogusClusterMod"
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("ClusterModule bogusClusterMod not found"))
			})
		})

		Context("Delete VM", func() {
			const zoneName = "az-1"

			BeforeEach(func() {
				// Explicitly place the VM into one of the zones that the test context will create.
				vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
			})

			JustBeforeEach(func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			})

			Context("when the VM is off", func() {
				BeforeEach(func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
				})

				It("deletes the VM", func() {
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))

					uniqueID := vm.Status.UniqueID
					Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

					Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
					Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
				})
			})

			It("when the VM is on", func() {
				Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

				uniqueID := vm.Status.UniqueID
				Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

				// This checks that we power off the VM prior to deletion.
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			})

			It("returns success when VM does not exist", func() {
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
			})

			It("returns NotFound when VM does not exist", func() {
				_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
			})

			It("Deletes existing VM when zone info is missing", func() {
				_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				uniqueID := vm.Status.UniqueID
				Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

				Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
				delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)

				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			})
		})

		Context("Guest Heartbeat", func() {
			JustBeforeEach(func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			})

			It("return guest heartbeat", func() {
				heartbeat, err := vmProvider.GetVirtualMachineGuestHeartbeat(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
				// Just testing for property query: field not set in vcsim.
				Expect(heartbeat).To(BeEmpty())
			})
		})

		Context("Web console ticket", func() {
			JustBeforeEach(func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			})

			It("return ticket", func() {
				// vcsim doesn't implement this yet so expect an error.
				_, err := vmProvider.GetVirtualMachineWebMKSTicket(ctx, vm, "foo")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not implement: AcquireTicket"))
			})
		})

		Context("VM hardware version", func() {
			JustBeforeEach(func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			})

			It("return version", func() {
				version, err := vmProvider.GetVirtualMachineHardwareVersion(ctx, vm)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal(vimtypes.VMX9))
			})
		})

		Context("Create/Update/Delete ISO backed VirtualMachine", func() {
			var (
				vm      *vmopv1.VirtualMachine
				vmClass *vmopv1.VirtualMachineClass
			)

			BeforeEach(func() {
				vmClass = builder.DummyVirtualMachineClassGenName()
				vm = builder.DummyBasicVirtualMachine("test-vm", "")

				// Reduce diff from old tests: by default don't create an NIC.
				if vm.Spec.Network == nil {
					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
				}
				vm.Spec.Network.Disabled = true
			})

			JustBeforeEach(func() {
				vmClass.Namespace = nsInfo.Namespace
				Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

				clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryIsoImageName}, clusterVMImage)).To(Succeed())

				vm.Namespace = nsInfo.Namespace
				vm.Spec.ClassName = vmClass.Name
				vm.Spec.ImageName = clusterVMImage.Name
				vm.Spec.Image.Kind = cvmiKind
				vm.Spec.Image.Name = clusterVMImage.Name
				vm.Spec.StorageClass = ctx.StorageClassName
				vm.Spec.Cdrom = []vmopv1.VirtualMachineCdromSpec{{
					Name: "cdrom0",
					Image: vmopv1.VirtualMachineImageRef{
						Name: cvmiKind,
						Kind: clusterVMImage.Name,
					},
				}}
			})

			Context("return config", func() {
				JustBeforeEach(func() {
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				})

				It("return config.files", func() {
					vmPathName := "config.files.vmPathName"
					props, err := vmProvider.GetVirtualMachineProperties(ctx, vm, []string{vmPathName})
					Expect(err).NotTo(HaveOccurred())
					var path object.DatastorePath
					path.FromString(props[vmPathName].(string))
					Expect(path.Datastore).NotTo(BeEmpty())
				})
			})
		})
	})
}

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
