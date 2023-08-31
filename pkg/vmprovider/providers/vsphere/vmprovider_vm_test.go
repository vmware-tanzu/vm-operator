// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"bytes"
	goctx "context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/cluster"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmTests() {

	const (
		dvpgName = "DC0_DVPG0"
	)

	var (
		initObjects            []client.Object
		testConfig             builder.VCSimTestConfig
		ctx                    *builder.TestContextForVCSim
		vmProvider             vmprovider.VirtualMachineProviderInterface
		nsInfo                 builder.WorkloadNamespaceInfo
		oldNetworkProviderType string
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
		oldNetworkProviderType = os.Getenv(lib.NetworkProviderType)
		Expect(os.Setenv(lib.NetworkProviderType, lib.NetworkProviderTypeNamed)).To(Succeed())
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
		ctx.Context = goctx.WithValue(ctx.Context, context.MaxDeployThreadsContextKey, 16)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
		Expect(os.Setenv(lib.NetworkProviderType, oldNetworkProviderType)).To(Succeed())
	})

	Context("Create/Update/Delete VirtualMachine", func() {
		var (
			vm      *vmopv1.VirtualMachine
			vmClass *vmopv1.VirtualMachineClass
		)

		BeforeEach(func() {
			testConfig.WithContentLibrary = true
			vmClass = builder.DummyVirtualMachineClass()
			vm = builder.DummyBasicVirtualMachine("test-vm", "")
		})

		AfterEach(func() {
			vmClass = nil
			vm = nil
		})

		JustBeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

			vmClassBinding := builder.DummyVirtualMachineClassBinding(vmClass.Name, nsInfo.Namespace)
			Expect(ctx.Client.Create(ctx, vmClassBinding)).To(Succeed())

			vmImage := &vmopv1.VirtualMachineImage{}
			if testConfig.WithContentLibrary {
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryImageName}, vmImage)).To(Succeed())
			} else {
				// BMV: Without a CL is broken - and has been for a long while - since we assume the
				// VM Image will always have a ContentLibraryProvider owner. Hack around that here so
				// we can continue to test the VM clone path.
				vsphere.SkipVMImageCLProviderCheck = true

				// Use the default VM by vcsim as the source.
				vmImage = builder.DummyVirtualMachineImage("DC0_C0_RP0_VM0")
				Expect(ctx.Client.Create(ctx, vmImage)).To(Succeed())
			}

			vm.Namespace = nsInfo.Namespace
			vm.Spec.ClassName = vmClass.Name
			vm.Spec.ImageName = vmImage.Name
			vm.Spec.StorageClass = ctx.StorageClassName
		})

		AfterEach(func() {
			vsphere.SkipVMImageCLProviderCheck = false
		})

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

		Context("VMClassAsConfigDaynDate FSS is enabled", func() {

			var (
				vcVM       *object.VirtualMachine
				configSpec *types.VirtualMachineConfigSpec
				ethCard    types.VirtualEthernetCard
			)

			BeforeEach(func() {
				testConfig.WithVMClassAsConfigDaynDate = true

				ethCard = types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Key: 4000,
						DeviceInfo: &types.Description{
							Label:   "test-configspec-nic-label",
							Summary: "VM Network",
						},
						SlotInfo: &types.VirtualDevicePciBusSlotInfo{
							VirtualDeviceBusSlotInfo: types.VirtualDeviceBusSlotInfo{},
							PciSlotNumber:            32,
						},
						ControllerKey: 100,
					},
					AddressType: string(types.VirtualEthernetCardMacTypeGenerated),
					MacAddress:  "00:0c:29:93:d7:27",
					ResourceAllocation: &types.VirtualEthernetCardResourceAllocation{
						Reservation: pointer.Int64(42),
					},
				}
			})

			JustBeforeEach(func() {

				if configSpec != nil {
					var w bytes.Buffer
					enc := types.NewJSONEncoder(&w)
					Expect(enc.Encode(configSpec)).To(Succeed())

					// Update the VM Class with the XML.
					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())
					vmClass.Spec.ConfigSpec = w.Bytes()
					Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())
				}

				vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{
					{
						// Use the DVPG network so the updateEthCardDeviceChanges detects a device change.
						// If we use the "VM Network", then it won't detect any device changes since we
						// only compare the device name and not the type of the ethernet card.
						NetworkName: dvpgName,
					},
				}

				var err error
				vcVM, err = createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				vcVM = nil
				configSpec = nil
			})

			Context("VM Class has no ConfigSpec", func() {
				BeforeEach(func() {
					configSpec = nil
				})

				It("still creates VM", func() {
					Expect(vm.Status.Phase).To(Equal(vmopv1.Created))

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
					Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
				})
			})

			Context("ConfigSpec specifies hardware spec", func() {
				BeforeEach(func() {
					configSpec = &types.VirtualMachineConfigSpec{
						Name:     "dummy-VM",
						NumCPUs:  7,
						MemoryMB: 5102,
					}
				})

				It("CPU and memory from ConfigSpec are ignored", func() {
					Expect(vm.Status.Phase).To(Equal(vmopv1.Created))

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
					Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
					Expect(o.Summary.Config.NumCpu).ToNot(BeEquivalentTo(configSpec.NumCPUs))
					Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
					Expect(o.Summary.Config.MemorySizeMB).ToNot(BeEquivalentTo(configSpec.MemoryMB))
				})
			})

			Context("VM Class spec CPU reservation & limits are non-zero and ConfigSpec specifies CPU reservation", func() {
				BeforeEach(func() {
					vmClass.Spec.Policies.Resources.Requests.Cpu = resource.MustParse("2")
					vmClass.Spec.Policies.Resources.Limits.Cpu = resource.MustParse("2")

					// Specify a CPU reservation via ConfigSpec
					rsv := int64(6)
					configSpec = &types.VirtualMachineConfigSpec{
						CpuAllocation: &types.ResourceAllocationInfo{
							Reservation: &rsv,
						},
					}
				})

				It("VM gets CPU reservation from VM Class spec", func() {
					Expect(vm.Status.Phase).To(Equal(vmopv1.Created))

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					// Freq used by vcsim
					const freq = 2294

					reservation := o.Config.CpuAllocation.Reservation
					vmClassCPURes := virtualmachine.CPUQuantityToMhz(vmClass.Spec.Policies.Resources.Requests.Cpu, freq)
					Expect(reservation).ToNot(BeNil())
					Expect(*reservation).To(Equal(vmClassCPURes))
					Expect(*reservation).ToNot(Equal(*configSpec.CpuAllocation.Reservation))
					limit := o.Config.CpuAllocation.Limit
					Expect(limit).ToNot(BeNil())
					Expect(*limit).To(Equal(virtualmachine.CPUQuantityToMhz(vmClass.Spec.Policies.Resources.Limits.Cpu, freq)))
				})
			})

			Context("VM Class spec CPU reservations are zero and ConfigSpec specifies CPU reservation", func() {
				BeforeEach(func() {
					vmClass.Spec.Policies.Resources.Requests.Cpu = resource.MustParse("0")
					vmClass.Spec.Policies.Resources.Limits.Cpu = resource.MustParse("0")

					// Specify a CPU reservation via ConfigSpec
					rsv := int64(6)
					configSpec = &types.VirtualMachineConfigSpec{
						Name: "dummy-VM",
						CpuAllocation: &types.ResourceAllocationInfo{
							Reservation: &rsv,
						},
					}
				})

				It("VM gets CPU reservation from ConfigSpec", func() {
					Expect(vm.Status.Phase).To(Equal(vmopv1.Created))

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					// Freq used by vcsim
					const freq = 2294

					reservation := o.Config.CpuAllocation.Reservation
					vmClassCPURes := virtualmachine.CPUQuantityToMhz(vmClass.Spec.Policies.Resources.Requests.Cpu, freq)
					Expect(reservation).ToNot(BeNil())
					Expect(*reservation).ToNot(Equal(vmClassCPURes))
					Expect(*reservation).To(Equal(*configSpec.CpuAllocation.Reservation))
				})
			})

			Context("VM Class spec Memory reservation & limits are non-zero and ConfigSpec specifies memory reservation", func() {
				BeforeEach(func() {
					vmClass.Spec.Policies.Resources.Requests.Memory = resource.MustParse("4Mi")
					vmClass.Spec.Policies.Resources.Limits.Memory = resource.MustParse("4Mi")

					// Specify a Memory reservation via ConfigSpec
					rsv := int64(5120)
					configSpec = &types.VirtualMachineConfigSpec{
						MemoryAllocation: &types.ResourceAllocationInfo{
							Reservation: &rsv,
						},
					}
				})

				It("VM gets memory reservation from VM Class spec", func() {
					Expect(vm.Status.Phase).To(Equal(vmopv1.Created))

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					reservation := o.Config.MemoryAllocation.Reservation
					vmClassMemoryRes := virtualmachine.MemoryQuantityToMb(vmClass.Spec.Policies.Resources.Requests.Memory)
					Expect(reservation).ToNot(BeNil())
					Expect(*reservation).To(Equal(vmClassMemoryRes))
					Expect(*reservation).ToNot(Equal(*configSpec.MemoryAllocation.Reservation))
					limit := o.Config.MemoryAllocation.Limit
					Expect(limit).ToNot(BeNil())
					Expect(*limit).To(Equal(virtualmachine.MemoryQuantityToMb(vmClass.Spec.Policies.Resources.Limits.Memory)))
				})
			})

			Context("VM Class spec Memory reservations are zero and ConfigSpec specifies memory reservation", func() {
				BeforeEach(func() {
					vmClass.Spec.Policies.Resources.Requests.Memory = resource.MustParse("0Mi")
					vmClass.Spec.Policies.Resources.Limits.Memory = resource.MustParse("0Mi")

					// Specify a Memory reservation via ConfigSpec
					rsv := int64(5120)
					configSpec = &types.VirtualMachineConfigSpec{
						Name: "dummy-VM",
						MemoryAllocation: &types.ResourceAllocationInfo{
							Reservation: &rsv,
						},
					}
				})

				It("VM gets memory reservation from ConfigSpec", func() {
					Expect(vm.Status.Phase).To(Equal(vmopv1.Created))

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					reservation := o.Config.MemoryAllocation.Reservation
					vmClassMemoryRes := virtualmachine.MemoryQuantityToMb(vmClass.Spec.Policies.Resources.Requests.Memory)
					Expect(reservation).ToNot(BeNil())
					Expect(*reservation).ToNot(Equal(vmClassMemoryRes))
					Expect(*reservation).To(Equal(*configSpec.MemoryAllocation.Reservation))
				})
			})

			Context("VM Class ConfigSpec specifies a network interface", func() {

				BeforeEach(func() {
					// Create the ConfigSpec with an ethernet card.
					configSpec = &types.VirtualMachineConfigSpec{
						Name: "dummy-VM",
						DeviceChange: []types.BaseVirtualDeviceConfigSpec{
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualE1000{
									VirtualEthernetCard: ethCard,
								},
							},
						},
					}
				})

				It("Reconfigures the VM with the NIC specified in ConfigSpec", func() {
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					l := devList.SelectByType(&types.VirtualEthernetCard{})
					Expect(l).To(HaveLen(1))

					dev := l[0].GetVirtualDevice()
					backing, ok := dev.Backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(ok).Should(BeTrue())
					_, dvpg := getDVPG(ctx, dvpgName)
					Expect(backing.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))

					ethDevice, ok := l[0].(*types.VirtualE1000)
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
				})
			})

			Context("ConfigSpec does not specify any network interfaces", func() {

				BeforeEach(func() {
					configSpec = &types.VirtualMachineConfigSpec{
						Name: "dummy-VM",
					}
				})

				It("Reconfigures the VM with the default NIC settings from provider", func() {
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					l := devList.SelectByType(&types.VirtualEthernetCard{})
					Expect(l).To(HaveLen(1))

					dev := l[0].GetVirtualDevice()
					backing, ok := dev.Backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
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
					configSpec = &types.VirtualMachineConfigSpec{
						Name: "dummy-VM",
						DeviceChange: []types.BaseVirtualDeviceConfigSpec{
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIPassthrough{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
											Vgpu: "profile-from-configspec",
										},
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIPassthrough{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPCIPassthroughDynamicBackingInfo{
											AllowedDevice: []types.VirtualPCIPassthroughAllowedDevice{
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

				It("GPU and DirectPath devices from VM Class Spec.Devices are ignored", func() {
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					p := devList.SelectByType(&types.VirtualPCIPassthrough{})
					Expect(p).To(HaveLen(2))

					pciDev1 := p[0].GetVirtualDevice()
					pciBacking1, ok1 := pciDev1.Backing.(*types.VirtualPCIPassthroughVmiopBackingInfo)
					Expect(ok1).Should(BeTrue())
					Expect(pciBacking1.Vgpu).To(Equal("profile-from-configspec"))

					pciDev2 := p[1].GetVirtualDevice()
					pciBacking2, ok2 := pciDev2.Backing.(*types.VirtualPCIPassthroughDynamicBackingInfo)
					Expect(ok2).Should(BeTrue())
					Expect(pciBacking2.AllowedDevice).To(HaveLen(1))
					Expect(pciBacking2.AllowedDevice[0].VendorId).To(Equal(int32(52)))
					Expect(pciBacking2.AllowedDevice[0].DeviceId).To(Equal(int32(53)))
					Expect(pciBacking2.CustomLabel).To(Equal("label-from-configspec"))
				})
			})

			Context("VM Class Config specifies an ethCard, a GPU and a DDPIO device", func() {
				BeforeEach(func() {
					// Create the ConfigSpec with an ethernet card, a GPU and a DDPIO device.
					configSpec = &types.VirtualMachineConfigSpec{
						Name: "dummy-VM",
						DeviceChange: []types.BaseVirtualDeviceConfigSpec{
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualE1000{
									VirtualEthernetCard: ethCard,
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIPassthrough{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
											Vgpu: "SampleProfile2",
										},
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIPassthrough{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPCIPassthroughDynamicBackingInfo{
											AllowedDevice: []types.VirtualPCIPassthroughAllowedDevice{
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
					l := devList.SelectByType(&types.VirtualEthernetCard{})
					Expect(l).To(HaveLen(1))

					dev := l[0].GetVirtualDevice()
					backing, ok := dev.Backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
					Expect(ok).Should(BeTrue())
					_, dvpg := getDVPG(ctx, dvpgName)
					Expect(backing.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))

					ethDevice, ok := l[0].(*types.VirtualE1000)
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

					p := devList.SelectByType(&types.VirtualPCIPassthrough{})
					Expect(p).To(HaveLen(2))
					pciDev1 := p[0].GetVirtualDevice()
					pciBacking1, ok1 := pciDev1.Backing.(*types.VirtualPCIPassthroughVmiopBackingInfo)
					Expect(ok1).Should(BeTrue())
					Expect(pciBacking1.Vgpu).To(Equal("SampleProfile2"))
					pciDev2 := p[1].GetVirtualDevice()
					pciBacking2, ok2 := pciDev2.Backing.(*types.VirtualPCIPassthroughDynamicBackingInfo)
					Expect(ok2).Should(BeTrue())
					Expect(pciBacking2.AllowedDevice).To(HaveLen(1))
					Expect(pciBacking2.AllowedDevice[0].VendorId).To(Equal(int32(52)))
					Expect(pciBacking2.AllowedDevice[0].DeviceId).To(Equal(int32(53)))
					Expect(pciBacking2.CustomLabel).To(Equal("SampleLabel2"))

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())

					// cpu and memory check from vm class
					Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
					Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
				})
			})

			Context("VM Class Config specifies disks, disk controllers, other miscellaneous devices", func() {
				BeforeEach(func() {
					// Create the ConfigSpec with disks, disk controller and
					// some miscellaneous devices -- pointing device, video card etc.
					// This works fine with vcsim and helps with testing adding misc devices -- the simulator can
					// still reconfigure the VM with default device types like pointing devices, keyboard, video card etc..
					// But vSphere has some restrictions with reconfiguring a VM with new default device types via config spec
					// and usually ignored.
					configSpec = &types.VirtualMachineConfigSpec{
						Name: "dummy-VM",
						DeviceChange: []types.BaseVirtualDeviceConfigSpec{
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPointingDevice{
									VirtualDevice: types.VirtualDevice{
										Backing: &types.VirtualPointingDeviceDeviceBackingInfo{
											HostPointingDevice: "autodetect",
										},
										Key:           700,
										ControllerKey: 300,
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPS2Controller{
									VirtualController: types.VirtualController{
										Device: []int32{700},
										VirtualDevice: types.VirtualDevice{
											Key: 300,
										},
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualMachineVideoCard{
									UseAutoDetect: &[]bool{false}[0],
									NumDisplays:   1,
									VirtualDevice: types.VirtualDevice{
										Key:           500,
										ControllerKey: 100,
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualPCIController{
									VirtualController: types.VirtualController{
										Device: []int32{500},
										VirtualDevice: types.VirtualDevice{
											Key: 100,
										},
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualDisk{
									CapacityInBytes: 1024,
									VirtualDevice: types.VirtualDevice{
										Key: -42,
										Backing: &types.VirtualDiskFlatVer2BackingInfo{
											ThinProvisioned: pointer.Bool(true),
										},
									},
								},
							},
							&types.VirtualDeviceConfigSpec{
								Operation: types.VirtualDeviceConfigSpecOperationAdd,
								Device: &types.VirtualSCSIController{
									VirtualController: types.VirtualController{
										Device: []int32{-42},
									},
								},
							},
						},
					}
				})

				It("Reconfigures the VM with all misc devices in ConfigSpec expect disk and disk controllers", func() {
					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					// VM already has a default pointing device and the spec adds one more
					// more info about the default device is unknown to assert on
					pointingDev := devList.SelectByType(&types.VirtualPointingDevice{})
					Expect(pointingDev).To(HaveLen(2))
					dev := pointingDev[0].GetVirtualDevice()
					backing, ok := dev.Backing.(*types.VirtualPointingDeviceDeviceBackingInfo)
					Expect(ok).Should(BeTrue())
					Expect(backing.HostPointingDevice).To(Equal("autodetect"))
					Expect(dev.Key).To(Equal(int32(700)))
					Expect(dev.ControllerKey).To(Equal(int32(300)))

					ps2cont := devList.SelectByType(&types.VirtualPS2Controller{})
					Expect(ps2cont).To(HaveLen(1))
					dev = ps2cont[0].GetVirtualDevice()
					Expect(dev.Key).To(Equal(int32(300)))

					pcicont := devList.SelectByType(&types.VirtualPCIController{})
					Expect(pcicont).To(HaveLen(1))
					dev = pcicont[0].GetVirtualDevice()
					Expect(dev.Key).To(Equal(int32(100)))

					// VM already has a default video card and the spec adds one more
					// more info about the default device is unknown to assert on
					video := devList.SelectByType(&types.VirtualMachineVideoCard{})
					Expect(video).To(HaveLen(2))
					dev = video[0].GetVirtualDevice()
					Expect(dev.Key).To(Equal(int32(500)))
					Expect(dev.ControllerKey).To(Equal(int32(100)))

					// disk and disk controllers from config spec should not get added,
					// since they get processed out in CreateConfigSpec
					diskcont := devList.SelectByType(&types.VirtualSCSIController{})
					Expect(diskcont).To(HaveLen(0))

					// only preexisting disk should be present on VM -- len: 1
					disk := devList.SelectByType(&types.VirtualDisk{})
					Expect(disk).To(HaveLen(1))
					dev = disk[0].GetVirtualDevice()
					Expect(dev.Key).ToNot(Equal(int32(-42)))
				})
			})

			Context("VM Class Config does not specify a hardware version", func() {

				Context("VM Class has vGPU and/or DDPIO devices", func() {
					BeforeEach(func() {
						// Create the ConfigSpec with a GPU and a DDPIO device.
						configSpec = &types.VirtualMachineConfigSpec{
							Name: "dummy-VM",
							DeviceChange: []types.BaseVirtualDeviceConfigSpec{
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualPCIPassthrough{
										VirtualDevice: types.VirtualDevice{
											Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
												Vgpu: "profile-from-configspec",
											},
										},
									},
								},
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualPCIPassthrough{
										VirtualDevice: types.VirtualDevice{
											Backing: &types.VirtualPCIPassthroughDynamicBackingInfo{
												AllowedDevice: []types.VirtualPCIPassthroughAllowedDevice{
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
						Expect(o.Config.Version).To(Equal(fmt.Sprintf("vmx-%d", constants.MinSupportedHWVersionForPCIPassthruDevices)))
					})
				})

				Context("VM Class has vGPU and/or DDPIO devices and VM spec has a PVC", func() {
					BeforeEach(func() {
						// Create the ConfigSpec with a GPU and a DDPIO device.
						configSpec = &types.VirtualMachineConfigSpec{
							Name: "dummy-VM",
							DeviceChange: []types.BaseVirtualDeviceConfigSpec{
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualPCIPassthrough{
										VirtualDevice: types.VirtualDevice{
											Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
												Vgpu: "profile-from-configspec",
											},
										},
									},
								},
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualPCIPassthrough{
										VirtualDevice: types.VirtualDevice{
											Backing: &types.VirtualPCIPassthroughDynamicBackingInfo{
												AllowedDevice: []types.VirtualPCIPassthroughAllowedDevice{
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
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "pvc-claim-1",
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
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Version).To(Equal(fmt.Sprintf("vmx-%d", constants.MinSupportedHWVersionForPCIPassthruDevices)))
					})
				})

				Context("VM spec has a PVC", func() {
					BeforeEach(func() {
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "dummy-vol",
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "pvc-claim-1",
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
						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						Expect(o.Config.Version).To(Equal(fmt.Sprintf("vmx-%d", constants.MinSupportedHWVersionForPVC)))
					})
				})
			})

			Context("VMClassAsConfig FSS is Enabled", func() {

				BeforeEach(func() {
					testConfig.WithVMClassAsConfig = true
				})

				When("configSpec has disk and disk controllers", func() {
					BeforeEach(func() {
						configSpec = &types.VirtualMachineConfigSpec{
							Name: "dummy-VM",
							DeviceChange: []types.BaseVirtualDeviceConfigSpec{
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualSATAController{
										VirtualController: types.VirtualController{
											VirtualDevice: types.VirtualDevice{
												Key: 101,
											},
										},
									},
								},
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualSCSIController{
										VirtualController: types.VirtualController{
											VirtualDevice: types.VirtualDevice{
												Key: 103,
											},
										},
									},
								},
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualNVMEController{
										VirtualController: types.VirtualController{
											VirtualDevice: types.VirtualDevice{
												Key: 104,
											},
										},
									},
								},
								&types.VirtualDeviceConfigSpec{
									Operation: types.VirtualDeviceConfigSpecOperationAdd,
									Device: &types.VirtualDisk{
										CapacityInBytes: 1024,
										VirtualDevice: types.VirtualDevice{
											Key: -42,
											Backing: &types.VirtualDiskFlatVer2BackingInfo{
												ThinProvisioned: pointer.Bool(true),
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
						satacont := devList.SelectByType(&types.VirtualSATAController{})
						Expect(satacont).To(HaveLen(1))
						dev := satacont[0].GetVirtualDevice()
						Expect(dev.Key).To(Equal(int32(101)))

						scsicont := devList.SelectByType(&types.VirtualSCSIController{})
						Expect(scsicont).To(HaveLen(1))
						dev = scsicont[0].GetVirtualDevice()
						Expect(dev.Key).To(Equal(int32(103)))

						nvmecont := devList.SelectByType(&types.VirtualNVMEController{})
						Expect(nvmecont).To(HaveLen(1))
						dev = nvmecont[0].GetVirtualDevice()
						Expect(dev.Key).To(Equal(int32(104)))

						// only preexisting disk should be present on VM -- len: 1
						disks := devList.SelectByType(&types.VirtualDisk{})
						Expect(disks).To(HaveLen(1))
						dev1 := disks[0].GetVirtualDevice()
						Expect(dev1.Key).ToNot(Equal(int32(-42)))
					})
				})
			})
		})

		Context("CreateOrUpdate VM", func() {

			It("Basic VM", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

				By("has expected Status values", func() {
					Expect(vm.Status.Phase).To(Equal(vmopv1.Created))
					Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))
					Expect(vm.Status.Host).ToNot(BeEmpty())
					Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
					Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))
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
					Expect(o.Summary.Runtime.PowerState).To(Equal(types.VirtualMachinePowerStatePoweredOn))
				})

				vmClass := &vmopv1.VirtualMachineClass{}
				Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())
				vmClassRes := vmClass.Spec.Policies.Resources

				By("has expected CpuAllocation", func() {
					Expect(o.Config.CpuAllocation).ToNot(BeNil())

					// The vcsim ESX hardcoded CPU frequency that is not exported by govmomi.
					const freq = 2294

					reservation := o.Config.CpuAllocation.Reservation
					Expect(reservation).ToNot(BeNil())
					Expect(*reservation).To(Equal(virtualmachine.CPUQuantityToMhz(vmClassRes.Requests.Cpu, freq)))
					limit := o.Config.CpuAllocation.Limit
					Expect(limit).ToNot(BeNil())
					Expect(*limit).To(Equal(virtualmachine.CPUQuantityToMhz(vmClassRes.Limits.Cpu, freq)))
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

				// TODO: More assertions!
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

				It("VM should have expected PCI devices from VM Class", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					p := devList.SelectByType(&types.VirtualPCIPassthrough{})
					Expect(p).To(HaveLen(2))
					pciDev1 := p[0].GetVirtualDevice()
					pciBacking1, ok1 := pciDev1.Backing.(*types.VirtualPCIPassthroughVmiopBackingInfo)
					Expect(ok1).Should(BeTrue())
					Expect(pciBacking1.Vgpu).To(Equal("profile-from-class-without-class-as-config-fss"))
					pciDev2 := p[1].GetVirtualDevice()
					pciBacking2, ok2 := pciDev2.Backing.(*types.VirtualPCIPassthroughDynamicBackingInfo)
					Expect(ok2).Should(BeTrue())
					Expect(pciBacking2.AllowedDevice).To(HaveLen(1))
					Expect(pciBacking2.AllowedDevice[0].VendorId).To(Equal(int32(59)))
					Expect(pciBacking2.AllowedDevice[0].DeviceId).To(Equal(int32(60)))
					Expect(pciBacking2.CustomLabel).To(Equal("label-from-class-without-class-as-config-fss"))
				})
			})

			Context("Prereq args and Conditions", func() {
				readyCondition := *conditions.TrueCondition(vmopv1.VirtualMachinePrereqReadyCondition)

				Context("VM is poweredOn", func() {
					It("Returns success even if the prereq args are missing", func() {
						_, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						c := conditions.Get(vm, vmopv1.VirtualMachinePrereqReadyCondition)
						Expect(c).ToNot(BeNil())
						Expect(*c).To(conditions.MatchCondition(readyCondition))

						Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1.VirtualMachineClassBinding{})).To(Succeed())

						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).NotTo(HaveOccurred())

						c = conditions.Get(vm, vmopv1.VirtualMachinePrereqReadyCondition)
						Expect(c).ToNot(BeNil())
						Expect(*c).To(conditions.MatchCondition(readyCondition))
					})
				})

				Context("VM is in poweredOff to poweredOn transition", func() {
					It("Returns success if VM image is missing and this VM is not first boot", func() {
						_, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1.VirtualMachineImage{})).To(Succeed())

						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOn
						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).NotTo(HaveOccurred())

						c := conditions.Get(vm, vmopv1.VirtualMachinePrereqReadyCondition)
						Expect(c).ToNot(BeNil())
						Expect(*c).To(conditions.MatchCondition(readyCondition))
					})

					It("Returns error if VM image is missing and this VM is first boot", func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
						_, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1.VirtualMachineImage{})).To(Succeed())

						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOn
						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).To(HaveOccurred())

						c := conditions.Get(vm, vmopv1.VirtualMachinePrereqReadyCondition)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(corev1.ConditionFalse))
					})

					It("Sets expected PrereqReady condition", func() {
						_, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						c := conditions.Get(vm, vmopv1.VirtualMachinePrereqReadyCondition)
						Expect(c).ToNot(BeNil())
						Expect(*c).To(conditions.MatchCondition(readyCondition))

						// Use the class binding removal to toggle the condition state.
						Expect(ctx.Client.DeleteAllOf(ctx, &vmopv1.VirtualMachineClassBinding{})).To(Succeed())

						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOn
						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).To(HaveOccurred())

						c = conditions.Get(vm, vmopv1.VirtualMachinePrereqReadyCondition)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(corev1.ConditionFalse))

						// Recreate the class binding to put things back.
						vmClassBinding := builder.DummyVirtualMachineClassBinding(vm.Spec.ClassName, vm.Namespace)
						Expect(ctx.Client.Create(ctx, vmClassBinding)).To(Succeed())

						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						c = conditions.Get(vm, vmopv1.VirtualMachinePrereqReadyCondition)
						Expect(c).ToNot(BeNil())
						Expect(*c).To(conditions.MatchCondition(readyCondition))
					})
				})
			})

			Context("Without Storage Class", func() {
				BeforeEach(func() {
					testConfig.WithoutStorageClass = true
				})

				It("Creates VM", func() {
					Expect(vm.Spec.StorageClass).To(BeEmpty())

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
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
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					By("has expected Status values", func() {
						Expect(vm.Status.Phase).To(Equal(vmopv1.Created))
						Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))
						Expect(vm.Status.Host).ToNot(BeEmpty())
						Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
						Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))
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
						Expect(o.Summary.Runtime.PowerState).To(Equal(types.VirtualMachinePowerStatePoweredOn))
					})

					By("has expected hardware config", func() {
						vmClass := &vmopv1.VirtualMachineClass{}
						Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())
						Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
						Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
					})

					// TODO: More assertions!
				})
			})

			It("Create VM from VMTX in ContentLibrary", func() {
				imageName := "test-vm-vmtx"

				ctx.ContentLibraryItemTemplate("DC0_C0_RP0_VM0", imageName)
				vm.Spec.ImageName = imageName

				_, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("When fault domains is enabled", func() {
				BeforeEach(func() {
					testConfig.WithFaultDomains = true
				})

				It("creates VM in placement selected zone", func() {
					Expect(vm.Labels).ToNot(HaveKey(topology.KubernetesTopologyZoneLabelKey))
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
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

				It("creates VM in assigned zone", func() {
					azName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))] //nolint:gosec
					vm.Labels[topology.KubernetesTopologyZoneLabelKey] = azName

					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					By("VM is created in the zone's ResourcePool", func() {
						rp, err := vcVM.ResourcePool(ctx)
						Expect(err).ToNot(HaveOccurred())
						nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, azName, "")
						Expect(nsRP).ToNot(BeNil())
						Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
					})
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
					isVolumes := instancestorage.FilterVolumes(vm)
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
					_, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())
				})

				It("create VM with instance storage", func() {
					Expect(vm.Spec.Volumes).To(BeEmpty())

					vmClass := &vmopv1.VirtualMachineClass{}
					Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: vm.Spec.ClassName}, vmClass)).To(Succeed())
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

					err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
					Expect(err).To(MatchError("instance storage PVCs are not bound yet"))

					By("Instance storage volumes should be added to VM", func() {
						Expect(instancestorage.IsConfigured(vm)).To(BeTrue())
						expectInstanceStorageVolumes(vm, vmClass.Spec.Hardware.InstanceStorage)
					})
					isVol0 := vm.Spec.Volumes[0]

					By("Placement should have been done", func() {
						Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
						Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
					})

					By("simulate volume controller workflow", func() {
						// Simulate what would be set by volume controller.
						vm.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = ""

						err = vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
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
						_, err = createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})

			It("Powers VM off", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePoweredOn))
				vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
				Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())

				Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePoweredOff))
				state, err := vcVM.PowerState(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(state).To(Equal(types.VirtualMachinePowerStatePoweredOff))
			})

			It("returns error when StorageClass is required but none specified", func() {
				vm.Spec.StorageClass = ""
				err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).To(MatchError("StorageClass is required but not specified"))
			})

			It("Can be called multiple times", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				var o mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
				modified := o.Config.Modified

				_, err = createOrUpdateAndGetVcVM(ctx, vm)
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

						vm.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
							ConfigMapName: configMap.Name,
							Transport:     vmopv1.VirtualMachineMetadataExtraConfigTransport,
						}
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
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

					It("Metadata data is included in ExtraConfig", func() {
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

				Context("Cloudinint Transport without userdata", func() {
					var ec map[string]interface{}

					It("Metadata data is included in ExtraConfig", func() {
						vm.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
							Transport: vmopv1.VirtualMachineMetadataCloudInitTransport,
						}
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						ec = map[string]interface{}{}
						for _, option := range o.Config.ExtraConfig {
							if val := option.GetOptionValue(); val != nil {
								ec[val.Key] = val.Value.(string)
							}
						}
						By("Should include default keys and values", func() {
							Expect(ec).To(HaveKeyWithValue("disk.enableUUID", "TRUE"))
							Expect(ec).To(HaveKeyWithValue("vmware.tools.gosc.ignoretoolscheck", "TRUE"))
						})

						By("Should include guestinfo metadata and no guestinfo userdata", func() {
							Expect(ec).To(HaveKey("guestinfo.metadata"))
							Expect(ec).To(HaveKey("guestinfo.metadata.encoding"))
							Expect(ec).ToNot(HaveKey("guestinfo.userdata"))
						})
					})
				})

				Context("Cloudinint Transport with userdata", func() {
					var ec map[string]interface{}

					It("Metadata data is included in ExtraConfig", func() {
						configMap := &corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								GenerateName: "md-configmap-",
								Namespace:    vm.Namespace,
							},
							Data: map[string]string{
								"user-data": "data",
							},
						}
						Expect(ctx.Client.Create(ctx, configMap)).To(Succeed())

						vm.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
							ConfigMapName: configMap.Name,
							Transport:     vmopv1.VirtualMachineMetadataCloudInitTransport,
						}

						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						ec = map[string]interface{}{}
						for _, option := range o.Config.ExtraConfig {
							if val := option.GetOptionValue(); val != nil {
								ec[val.Key] = val.Value.(string)
							}
						}
						By("Should include default keys and values", func() {
							Expect(ec).To(HaveKeyWithValue("disk.enableUUID", "TRUE"))
							Expect(ec).To(HaveKeyWithValue("vmware.tools.gosc.ignoretoolscheck", "TRUE"))
						})

						By("Should include guestinfo metadata and userdata", func() {
							Expect(ec).To(HaveKey("guestinfo.metadata"))
							Expect(ec).To(HaveKey("guestinfo.metadata.encoding"))
							Expect(ec).To(HaveKey("guestinfo.userdata"))
							Expect(ec).To(HaveKey("guestinfo.userdata.encoding"))
						})
					})
				})
			})

			Context("Network", func() {

				It("Should not have a nic", func() {
					vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					var o mo.VirtualMachine
					Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

					devList := object.VirtualDeviceList(o.Config.Hardware.Device)
					l := devList.SelectByType(&types.VirtualEthernetCard{})
					Expect(l).To(BeEmpty())
				})

				Context("Multiple NICs are specified", func() {
					BeforeEach(func() {
						vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{
							{
								NetworkName:      "VM Network",
								EthernetCardType: "e1000",
							},
							{
								NetworkName: dvpgName,
							},
						}
					})

					It("Has expected devices", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						devList := object.VirtualDeviceList(o.Config.Hardware.Device)
						l := devList.SelectByType(&types.VirtualEthernetCard{})
						Expect(l).To(HaveLen(2))

						dev1 := l[0].GetVirtualDevice()
						backing1, ok := dev1.Backing.(*types.VirtualEthernetCardNetworkBackingInfo)
						Expect(ok).Should(BeTrue())
						Expect(backing1.DeviceName).To(Equal("VM Network"))

						dev2 := l[1].GetVirtualDevice()
						backing2, ok := dev2.Backing.(*types.VirtualEthernetCardDistributedVirtualPortBackingInfo)
						Expect(ok).Should(BeTrue())
						_, dvpg := getDVPG(ctx, dvpgName)
						Expect(backing2.Port.PortgroupKey).To(Equal(dvpg.Reference().Value))
					})
				})
			})

			Context("Disks", func() {

				Context("VM has thin provisioning", func() {
					BeforeEach(func() {
						vm.Spec.AdvancedOptions = &vmopv1.VirtualMachineAdvancedOptions{
							DefaultVolumeProvisioningOptions: &vmopv1.VirtualMachineVolumeProvisioningOptions{
								ThinProvisioned: pointer.Bool(true),
							},
						}
					})

					It("Succeeds", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.ThinProvisioned).To(PointTo(BeTrue()))
					})
				})

				XContext("VM has thick provisioning", func() {
					BeforeEach(func() {
						vm.Spec.AdvancedOptions = &vmopv1.VirtualMachineAdvancedOptions{
							DefaultVolumeProvisioningOptions: &vmopv1.VirtualMachineVolumeProvisioningOptions{
								ThinProvisioned: pointer.Bool(false),
							},
						}
					})

					It("Succeeds", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
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
						vm.Spec.AdvancedOptions = &vmopv1.VirtualMachineAdvancedOptions{
							DefaultVolumeProvisioningOptions: &vmopv1.VirtualMachineVolumeProvisioningOptions{
								EagerZeroed: pointer.Bool(true),
							},
						}
					})

					It("Succeeds", func() {
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

						/* vcsim CL deploy has "eagerZeroedThick" but that isn't reflected for this disk. */
						_, backing := getVMHomeDisk(ctx, vcVM, o)
						Expect(backing.EagerlyScrub).To(PointTo(BeTrue()))
					})
				})

				Context("Should resize root disk", func() {
					newSize := resource.MustParse("4242Gi")

					It("Succeeds", func() {
						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
						vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
						Expect(err).ToNot(HaveOccurred())

						var o mo.VirtualMachine
						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						disk, _ := getVMHomeDisk(ctx, vcVM, o)
						Expect(disk.CapacityInBytes).ToNot(BeEquivalentTo(newSize.Value()))
						// This is almost always 203 but sometimes it isn't for some reason, so fetch it.
						deviceKey := int(disk.Key)

						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: "this-api-stinks",
								VsphereVolume: &vmopv1.VsphereVolumeSource{
									Capacity: corev1.ResourceList{
										corev1.ResourceEphemeralStorage: newSize,
									},
									DeviceKey: &deviceKey,
								},
							},
						}

						vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOn
						Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())

						Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
						disk, _ = getVMHomeDisk(ctx, vcVM, o)
						Expect(disk.CapacityInBytes).To(BeEquivalentTo(newSize.Value()))
					})
				})
			})

			Context("CNS Volumes", func() {
				cnsVolumeName := "cns-volume-1"

				It("CSI Volumes workflow", func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
					_, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOn
					By("Add CNS volume to VM", func() {
						vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
							{
								Name: cnsVolumeName,
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "pvc-volume-1",
									},
								},
							},
						}

						err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("status update pending for persistent volume: %s on VM", cnsVolumeName)))
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePoweredOff))
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

						err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("persistent volume: %s not attached to VM", cnsVolumeName)))
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePoweredOff))
					})

					By("CNS volume is attached", func() {
						vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
							{
								Name:     cnsVolumeName,
								Attached: true,
							},
						}
						Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())
						Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePoweredOn))
					})
				})
			})

			Context("When fault domains is enabled", func() {
				const zoneName = "az-1"

				BeforeEach(func() {
					testConfig.WithFaultDomains = true
					// Explicitly place the VM into one of the zones that the test context will create.
					vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
				})

				It("Reverse lookups existing VM into correct zone", func() {
					_, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
					Expect(vm.Status.Zone).To(Equal(zoneName))
					delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)

					Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())
					Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
					Expect(vm.Status.Zone).To(Equal(zoneName))
				})
			})
		})

		Context("VM SetResourcePolicy", func() {
			var resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy

			JustBeforeEach(func() {
				resourcePolicyName := "test-policy"
				resourcePolicy = getVirtualMachineSetResourcePolicy(resourcePolicyName, nsInfo.Namespace)
				Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
				Expect(ctx.Client.Create(ctx, resourcePolicy)).To(Succeed())

				vm.Annotations["vsphere-cluster-module-group"] = resourcePolicy.Spec.ClusterModules[0].GroupName
				vm.Spec.ResourcePolicyName = resourcePolicy.Name
			})

			AfterEach(func() {
				resourcePolicy = nil
			})

			It("VM is created in child Folder and ResourcePool", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				By("has expected inventory path", func() {
					Expect(vcVM.InventoryPath).To(HaveSuffix(
						fmt.Sprintf("/%s/%s/%s", nsInfo.Namespace, resourcePolicy.Spec.Folder.Name, vm.Name)))
				})

				By("has expected namespace resource pool", func() {
					rp, err := vcVM.ResourcePool(ctx)
					Expect(err).ToNot(HaveOccurred())
					childRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", resourcePolicy.Spec.ResourcePool.Name)
					Expect(childRP).ToNot(BeNil())
					Expect(rp.Reference().Value).To(Equal(childRP.Reference().Value))
				})
			})

			It("Cluster Modules", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vm)
				Expect(err).ToNot(HaveOccurred())

				members, err := cluster.NewManager(ctx.RestClient).ListModuleMembers(ctx, resourcePolicy.Status.ClusterModules[0].ModuleUuid)
				Expect(err).ToNot(HaveOccurred())
				Expect(members).To(ContainElements(vcVM.Reference()))
			})

			It("Returns error with non-existence cluster module", func() {
				vm.Annotations["vsphere-cluster-module-group"] = "bogusClusterMod"
				err := vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)
				Expect(err).To(MatchError("ClusterModule bogusClusterMod not found"))
			})
		})

		Context("Delete VM", func() {
			JustBeforeEach(func() {
				Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())
			})

			Context("when the VM is off", func() {
				BeforeEach(func() {
					vm.Spec.PowerState = vmopv1.VirtualMachinePoweredOff
				})

				It("deletes the VM", func() {
					Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePoweredOff))

					uniqueID := vm.Status.UniqueID
					Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

					Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
					Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
				})
			})

			It("when the VM is on", func() {
				Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePoweredOn))

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

			Context("When fault domains is enabled", func() {
				const zoneName = "az-1"

				BeforeEach(func() {
					testConfig.WithFaultDomains = true
					// Explicitly place the VM into one of the zones that the test context will create.
					vm.Labels[topology.KubernetesTopologyZoneLabelKey] = zoneName
				})

				It("returns NotFound when VM does not exist", func() {
					_, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
					delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)
					Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
				})

				It("Deletes existing VM when zone info is missing", func() {
					_, err := createOrUpdateAndGetVcVM(ctx, vm)
					Expect(err).ToNot(HaveOccurred())

					uniqueID := vm.Status.UniqueID
					Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

					Expect(vm.Labels).To(HaveKeyWithValue(topology.KubernetesTopologyZoneLabelKey, zoneName))
					delete(vm.Labels, topology.KubernetesTopologyZoneLabelKey)

					Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
					Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
				})
			})
		})

		Context("Guest Heartbeat", func() {
			JustBeforeEach(func() {
				Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())
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
				Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())
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
				Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())
			})

			It("return version", func() {
				version, err := vmProvider.GetVirtualMachineHardwareVersion(ctx, vm)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal(int32(9)))
			})
		})

		Context("ResVMToVirtualMachineImage", func() {
			JustBeforeEach(func() {
				Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())
			})

			// ResVMToVirtualMachineImage isn't actually used.
			It("returns a VirtualMachineImage", func() {
				vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
				Expect(vcVM).ToNot(BeNil())

				// TODO: Need to convert this VM to a vApp (and back).
				// annotations := map[string]string{}
				// annotations[versionKey] = versionVal

				image, err := vsphere.ResVMToVirtualMachineImage(ctx, vcVM)
				Expect(err).ToNot(HaveOccurred())
				Expect(image).ToNot(BeNil())
				Expect(image.Name).ToNot(BeEmpty())
				Expect(image.Name).Should(Equal(vcVM.Name()))
				// Expect(image.Annotations).ToNot(BeEmpty())
				// Expect(image.Annotations).To(HaveKeyWithValue(versionKey, versionVal))
			})
		})
	})
}

// getVMHomeDisk gets the VM's "home" disk. It makes some assumptions about the backing and disk name.
func getVMHomeDisk(
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	o mo.VirtualMachine) (*types.VirtualDisk, *types.VirtualDiskFlatVer2BackingInfo) {

	ExpectWithOffset(1, vcVM.Name()).ToNot(BeEmpty())
	ExpectWithOffset(1, o.Datastore).ToNot(BeEmpty())
	var dso mo.Datastore
	ExpectWithOffset(1, vcVM.Properties(ctx, o.Datastore[0], nil, &dso)).To(Succeed())

	devList := object.VirtualDeviceList(o.Config.Hardware.Device)
	l := devList.SelectByBackingInfo(&types.VirtualDiskFlatVer2BackingInfo{
		VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{
			FileName: fmt.Sprintf("[%s] %s/disk-0.vmdk", dso.Name, vcVM.Name()),
		},
	})
	ExpectWithOffset(1, l).To(HaveLen(1))

	disk := l[0].(*types.VirtualDisk)
	backing := disk.Backing.(*types.VirtualDiskFlatVer2BackingInfo)

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
