// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualcontroller_test

import (
	"context"
	"fmt"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/virtualcontroller"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", func() {
	It("should return a reconciler", func() {
		Expect(virtualcontroller.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", func() {
	It("should return 'virtualcontroller'", func() {
		Expect(virtualcontroller.New().Name()).To(Equal("virtualcontroller"))
	})
})

var _ = Describe("OnResult", func() {
	It("should return nil", func() {
		var ctx context.Context
		Expect(virtualcontroller.New().OnResult(ctx, nil, mo.VirtualMachine{}, nil)).To(Succeed())
	})
})

var _ = Describe("Reconcile", func() {

	var (
		r          vmconfig.Reconciler
		ctx        context.Context
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		moVM       mo.VirtualMachine
		vm         *vmopv1.VirtualMachine
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		r = virtualcontroller.New()

		vcsimCtx := builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()), builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = vmconfig.WithContext(ctx)

		vimClient = vcsimCtx.VCClient.Client

		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
			Runtime: vimtypes.VirtualMachineRuntimeInfo{
				PowerState: vimtypes.VirtualMachinePowerStatePoweredOff,
			},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "my-namespace",
				Name:        "my-vm",
				Annotations: map[string]string{},
			},
			Spec: vmopv1.VirtualMachineSpec{},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	Context("a panic is expected", func() {

		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})

			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})

		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})

			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})

		When("vimClient is nil", func() {
			JustBeforeEach(func() {
				vimClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vimClient is nil"))
			})
		})

		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})

		When("configSpec is nil", func() {
			JustBeforeEach(func() {
				configSpec = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("configSpec is nil"))
			})
		})
	})

	Context("no panic is expected", func() {
		const (
			pciControllerKey int32 = 0
		)

		When("PCI controller is not present", func() {
			BeforeEach(func() {
				moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{}
			})

			It("should return error", func() {
				Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).
					To(MatchError("PCI Controller not presented"))
			})
		})

		When("PCI controller is present", func() {
			BeforeEach(func() {
				moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
					&vimtypes.VirtualPCIController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 0,
							},
						},
					},
				}
			})

			It("should succeed without any device change", func() {
				Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
				Expect(configSpec.DeviceChange).To(HaveLen(0))
			})

			When("IDE controller is specified in spec", func() {
				BeforeEach(func() {
					vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
						IDEControllers: []vmopv1.IDEControllerSpec{
							{
								BusNumber: 0,
							},
						},
					}
				})

				It("should create a new IDE controller", func() {
					Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
					Expect(configSpec.DeviceChange).To(HaveLen(1))
					assertIDEControllerDeviceChange(
						configSpec.DeviceChange,
						0,
						vimtypes.VirtualDeviceConfigSpecOperationAdd,
						int32(0),
						ptr.To[int32](-1),
					)
				})

				When("VM is powered on", func() {
					BeforeEach(func() {
						moVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
					})

					It("should create a new IDE controller", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(1))
						assertIDEControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(0),
							ptr.To[int32](-1),
						)
					})
				})

				When("IDE controller is already configured in VM", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualIDEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 0,
								},
							},
						)
					})

					It("should not create a new IDE controller", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(0))
					})
				})

				When("VM has a different IDE controller", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualIDEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 1,
								},
							},
						)
					})

					It("should create a new IDE controller, and remove the old one", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))

						assertIDEControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationRemove,
							int32(1),
							nil,
						)
						assertIDEControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(0),
							ptr.To[int32](-1),
						)
					})
				})

				When("VM has this controller and is adding another in spec", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualIDEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 0,
								},
							},
						)
						vm.Spec.Hardware.IDEControllers = append(vm.Spec.Hardware.IDEControllers,
							vmopv1.IDEControllerSpec{
								BusNumber: 1,
							},
						)
					})

					It("should create a new IDE controller", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(1))
						assertIDEControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(1),
							ptr.To[int32](-1),
						)
					})
				})
			})

			When("NVME controller is specified in spec", func() {
				BeforeEach(func() {
					vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
						NVMEControllers: []vmopv1.NVMEControllerSpec{
							{
								BusNumber:   0,
								SharingMode: vmopv1.VirtualControllerSharingModeNone,
							},
						},
					}
				})

				It("should create a new NVME controller", func() {
					Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
					Expect(configSpec.DeviceChange).To(HaveLen(1))
					assertNVMEControllerDeviceChange(
						configSpec.DeviceChange,
						0,
						vimtypes.VirtualDeviceConfigSpecOperationAdd,
						int32(0),
						vimtypes.VirtualNVMEControllerSharingNoSharing,
					)
				})

				When("NVME controller has invalid sharing type Virtual", func() {
					BeforeEach(func() {
						Expect(vm.Spec.Hardware.NVMEControllers).To(HaveLen(1))
						vm.Spec.Hardware.NVMEControllers[0].SharingMode = vmopv1.VirtualControllerSharingModeVirtual
					})

					It("should skip this controller", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(0))
					})
				})

				When("NVME controller is already configured in VM", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualNVMEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 0,
								},
								SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
							},
						)
					})

					It("should not create a new NVME controller", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(0))
					})
				})

				When("VM has a different NVME controller", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualNVMEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 1,
								},
								SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
							},
						)
					})

					It("should create a new NVME controller, and remove the old one", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))

						assertNVMEControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationRemove,
							int32(1),
							vimtypes.VirtualNVMEControllerSharingNoSharing,
						)
						assertNVMEControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(0),
							vimtypes.VirtualNVMEControllerSharingNoSharing,
						)
					})
				})

				When("VM has this controller and is adding two more in spec", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualNVMEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 0,
								},
								SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
							},
						)
						vm.Spec.Hardware.NVMEControllers = append(vm.Spec.Hardware.NVMEControllers,
							vmopv1.NVMEControllerSpec{
								BusNumber:   1,
								SharingMode: vmopv1.VirtualControllerSharingModePhysical,
							},
							vmopv1.NVMEControllerSpec{
								BusNumber:   2,
								SharingMode: vmopv1.VirtualControllerSharingModePhysical,
							},
						)
					})

					It("should create two more NVME controllers, and having different deviceKey", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))
						d1, ok := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualNVMEController)
						Expect(ok).To(BeTrue())
						d2, ok := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualNVMEController)
						Expect(ok).To(BeTrue())
						// Compare DeviceKey first since it's not deterministic
						// which device will get what key value.
						Expect([]int32{-1, -2}).To(ConsistOf(
							d1.Key,
							d2.Key,
						))

						// Sort the DeviceChange by bus number so the order is
						// deterministic.
						sort.SliceStable(configSpec.DeviceChange, func(i, j int) bool {
							return configSpec.DeviceChange[i].GetVirtualDeviceConfigSpec().
								Device.(*vimtypes.VirtualNVMEController).BusNumber <
								configSpec.DeviceChange[j].GetVirtualDeviceConfigSpec().
									Device.(*vimtypes.VirtualNVMEController).BusNumber
						})
						assertNVMEControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(1),
							vimtypes.VirtualNVMEControllerSharingPhysicalSharing,
						)
						assertNVMEControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(2),
							vimtypes.VirtualNVMEControllerSharingPhysicalSharing,
						)
					})
				})

				When("NVME controller is specified in spec with different sharing mode", func() {
					BeforeEach(func() {
						Expect(vm.Spec.Hardware.NVMEControllers).To(HaveLen(1))
						vm.Spec.Hardware.NVMEControllers[0].SharingMode = vmopv1.VirtualControllerSharingModePhysical
					})

					It("should edit the NVME controller with different sharing mode", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(1))
						assertNVMEControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationEdit,
							int32(0),
							vimtypes.VirtualNVMEControllerSharingPhysicalSharing,
						)
					})
				})
			})

			When("SATA controller is specified in spec", func() {
				BeforeEach(func() {
					vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
						SATAControllers: []vmopv1.SATAControllerSpec{
							{
								BusNumber: 0,
							},
						},
					}
				})

				It("should create a new SATA controller", func() {
					Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
					Expect(configSpec.DeviceChange).To(HaveLen(1))
					assertSATAControllerDeviceChange(
						configSpec.DeviceChange,
						0,
						vimtypes.VirtualDeviceConfigSpecOperationAdd,
						int32(0),
					)
				})

				When("SATA controller is already configured in VM", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualAHCIController{
								VirtualSATAController: vimtypes.VirtualSATAController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											ControllerKey: pciControllerKey,
										},
										BusNumber: 0,
									},
								},
							},
						)
					})

					It("should not create a new SATA controller", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(0))
					})
				})

				When("VM has a different SATA controller", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualAHCIController{
								VirtualSATAController: vimtypes.VirtualSATAController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											ControllerKey: pciControllerKey,
										},
										BusNumber: 1,
									},
								},
							},
						)
					})

					It("should create a new SATA controller, and remove the old one", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))

						assertSATAControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationRemove,
							int32(1),
						)
						assertSATAControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(0),
						)
					})
				})

				When("VM has this controller and is adding two more in spec", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualAHCIController{
								VirtualSATAController: vimtypes.VirtualSATAController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											ControllerKey: pciControllerKey,
										},
										BusNumber: 0,
									},
								},
							},
						)
						vm.Spec.Hardware.SATAControllers = append(vm.Spec.Hardware.SATAControllers,
							vmopv1.SATAControllerSpec{
								BusNumber: 1,
							},
							vmopv1.SATAControllerSpec{
								BusNumber: 2,
							},
						)
					})

					It("should create two more SATA controllers, and having different deviceKey", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))
						d1, ok := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualAHCIController)
						Expect(ok).To(BeTrue())
						d2, ok := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualAHCIController)
						Expect(ok).To(BeTrue())
						// Compare DeviceKey first since it's not deterministic
						// which device will get what key value.
						Expect([]int32{-1, -2}).To(ConsistOf(
							d1.Key,
							d2.Key,
						))
						// Sort the DeviceChange by bus number so the order is
						// deterministic.
						sort.SliceStable(configSpec.DeviceChange, func(i, j int) bool {
							return configSpec.DeviceChange[i].GetVirtualDeviceConfigSpec().
								Device.(*vimtypes.VirtualAHCIController).BusNumber <
								configSpec.DeviceChange[j].GetVirtualDeviceConfigSpec().
									Device.(*vimtypes.VirtualAHCIController).BusNumber
						})
						assertSATAControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(1),
						)
						assertSATAControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(2),
						)
					})
				})
			})

			When("SCSI controller is specified in spec", func() {
				BeforeEach(func() {
					vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
						SCSIControllers: []vmopv1.SCSIControllerSpec{
							{
								BusNumber:   0,
								Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
								SharingMode: vmopv1.VirtualControllerSharingModeNone,
							},
						},
					}
				})

				It("should create a new SCSI controller", func() {
					Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
					Expect(configSpec.DeviceChange).To(HaveLen(1))
					assertSCSIControllerDeviceChange(
						configSpec.DeviceChange,
						0,
						vimtypes.VirtualDeviceConfigSpecOperationAdd,
						int32(0),
						vimtypes.VirtualSCSISharingNoSharing,
						vmopv1.SCSIControllerTypeParaVirtualSCSI)
				})

				DescribeTable("SCSI controller is already in VM with different subtype",
					func(dev vimtypes.BaseVirtualSCSIController, controllerType vmopv1.SCSIControllerType) {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							dev.(vimtypes.BaseVirtualDevice),
						)
						Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
						vm.Spec.Hardware.SCSIControllers[0].Type = controllerType
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(0))
					},

					Entry("ParaVirtual", &vimtypes.ParaVirtualSCSIController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: pciControllerKey,
								},
								BusNumber: 0,
							},
							SharedBus: vimtypes.VirtualSCSISharingNoSharing,
						},
					}, vmopv1.SCSIControllerTypeParaVirtualSCSI),
					Entry("BusLogic", &vimtypes.VirtualBusLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: pciControllerKey,
								},
								BusNumber: 0,
							},
							SharedBus: vimtypes.VirtualSCSISharingNoSharing,
						},
					}, vmopv1.SCSIControllerTypeBusLogic),
					Entry("LsiLogic", &vimtypes.VirtualLsiLogicController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: pciControllerKey,
								},
								BusNumber: 0,
							},
							SharedBus: vimtypes.VirtualSCSISharingNoSharing,
						},
					}, vmopv1.SCSIControllerTypeLsiLogic),
					Entry("LsiLogicSAS", &vimtypes.VirtualLsiLogicSASController{
						VirtualSCSIController: vimtypes.VirtualSCSIController{
							VirtualController: vimtypes.VirtualController{
								VirtualDevice: vimtypes.VirtualDevice{
									ControllerKey: pciControllerKey,
								},
								BusNumber: 0,
							},
							SharedBus: vimtypes.VirtualSCSISharingNoSharing,
						},
					}, vmopv1.SCSIControllerTypeLsiLogicSAS),
				)

				When("VM has a different SCSI controller", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualBusLogicController{
								VirtualSCSIController: vimtypes.VirtualSCSIController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											ControllerKey: pciControllerKey,
										},
										BusNumber: 1,
									},
									SharedBus: vimtypes.VirtualSCSISharingPhysicalSharing,
								},
							},
						)
					})

					It("should create a new SCSI controller, and remove the old one", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))

						assertSCSIControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationRemove,
							int32(1),
							vimtypes.VirtualSCSISharingPhysicalSharing,
							vmopv1.SCSIControllerTypeBusLogic,
						)
						assertSCSIControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(0),
							vimtypes.VirtualSCSISharingNoSharing,
							vmopv1.SCSIControllerTypeParaVirtualSCSI,
						)
					})
				})

				When("VM has this controller and is adding two more in spec", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.ParaVirtualSCSIController{
								VirtualSCSIController: vimtypes.VirtualSCSIController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											ControllerKey: pciControllerKey,
										},
										BusNumber: 0,
									},
									SharedBus: vimtypes.VirtualSCSISharingNoSharing,
								},
							},
						)
						vm.Spec.Hardware.SCSIControllers = append(vm.Spec.Hardware.SCSIControllers,
							vmopv1.SCSIControllerSpec{
								BusNumber:   1,
								Type:        vmopv1.SCSIControllerTypeLsiLogic,
								SharingMode: vmopv1.VirtualControllerSharingModePhysical,
							},
							vmopv1.SCSIControllerSpec{
								BusNumber:   2,
								Type:        vmopv1.SCSIControllerTypeLsiLogicSAS,
								SharingMode: vmopv1.VirtualControllerSharingModePhysical,
							},
						)
					})

					It("should create two more SCSI controllers, and having different deviceKey", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))
						d1, ok := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec().Device.(vimtypes.BaseVirtualSCSIController)
						Expect(ok).To(BeTrue())
						d2, ok := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec().Device.(vimtypes.BaseVirtualSCSIController)
						Expect(ok).To(BeTrue())
						// Compare DeviceKey first since it's not deterministic
						// which device will get what key value.
						Expect([]int32{-1, -2}).To(ConsistOf(
							d1.GetVirtualSCSIController().Key,
							d2.GetVirtualSCSIController().Key,
						))

						// Sort the DeviceChange by bus number so the order is
						// deterministic.
						sort.SliceStable(configSpec.DeviceChange, func(i, j int) bool {
							return configSpec.DeviceChange[i].GetVirtualDeviceConfigSpec().
								Device.(vimtypes.BaseVirtualSCSIController).GetVirtualSCSIController().BusNumber <
								configSpec.DeviceChange[j].GetVirtualDeviceConfigSpec().
									Device.(vimtypes.BaseVirtualSCSIController).GetVirtualSCSIController().BusNumber
						})
						assertSCSIControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(1),
							vimtypes.VirtualSCSISharingPhysicalSharing,
							vmopv1.SCSIControllerTypeLsiLogic)
						assertSCSIControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(2),
							vimtypes.VirtualSCSISharingPhysicalSharing,
							vmopv1.SCSIControllerTypeLsiLogicSAS)
					})
				})

				When("SCSI controller is specified in spec with different sharing mode", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.ParaVirtualSCSIController{
								VirtualSCSIController: vimtypes.VirtualSCSIController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											ControllerKey: pciControllerKey,
										},
										BusNumber: 0,
									},
									SharedBus: vimtypes.VirtualSCSISharingNoSharing,
								},
							},
						)
						Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
						vm.Spec.Hardware.SCSIControllers[0].SharingMode = vmopv1.VirtualControllerSharingModePhysical
					})

					It("should edit the SCSI controller with different sharing mode", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(1))
						assertSCSIControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationEdit,
							int32(0),
							vimtypes.VirtualSCSISharingPhysicalSharing,
							vmopv1.SCSIControllerTypeParaVirtualSCSI)
					})
				})

				When("SCSI controller is specified in spec with different type", func() {
					BeforeEach(func() {
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualBusLogicController{
								VirtualSCSIController: vimtypes.VirtualSCSIController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											ControllerKey: pciControllerKey,
										},
										BusNumber: 0,
									},
									SharedBus: vimtypes.VirtualSCSISharingNoSharing,
								},
							},
						)
					})

					It("should have two device changes, one to remove the old controller and one to add the new one", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))
						assertSCSIControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationRemove,
							int32(0),
							vimtypes.VirtualSCSISharingNoSharing,
							vmopv1.SCSIControllerTypeBusLogic)

						assertSCSIControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(0),
							vimtypes.VirtualSCSISharingNoSharing,
							vmopv1.SCSIControllerTypeParaVirtualSCSI)
					})
				})
			})

			When("VM is PoweredOn", func() {
				BeforeEach(func() {
					moVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
				})

				When("There are add and delete device changes", func() {
					BeforeEach(func() {
						vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							IDEControllers: []vmopv1.IDEControllerSpec{
								{
									BusNumber: 0,
								},
							},
						}

						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualIDEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 1,
								},
							},
						)
					})

					It("should add both device changes", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(2))

						assertIDEControllerDeviceChange(
							configSpec.DeviceChange,
							0,
							vimtypes.VirtualDeviceConfigSpecOperationRemove,
							int32(1),
							nil,
						)
						assertIDEControllerDeviceChange(
							configSpec.DeviceChange,
							1,
							vimtypes.VirtualDeviceConfigSpecOperationAdd,
							int32(0),
							ptr.To[int32](-1),
						)
					})
				})

				When("There are edit device change", func() {
					BeforeEach(func() {
						vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
							NVMEControllers: []vmopv1.NVMEControllerSpec{
								{
									BusNumber:   0,
									SharingMode: vmopv1.VirtualControllerSharingModeNone,
								},
							},
						}
						moVM.Config.Hardware.Device = append(moVM.Config.Hardware.Device,
							&vimtypes.VirtualNVMEController{
								VirtualController: vimtypes.VirtualController{
									VirtualDevice: vimtypes.VirtualDevice{
										ControllerKey: pciControllerKey,
									},
									BusNumber: 0,
								},
								SharedBus: string(vimtypes.VirtualNVMEControllerSharingPhysicalSharing),
							},
						)
					})

					It("should succeed but not add the edit event to device changes", func() {
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
						Expect(configSpec.DeviceChange).To(HaveLen(0))
					})
				})
			})
		})
	})
})

func assertIDEControllerDeviceChange(
	deviceChange []vimtypes.BaseVirtualDeviceConfigSpec,
	index int,
	expectedOperation vimtypes.VirtualDeviceConfigSpecOperation,
	busNumber int32,
	deviceKey *int32,
) {

	GinkgoHelper()

	Expect(len(deviceChange)).To(BeNumerically(">", index), "deviceChange length is less than or equal to index")

	dev := deviceChange[index].GetVirtualDeviceConfigSpec()
	Expect(dev.Operation).
		To(BeAssignableToTypeOf(expectedOperation), "operation doesn't match")

	devIDE, ok := dev.Device.(*vimtypes.VirtualIDEController)
	Expect(ok).To(BeTrue(), "device is not a VirtualIDEController")
	Expect(devIDE.BusNumber).To(Equal(busNumber), "bus number doesn't match")
	if deviceKey != nil {
		Expect(devIDE.Key).To(Equal(*deviceKey))
	}
}

func assertNVMEControllerDeviceChange(
	deviceChange []vimtypes.BaseVirtualDeviceConfigSpec,
	index int,
	expectedOperation vimtypes.VirtualDeviceConfigSpecOperation,
	busNumber int32,
	sharingMode vimtypes.VirtualNVMEControllerSharing,
) {

	GinkgoHelper()

	Expect(len(deviceChange)).To(BeNumerically(">", index), "deviceChange length is less than or equal to index")

	dev := deviceChange[index].GetVirtualDeviceConfigSpec()
	Expect(dev.Operation).
		To(BeAssignableToTypeOf(expectedOperation), "operation doesn't match")

	devNVME, ok := dev.Device.(*vimtypes.VirtualNVMEController)
	Expect(ok).To(BeTrue(), "device is not a VirtualNVMEController")
	Expect(devNVME.BusNumber).To(Equal(busNumber), "bus number doesn't match")
	Expect(devNVME.SharedBus).To(Equal(string(sharingMode)), "sharing mode doesn't match")
}

func assertSATAControllerDeviceChange(
	deviceChange []vimtypes.BaseVirtualDeviceConfigSpec,
	index int,
	expectedOperation vimtypes.VirtualDeviceConfigSpecOperation,
	busNumber int32,
) {

	GinkgoHelper()

	Expect(len(deviceChange)).To(BeNumerically(">", index), "deviceChange length is less than or equal to index")

	dev := deviceChange[index].GetVirtualDeviceConfigSpec()
	Expect(dev.Operation).
		To(BeAssignableToTypeOf(expectedOperation), "operation doesn't match")

	devSATA, ok := dev.Device.(*vimtypes.VirtualAHCIController)
	Expect(ok).To(BeTrue(), "device is not a VirtualAHCIController")
	Expect(devSATA.BusNumber).To(Equal(busNumber), "bus number doesn't match")
}

func assertSCSIControllerDeviceChange(
	deviceChange []vimtypes.BaseVirtualDeviceConfigSpec,
	index int,
	expectedOperation vimtypes.VirtualDeviceConfigSpecOperation,
	busNumber int32,
	sharingMode vimtypes.VirtualSCSISharing,
	scsiControllerType vmopv1.SCSIControllerType,
) {

	GinkgoHelper()

	Expect(len(deviceChange)).To(BeNumerically(">", index), "deviceChange length is less than or equal to index")

	dev := deviceChange[index].GetVirtualDeviceConfigSpec()
	Expect(dev.Operation).
		To(BeAssignableToTypeOf(expectedOperation), "operation doesn't match")

	devSCSI, ok := dev.Device.(vimtypes.BaseVirtualSCSIController)
	Expect(ok).To(BeTrue(), fmt.Sprintf("device is not a BaseVirtualSCSIController: %v", dev.Device))
	Expect(virtualcontroller.SCSIControllerTypeMatch(devSCSI, scsiControllerType)).To(BeTrue(), fmt.Sprintf("SCSI controller type doesn't match: %v", dev.Device))

	devSCSIController := devSCSI.GetVirtualSCSIController()
	Expect(devSCSIController.GetVirtualSCSIController().BusNumber).To(Equal(busNumber), "bus number doesn't match")
	Expect(devSCSIController.SharedBus).To(Equal(sharingMode), "sharing mode doesn't match")
}
