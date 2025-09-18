// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"path"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/google/uuid"
	vimcrypto "github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/crypto"
	vmconfpolicy "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/policy"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

const (
	// Hardcoded vcsim CPU frequency.
	vcsimCPUFreq = 2294

	cvmiKind = "ClusterVirtualMachineImage"
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

			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
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
						When("spec.instanceUUID matches existing VM", func() {
							JustBeforeEach(func() {
								vm.Spec.InstanceUUID = instanceUUID
							})
							It("should error when getting class", func() {
								assertClassNotFound("")
							})
						})
						When("spec.instanceUUID does not match existing VM", func() {
							JustBeforeEach(func() {
								vm.Spec.InstanceUUID = uuid.NewString()
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
		})

		Context("CreateOrUpdate VM", func() {

			var zoneName string

			JustBeforeEach(func() {
				zoneName = ctx.GetFirstZoneName()
				// Explicitly place the VM into one of the zones that the test context will create.
				vm.Labels[corev1.LabelTopologyZone] = zoneName
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
			})

			Context("VirtualMachineGroup", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
						config.Features.VMGroups = true
					})
				})

				var vmg vmopv1.VirtualMachineGroup
				JustBeforeEach(func() {
					// Remove explicit zone label so group placement can work
					if vm.Labels != nil {
						delete(vm.Labels, corev1.LabelTopologyZone)
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
					}

					vmg = vmopv1.VirtualMachineGroup{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "vmg",
							Namespace: vm.Namespace,
						},
					}
					Expect(ctx.Client.Create(ctx, &vmg)).To(Succeed())
				})

				Context("VM Creation", func() {
					When("spec.groupName is set to a non-existent group", func() {
						JustBeforeEach(func() {
							vm.Spec.GroupName = "vmg-invalid"
						})
						Specify("it should return an error creating VM", func() {
							err := createOrUpdateVM(ctx, vmProvider, vm)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring("VM is not linked to its group"))
						})
					})

					When("spec.groupName is set to a group to which the VM does not belong", func() {
						JustBeforeEach(func() {
							vm.Spec.GroupName = vmg.Name
						})
						Specify("it should return an error creating VM", func() {
							err := createOrUpdateVM(ctx, vmProvider, vm)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring("VM is not linked to its group"))
						})
					})

					When("spec.groupName is set to a group to which the VM does belong", func() {
						JustBeforeEach(func() {
							vm.Spec.GroupName = vmg.Name
							vmg.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
								{
									Members: []vmopv1.GroupMember{
										{
											Name: vm.Name,
											Kind: "VirtualMachine",
										},
									},
								},
							}
							Expect(ctx.Client.Update(ctx, &vmg)).To(Succeed())
						})

						When("VM Group placement condition is not ready", func() {
							JustBeforeEach(func() {
								vmg.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
									{
										Name: vm.Name,
										Kind: "VirtualMachine",
										Conditions: []metav1.Condition{
											{
												Type:   vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
												Status: metav1.ConditionFalse,
											},
										},
									},
								}
								Expect(ctx.Client.Status().Update(ctx, &vmg)).To(Succeed())
							})
							Specify("it should return an error creating VM", func() {
								err := createOrUpdateVM(ctx, vmProvider, vm)
								Expect(err).To(HaveOccurred())
								Expect(err.Error()).To(ContainSubstring("VM Group placement is not ready"))
							})
						})

						When("VM Group placement condition is ready", func() {
							var (
								groupZone string
								groupHost string
								groupPool string
							)
							JustBeforeEach(func() {
								// Ensure the group zone is different to verify the placement actually from group.
								Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
								groupZone = ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
								for groupZone == vm.Labels[corev1.LabelTopologyZone] {
									groupZone = ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
								}

								ccrs := ctx.GetAZClusterComputes(groupZone)
								Expect(ccrs).ToNot(BeEmpty())
								ccr := ccrs[0]
								hosts, err := ccr.Hosts(ctx)
								Expect(err).ToNot(HaveOccurred())
								Expect(hosts).ToNot(BeEmpty())
								groupHost = hosts[0].Reference().Value

								nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, groupZone, "")
								Expect(nsRP).ToNot(BeNil())
								groupPool = nsRP.Reference().Value

								vmg.Status.Members = []vmopv1.VirtualMachineGroupMemberStatus{
									{
										Name: vm.Name,
										Kind: "VirtualMachine",
										Conditions: []metav1.Condition{
											{
												Type:   vmopv1.VirtualMachineGroupMemberConditionPlacementReady,
												Status: metav1.ConditionTrue,
											},
										},
										Placement: &vmopv1.VirtualMachinePlacementStatus{
											Zone: groupZone,
											Node: groupHost,
											Pool: groupPool,
										},
									},
								}
								Expect(ctx.Client.Status().Update(ctx, &vmg)).To(Succeed())
							})
							Specify("it should successfully create VM from group's placement", func() {
								vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
								Expect(err).ToNot(HaveOccurred())
								By("VM is placed in the expected zone from group", func() {
									Expect(vm.Status.Zone).To(Equal(groupZone))
								})
								By("VM is placed in the expected host from group", func() {
									vmHost, err := vcVM.HostSystem(ctx)
									Expect(err).ToNot(HaveOccurred())
									Expect(vmHost.Reference().Value).To(Equal(groupHost))
								})
								By("VM is created in the expected pool from group", func() {
									rp, err := vcVM.ResourcePool(ctx)
									Expect(err).ToNot(HaveOccurred())
									Expect(rp.Reference().Value).To(Equal(groupPool))
								})
								By("VM has expected group linked condition", func() {
									Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
								})
							})
						})
					})
				})

				Context("VM Update", func() {
					JustBeforeEach(func() {
						// Unset groupName to ensure the VM can be created.
						vm.Spec.GroupName = ""
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					})

					When("spec.groupName is set to a non-existent group", func() {
						JustBeforeEach(func() {
							vm.Spec.GroupName = "vmg-invalid"
						})
						Specify("vm should have group linked condition set to false", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
						})
					})

					When("spec.groupName is set to a group to which the VM does not belong", func() {
						JustBeforeEach(func() {
							vm.Spec.GroupName = vmg.Name
						})
						Specify("vm should have group linked condition set to false", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
						})
					})

					When("spec.groupName is set to a group to which the VM does belong", func() {
						JustBeforeEach(func() {
							vm.Spec.GroupName = vmg.Name
							vmg.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{
								{
									Members: []vmopv1.GroupMember{
										{
											Name: vm.Name,
											Kind: "VirtualMachine",
										},
									},
								},
							}
							Expect(ctx.Client.Update(ctx, &vmg)).To(Succeed())
						})
						Specify("vm should have group linked condition set to true", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)).To(BeTrue())
						})

						When("spec.groupName no longer points to group", func() {
							Specify("vm should no longer have group linked condition", func() {
								vm.Spec.GroupName = ""
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								c := conditions.Get(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked)
								Expect(c).To(BeNil())
							})
						})
					})
				})

				Context("Zone Label Override for VM Groups", func() {
					var (
						vm       *vmopv1.VirtualMachine
						vmGroup  *vmopv1.VirtualMachineGroup
						vmClass  *vmopv1.VirtualMachineClass
						zoneName string
					)

					BeforeEach(func() {
						vmClass = builder.DummyVirtualMachineClassGenName()
						vm = builder.DummyBasicVirtualMachine("test-vm-zone-override", "")
						vmGroup = &vmopv1.VirtualMachineGroup{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-group-zone-override",
							},
							Spec: vmopv1.VirtualMachineGroupSpec{
								BootOrder: []vmopv1.VirtualMachineGroupBootOrderGroup{
									{
										Members: []vmopv1.GroupMember{
											{Kind: "VirtualMachine", Name: vm.ObjectMeta.Name},
										},
									},
								},
							},
						}
					})

					JustBeforeEach(func() {
						pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
							config.Features.VMGroups = true
						})

						vmClass.Namespace = nsInfo.Namespace
						Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

						vmGroup.Namespace = nsInfo.Namespace
						Expect(ctx.Client.Create(ctx, vmGroup)).To(Succeed())

						clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryImageName}, clusterVMImage)).To(Succeed())

						vm.Namespace = nsInfo.Namespace
						vm.Spec.ClassName = vmClass.Name
						vm.Spec.ImageName = clusterVMImage.Name
						vm.Spec.Image.Kind = cvmiKind
						vm.Spec.Image.Name = clusterVMImage.Name
						vm.Spec.StorageClass = ctx.StorageClassName

						vm.Spec.GroupName = vmGroup.Name

						zoneName = ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
						vm.Labels = map[string]string{
							corev1.LabelTopologyZone: zoneName,
						}

						Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
					})

					Context("when VM has explicit zone label and is part of group", func() {
						It("should create VM in specified zone, not using group placement", func() {
							err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
							Expect(err).To(HaveOccurred())
							Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue())
							Expect(err).To(MatchError(vsphere.ErrCreate))

							// Verify VM was created
							Expect(vm.Status.UniqueID).ToNot(BeEmpty())
						})
					})

					Context("when VM has explicit zone label but is not linked to group", func() {
						BeforeEach(func() {
							vmGroup.Spec.BootOrder = []vmopv1.VirtualMachineGroupBootOrderGroup{}
						})

						It("should fail to create VM", func() {
							err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring("VM is not linked to its group"))
						})
					})
				})
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
					Expect(vm.Status.NodeName).ToNot(BeEmpty())
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

			DescribeTable("VM is not connected",
				func(state vimtypes.VirtualMachineConnectionState) {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var moVM mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

					sctx := ctx.SimulatorContext()
					sctx.WithLock(
						vcVM.Reference(),
						func() {
							vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
							vm.Summary.Runtime.ConnectionState = state
						})

					_, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)

					if state == "" {
						Expect(err).ToNot(HaveOccurred())
					} else {
						Expect(err).To(HaveOccurred())
						var noRequeueErr pkgerr.NoRequeueError
						Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
						Expect(noRequeueErr.Message).To(Equal(
							fmt.Sprintf("unsupported connection state: %s", state)))
					}
				},
				Entry("empty", vimtypes.VirtualMachineConnectionState("")),
				Entry("disconnected", vimtypes.VirtualMachineConnectionStateDisconnected),
				Entry("inaccessible", vimtypes.VirtualMachineConnectionStateInaccessible),
				Entry("invalid", vimtypes.VirtualMachineConnectionStateInvalid),
				Entry("orphaned", vimtypes.VirtualMachineConnectionStateOrphaned),
			)

			// TODO(akutz) Promote this block when the FSS WCP_VMService_FastDeploy is
			//             removed.
			When("FSS WCP_VMService_FastDeploy is enabled", func() {

				var (
					vmic vmopv1.VirtualMachineImageCache
				)

				BeforeEach(func() {
					testConfig.WithContentLibrary = true
					pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
						config.Features.FastDeploy = true
					})
				})

				JustBeforeEach(func() {
					vmicName := pkgutil.VMIName(ctx.ContentLibraryItemID)
					vmic = vmopv1.VirtualMachineImageCache{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: pkgcfg.FromContext(ctx).PodNamespace,
							Name:      vmicName,
						},
					}
					Expect(ctx.Client.Create(ctx, &vmic)).To(Succeed())

					vmicm := corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: vmic.Namespace,
							Name:      vmic.Name,
						},
						Data: map[string]string{
							"value": ovfEnvelopeYAML,
						},
					}
					Expect(ctx.Client.Create(ctx, &vmicm)).To(Succeed())
				})

				assertVMICNotReady := func(err error, msg, name, dcID, dsID string) {
					var e pkgerr.VMICacheNotReadyError
					ExpectWithOffset(1, errors.As(err, &e)).To(BeTrue())
					ExpectWithOffset(1, e.Message).To(Equal(msg))
					ExpectWithOffset(1, e.Name).To(Equal(name))
					ExpectWithOffset(1, e.DatacenterID).To(Equal(dcID))
					ExpectWithOffset(1, e.DatastoreID).To(Equal(dsID))
				}

				When("ovf is not ready", func() {
					It("should fail", func() {
						_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						assertVMICNotReady(
							err,
							"hardware not ready",
							vmic.Name,
							"",
							"")
					})
				})

				When("ovf is ready", func() {
					JustBeforeEach(func() {
						vmic.Status = vmopv1.VirtualMachineImageCacheStatus{
							OVF: &vmopv1.VirtualMachineImageCacheOVFStatus{
								ConfigMapName:   vmic.Name,
								ProviderVersion: ctx.ContentLibraryItemVersion,
							},
							Conditions: []metav1.Condition{
								{
									Type:   vmopv1.VirtualMachineImageCacheConditionHardwareReady,
									Status: metav1.ConditionTrue,
								},
							},
						}
						Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())
					})

					When("files are not ready", func() {

						It("should fail", func() {
							_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							assertVMICNotReady(
								err,
								"cached files not ready",
								vmic.Name,
								ctx.Datacenter.Reference().Value,
								ctx.Datastore.Reference().Value)
						})
					})

					When("files are ready", func() {

						BeforeEach(func() {
							// Ensure the VM has a UID so the VM path is stable.
							vm.UID = types.UID("123")

							configSpec := vimtypes.VirtualMachineConfigSpec{
								ExtraConfig: []vimtypes.BaseOptionValue{
									&vimtypes.OptionValue{
										Key:   "fu",
										Value: "bar",
									},
								},
							}

							var w bytes.Buffer
							enc := vimtypes.NewJSONEncoder(&w)
							Expect(enc.Encode(configSpec)).To(Succeed())

							vmClass.Spec.ConfigSpec = w.Bytes()
						})

						JustBeforeEach(func() {
							conditions.MarkTrue(
								&vmic,
								vmopv1.VirtualMachineImageCacheConditionFilesReady)
							vmic.Status.Locations = []vmopv1.VirtualMachineImageCacheLocationStatus{
								{
									DatacenterID: ctx.Datacenter.Reference().Value,
									DatastoreID:  ctx.Datastore.Reference().Value,
									Files: []vmopv1.VirtualMachineImageCacheFileStatus{
										{
											ID:       ctx.ContentLibraryItemDiskPath,
											Type:     vmopv1.VirtualMachineImageCacheFileTypeDisk,
											DiskType: vmopv1.VolumeTypeClassic,
										},
										{
											ID:   ctx.ContentLibraryItemNVRAMPath,
											Type: vmopv1.VirtualMachineImageCacheFileTypeOther,
										},
									},
									Conditions: []metav1.Condition{
										{
											Type:   vmopv1.ReadyConditionType,
											Status: metav1.ConditionTrue,
										},
									},
								},
							}
							Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())

							libMgr := library.NewManager(ctx.RestClient)
							Expect(libMgr.SyncLibraryItem(ctx,
								&library.Item{ID: ctx.ContentLibraryItemID},
								true)).To(Succeed())
						})

						It("should succeed", func() {
							vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							var moVM mo.VirtualMachine
							Expect(vcVM.Properties(
								ctx,
								vcVM.Reference(),
								[]string{"config.extraConfig"},
								&moVM)).To(Succeed())
							ec := object.OptionValueList(moVM.Config.ExtraConfig)
							v1, _ := ec.GetString("hello")
							Expect(v1).To(Equal("world"))
							v2, _ := ec.GetString("fu")
							Expect(v2).To(Equal("bar"))
							v3, _ := ec.GetString(pkgconst.VMProvKeepDisksExtraConfigKey)
							Expect(v3).To(Equal(path.Base(ctx.ContentLibraryItemDiskPath)))
						})
					})
				})

			})

			When("using async create", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
						config.AsyncCreateEnabled = true
						config.AsyncSignalEnabled = true
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

				When("there is an error creating the VM", func() {
					JustBeforeEach(func() {
						ctx.SimulatorContext().Map.Handler = func(
							ctx *simulator.Context,
							m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

							if m.Name == "ImportVApp" {
								return nil, &vimtypes.InvalidRequest{}
							}
							return nil, nil
						}
					})

					It("should fail to create the VM without an NPE", func() {
						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Eventually(func(g Gomega) {
							g.Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), vm)).To(Succeed())
							g.Expect(vm.Status.UniqueID).To(BeEmpty())
							c := conditions.Get(vm, vmopv1.VirtualMachineConditionCreated)
							g.Expect(c).ToNot(BeNil())
							g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
							g.Expect(c.Reason).To(Equal("Error"))
							g.Expect(c.Message).To(Equal("deploy error: ServerFaultCode: InvalidRequest"))
						}).Should(Succeed())
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
									defer GinkgoRecover()
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
							Expect(createErrs).Should(HaveLen(1))
							Expect(createErrs[0]).To(MatchError(vsphere.ErrCreate))

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

				if vm.Annotations == nil {
					vm.Annotations = make(map[string]string)
				}
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
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					vm.Spec.InstanceUUID = o.Config.InstanceUuid

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
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
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
										config.AsyncCreateEnabled = false
										config.AsyncSignalEnabled = true
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
										config.AsyncCreateEnabled = true
										config.AsyncSignalEnabled = true
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
								vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
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
					// For old behavior, we'll fallback to these standalone fields when the class
					// does not have a ConfigSpec.
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

				It("VM should have PCI devices from VM Class", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					p := devList.SelectByType(&vimtypes.VirtualPCIPassthrough{})
					Expect(p).To(HaveLen(2))
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
						Expect(vm.Status.NodeName).ToNot(BeEmpty())
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
					delete(vm.Labels, corev1.LabelTopologyZone)
				})

				It("creates VM in placement selected zone", func() {
					Expect(vm.Labels).ToNot(HaveKey(corev1.LabelTopologyZone))
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					azName, ok := vm.Labels[corev1.LabelTopologyZone]
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
				vm.Labels[corev1.LabelTopologyZone] = azName

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
					delete(vm.Labels, corev1.LabelTopologyZone)

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
								Network: &vmopv1common.PartialObjectRef{Name: "VM Network"},
							},
							{
								Name:    "eth1",
								Network: &vmopv1common.PartialObjectRef{Name: dvpgName},
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
						vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThin
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
						vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThick
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
						vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThickEagerZero
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

			Context("Snapshot revert", func() {
				var (
					vmSnapshot *vmopv1.VirtualMachineSnapshot
				)

				BeforeEach(func() {
					testConfig.WithVMSnapshots = true
					vmSnapshot = builder.DummyVirtualMachineSnapshot("", "test-revert-snap", vm.Name)
				})

				JustBeforeEach(func() {
					vmSnapshot.Namespace = nsInfo.Namespace
				})

				Context("findDesiredSnapshot error handling", func() {
					It("should return regular error (not NoRequeueError) when multiple snapshots exist", func() {
						// Create VM first to get vcVM reference
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Create multiple snapshots with the same name
						task, err := vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "first snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						task, err = vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "second snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Mark the snapshot as ready.
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						// Create the snapshot CR to which the VM should revert
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)

						// This should return an error because findDesiredSnapshot should return an error
						// when there are multiple snapshots with the same name
						err = createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("resolves to 2 snapshots"))

						// Verify that the error causes a requeue (not a NoRequeueError)
						Expect(pkgerr.IsNoRequeueError(err)).To(BeFalse(), "Multiple snapshots error should cause requeue")
					})
				})

				Context("when VM has no snapshots", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)
					})

					It("should not trigger a revert (new snapshot workflow)", func() {
						// Create the snapshot CR but don't create actual vCenter snapshot
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("no snapshots for this VM"))
					})
				})

				Context("when desired snapshot CR doesn't exist", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)
					})

					It("should fail with snapshot CR not found error", func() {
						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("virtualmachinesnapshots.vmoperator.vmware.com \"test-revert-snap\" not found"))

						Expect(conditions.IsFalse(vm,
							vmopv1.VirtualMachineSnapshotRevertSucceeded,
						)).To(BeTrue())
						Expect(conditions.GetReason(vm,
							vmopv1.VirtualMachineSnapshotRevertSucceeded,
						)).To(Equal(vmopv1.VirtualMachineSnapshotRevertFailedReason))
					})
				})

				Context("when desired snapshot CR is not ready", func() {
					BeforeEach(func() {
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)
					})

					JustBeforeEach(func() {
						// Create snapshot CR but don't mark it as ready.
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())
					})

					When("snapshot is not created", func() {
						It("should fail with snapshot CR not ready error", func() {
							err := createOrUpdateVM(ctx, vmProvider, vm)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring(
								fmt.Sprintf("skipping revert for not-ready snapshot %q",
									vmSnapshot.Name)))
						})
					})

					When("snapshot is created but not ready", func() {
						It("should fail with snapshot CR not ready error", func() {
							// Mark the snapshot as created but not ready.
							conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
							Expect(ctx.Client.Status().Update(ctx, vmSnapshot)).To(Succeed())

							err := createOrUpdateVM(ctx, vmProvider, vm)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring(
								fmt.Sprintf("skipping revert for not-ready snapshot %q",
									vmSnapshot.Name)))
						})
					})
				})

				Context("revert to current snapshot", func() {
					It("should succeed", func() {
						// Create snapshot CR to trigger a snapshot workflow.
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())
						// Create VM so snapshot is also created.
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

						// Mark the snapshot as ready so that revert can proceed.
						Expect(ctx.Client.Get(ctx,
							client.ObjectKeyFromObject(vmSnapshot), vmSnapshot)).To(Succeed())
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Status().Update(ctx, vmSnapshot)).To(Succeed())

						// Set desired snapshot to point to the above snapshot.
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)

						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

						// Verify VM status reflects current snapshot.
						Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
						Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.CurrentSnapshot.Reference).To(Not(BeNil()))
						Expect(vm.Status.CurrentSnapshot.Reference.Name).To(Equal(vmSnapshot.Name))

						// Verify the status has root snapshots.
						Expect(vm.Status.RootSnapshots).ToNot(BeNil())
						Expect(vm.Status.RootSnapshots).To(HaveLen(1))
						Expect(vm.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.RootSnapshots[0].Reference).To(Not(BeNil()))
						Expect(vm.Status.RootSnapshots[0].Reference.Name).To(Equal(vmSnapshot.Name))
					})
				})

				Context("when reverting to valid snapshot", func() {
					var secondSnapshot *vmopv1.VirtualMachineSnapshot

					It("should successfully revert to desired snapshot", func() {
						// Create VM first
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Create first snapshot in vCenter
						task, err := vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "first snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create first snapshot CR
						// Mark the snapshot as created so that the snapshot workflow doesn't try to create it.
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
						// Mark the snapshot as ready so that the revert snapshot workflow can proceed.
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

						// Create second snapshot
						secondSnapshot = builder.DummyVirtualMachineSnapshot("", "test-second-snap", vm.Name)
						secondSnapshot.Namespace = nsInfo.Namespace

						task, err = vcVM.CreateSnapshot(ctx, secondSnapshot.Name, "second snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create second snapshot CR
						// Mark the snapshot as completed so that the snapshot workflow doesn't try to create it.
						conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
						// Mark the snapshot as ready so that the revert snapshot workflow can proceed.
						conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						// Snapshot should be owned by the VM resource.
						Expect(controllerutil.SetOwnerReference(&o, secondSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Create(ctx, secondSnapshot)).To(Succeed())

						// Set desired snapshot to first snapshot (revert from second to first)
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)
						err = createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Verify VM status reflects the reverted snapshot
						Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
						Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.CurrentSnapshot.Reference).To(Not(BeNil()))
						Expect(vm.Status.CurrentSnapshot.Reference.Name).To(Equal(vmSnapshot.Name))

						// Verify the spec.currentSnapshot is cleared.
						Expect(vm.Spec.CurrentSnapshot).To(BeNil())

						// Verify the status has root snapshots.
						Expect(vm.Status.RootSnapshots).ToNot(BeNil())
						Expect(vm.Status.RootSnapshots).To(HaveLen(1))
						Expect(vm.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.RootSnapshots[0].Reference).To(Not(BeNil()))
						Expect(vm.Status.RootSnapshots[0].Reference.Name).To(Equal(vmSnapshot.Name))

						// Verify the snapshot is actually current in vCenter
						var moVM mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
						Expect(moVM.Snapshot).ToNot(BeNil())
						Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())

						// Find the snapshot name in the tree to verify it matches
						currentSnap, err := virtualmachine.FindSnapshot(moVM, moVM.Snapshot.CurrentSnapshot.Value)
						Expect(err).ToNot(HaveOccurred())
						Expect(currentSnap).ToNot(BeNil())
						Expect(currentSnap.Name).To(Equal(vmSnapshot.Name))
					})

					Context("and the snapshot was taken when VM was powered on and is now powered off", func() {
						It("should successfully power on the VM after reverting to a Snapshot in PoweredOn state", func() {
							// Create VM first
							vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							// Create first snapshot in vCenter
							task, err := vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "first snapshot", false, false)
							Expect(err).ToNot(HaveOccurred())
							Expect(task.Wait(ctx)).To(Succeed())

							// Create first snapshot CR
							// Mark the snapshot as completed so that the snapshot workflow doesn't try to create it.
							conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
							// Mark the snapshot as ready so that the revert snapshot workflow can proceed.
							conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
							Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

							// Verify the snapshot is actually current in vCenter
							var moVM mo.VirtualMachine
							Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
							Expect(moVM.Snapshot).ToNot(BeNil())

							// verify that the snapshot's power state is powered off
							currentSnapshot, err := virtualmachine.FindSnapshot(moVM, moVM.Snapshot.CurrentSnapshot.Value)
							Expect(err).ToNot(HaveOccurred())
							Expect(currentSnapshot).ToNot(BeNil())
							Expect(currentSnapshot.State).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))

							// Snapshot should be owned by the VM resource.
							Expect(controllerutil.SetOwnerReference(vm, vmSnapshot, ctx.Scheme)).To(Succeed())
							Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

							// Create second snapshot
							secondSnapshot = builder.DummyVirtualMachineSnapshotWithMemory("", "test-second-snap", vm.Name)
							secondSnapshot.Namespace = nsInfo.Namespace

							task, err = vcVM.CreateSnapshot(ctx, secondSnapshot.Name, "second snapshot", true, false)
							Expect(err).ToNot(HaveOccurred())
							Expect(task.Wait(ctx)).To(Succeed())

							// Create second snapshot CR
							// Mark the snapshot as completed so that the snapshot workflow doesn't try to create it.
							conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
							// Mark the snapshot as ready so that the revert snapshot workflow can proceed.
							conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
							// Snapshot should be owned by the VM resource.
							Expect(controllerutil.SetOwnerReference(vm, secondSnapshot, ctx.Scheme)).To(Succeed())
							Expect(ctx.Client.Create(ctx, secondSnapshot)).To(Succeed())

							// Verify the VM is powered on
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
							state, err := vcVM.PowerState(ctx)
							Expect(err).ToNot(HaveOccurred())
							Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))

							// Set desired snapshot to first snapshot (revert from second to first)
							vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)

							// Revert to the first snapshot
							err = createOrUpdateVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							// Verify VM status reflects the reverted snapshot
							Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
							Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
							Expect(vm.Status.CurrentSnapshot.Reference).To(Not(BeNil()))
							Expect(vm.Status.CurrentSnapshot.Reference.Name).To(Equal(vmSnapshot.Name))

							// Verify the spec.currentSnapshot is cleared.
							Expect(vm.Spec.CurrentSnapshot).To(BeNil())

							// Verify the VM is powered off
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
							state, err = vcVM.PowerState(ctx)
							Expect(err).ToNot(HaveOccurred())
							Expect(state).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOff))
						})
					})
				})

				// Simulate an Imported Snapshot scenario by
				//	- creating the VC VM and VM while ensuring the backup is not taken.
				//	- creating a snapshot on VC AND THEN only creating the VMSnapshot CR so that the ExtraConfig is
				// 	  not stamped by the controller.
				// 	- change some bits in the VM CR and take a second snapshot. This snapshot can be taken through a
				//	  VMSnapshot. This is needed to make sure that we are not reverting to a snapshot that the VM is
				//    running off at the same time.
				//	- Now, revert the VM to the first snapshot. It is expected that the spec fields would now be approximated.
				Context("when reverting to imported snapshot", func() {
					var secondSnapshot *vmopv1.VirtualMachineSnapshot

					BeforeEach(func() {
						pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
							config.Features.VMImportNewNet = true
						})
					})
					It("should fail the revert if the snapshot wasn't imported", func() {
						if vm.Labels == nil {
							vm.Labels = make(map[string]string)
						}

						// skip creation of backup VMResourceYAMLExtraConfigKey
						// by setting the CAPV cluster role label
						vm.Labels[kubeutil.CAPVClusterRoleLabelKey] = ""

						// Create VM first
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// make sure the VM doesn't have the ExtraConfig stamped
						var moVM mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
						Expect(moVM.Config.ExtraConfig).ToNot(BeNil())
						ecMap := pkgutil.OptionValues(moVM.Config.ExtraConfig).StringMap()
						Expect(ecMap).ToNot(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))

						// Create first snapshot in vCenter
						task, err := vcVM.CreateSnapshot(
							ctx, vmSnapshot.Name, "first snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create first snapshot CR and Mark the snapshot as ready
						// so that the snapshot workflow doesn't try to create it.
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(
							ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(
							&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

						// mark the snapshot as ready because snapshot workflow
						// will skip because of the CAPV cluster role label
						cur := &vmopv1.VirtualMachineSnapshot{}
						Expect(ctx.Client.Get(
							ctx, client.ObjectKeyFromObject(vmSnapshot), cur)).To(Succeed())
						conditions.MarkTrue(cur, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Status().Update(ctx, cur)).To(Succeed())

						// we don't need the CAPI label anymore
						labels := vm.Labels
						delete(labels, kubeutil.CAPVClusterRoleLabelKey)
						vm.Labels = labels
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

						// modify the VM Spec to tinker with some flag
						Expect(vm.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeHard))
						vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

						// Create second snapshot
						secondSnapshot = builder.DummyVirtualMachineSnapshot("", "test-second-snap", vm.Name)
						secondSnapshot.Namespace = nsInfo.Namespace

						task, err = vcVM.CreateSnapshot(ctx, secondSnapshot.Name, "second snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create second snapshot CR and Mark the snapshot as ready
						// so that the snapshot workflow doesn't try to create it.
						conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						// Snapshot should be owned by the VM resource.
						Expect(controllerutil.SetOwnerReference(&o, secondSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Create(ctx, secondSnapshot)).To(Succeed())

						// Set desired snapshot to first snapshot (perform a revert from second to first)
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)

						err = createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("no VM YAML in snapshot config"))
						Expect(conditions.IsFalse(vm,
							vmopv1.VirtualMachineSnapshotRevertSucceeded)).To(BeTrue())
						Expect(conditions.GetReason(vm,
							vmopv1.VirtualMachineSnapshotRevertSucceeded,
						)).To(Equal(vmopv1.VirtualMachineSnapshotRevertFailedInvalidVMManifestReason))
					})

					It("should successfully revert to desired snapshot and approximate the VM Spec", func() {
						if vm.Labels == nil {
							vm.Labels = make(map[string]string)
						}

						// skip creation of backup VMResourceYAMLExtraConfigKey by setting the CAPV cluster role label
						vm.Labels[kubeutil.CAPVClusterRoleLabelKey] = ""

						// Create VM first
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// make sure the VM doesn't have the ExtraConfig stamped
						var moVM mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
						Expect(moVM.Config.ExtraConfig).ToNot(BeNil())
						ecMap := pkgutil.OptionValues(moVM.Config.ExtraConfig).StringMap()
						Expect(ecMap).ToNot(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))

						// Create first snapshot in vCenter
						task, err := vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "first snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create first snapshot CR
						// Mark the snapshot as created so that the snapshot workflow doesn't try to create it.
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
						// Mark the snapshot as ready so that the snapshot workflow doesn't try to create it.
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						vmSnapshot.Annotations[vmopv1.ImportedSnapshotAnnotation] = ""
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

						// mark the snapshot as ready because snapshot workflow will skip because of the CAPV cluster role label
						cur := &vmopv1.VirtualMachineSnapshot{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vmSnapshot), cur)).To(Succeed())
						conditions.MarkTrue(cur, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Status().Update(ctx, cur)).To(Succeed())

						// we don't need the CAPI label anymore
						labels := vm.Labels
						delete(labels, kubeutil.CAPVClusterRoleLabelKey)
						vm.Labels = labels
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

						// modify the VM Spec to tinker with some flag
						Expect(vm.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeHard))
						vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

						// Create second snapshot
						secondSnapshot = builder.DummyVirtualMachineSnapshot("", "test-second-snap", vm.Name)
						secondSnapshot.Namespace = nsInfo.Namespace

						task, err = vcVM.CreateSnapshot(ctx, secondSnapshot.Name, "second snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create second snapshot CR
						// Mark the snapshot as created so that the snapshot workflow doesn't try to create it.
						conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)
						// Mark the snapshot as ready so that the revert snapshot workflow can proceed.
						conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						// Snapshot should be owned by the VM resource.
						Expect(controllerutil.SetOwnerReference(&o, secondSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Create(ctx, secondSnapshot)).To(Succeed())

						// Set desired snapshot to first snapshot (perform a revert from second to first)
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)

						err = createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Verify VM status reflects the reverted snapshot
						Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
						Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.CurrentSnapshot.Reference).To(Not(BeNil()))
						Expect(vm.Status.CurrentSnapshot.Reference.Name).To(Equal(vmSnapshot.Name))

						// Verify the revert operation reverted to the expected values
						Expect(vm.Spec.PowerOffMode).To(Equal(vmopv1.VirtualMachinePowerOpModeTrySoft))
						Expect(vm.Spec.Volumes).To(BeEmpty())
					})
				})

				Context("when VM spec has nil CurrentSnapshot, but the VC VM has a snapshot", func() {
					It("should not attempt revert and update status correctly", func() {

						// Create VM with snapshot but don't set desired snapshot
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Create snapshot in vCenter
						task, err := vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "test snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create snapshot CR with the owner reference to the VM.
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

						// Explicitly set CurrentSnapshot to nil
						vm.Spec.CurrentSnapshot = nil

						err = createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Status should reflect the actual current snapshot
						Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
						Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.CurrentSnapshot.Reference).To(Not(BeNil()))
						Expect(vm.Status.CurrentSnapshot.Reference.Name).To(Equal(vmSnapshot.Name))

						// Verify the status has root snapshots.
						Expect(vm.Status.RootSnapshots).ToNot(BeNil())
						Expect(vm.Status.RootSnapshots).To(HaveLen(1))
						Expect(vm.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.RootSnapshots[0].Reference).To(Not(BeNil()))
						Expect(vm.Status.RootSnapshots[0].Reference.Name).To(Equal(vmSnapshot.Name))
					})
				})

				Context("when VM is a VKS/TKG node", func() {
					It("should skip snapshot revert for VKS/TKG nodes", func() {
						// Add CAPI labels to mark VM as VKS/TKG node
						if vm.Labels == nil {
							vm.Labels = make(map[string]string)
						}
						vm.Labels[kubeutil.CAPWClusterRoleLabelKey] = "worker"
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

						// Create VM first
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Create snapshot in vCenter
						task, err := vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "test snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create snapshot CR and mark it as ready
						conditions.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o := vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, vmSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, vmSnapshot)).To(Succeed())

						// Create a second snapshot in vCenter
						secondSnapshot := builder.DummyVirtualMachineSnapshot("", "test-second-snap", vm.Name)
						secondSnapshot.Namespace = nsInfo.Namespace

						task, err = vcVM.CreateSnapshot(ctx, vmSnapshot.Name, "test snapshot", false, false)
						Expect(err).ToNot(HaveOccurred())
						Expect(task.Wait(ctx)).To(Succeed())

						// Create snapshot CR and mark it as ready
						conditions.MarkTrue(secondSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)
						Expect(ctx.Client.Create(ctx, secondSnapshot)).To(Succeed())

						// Snapshot should be owned by the VM resource.
						o = vmopv1.VirtualMachine{}
						Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
						Expect(controllerutil.SetOwnerReference(&o, secondSnapshot, ctx.Scheme)).To(Succeed())
						Expect(ctx.Client.Update(ctx, secondSnapshot)).To(Succeed())

						// Set desired snapshot to trigger a revert to the first snapshot.
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)

						err = createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// VM status should still point to first snapshot because revert was skipped
						Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
						Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineSnapshotRevertSucceeded)).To(BeTrue())
						Expect(conditions.GetReason(vm, vmopv1.VirtualMachineSnapshotRevertSucceeded)).To(Equal(vmopv1.VirtualMachineSnapshotRevertSkippedReason))
						Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
						Expect(vm.Status.CurrentSnapshot.Reference).To(Not(BeNil()))
						Expect(vm.Status.CurrentSnapshot.Reference.Name).To(Equal(vmSnapshot.Name))

						// Verify the snapshot in vCenter is still the original one (no revert happened)
						var moVM mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
						Expect(moVM.Snapshot).ToNot(BeNil())
						Expect(moVM.Snapshot.CurrentSnapshot).ToNot(BeNil())

						// The current snapshot name should still be the original
						currentSnap, err := virtualmachine.FindSnapshot(moVM, moVM.Snapshot.CurrentSnapshot.Value)
						Expect(err).ToNot(HaveOccurred())
						Expect(currentSnap).ToNot(BeNil())
						Expect(currentSnap.Name).To(Equal(vmSnapshot.Name))
					})
				})

				Context("when snapshot revert annotation is present", func() {
					It("should skip VM reconciliation when revert annotation exists", func() {
						// Create VM first
						_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Set the revert in progress annotation manually
						if vm.Annotations == nil {
							vm.Annotations = make(map[string]string)
						}
						vm.Annotations[pkgconst.VirtualMachineSnapshotRevertInProgressAnnotationKey] = ""
						Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

						// Hack: set the label to indicate that this VM is a VKS node otherwise, a
						// successful backup returns a NoRequeue error expecting the watcher to
						// queue the request.
						vm.Labels[kubeutil.CAPVClusterRoleLabelKey] = ""

						// Attempt to reconcile VM - should return NoRequeueError due to annotation
						err = vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
						Expect(err).To(HaveOccurred())
						Expect(pkgerr.IsNoRequeueError(err)).To(BeTrue(), "Should return NoRequeueError when annotation is present")
						Expect(err.Error()).To(ContainSubstring("snapshot revert in progress"))
					})
				})

				Context("when snapshot revert fails and revert is aborted", func() {
					It("should clear the revert succeeded condition", func() {
						vm.Spec.CurrentSnapshot = snapshotPartialRef(vmSnapshot.Name)

						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(
							ContainSubstring("virtualmachinesnapshots.vmoperator.vmware.com " +
								"\"test-revert-snap\" not found"))

						Expect(conditions.IsFalse(vm,
							vmopv1.VirtualMachineSnapshotRevertSucceeded,
						)).To(BeTrue())
						Expect(conditions.GetReason(vm,
							vmopv1.VirtualMachineSnapshotRevertSucceeded,
						)).To(Equal(vmopv1.VirtualMachineSnapshotRevertFailedReason))

						vm.Spec.CurrentSnapshot = nil
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

						Expect(conditions.Get(vm,
							vmopv1.VirtualMachineSnapshotRevertSucceeded),
						).To(BeNil())
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

				Expect(vm.Labels).To(HaveKeyWithValue(corev1.LabelTopologyZone, zoneName))
				Expect(vm.Status.Zone).To(Equal(zoneName))
				delete(vm.Labels, corev1.LabelTopologyZone)

				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Labels).To(HaveKeyWithValue(corev1.LabelTopologyZone, zoneName))
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

			When("a cluster module is specified without resource policy", func() {
				JustBeforeEach(func() {
					vm.Spec.Reserved.ResourcePolicyName = ""
				})

				It("returns error", func() {
					_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("cannot set cluster module without resource policy"))
				})
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
						vm.Labels[corev1.LabelTopologyZone],
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
				clusterModName := "bogusClusterMod"
				vm.Annotations["vsphere-cluster-module-group"] = clusterModName
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("ClusterModule %q not found", clusterModName)))
			})
		})

		Context("Delete VM", func() {
			const zoneName = "az-1"

			BeforeEach(func() {
				// Explicitly place the VM into one of the zones that the test context will create.
				vm.Labels[corev1.LabelTopologyZone] = zoneName
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
				delete(vm.Labels, corev1.LabelTopologyZone)
				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
			})

			It("Deletes existing VM when zone info is missing", func() {
				_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				uniqueID := vm.Status.UniqueID
				Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

				Expect(vm.Labels).To(HaveKeyWithValue(corev1.LabelTopologyZone, zoneName))
				delete(vm.Labels, corev1.LabelTopologyZone)

				Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
			})

			DescribeTable("VM is not connected",
				func(state vimtypes.VirtualMachineConnectionState) {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					var moVM mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

					sctx := ctx.SimulatorContext()
					sctx.WithLock(
						vcVM.Reference(),
						func() {
							vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
							vm.Summary.Runtime.ConnectionState = state
						})

					err = vmProvider.DeleteVirtualMachine(ctx, vm)

					if state == "" {
						Expect(err).ToNot(HaveOccurred())
						Expect(ctx.GetVMFromMoID(vm.Status.UniqueID)).To(BeNil())
					} else {
						Expect(err).To(HaveOccurred())
						var noRequeueErr pkgerr.NoRequeueError
						Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
						Expect(noRequeueErr.Message).To(Equal(
							fmt.Sprintf("unsupported connection state: %s", state)))
						Expect(ctx.GetVMFromMoID(vm.Status.UniqueID)).ToNot(BeNil())
					}
				},
				Entry("empty", vimtypes.VirtualMachineConnectionState("")),
				Entry("disconnected", vimtypes.VirtualMachineConnectionStateDisconnected),
				Entry("inaccessible", vimtypes.VirtualMachineConnectionStateInaccessible),
				Entry("invalid", vimtypes.VirtualMachineConnectionStateInvalid),
				Entry("orphaned", vimtypes.VirtualMachineConnectionStateOrphaned),
			)
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
				vm = builder.DummyBasicVirtualMachine("test-vm-iso", "")

				// Reduce diff from old tests: by default don't create an NIC.
				if vm.Spec.Network == nil {
					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
				}
				vm.Spec.Network.Disabled = true
			})

			JustBeforeEach(func() {
				vmClass.Namespace = nsInfo.Namespace
				Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

				// Add required objects to get CD-ROM backing file name.
				cvmiName := "vmi-iso"
				objs := builder.DummyImageAndItemObjectsForCdromBacking(cvmiName, "", cvmiKind, "test-file.iso", ctx.ContentLibraryIsoItemID, true, true, resource.MustParse("100Mi"), true, true, "ISO")
				for _, obj := range objs {
					Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
				}

				vm.Namespace = nsInfo.Namespace
				vm.Spec.ClassName = vmClass.Name
				vm.Spec.ImageName = cvmiName
				vm.Spec.Image.Kind = cvmiKind
				vm.Spec.Image.Name = cvmiName
				vm.Spec.StorageClass = ctx.StorageClassName
				vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					Cdrom: []vmopv1.VirtualMachineCdromSpec{{
						Name: "cdrom0",
						Image: vmopv1.VirtualMachineImageRef{
							Name: cvmiName,
							Kind: cvmiKind,
						},
					}},
				}

				Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
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

		Context("Power states", func() {

			getLastRestartTime := func(moVM mo.VirtualMachine) string {
				for i := range moVM.Config.ExtraConfig {
					ov := moVM.Config.ExtraConfig[i].GetOptionValue()
					if ov.Key == "vmservice.lastRestartTime" {
						return ov.Value.(string)
					}
				}
				return ""
			}

			var (
				vcVM *object.VirtualMachine
				moVM mo.VirtualMachine
			)

			JustBeforeEach(func() {
				var err error
				moVM = mo.VirtualMachine{}
				vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
			})

			When("vcVM is powered on", func() {
				JustBeforeEach(func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
				})

				When("power state is not changed", func() {
					BeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})
					It("should not return an error", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					})
				})

				When("powering off the VM", func() {
					JustBeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					})

					When("power state should not be updated", func() {
						const expectedPowerState = vmopv1.VirtualMachinePowerStateOn
						When("vm is paused by devops", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									vmopv1.PauseAnnotation: "true",
								}
							})
							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrIsPaused)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})
						When("vm is paused by admin", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									vmopv1.PauseAnnotation: "true",
								}
								t, err := vcVM.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
									ExtraConfig: []vimtypes.BaseOptionValue{
										&vimtypes.OptionValue{
											Key:   vmopv1.PauseVMExtraConfigKey,
											Value: "true",
										},
									},
								})
								Expect(err).ToNot(HaveOccurred())
								Expect(t.Wait(ctx)).To(Succeed())
							})
							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrIsPaused)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})

						When("vm has running task", func() {
							var (
								reg     *simulator.Registry
								simCtx  *simulator.Context
								taskRef vimtypes.ManagedObjectReference
							)

							JustBeforeEach(func() {
								simCtx = ctx.SimulatorContext()
								reg = simCtx.Map
								taskRef = reg.Put(&mo.Task{
									Info: vimtypes.TaskInfo{
										State:         vimtypes.TaskInfoStateRunning,
										DescriptionId: "fake.task.1",
									},
								}).Reference()

								vmRef := vimtypes.ManagedObjectReference{
									Type:  string(vimtypes.ManagedObjectTypeVirtualMachine),
									Value: vm.Status.UniqueID,
								}

								reg.WithLock(
									simCtx,
									vmRef,
									func() {
										vm := reg.Get(vmRef).(*simulator.VirtualMachine)
										vm.RecentTask = append(vm.RecentTask, taskRef)
									})

							})

							AfterEach(func() {
								reg.Remove(simCtx, taskRef)
							})

							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrHasTask)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})

					})

					DescribeTable("powerOffModes",
						func(mode vmopv1.VirtualMachinePowerOpMode) {
							vm.Spec.PowerOffMode = mode
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						},
						Entry("hard", vmopv1.VirtualMachinePowerOpModeHard),
						Entry("soft", vmopv1.VirtualMachinePowerOpModeSoft),
						Entry("trySoft", vmopv1.VirtualMachinePowerOpModeTrySoft),
					)

					When("there is a config error", func() {
						JustBeforeEach(func() {
							vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
								CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
									CloudConfig: &cloudinit.CloudConfig{
										RunCmd: json.RawMessage([]byte("invalid")),
									},
								},
							}
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						})
						It("should still power off the VM", func() {
							err := createOrUpdateVM(ctx, vmProvider, vm)
							Expect(err).To(HaveOccurred())
							Expect(err.Error()).To(ContainSubstring("failed to reconcile config: updating state failed with failed to create bootstrap data"))

							// Do it again to update status.
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).ToNot(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						})
					})
				})

				When("restarting the VM", func() {
					var (
						oldLastRestartTime string
					)

					JustBeforeEach(func() {
						oldLastRestartTime = getLastRestartTime(moVM)
						vm.Spec.NextRestartTime = time.Now().UTC().Format(time.RFC3339Nano)
					})

					When("restartMode is hard", func() {
						JustBeforeEach(func() {
							vm.Spec.RestartMode = vmopv1.VirtualMachinePowerOpModeHard
						})
						It("should restart the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
							newLastRestartTime := getLastRestartTime(moVM)
							Expect(newLastRestartTime).ToNot(BeEmpty())
							Expect(newLastRestartTime).ToNot(Equal(oldLastRestartTime))
						})
					})
					When("restartMode is soft", func() {
						JustBeforeEach(func() {
							vm.Spec.RestartMode = vmopv1.VirtualMachinePowerOpModeSoft
						})
						It("should return an error about lacking tools", func() {
							Expect(testutil.ContainsError(createOrUpdateVM(ctx, vmProvider, vm), "failed to soft restart vm ServerFaultCode: ToolsUnavailable")).To(BeTrue())
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
							newLastRestartTime := getLastRestartTime(moVM)
							Expect(newLastRestartTime).To(Equal(oldLastRestartTime))
						})
					})
					When("restartMode is trySoft", func() {
						JustBeforeEach(func() {
							vm.Spec.RestartMode = vmopv1.VirtualMachinePowerOpModeTrySoft
						})
						It("should restart the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
							newLastRestartTime := getLastRestartTime(moVM)
							Expect(newLastRestartTime).ToNot(BeEmpty())
							Expect(newLastRestartTime).ToNot(Equal(oldLastRestartTime))
						})
					})
				})

				When("suspending the VM", func() {
					JustBeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					})
					When("power state should not be updated", func() {
						const expectedPowerState = vmopv1.VirtualMachinePowerStateOn
						When("vm is paused by devops", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									vmopv1.PauseAnnotation: "true",
								}
							})
							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrIsPaused)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})
						When("vm is paused by admin", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									vmopv1.PauseAnnotation: "true",
								}
								t, err := vcVM.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
									ExtraConfig: []vimtypes.BaseOptionValue{
										&vimtypes.OptionValue{
											Key:   vmopv1.PauseVMExtraConfigKey,
											Value: "true",
										},
									},
								})
								Expect(err).ToNot(HaveOccurred())
								Expect(t.Wait(ctx)).To(Succeed())
							})
							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrIsPaused)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})

						When("vm has running task", func() {
							var (
								reg     *simulator.Registry
								simCtx  *simulator.Context
								taskRef vimtypes.ManagedObjectReference
							)

							JustBeforeEach(func() {
								simCtx = ctx.SimulatorContext()
								reg = simCtx.Map
								taskRef = reg.Put(&mo.Task{
									Info: vimtypes.TaskInfo{
										State:         vimtypes.TaskInfoStateRunning,
										DescriptionId: "fake.task.2",
									},
								}).Reference()

								vmRef := vimtypes.ManagedObjectReference{
									Type:  string(vimtypes.ManagedObjectTypeVirtualMachine),
									Value: vm.Status.UniqueID,
								}

								reg.WithLock(
									simCtx,
									vmRef,
									func() {
										vm := reg.Get(vmRef).(*simulator.VirtualMachine)
										vm.RecentTask = append(vm.RecentTask, taskRef)
									})

							})

							AfterEach(func() {
								reg.Remove(simCtx, taskRef)
							})

							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrHasTask)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})
					})

					When("suspendMode is hard", func() {
						JustBeforeEach(func() {
							vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeHard
						})
						It("should suspend the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
						})
					})
					When("suspendMode is soft", func() {
						JustBeforeEach(func() {
							vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeSoft
						})
						It("should suspend the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
						})
					})
					When("suspendMode is trySoft", func() {
						JustBeforeEach(func() {
							vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeTrySoft
						})
						It("should suspend the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
						})
					})
				})
			})

			When("vcVM is powered off", func() {
				JustBeforeEach(func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				})

				When("power state is not changed", func() {
					It("should not return an error", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})
				})

				When("powering on the VM", func() {

					JustBeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})

					When("power state should not be updated", func() {
						const expectedPowerState = vmopv1.VirtualMachinePowerStateOff
						When("vm is paused by devops", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									vmopv1.PauseAnnotation: "true",
								}
							})
							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrIsPaused)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})
						When("vm is paused by admin", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									vmopv1.PauseAnnotation: "true",
								}
								t, err := vcVM.Reconfigure(ctx, vimtypes.VirtualMachineConfigSpec{
									ExtraConfig: []vimtypes.BaseOptionValue{
										&vimtypes.OptionValue{
											Key:   vmopv1.PauseVMExtraConfigKey,
											Value: "true",
										},
									},
								})
								Expect(err).ToNot(HaveOccurred())
								Expect(t.Wait(ctx)).To(Succeed())
							})
							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrIsPaused)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})

						When("vm has running task", func() {
							var (
								reg     *simulator.Registry
								simCtx  *simulator.Context
								taskRef vimtypes.ManagedObjectReference
							)

							JustBeforeEach(func() {
								simCtx = ctx.SimulatorContext()
								reg = simCtx.Map
								taskRef = reg.Put(&mo.Task{
									Info: vimtypes.TaskInfo{
										State:         vimtypes.TaskInfoStateRunning,
										DescriptionId: "fake.task.3",
									},
								}).Reference()

								vmRef := vimtypes.ManagedObjectReference{
									Type:  string(vimtypes.ManagedObjectTypeVirtualMachine),
									Value: vm.Status.UniqueID,
								}

								reg.WithLock(
									simCtx,
									vmRef,
									func() {
										vm := reg.Get(vmRef).(*simulator.VirtualMachine)
										vm.RecentTask = append(vm.RecentTask, taskRef)
									})

							})

							AfterEach(func() {
								reg.Remove(simCtx, taskRef)
							})

							It("should not change the power state", func() {
								Expect(errors.Is(createOrUpdateVM(ctx, vmProvider, vm), vsphere.ErrHasTask)).To(BeTrue())
								Expect(vm.Status.PowerState).To(Equal(expectedPowerState))
							})
						})

					})

					When("there is a power on check annotation", func() {
						JustBeforeEach(func() {
							vm.Annotations = map[string]string{
								vmopv1.CheckAnnotationPowerOn + "/app": "reason",
							}
						})
						It("should not power on the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						})
					})

					When("there is a apply power state change time annotation", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.Features.VMGroups = true
							})
						})

						When("the time is in the future", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									pkgconst.ApplyPowerStateTimeAnnotation: time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano),
								}
							})

							It("should not power on the VM and requeue after remaining time", func() {
								err := createOrUpdateVM(ctx, vmProvider, vm)
								Expect(err).To(HaveOccurred())
								var requeueErr pkgerr.RequeueError
								Expect(errors.As(err, &requeueErr)).To(BeTrue())
								Expect(requeueErr.After).To(BeNumerically("~", time.Minute, time.Second))
							})
						})

						When("the time is in the past", func() {
							JustBeforeEach(func() {
								vm.Annotations = map[string]string{
									pkgconst.ApplyPowerStateTimeAnnotation: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
								}
							})

							It("should power on the VM and remove the annotation", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
								Expect(vm.Annotations).ToNot(HaveKey(pkgconst.ApplyPowerStateTimeAnnotation))
							})
						})
					})

					const (
						oldDiskSizeBytes = int64(31457280)
						newDiskSizeGi    = 20
						newDiskSizeBytes = int64(newDiskSizeGi * 1024 * 1024 * 1024)
					)

					When("the boot disk size is changed for non-ISO VMs", func() {
						JustBeforeEach(func() {
							vmDevs := object.VirtualDeviceList(moVM.Config.Hardware.Device)
							disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
							Expect(disks).To(HaveLen(1))
							Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
							diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
							Expect(diskCapacityBytes).To(Equal(oldDiskSizeBytes))

							q := resource.MustParse(fmt.Sprintf("%dGi", newDiskSizeGi))
							vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
								BootDiskCapacity: &q,
							}
							if vm.Spec.Hardware == nil {
								vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
							}
							vm.Spec.Hardware.Cdrom = nil
						})
						It("should power on the VM with the boot disk resized", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
							vmDevs := object.VirtualDeviceList(moVM.Config.Hardware.Device)
							disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
							Expect(disks).To(HaveLen(1))
							Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
							diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
							Expect(diskCapacityBytes).To(Equal(newDiskSizeBytes))
						})
					})

					When("there are no NICs", func() {
						JustBeforeEach(func() {
							vm.Spec.Network.Interfaces = nil
						})
						It("should power on the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
						})
					})

					When("there is a single NIC", func() {
						JustBeforeEach(func() {
							vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
								{
									Name: "eth0",
									Network: &vmopv1common.PartialObjectRef{
										Name: "VM Network",
									},
								},
							}
						})
						When("with networking disabled", func() {
							JustBeforeEach(func() {
								vm.Spec.Network.Disabled = true
							})
							It("should power on the VM", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
							})
						})
					})

					When("VM.Spec.GuestID is changed", func() {

						When("the guest ID value is invalid", func() {

							JustBeforeEach(func() {
								vm.Spec.GuestID = "invalid-guest-id"
							})

							It("should return an error and set the VM's Guest ID condition false", func() {
								err := createOrUpdateVM(ctx, vmProvider, vm)
								Expect(err.Error()).To(ContainSubstring("reconfigure VM task failed"))

								c := conditions.Get(vm, vmopv1.GuestIDReconfiguredCondition)
								Expect(c).ToNot(BeNil())
								expectedCondition := conditions.FalseCondition(
									vmopv1.GuestIDReconfiguredCondition,
									"Invalid",
									"The specified guest ID value is not supported: invalid-guest-id",
								)
								Expect(*c).To(conditions.MatchCondition(*expectedCondition))
							})
						})

						When("the guest ID value is valid", func() {

							JustBeforeEach(func() {
								vm.Spec.GuestID = "vmwarePhoton64Guest"
							})

							It("should power on the VM with the specified guest ID", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

								Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())
								Expect(moVM.Config.GuestId).To(Equal("vmwarePhoton64Guest"))
							})
						})

						When("the guest ID spec is removed", func() {

							JustBeforeEach(func() {
								vm.Spec.GuestID = ""
							})

							It("should clear the VM guest ID condition if previously set", func() {
								vm.Status.Conditions = []metav1.Condition{
									{
										Type:   vmopv1.GuestIDReconfiguredCondition,
										Status: metav1.ConditionFalse,
									},
								}

								// Customize
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

								Expect(conditions.Get(vm, vmopv1.GuestIDReconfiguredCondition)).To(BeNil())
							})
						})
					})

					When("VM has CD-ROM", func() {

						const (
							vmiName     = "vmi-iso"
							vmiKind     = "VirtualMachineImage"
							vmiFileName = "dummy.iso"
						)

						JustBeforeEach(func() {
							vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
								Cdrom: []vmopv1.VirtualMachineCdromSpec{
									{
										Name: "cdrom1",
										Image: vmopv1.VirtualMachineImageRef{
											Name: vmiName,
											Kind: vmiKind,
										},
										AllowGuestControl: ptr.To(true),
										Connected:         ptr.To(true),
									},
								},
							}
							testConfig.WithContentLibrary = true
						})

						JustBeforeEach(func() {
							// Add required objects to get CD-ROM backing file name.
							objs := builder.DummyImageAndItemObjectsForCdromBacking(
								vmiName,
								vm.Namespace,
								vmiKind,
								vmiFileName,
								ctx.ContentLibraryIsoItemID,
								true,
								true,
								resource.MustParse("100Mi"),
								true,
								true,
								"ISO")
							for _, obj := range objs {
								Expect(ctx.Client.Create(ctx, obj)).To(Succeed())
							}
						})

						assertPowerOnVMWithCDROM := func() {
							ExpectWithOffset(1, createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							ExpectWithOffset(1, vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

							ExpectWithOffset(1, vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

							cdromDeviceList := object.VirtualDeviceList(moVM.Config.Hardware.Device).SelectByType(&vimtypes.VirtualCdrom{})
							ExpectWithOffset(1, cdromDeviceList).To(HaveLen(1))
							cdrom := cdromDeviceList[0].(*vimtypes.VirtualCdrom)
							ExpectWithOffset(1, cdrom.Connectable.StartConnected).To(BeTrue())
							ExpectWithOffset(1, cdrom.Connectable.Connected).To(BeTrue())
							ExpectWithOffset(1, cdrom.Connectable.AllowGuestControl).To(BeTrue())
							ExpectWithOffset(1, cdrom.ControllerKey).ToNot(BeZero())
							ExpectWithOffset(1, cdrom.UnitNumber).ToNot(BeNil())
							ExpectWithOffset(1, cdrom.Backing).To(BeAssignableToTypeOf(&vimtypes.VirtualCdromIsoBackingInfo{}))
							backing := cdrom.Backing.(*vimtypes.VirtualCdromIsoBackingInfo)
							ExpectWithOffset(1, backing.FileName).To(Equal(vmiFileName))
						}

						assertNotPowerOnVMWithCDROM := func() {
							err := createOrUpdateVM(ctx, vmProvider, vm)
							ExpectWithOffset(1, err).To(HaveOccurred())
							ExpectWithOffset(1, err.Error()).To(ContainSubstring("no CD-ROM is found for image ref"))
						}

						It("should power on the VM with expected CD-ROM device", assertPowerOnVMWithCDROM)

						When("FSS Resize is enabled", func() {
							JustBeforeEach(func() {
								pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
									config.Features.VMResize = true
								})
							})
							It("should not power on the VM with expected CD-ROM device", assertNotPowerOnVMWithCDROM)
						})

						When("FSS Resize CPU & Memory is enabled", func() {
							JustBeforeEach(func() {
								pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
									config.Features.VMResizeCPUMemory = true
								})
							})
							It("should power on the VM with expected CD-ROM device", assertPowerOnVMWithCDROM)
						})

						When("the boot disk size is changed for VM with CD-ROM", func() {

							JustBeforeEach(func() {
								vmDevs := object.VirtualDeviceList(moVM.Config.Hardware.Device)
								disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
								Expect(disks).To(HaveLen(1))
								Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
								diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
								Expect(diskCapacityBytes).To(Equal(oldDiskSizeBytes))

								q := resource.MustParse(fmt.Sprintf("%dGi", newDiskSizeGi))
								vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
									BootDiskCapacity: &q,
								}
							})

							It("should power on the VM without the boot disk resized", func() {
								Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
								Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
								Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &moVM)).To(Succeed())

								vmDevs := object.VirtualDeviceList(moVM.Config.Hardware.Device)
								disks := vmDevs.SelectByType(&vimtypes.VirtualDisk{})
								Expect(disks).To(HaveLen(1))
								Expect(disks[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualDisk{}))
								diskCapacityBytes := disks[0].(*vimtypes.VirtualDisk).CapacityInBytes
								Expect(diskCapacityBytes).To(Equal(oldDiskSizeBytes))
							})
						})
					})
				})

				When("suspending the VM", func() {
					JustBeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					})
					When("suspendMode is hard", func() {
						JustBeforeEach(func() {
							vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeHard
						})
						It("should not suspend the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						})
					})
					When("suspendMode is soft", func() {
						JustBeforeEach(func() {
							vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeSoft
						})
						It("should not suspend the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						})
					})
					When("suspendMode is trySoft", func() {
						JustBeforeEach(func() {
							vm.Spec.SuspendMode = vmopv1.VirtualMachinePowerOpModeTrySoft
						})
						It("should not suspend the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						})
					})
				})

				When("there is a config error", func() {
					JustBeforeEach(func() {
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
								CloudConfig: &cloudinit.CloudConfig{
									RunCmd: json.RawMessage([]byte("invalid")),
								},
							},
						}
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})
					It("should not power on the VM", func() {
						err := createOrUpdateVM(ctx, vmProvider, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("failed to reconcile config: updating state failed with failed to create bootstrap data"))

						// Do it again to update status.
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).ToNot(Succeed())
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
					})
				})
			})

			When("vcVM is suspended", func() {
				JustBeforeEach(func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
					Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
				})

				When("power state is not changed", func() {
					It("should not return an error", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
					})
				})

				When("powering on the VM", func() {

					JustBeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					})

					It("should power on the VM", func() {
						Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
					})

					When("there is a power on check annotation", func() {
						JustBeforeEach(func() {
							vm.Annotations = map[string]string{
								vmopv1.CheckAnnotationPowerOn + "/app": "reason",
							}
						})
						It("should not power on the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
						})
					})
				})

				When("powering off the VM", func() {
					JustBeforeEach(func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
					})
					When("powerOffMode is hard", func() {
						JustBeforeEach(func() {
							vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeHard
						})
						It("should power off the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						})
					})
					When("powerOffMode is soft", func() {
						JustBeforeEach(func() {
							vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeSoft
						})
						It("should not power off the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateSuspended))
						})
					})
					When("powerOffMode is trySoft", func() {
						JustBeforeEach(func() {
							vm.Spec.PowerOffMode = vmopv1.VirtualMachinePowerOpModeTrySoft
						})
						It("should power off the VM", func() {
							Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
							Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
						})
					})
				})
			})
		})

		Context("Snapshot create", func() {
			var (
				snapshot1 *vmopv1.VirtualMachineSnapshot
				snapshot2 *vmopv1.VirtualMachineSnapshot
			)

			BeforeEach(func() {
				testConfig.WithVMSnapshots = true
			})

			Context("when no snapshots exist", func() {
				It("should complete without error", func() {
					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when one snapshot exists is not created", func() {
				JustBeforeEach(func() {
					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot1)).To(Succeed())
				})

				It("should process the snapshot", func() {
					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// Verify snapshot was processed
					updatedSnapshot := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())
					Expect(updatedSnapshot.Status.Quiesced).To(BeTrue())
					// Snapshot should be powered off since memory is not included in the snapshot
					Expect(updatedSnapshot.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
				})
			})

			Context("when multiple snapshots exist", func() {
				It("should process snapshots in order (oldest first)", func() {
					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					creationTimeStamp := metav1.NewTime(time.Now())
					snapshot1.CreationTimestamp = creationTimeStamp
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot1)).To(Succeed())

					later := metav1.NewTime(time.Now().Add(1 * time.Second))
					snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
					snapshot2.CreationTimestamp = later
					Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					Expect(controllerutil.SetOwnerReference(&o, snapshot2, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot2)).To(Succeed())

					// First reconcile should process snapshot1, and requeue to process snapshot2.
					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).To(HaveOccurred())
					Expect(pkgerr.IsRequeueError(err)).To(BeTrue())
					Expect(err.Error()).To(ContainSubstring("requeuing to process 1 remaining snapshots"))

					// Check that snapshot1 is marked as created
					updatedSnapshot1 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot1,
						vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())

					// Check that snapshot2 is not marked as created
					updatedSnapshot2 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot2.Name,
						Namespace: snapshot2.Namespace,
					}, updatedSnapshot2)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot2,
						vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeFalse())

					// Second reconcile should process snapshot2
					err = createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// Now snapshot2 should be marked as created
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot2.Name,
						Namespace: snapshot2.Namespace,
					}, updatedSnapshot2)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot2,
						vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())

					// Now snapshot1's children list should contain snapshot2
					Expect(ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)).To(Succeed())
					Expect(updatedSnapshot1.Status.Children).To(HaveLen(1))
					Expect(updatedSnapshot1.Status.Children[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
					Expect(updatedSnapshot1.Status.Children[0].Reference).To(Not(BeNil()))
					Expect(updatedSnapshot1.Status.Children[0].Reference.Name).To(Equal(snapshot2.Name))
				})
			})

			Context("when one snapshot is already in progress", func() {
				It("should not process any snapshots and requeues the request", func() {
					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())

					// Mark snapshot1 as in progress
					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					conditions.MarkFalse(snapshot1,
						vmopv1.VirtualMachineSnapshotCreatedCondition,
						vmopv1.VirtualMachineSnapshotCreationInProgressReason,
						"in progress")
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
					Expect(controllerutil.SetOwnerReference(&o, snapshot2, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).To(HaveOccurred())
					Expect(pkgerr.IsRequeueError(err)).To(BeTrue())

					// First snapshot should be created
					updatedSnapshot1 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)
					Expect(err).ToNot(HaveOccurred())

					// Snapshot1 is marked as created after this reconcile
					Expect(conditions.IsTrue(updatedSnapshot1, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())

					// Second snapshot will still be pending.  A requeue should have been triggered which will handle it.
					updatedSnapshot2 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot2.Name,
						Namespace: snapshot2.Namespace,
					}, updatedSnapshot2)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot2, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeFalse())
				})
			})

			Context("when snapshot is being deleted", func() {
				It("should skip all snapshot creation due to vSphere constraint", func() {
					// Mark snapshot1 as being deleted
					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					// Set a finalizer so we can mark the object for deletion.
					snapshot1.ObjectMeta.Finalizers = []string{"dummy-finalizer"}
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot1)).To(Succeed())

					snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
					Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())
					// Snapshot should be owned by the VM resource.
					Expect(controllerutil.SetOwnerReference(&o, snapshot2, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot2)).To(Succeed())

					// Delete the snapshot
					Expect(ctx.Client.Delete(ctx, snapshot1)).To(Succeed())

					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// snapshot1 should not be processed (being deleted)
					updatedSnapshot1 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot1, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeFalse())

					// snapshot2 should also not be processed due to the deletion constraint
					updatedSnapshot2 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot2.Name,
						Namespace: snapshot2.Namespace,
					}, updatedSnapshot2)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot2, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeFalse())
				})
			})

			Context("when snapshot already exists and has created condition", func() {
				It("should skip ready snapshot and process the next one", func() {
					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)

					// Mark snapshot1 as created
					conditions.MarkTrue(snapshot1, vmopv1.VirtualMachineSnapshotCreatedCondition)
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot1)).To(Succeed())

					Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					Expect(controllerutil.SetOwnerReference(&o, snapshot2, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot2)).To(Succeed())

					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// snapshot1 should remain created and not be processed again
					updatedSnapshot1 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot1,
						vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())

					// snapshot2 should be processed
					updatedSnapshot2 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot2.Name,
						Namespace: snapshot2.Namespace,
					}, updatedSnapshot2)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot2,
						vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())
				})
			})

			Context("when snapshot has nil VMRef", func() {
				It("should skip snapshot with nil VMRef and process the next one", func() {
					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)

					// Set snapshot1 VMRef to nil
					snapshot1.Spec.VMRef = nil
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot1)).To(Succeed())

					Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					Expect(controllerutil.SetOwnerReference(&o, snapshot2, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot2)).To(Succeed())

					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// snapshot1 should not be processed (nil VMRef)
					updatedSnapshot1 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot1, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeFalse())

					// snapshot2 should be processed
					updatedSnapshot2 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot2.Name,
						Namespace: snapshot2.Namespace,
					}, updatedSnapshot2)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot2, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())
				})
			})

			Context("when snapshot references different VM", func() {
				It("should skip snapshot for different VM and process the next one", func() {
					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)

					// Set snapshot1 to reference a different VM
					snapshot1.Spec.VMRef.Name = "different-vm"
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot1)).To(Succeed())

					Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					Expect(controllerutil.SetOwnerReference(&o, snapshot2, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot2)).To(Succeed())

					err := createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// snapshot1 should not be processed (different VM)
					updatedSnapshot1 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot1, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeFalse())

					// snapshot2 should be processed
					updatedSnapshot2 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot2.Name,
						Namespace: snapshot2.Namespace,
					}, updatedSnapshot2)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot2, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeTrue())
				})
			})

			Context("when VM is a VKS/TKG node", func() {
				It("should skip snapshot processing for VKS/TKG nodes", func() {
					// Add CAPI labels to mark VM as VKS/TKG node
					if vm.Labels == nil {
						vm.Labels = make(map[string]string)
					}
					vm.Labels[kubeutil.CAPWClusterRoleLabelKey] = "worker"
					Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

					// Create a vCenter VM and update the VM status to point to it
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
					Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

					// Snapshot should be owned by the VM resource.
					o := vmopv1.VirtualMachine{}
					Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), &o)).To(Succeed())
					Expect(controllerutil.SetOwnerReference(&o, snapshot1, ctx.Scheme)).To(Succeed())
					Expect(ctx.Client.Update(ctx, snapshot1)).To(Succeed())

					err = createOrUpdateVM(ctx, vmProvider, vm)
					Expect(err).ToNot(HaveOccurred())

					// Snapshot should not be processed (VM is VKS/TKG node)
					updatedSnapshot1 := &vmopv1.VirtualMachineSnapshot{}
					err = ctx.Client.Get(ctx, client.ObjectKey{
						Name:      snapshot1.Name,
						Namespace: snapshot1.Namespace,
					}, updatedSnapshot1)
					Expect(err).ToNot(HaveOccurred())
					Expect(conditions.IsTrue(updatedSnapshot1, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(BeFalse())

					// Verify no new snapshots were created on the vCenter VM
					var moVM mo.VirtualMachine
					err = vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)
					Expect(err).ToNot(HaveOccurred())
					Expect(moVM.Snapshot).To(BeNil())
				})
			})
		})

		Context("vSphere Policies", func() {

			var (
				policyTag1ID string
				policyTag2ID string
				policyTag3ID string

				tagMgr *tags.Manager
			)

			JustBeforeEach(func() {
				var err error

				tagMgr = tags.NewManager(ctx.RestClient)

				// Create a category for the policy tags
				categoryID, err := tagMgr.CreateCategory(ctx, &tags.Category{
					Name:            "my-policy-category",
					Description:     "Category for policy tags",
					AssociableTypes: []string{"VirtualMachine"},
				})
				Expect(err).ToNot(HaveOccurred())

				policyTag1ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
					Name:       "my-policy-tag-1",
					CategoryID: categoryID,
				})
				Expect(err).ToNot(HaveOccurred())

				policyTag2ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
					Name:       "my-policy-tag-2",
					CategoryID: categoryID,
				})
				Expect(err).ToNot(HaveOccurred())

				policyTag3ID, err = tagMgr.CreateTag(ctx, &tags.Tag{
					Name:       "my-policy-tag-3",
					CategoryID: categoryID,
				})
				Expect(err).ToNot(HaveOccurred())
			})

			When("Capability is enabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VSpherePolicies = true
					})
				})

				When("creating a VM", func() {
					When("async create is enabled", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.AsyncCreateEnabled = true
							})
						})
						It("should successfully create VM", func() {
							By("Setting up VM with policy evaluation objects", func() {
								// Set VM UID for proper PolicyEvaluation naming
								vm.UID = "test-vm-policy-uid"

								// Create a PolicyEvaluation object that will be found during policy reconciliation
								policyEval := &vspherepolv1.PolicyEvaluation{
									ObjectMeta: metav1.ObjectMeta{
										Namespace:  vm.Namespace,
										Name:       "vm-" + vm.Name,
										Generation: 1,
										OwnerReferences: []metav1.OwnerReference{
											{
												APIVersion:         vmopv1.GroupVersion.String(),
												Kind:               "VirtualMachine",
												Name:               vm.Name,
												UID:                vm.UID,
												Controller:         ptr.To(true),
												BlockOwnerDeletion: ptr.To(true),
											},
										},
									},
									Spec: vspherepolv1.PolicyEvaluationSpec{
										Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
											Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
												GuestID:     "ubuntu64Guest",
												GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
											},
										},
									},
									Status: vspherepolv1.PolicyEvaluationStatus{
										ObservedGeneration: 1,
										Policies: []vspherepolv1.PolicyEvaluationResult{
											{
												Name: "test-active-policy",
												Kind: "ComputePolicy",
												Tags: []string{policyTag1ID, policyTag2ID},
											},
										},
									},
								}

								// Create the PolicyEvaluation in the fake Kubernetes client
								Expect(ctx.Client.Create(ctx, policyEval)).To(Succeed())
							})

							vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							// Verify VM was created successfully
							var o mo.VirtualMachine
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

							// Verify placement condition is ready (indicates vmconfpolicy.Reconcile was called)
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())

							// Verify that policy tags were added to ExtraConfig
							By("VM has policy tags in ExtraConfig", func() {
								Expect(o.Config.ExtraConfig).ToNot(BeNil())

								ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

								// Verify tags are present
								Expect(ecMap).To(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
								activeTags := ecMap[vmconfpolicy.ExtraConfigPolicyTagsKey]
								Expect(activeTags).To(ContainSubstring(policyTag1ID))
								Expect(activeTags).To(ContainSubstring(policyTag2ID))
							})
						})

					})
					When("async create is disabled", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.AsyncCreateEnabled = false
							})
						})

						It("should successfully create VM and call vmconfpolicy.Reconcile during placement", func() {
							By("Setting up VM with policy evaluation objects", func() {
								// Set VM UID for proper PolicyEvaluation naming
								vm.UID = "test-vm-sync-policy-uid"

								// Create a PolicyEvaluation object that will be found during policy reconciliation
								policyEval := &vspherepolv1.PolicyEvaluation{
									ObjectMeta: metav1.ObjectMeta{
										Namespace:  vm.Namespace,
										Name:       "vm-" + vm.Name,
										Generation: 1,
										OwnerReferences: []metav1.OwnerReference{
											{
												APIVersion:         vmopv1.GroupVersion.String(),
												Kind:               "VirtualMachine",
												Name:               vm.Name,
												UID:                vm.UID,
												Controller:         ptr.To(true),
												BlockOwnerDeletion: ptr.To(true),
											},
										},
									},
									Spec: vspherepolv1.PolicyEvaluationSpec{
										Workload: &vspherepolv1.PolicyEvaluationWorkloadSpec{
											Guest: &vspherepolv1.PolicyEvaluationGuestSpec{
												GuestID:     "ubuntu64Guest",
												GuestFamily: vspherepolv1.GuestFamilyTypeLinux,
											},
										},
									},
									Status: vspherepolv1.PolicyEvaluationStatus{
										ObservedGeneration: 1,
										Policies: []vspherepolv1.PolicyEvaluationResult{
											{
												Name: "test-sync-active-policy",
												Kind: "ComputePolicy",
												Tags: []string{policyTag1ID, policyTag2ID},
											},
										},
									},
								}

								// Create the PolicyEvaluation in the fake Kubernetes client
								Expect(ctx.Client.Create(ctx, policyEval)).To(Succeed())
							})

							vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							// Verify VM was created successfully
							var o mo.VirtualMachine
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

							// Verify placement condition is ready (indicates vmconfpolicy.Reconcile was called)
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())

							// Verify that policy tags were added to ExtraConfig
							By("VM has policy tags in ExtraConfig", func() {
								Expect(o.Config.ExtraConfig).ToNot(BeNil())

								ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

								// Verify tags are present
								Expect(ecMap).To(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
								activeTags := ecMap[vmconfpolicy.ExtraConfigPolicyTagsKey]
								Expect(activeTags).To(ContainSubstring(policyTag1ID))
								Expect(activeTags).To(ContainSubstring(policyTag2ID))
							})
						})
					})
				})

				When("updating a VM", func() {
					It("should update VM with policy tags during reconfiguration", func() {
						// First create the VM
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						By("Adding a non-policy tag to the VM", func() {
							mgr := tags.NewManager(ctx.RestClient)

							Expect(mgr.AttachTag(ctx, ctx.TagID, vcVM.Reference())).To(Succeed())

							list, err := mgr.GetAttachedTags(ctx, vcVM.Reference())
							Expect(err).ToNot(HaveOccurred())
							Expect(list).To(HaveLen(1))
							Expect(list[0].ID).To(Equal(ctx.TagID))
						})

						By("Setting up PolicyEvaluation for update", func() {
							// Create a PolicyEvaluation object with updated tags
							policyEval := &vspherepolv1.PolicyEvaluation{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: vm.Namespace,
									Name:      "vm-" + vm.Name,
								},
							}

							Expect(ctx.Client.Get(
								ctx,
								client.ObjectKeyFromObject(policyEval),
								policyEval)).To(Succeed())

							// Create a PolicyEvaluation object with updated tags
							policyEval.Status = vspherepolv1.PolicyEvaluationStatus{
								ObservedGeneration: 0,
								Policies: []vspherepolv1.PolicyEvaluationResult{
									{
										Name: "test-updated-active-policy",
										Kind: "ComputePolicy",
										Tags: []string{policyTag1ID, policyTag2ID, policyTag3ID},
									},
								},
							}

							// Update the PolicyEvaluation in the fake Kubernetes client
							Expect(ctx.Client.Status().Update(ctx, policyEval)).To(Succeed())
						})

						// Trigger VM update.
						vcVM, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Get VM properties.
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						// Verify that updated policy tags were added to ExtraConfig
						By("VM has updated policy tags in ExtraConfig", func() {
							Expect(o.Config.ExtraConfig).ToNot(BeNil())

							ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

							// Verify updated tags are present
							Expect(ecMap).To(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
							activeTags := ecMap[vmconfpolicy.ExtraConfigPolicyTagsKey]
							Expect(activeTags).To(ContainSubstring(policyTag1ID))
							Expect(activeTags).To(ContainSubstring(policyTag2ID))
							Expect(activeTags).To(ContainSubstring(policyTag3ID))
						})
					})
				})
			})

			When("Capability is disabled", func() {
				JustBeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VSpherePolicies = false
					})
				})

				When("creating a VM", func() {
					When("async create is enabled", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.AsyncCreateEnabled = true
							})
						})
						It("should successfully create VM without calling vmconfpolicy.Reconcile", func() {
							vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							// Verify VM was created successfully
							var o mo.VirtualMachine
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

							// Verify placement condition is ready even without policy reconciliation
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
						})

					})
					When("async create is disabled", func() {
						JustBeforeEach(func() {
							pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
								config.AsyncCreateEnabled = false
							})
						})

						It("should successfully create VM without calling vmconfpolicy.Reconcile", func() {
							vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
							Expect(err).ToNot(HaveOccurred())

							// Verify VM was created successfully
							var o mo.VirtualMachine
							Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

							// Verify placement condition is ready even without policy reconciliation
							Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())

							// Verify that no policy tags were added to ExtraConfig (policy disabled)
							By("VM should not have policy tags in ExtraConfig", func() {
								if o.Config.ExtraConfig != nil {
									ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

									// Verify no tags are present
									Expect(ecMap).ToNot(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
								}
							})
						})
					})
				})

				When("updating a VM", func() {
					It("should update VM without adding policy tags", func() {
						// First create the VM
						_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Trigger VM update.
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
						Expect(err).ToNot(HaveOccurred())

						// Get VM properties.
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						// Verify that no policy tags were added to ExtraConfig during update
						By("VM should not have policy tags in ExtraConfig after update", func() {
							if o.Config.ExtraConfig != nil {
								ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()

								// Verify no tags are present
								Expect(ecMap).ToNot(HaveKey(vmconfpolicy.ExtraConfigPolicyTagsKey))
							}
						})
					})
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

func snapshotPartialRef(snapshotName string) *vmopv1.VirtualMachineSnapshotPartialRef {
	return &vmopv1.VirtualMachineSnapshotPartialRef{
		Name: snapshotName,
	}
}
