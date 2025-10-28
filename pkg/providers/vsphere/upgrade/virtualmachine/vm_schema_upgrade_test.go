// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	upgradevm "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/upgrade/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	testVMIName         = "test-vmi"
	testVMIFileName     = "test-file.iso"
	testLibItemID       = "test-item-id"
	testBiosUUID        = "test-bios-uuid"
	testInstanceUUID    = "test-instance-uuid"
	testNamespace       = "default"
	testDiskUUID1       = "disk-uuid-1"
	testDiskUUID2       = "disk-uuid-2"
	testPVCName1        = "pvc-1"
	testPVCName2        = "pvc-2"
	testPVCVolumeName1  = "pvc-volume-1"
	testPVCVolumeName2  = "pvc-volume-2"
	testAttachmentName1 = "vm-attachment-1"
	testAttachmentName2 = "vm-attachment-2"
	testDiskFileName1   = "[datastore1] vm/disk1.vmdk"
	testDiskFileName2   = "[datastore1] vm/disk2.vmdk"
	testDiskCapacity1   = 10737418240
	testDiskCapacity2   = 21474836480
	testControllerKey   = 1000
	testDiskKey1        = 2000
	testDiskKey2        = 2001
	testMismatchValue   = 99
)

var _ = Describe("ReconcileSchemaUpgrade", func() {

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		vm        *vmopv1.VirtualMachine
		moVM      mo.VirtualMachine
	)

	BeforeEach(func() {
		pkg.BuildVersion = "v1.2.3"
		ctx = pkgcfg.NewContextWithDefaultConfig()
		ctx = logr.NewContext(ctx, klog.Background())

		k8sInitObjs := builder.DummyImageAndItemObjectsForCdromBacking(
			testVMIName, "default", "VirtualMachineImage", testVMIFileName, testLibItemID,
			true, true, resource.MustParse("100Mi"), true, true, imgregv1a1.ContentLibraryItemTypeIso)

		k8sClient = builder.NewFakeClient(k8sInitObjs...)

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				UID:       types.UID("abc-123"),
				Namespace: "default",
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
	})

	When("it should panic", func() {
		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
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
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})

		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})

		When("moVM.config is nil", func() {
			BeforeEach(func() {
				moVM.Config = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
				}
				Expect(fn).To(PanicWith("moVM.config is nil"))
			})
		})
	})

	When("it should not panic", func() {
		var expectedErr error

		BeforeEach(func() {
			expectedErr = upgradevm.ErrUpgradeSchema
		})
		JustBeforeEach(func() {
			err := upgradevm.ReconcileSchemaUpgrade(
				ctx,
				k8sClient,
				vm,
				moVM)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}
		})

		When("the object is already upgraded", func() {
			BeforeEach(func() {
				expectedErr = nil

				vm.Annotations = map[string]string{
					pkgconst.UpgradedToBuildVersionAnnotationKey:  pkgcfg.FromContext(ctx).BuildVersion,
					pkgconst.UpgradedToSchemaVersionAnnotationKey: vmopv1.GroupVersion.Version,
				}
				vm.Spec.InstanceUUID = ""
				vm.Spec.BiosUUID = ""
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						InstanceID: "",
					},
				}
				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					InstanceUuid: "123",
					Uuid:         "123",
				}
			})
			It("should not modify any of the fields it would otherwise upgrade", func() {
				Expect(vm.Spec.InstanceUUID).To(BeEmpty())
				Expect(vm.Spec.BiosUUID).To(BeEmpty())
				Expect(vm.Spec.Bootstrap.CloudInit.InstanceID).To(BeEmpty())
			})
		})

		When("the object needs to be upgraded", func() {
			BeforeEach(func() {
				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Uuid:         "test-bios-uuid",
					InstanceUuid: "test-instance-uuid",
				}
			})

			assertUpgraded := func() {
				ExpectWithOffset(1, vm.Annotations).To(HaveKeyWithValue(
					pkgconst.UpgradedToBuildVersionAnnotationKey,
					pkgcfg.FromContext(ctx).BuildVersion))
				ExpectWithOffset(1, vm.Annotations).To(HaveKeyWithValue(
					pkgconst.UpgradedToSchemaVersionAnnotationKey,
					vmopv1.GroupVersion.Version))
			}

			Context("BIOS UUID reconciliation", func() {
				When("BIOS UUID is empty in VM spec", func() {
					It("should set BIOS UUID from moVM", func() {
						assertUpgraded()
						Expect(vm.Spec.BiosUUID).To(Equal("test-bios-uuid"))
					})
				})

				When("BIOS UUID is already set in VM spec", func() {
					BeforeEach(func() {
						vm.Spec.BiosUUID = "existing-bios-uuid"
					})
					It("should not modify existing BIOS UUID", func() {
						Expect(vm.Spec.BiosUUID).To(Equal("existing-bios-uuid"))
					})
				})

				When("moVM config UUID is empty", func() {
					BeforeEach(func() {
						moVM.Config.Uuid = ""
					})
					It("should not set BIOS UUID", func() {
						Expect(vm.Spec.BiosUUID).To(BeEmpty())
					})
				})
			})

			Context("Instance UUID reconciliation", func() {
				When("Instance UUID is empty in VM spec", func() {
					It("should set Instance UUID from moVM", func() {
						assertUpgraded()
						Expect(vm.Spec.InstanceUUID).To(Equal("test-instance-uuid"))
					})
				})

				When("Instance UUID is already set in VM spec", func() {
					BeforeEach(func() {
						vm.Spec.InstanceUUID = "existing-instance-uuid"
					})
					It("should not modify existing Instance UUID", func() {
						Expect(vm.Spec.InstanceUUID).To(Equal("existing-instance-uuid"))
					})
				})

				When("moVM config InstanceUuid is empty", func() {
					BeforeEach(func() {
						moVM.Config.InstanceUuid = ""
					})
					It("should not set Instance UUID", func() {
						Expect(vm.Spec.InstanceUUID).To(BeEmpty())
					})
				})
			})

			Context("CloudInit instance UUID reconciliation", func() {
				When("VM has CloudInit bootstrap configuration", func() {
					BeforeEach(func() {
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
					})

					When("CloudInit InstanceID is empty", func() {
						It("should generate CloudInit InstanceID", func() {
							assertUpgraded()
							Expect(vm.Spec.Bootstrap.CloudInit.InstanceID).ToNot(BeEmpty())
						})
					})

					When("CloudInit InstanceID is already set", func() {
						BeforeEach(func() {
							vm.Spec.Bootstrap.CloudInit.InstanceID = "existing-cloud-init-id"
						})
						It("should not modify existing CloudInit InstanceID", func() {
							Expect(vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal("existing-cloud-init-id"))
						})
					})
				})

				When("VM has no bootstrap configuration", func() {
					It("should not set CloudInit InstanceID", func() {
						Expect(vm.Spec.Bootstrap).To(BeNil())
					})
				})

				When("VM has bootstrap but no CloudInit configuration", func() {
					BeforeEach(func() {
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
					})
					It("should not set CloudInit InstanceID", func() {
						Expect(vm.Spec.Bootstrap.CloudInit).To(BeNil())
					})
				})
			})

			Context("Controller reconciliation", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
						config.Features.VMSharedDisks = true
					})
				})

				Context("IDE Controllers", func() {
					BeforeEach(func() {
						moVM.Config.Hardware = vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualIDEController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 200,
										},
										BusNumber: 0,
									},
								},
							},
						}
					})

					It("should add IDE controller to VM spec", func() {
						Expect(vm.Spec.Hardware).ToNot(BeNil())
						Expect(vm.Spec.Hardware.IDEControllers).To(HaveLen(1))
						Expect(vm.Spec.Hardware.IDEControllers[0].BusNumber).To(Equal(int32(0)))
					})

					When("IDE controller already exists in VM spec", func() {
						BeforeEach(func() {
							vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
								IDEControllers: []vmopv1.IDEControllerSpec{
									{BusNumber: 0},
								},
							}
						})
						It("should not add duplicate IDE controller", func() {
							Expect(vm.Spec.Hardware.IDEControllers).To(HaveLen(1))
						})
					})
				})

				Context("NVME Controllers", func() {
					BeforeEach(func() {
						moVM.Config.Hardware = vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualNVMEController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 300,
											SlotInfo: &vimtypes.VirtualDevicePciBusSlotInfo{
												PciSlotNumber: 32,
											},
										},
										BusNumber: 0,
									},
									SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
								},
							},
						}
					})

					It("should add NVME controller to VM spec", func() {
						Expect(vm.Spec.Hardware).ToNot(BeNil())
						Expect(vm.Spec.Hardware.NVMEControllers).To(HaveLen(1))
						controller := vm.Spec.Hardware.NVMEControllers[0]
						Expect(controller.BusNumber).To(Equal(int32(0)))
						Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
						Expect(controller.PCISlotNumber).To(HaveValue(Equal(int32(32))))
					})

					When("NVME controller already exists in VM spec", func() {
						BeforeEach(func() {
							vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
								NVMEControllers: []vmopv1.NVMEControllerSpec{
									{BusNumber: 0},
								},
							}
						})
						It("should not add duplicate NVME controller", func() {
							Expect(vm.Spec.Hardware.NVMEControllers).To(HaveLen(1))
						})
					})

					When("NVME controller has physical sharing", func() {
						BeforeEach(func() {
							moVM.Config.Hardware.Device[0].(*vimtypes.VirtualNVMEController).SharedBus = string(vimtypes.VirtualNVMEControllerSharingPhysicalSharing)
						})
						It("should set physical sharing mode", func() {
							controller := vm.Spec.Hardware.NVMEControllers[0]
							Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
						})
					})
				})

				Context("SATA Controllers", func() {
					BeforeEach(func() {
						moVM.Config.Hardware = vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualAHCIController{
									VirtualSATAController: vimtypes.VirtualSATAController{
										VirtualController: vimtypes.VirtualController{
											VirtualDevice: vimtypes.VirtualDevice{
												Key: 400,
												SlotInfo: &vimtypes.VirtualDevicePciBusSlotInfo{
													PciSlotNumber: 33,
												},
											},
											BusNumber: 0,
										},
									},
								},
							},
						}
					})

					It("should add SATA controller to VM spec", func() {
						Expect(vm.Spec.Hardware).ToNot(BeNil())
						Expect(vm.Spec.Hardware.SATAControllers).To(HaveLen(1))
						controller := vm.Spec.Hardware.SATAControllers[0]
						Expect(controller.BusNumber).To(Equal(int32(0)))
						Expect(controller.PCISlotNumber).To(HaveValue(Equal(int32(33))))
					})

					When("SATA controller already exists in VM spec", func() {
						BeforeEach(func() {
							vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
								SATAControllers: []vmopv1.SATAControllerSpec{
									{BusNumber: 0},
								},
							}
						})
						It("should not add duplicate SATA controller", func() {
							Expect(vm.Spec.Hardware.SATAControllers).To(HaveLen(1))
						})
					})
				})

				Context("SCSI Controllers", func() {
					When("ParaVirtual SCSI Controller", func() {
						BeforeEach(func() {
							moVM.Config.Hardware = vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.ParaVirtualSCSIController{
										VirtualSCSIController: vimtypes.VirtualSCSIController{
											VirtualController: vimtypes.VirtualController{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 1000,
													SlotInfo: &vimtypes.VirtualDevicePciBusSlotInfo{
														PciSlotNumber: 16,
													},
												},
												BusNumber: 0,
											},
											SharedBus: vimtypes.VirtualSCSISharingNoSharing,
										},
									},
								},
							}
						})

						It("should add ParaVirtual SCSI controller to VM spec", func() {
							Expect(vm.Spec.Hardware).ToNot(BeNil())
							Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
							controller := vm.Spec.Hardware.SCSIControllers[0]
							Expect(controller.BusNumber).To(Equal(int32(0)))
							Expect(controller.Type).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))
							Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
							Expect(controller.PCISlotNumber).To(HaveValue(Equal(int32(16))))
						})
					})

					When("LSI Logic SCSI Controller", func() {
						BeforeEach(func() {
							moVM.Config.Hardware = vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualLsiLogicController{
										VirtualSCSIController: vimtypes.VirtualSCSIController{
											VirtualController: vimtypes.VirtualController{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 1001,
												},
												BusNumber: 1,
											},
											SharedBus: vimtypes.VirtualSCSISharingPhysicalSharing,
										},
									},
								},
							}
						})

						It("should add LSI Logic SCSI controller to VM spec", func() {
							Expect(vm.Spec.Hardware).ToNot(BeNil())
							Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
							controller := vm.Spec.Hardware.SCSIControllers[0]
							Expect(controller.BusNumber).To(Equal(int32(1)))
							Expect(controller.Type).To(Equal(vmopv1.SCSIControllerTypeLsiLogic))
							Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
						})
					})

					When("LSI Logic SAS SCSI Controller", func() {
						BeforeEach(func() {
							moVM.Config.Hardware = vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualLsiLogicSASController{
										VirtualSCSIController: vimtypes.VirtualSCSIController{
											VirtualController: vimtypes.VirtualController{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 1002,
												},
												BusNumber: 2,
											},
											SharedBus: vimtypes.VirtualSCSISharingVirtualSharing,
										},
									},
								},
							}
						})

						It("should add LSI Logic SAS SCSI controller to VM spec", func() {
							Expect(vm.Spec.Hardware).ToNot(BeNil())
							Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
							controller := vm.Spec.Hardware.SCSIControllers[0]
							Expect(controller.BusNumber).To(Equal(int32(2)))
							Expect(controller.Type).To(Equal(vmopv1.SCSIControllerTypeLsiLogicSAS))
							Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeVirtual))
						})
					})

					When("Bus Logic SCSI Controller", func() {
						BeforeEach(func() {
							moVM.Config.Hardware = vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualBusLogicController{
										VirtualSCSIController: vimtypes.VirtualSCSIController{
											VirtualController: vimtypes.VirtualController{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 1003,
												},
												BusNumber: 3,
											},
										},
									},
								},
							}
						})

						It("should add Bus Logic SCSI controller to VM spec", func() {
							Expect(vm.Spec.Hardware).ToNot(BeNil())
							Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
							controller := vm.Spec.Hardware.SCSIControllers[0]
							Expect(controller.BusNumber).To(Equal(int32(3)))
							Expect(controller.Type).To(Equal(vmopv1.SCSIControllerTypeBusLogic))
						})
					})

					When("SCSI controller already exists in VM spec", func() {
						BeforeEach(func() {
							vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
								SCSIControllers: []vmopv1.SCSIControllerSpec{
									{BusNumber: 0},
								},
							}
							moVM.Config.Hardware = vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.ParaVirtualSCSIController{
										VirtualSCSIController: vimtypes.VirtualSCSIController{
											VirtualController: vimtypes.VirtualController{
												VirtualDevice: vimtypes.VirtualDevice{
													Key: 1000,
												},
												BusNumber: 0,
											},
										},
									},
								},
							}
						})
						It("should not add duplicate SCSI controller", func() {
							Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
						})
					})
				})

				Context("Multiple controllers", func() {
					BeforeEach(func() {
						moVM.Config.Hardware = vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualIDEController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{Key: 200},
										BusNumber:     0,
									},
								},
								&vimtypes.VirtualNVMEController{
									VirtualController: vimtypes.VirtualController{
										VirtualDevice: vimtypes.VirtualDevice{Key: 300},
										BusNumber:     0,
									},
									SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
								},
								&vimtypes.ParaVirtualSCSIController{
									VirtualSCSIController: vimtypes.VirtualSCSIController{
										VirtualController: vimtypes.VirtualController{
											VirtualDevice: vimtypes.VirtualDevice{Key: 1000},
											BusNumber:     0,
										},
									},
								},
							},
						}
					})

					It("should add all controller types", func() {
						Expect(vm.Spec.Hardware).ToNot(BeNil())
						Expect(vm.Spec.Hardware.IDEControllers).To(HaveLen(1))
						Expect(vm.Spec.Hardware.NVMEControllers).To(HaveLen(1))
						Expect(vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
					})
				})
			})

		})
	})

	Context("reconcileVirtualCDROMs", func() {
		var (
			cdromSpec   vmopv1.VirtualMachineCdromSpec
			moVM        mo.VirtualMachine
			expectedErr error
		)

		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
			expectedErr = upgradevm.ErrUpgradeSchema
			cdromSpec = vmopv1.VirtualMachineCdromSpec{
				Image: vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: "test-vmi",
				},
			}
			vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
				Cdrom: []vmopv1.VirtualMachineCdromSpec{cdromSpec},
			}
			moVM = mo.VirtualMachine{
				Config: &vimtypes.VirtualMachineConfigInfo{
					Hardware: vimtypes.VirtualHardware{
						Device: []vimtypes.BaseVirtualDevice{},
					},
				},
			}
		})

		JustBeforeEach(func() {
			err := upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}
		})

		When("VM has no CD-ROM specs", func() {
			BeforeEach(func() {
				vm.Spec.Hardware = nil
			})

			It("should skip reconciliation", func() {
				Expect(vm.Spec.Hardware).To(BeNil())
			})
		})

		When("VM has empty CD-ROM specs", func() {
			BeforeEach(func() {
				vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					Cdrom: []vmopv1.VirtualMachineCdromSpec{},
				}
			})

			It("should skip reconciliation", func() {
				Expect(vm.Spec.Hardware.Cdrom).To(BeEmpty())
			})
		})

		When("VM has CD-ROM specs but no matching devices", func() {
			BeforeEach(func() {
				// Add a CD-ROM device with different backing file.
				cdromDevice := &vimtypes.VirtualCdrom{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
						DeviceInfo: &vimtypes.Description{
							Label: "CD/DVD drive 1",
						},
						ControllerKey: 200,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualCdromIsoBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "different-file.iso",
							},
						},
					},
				}
				moVM.Config.Hardware.Device = append(
					moVM.Config.Hardware.Device, cdromDevice)
			})

			It("should not modify CD-ROM specs", func() {
				originalSpec := vm.Spec.Hardware.Cdrom[0]
				Expect(vm.Spec.Hardware.Cdrom[0]).To(Equal(originalSpec))
			})
		})

		When("VM has CD-ROM specs with matching backing files", func() {
			var (
				cdromDevice   *vimtypes.VirtualCdrom
				ideController *vimtypes.VirtualIDEController
			)

			BeforeEach(func() {
				ideController = &vimtypes.VirtualIDEController{
					VirtualController: vimtypes.VirtualController{
						VirtualDevice: vimtypes.VirtualDevice{
							Key: 200,
							DeviceInfo: &vimtypes.Description{
								Label: "IDE 0",
							},
						},
						BusNumber: 0,
					},
				}

				cdromDevice = &vimtypes.VirtualCdrom{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
						DeviceInfo: &vimtypes.Description{
							Label: "CD/DVD drive 1",
						},
						ControllerKey: 200,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualCdromIsoBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "test-file.iso", // This should match the resolved file name
							},
						},
					},
				}

				moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
					ideController, cdromDevice,
				}

				vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							DeviceKey: 200,
							Type:      vmopv1.VirtualControllerTypeIDE,
							BusNumber: 0,
						},
					},
				}
			})

			It("should backfill controller information when backing files match", func() {
				Expect(vm.Spec.Hardware.Cdrom).To(HaveLen(1))
				cdromSpec := vm.Spec.Hardware.Cdrom[0]
				Expect(cdromSpec.ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(cdromSpec.ControllerBusNumber).To(HaveValue(Equal(int32(0))))
				Expect(cdromSpec.UnitNumber).To(HaveValue(Equal(int32(0))))
			})

			When("controller type is empty", func() {
				BeforeEach(func() {
					vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(0))
					vm.Spec.Hardware.Cdrom[0].ControllerType = ""
					vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(0))
				})

				It("should update when controller type is empty", func() {
					cdromSpec := vm.Spec.Hardware.Cdrom[0]
					Expect(cdromSpec.ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				})
			})
		})

		When("VM has CD-ROM specs with SATA controller", func() {
			var (
				cdromDevice    *vimtypes.VirtualCdrom
				sataController *vimtypes.VirtualAHCIController
			)

			BeforeEach(func() {
				sataController = &vimtypes.VirtualAHCIController{
					VirtualSATAController: vimtypes.VirtualSATAController{
						VirtualController: vimtypes.VirtualController{
							VirtualDevice: vimtypes.VirtualDevice{
								Key: 300,
								DeviceInfo: &vimtypes.Description{
									Label: "SATA 0",
								},
							},
							BusNumber: 0,
						},
					},
				}

				cdromDevice = &vimtypes.VirtualCdrom{
					VirtualDevice: vimtypes.VirtualDevice{
						Key: 100,
						DeviceInfo: &vimtypes.Description{
							Label: "CD/DVD drive 1",
						},
						ControllerKey: 300,
						UnitNumber:    ptr.To(int32(0)),
						Backing: &vimtypes.VirtualCdromIsoBackingInfo{
							VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
								FileName: "test-file.iso",
							},
						},
					},
				}

				moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
					sataController, cdromDevice,
				}

				vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							DeviceKey: 300,
							Type:      vmopv1.VirtualControllerTypeSATA,
							BusNumber: 0,
						},
					},
				}
			})

			It("should backfill SATA controller information when backing files match", func() {
				Expect(vm.Spec.Hardware.Cdrom).To(HaveLen(1))
				cdromSpec := vm.Spec.Hardware.Cdrom[0]
				Expect(cdromSpec.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSATA))
				Expect(cdromSpec.ControllerBusNumber).To(HaveValue(Equal(int32(0))))
				Expect(cdromSpec.UnitNumber).To(HaveValue(Equal(int32(0))))
			})
		})

	})

	// Context reconcileVirtualDisks tests the backfilling of PVC volume placement
	// information (UnitNumber, ControllerType, ControllerBusNumber) from the
	// vSphere VM hardware configuration. These tests verify that the atomic
	// backfill logic works correctly - all three fields are backfilled together
	// or none at all if any conflict is detected.
	Context("reconcileVirtualDisks", func() {
		var (
			pvcVolume1  vmopv1.VirtualMachineVolume
			pvcVolume2  vmopv1.VirtualMachineVolume
			diskDevice1 *vimtypes.VirtualDisk
			diskDevice2 *vimtypes.VirtualDisk
			scsiCtrl    *vimtypes.ParaVirtualSCSIController
			attachment1 *cnsv1alpha1.CnsNodeVmAttachment
			attachment2 *cnsv1alpha1.CnsNodeVmAttachment
			expectedErr error
		)

		// Test helper functions for this Context

		// assertPVCPlacementPopulated verifies all placement fields are set correctly
		assertPVCPlacementPopulated := func(pvc *vmopv1.PersistentVolumeClaimVolumeSource, unitNumber, busNumber int32, controllerType vmopv1.VirtualControllerType) {
			Expect(pvc.UnitNumber).To(HaveValue(Equal(unitNumber)))
			Expect(pvc.ControllerType).To(Equal(controllerType))
			Expect(pvc.ControllerBusNumber).To(HaveValue(Equal(busNumber)))
		}

		// assertPVCPlacementEmpty verifies all placement fields are empty
		assertPVCPlacementEmpty := func(pvc *vmopv1.PersistentVolumeClaimVolumeSource) {
			Expect(pvc.UnitNumber).To(BeNil())
			Expect(pvc.ControllerType).To(BeEmpty())
			Expect(pvc.ControllerBusNumber).To(BeNil())
		}

		// createControllerStatus creates a VirtualControllerStatus for tests
		createControllerStatus := func(deviceKey, busNumber int32, controllerType vmopv1.VirtualControllerType) vmopv1.VirtualControllerStatus {
			return vmopv1.VirtualControllerStatus{
				DeviceKey: deviceKey,
				Type:      controllerType,
				BusNumber: busNumber,
			}
		}

		// setupK8sClientWithAttachments creates a fake Kubernetes client with CNS
		// attachment indexing. This is required for GetCnsNodeVMAttachmentsForVM to work
		// properly in tests, as it relies on indexing by spec.nodeuuid field.
		setupK8sClientWithAttachments := func(attachments ...*cnsv1alpha1.CnsNodeVmAttachment) {
			scheme := builder.NewScheme()
			objs := make([]ctrlclient.Object, len(attachments))
			for i, att := range attachments {
				objs[i] = att
			}
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				WithStatusSubresource(builder.KnownObjectTypes()...).
				WithIndex(
					&cnsv1alpha1.CnsNodeVmAttachment{},
					"spec.nodeuuid",
					func(rawObj ctrlclient.Object) []string {
						attachment := rawObj.(*cnsv1alpha1.CnsNodeVmAttachment)
						return []string{attachment.Spec.NodeUUID}
					}).
				Build()
		}

		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
			expectedErr = upgradevm.ErrUpgradeSchema

			scsiCtrl = builder.DummySCSIController(testControllerKey, 0)

			diskDevice1 = builder.DummyVirtualDisk(testDiskKey1, testControllerKey, 0, testDiskFileName1, testDiskUUID1, testDiskCapacity1)
			diskDevice2 = builder.DummyVirtualDisk(testDiskKey2, testControllerKey, 1, testDiskFileName2, testDiskUUID2, testDiskCapacity2)

			pvcVolume1 = builder.DummyPVCVolume(testPVCVolumeName1, testPVCName1)
			pvcVolume2 = builder.DummyPVCVolume(testPVCVolumeName2, testPVCName2)

			attachment1 = builder.DummyCnsNodeVMAttachment(testAttachmentName1, testNamespace, testBiosUUID, testPVCName1, testDiskUUID1, true)
			attachment2 = builder.DummyCnsNodeVMAttachment(testAttachmentName2, testNamespace, testBiosUUID, testPVCName2, testDiskUUID2, true)

			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{pvcVolume1, pvcVolume2}
			vm.Status.BiosUUID = testBiosUUID
			vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
				Controllers: []vmopv1.VirtualControllerStatus{
					createControllerStatus(testControllerKey, 0, vmopv1.VirtualControllerTypeSCSI),
				},
			}

			moVM.Config = &vimtypes.VirtualMachineConfigInfo{
				Uuid:         testBiosUUID,
				InstanceUuid: testInstanceUUID,
				Hardware: vimtypes.VirtualHardware{
					Device: []vimtypes.BaseVirtualDevice{
						scsiCtrl,
						diskDevice1,
						diskDevice2,
					},
				},
			}

			setupK8sClientWithAttachments(attachment1, attachment2)
		})

		JustBeforeEach(func() {
			err := upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}
		})

		When("VM has no BiosUUID in status", func() {
			BeforeEach(func() {
				vm.Status.BiosUUID = ""
			})

			It("should not backfill PVC volumes", func() {
				assertPVCPlacementEmpty(vm.Spec.Volumes[0].PersistentVolumeClaim)
			})
		})

		When("VM has no volumes", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = nil
			})

			It("should skip reconciliation", func() {
				Expect(vm.Spec.Volumes).To(BeNil())
			})
		})

		When("VM has volumes with all fields already populated", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber = ptr.To(int32(0))
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber = ptr.To(int32(0))
				vm.Spec.Volumes[1].PersistentVolumeClaim.UnitNumber = ptr.To(int32(1))
				vm.Spec.Volumes[1].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
				vm.Spec.Volumes[1].PersistentVolumeClaim.ControllerBusNumber = ptr.To(int32(0))
			})

			It("should not modify already populated fields", func() {
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber).To(HaveValue(Equal(int32(0))))
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
				Expect(vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).To(HaveValue(Equal(int32(0))))
			})
		})

		When("VM has PVC volumes needing backfill", func() {
			It("should backfill all placement info", func() {
				assertPVCPlacementPopulated(vm.Spec.Volumes[0].PersistentVolumeClaim, 0, 0, vmopv1.VirtualControllerTypeSCSI)
				assertPVCPlacementPopulated(vm.Spec.Volumes[1].PersistentVolumeClaim, 1, 0, vmopv1.VirtualControllerTypeSCSI)
			})
		})

		When("VM has PVC volume without CNS attachment", func() {
			BeforeEach(func() {
				setupK8sClientWithAttachments(attachment1)
			})

			It("should only backfill volumes with attachments", func() {
				assertPVCPlacementPopulated(vm.Spec.Volumes[0].PersistentVolumeClaim, 0, 0, vmopv1.VirtualControllerTypeSCSI)
				assertPVCPlacementEmpty(vm.Spec.Volumes[1].PersistentVolumeClaim)
			})
		})

		When("CNS attachment is not attached", func() {
			BeforeEach(func() {
				unattachedAttachment := attachment1.DeepCopy()
				unattachedAttachment.Status.Attached = false

				setupK8sClientWithAttachments(unattachedAttachment, attachment2)
			})

			It("should backfill volumes even when not attached", func() {
				// The implementation ignores Attached status since we are checking
				// directly against the attached virtual devices.
				assertPVCPlacementPopulated(vm.Spec.Volumes[0].PersistentVolumeClaim, 0, 0, vmopv1.VirtualControllerTypeSCSI)
			})
		})

		When("CNS attachment has no disk UUID", func() {
			BeforeEach(func() {
				noDiskUUIDAttachment := attachment1.DeepCopy()
				noDiskUUIDAttachment.Status.AttachmentMetadata = map[string]string{}

				setupK8sClientWithAttachments(noDiskUUIDAttachment, attachment2)
			})

			It("should not backfill volumes without disk UUID", func() {
				assertPVCPlacementEmpty(vm.Spec.Volumes[0].PersistentVolumeClaim)
			})
		})

		When("disk UUID is not found in VM hardware", func() {
			BeforeEach(func() {
				diskDevice1.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo).Uuid = "different-uuid"
			})

			It("should not backfill volumes with non-matching disk UUID", func() {
				assertPVCPlacementEmpty(vm.Spec.Volumes[0].PersistentVolumeClaim)
			})
		})

		When("controller is not found in status", func() {
			BeforeEach(func() {
				vm.Status.Hardware.Controllers = []vmopv1.VirtualControllerStatus{}
			})

			It("should not backfill volumes without controller info", func() {
				assertPVCPlacementEmpty(vm.Spec.Volumes[0].PersistentVolumeClaim)
			})
		})

		// These tests verify that when a single placement field is already set but
		// conflicts with the vSphere VM hardware, no backfilling occurs. This ensures
		// atomic backfilling - we don't partially update fields when conflicts exist.
		When("PVC volume has mismatched unit number", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber = ptr.To(int32(testMismatchValue))
			})

			It("should not backfill due to mismatch", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.UnitNumber).To(HaveValue(Equal(int32(testMismatchValue))))
				Expect(pvc1.ControllerType).To(BeEmpty())
				Expect(pvc1.ControllerBusNumber).To(BeNil())
			})
		})

		When("PVC volume has mismatched controller bus number", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber = ptr.To(int32(testMismatchValue))
			})

			It("should not backfill due to mismatch", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.ControllerBusNumber).To(HaveValue(Equal(int32(testMismatchValue))))
				Expect(pvc1.UnitNumber).To(BeNil())
				Expect(pvc1.ControllerType).To(BeEmpty())
			})
		})

		When("PVC volume has mismatched controller type", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeIDE
			})

			It("should not backfill due to mismatch", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(pvc1.UnitNumber).To(BeNil())
				Expect(pvc1.ControllerBusNumber).To(BeNil())
			})
		})

		// These tests verify mixed conflict scenarios where some fields match the
		// vSphere VM hardware but others don't. The atomic backfill logic should
		// reject the backfill entirely when any field conflicts, even if others match.
		When("PVC volume has mixed conflicts - UnitNumber matches but ControllerType is different", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber = ptr.To(int32(0))
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeNVME
			})

			It("should not backfill due to controller type mismatch", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.UnitNumber).To(HaveValue(Equal(int32(0))))
				Expect(pvc1.ControllerType).To(Equal(vmopv1.VirtualControllerTypeNVME))
				Expect(pvc1.ControllerBusNumber).To(BeNil())
			})
		})

		When("PVC volume has mixed conflicts - ControllerType matches but UnitNumber is different", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber = ptr.To(int32(testMismatchValue))
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
			})

			It("should not backfill due to unit number mismatch", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.UnitNumber).To(HaveValue(Equal(int32(testMismatchValue))))
				Expect(pvc1.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
				Expect(pvc1.ControllerBusNumber).To(BeNil())
			})
		})

		When("PVC volume has mixed conflicts - ControllerBusNumber matches but ControllerType is different", func() {
			BeforeEach(func() {
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber = ptr.To(int32(0))
				vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeNVME
			})

			It("should not backfill due to controller type mismatch", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.ControllerBusNumber).To(HaveValue(Equal(int32(0))))
				Expect(pvc1.ControllerType).To(Equal(vmopv1.VirtualControllerTypeNVME))
				Expect(pvc1.UnitNumber).To(BeNil())
			})
		})

		// These tests verify partial backfill scenarios where one field is already
		// set with a value that matches the vSphere VM hardware. The remaining empty
		// fields should be backfilled since there are no conflicts.
		When("PVC volume has partial backfill with matching values", func() {
			When("only UnitNumber is set", func() {
				BeforeEach(func() {
					vm.Spec.Volumes[0].PersistentVolumeClaim.UnitNumber = ptr.To(int32(0))
				})

				It("should backfill remaining fields when unit number matches", func() {
					assertPVCPlacementPopulated(vm.Spec.Volumes[0].PersistentVolumeClaim, 0, 0, vmopv1.VirtualControllerTypeSCSI)
				})
			})

			When("only ControllerType is set", func() {
				BeforeEach(func() {
					vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
				})

				It("should backfill remaining fields when controller type matches", func() {
					assertPVCPlacementPopulated(vm.Spec.Volumes[0].PersistentVolumeClaim, 0, 0, vmopv1.VirtualControllerTypeSCSI)
				})
			})

			When("only ControllerBusNumber is set", func() {
				BeforeEach(func() {
					vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber = ptr.To(int32(0))
				})

				It("should backfill remaining fields when bus number matches", func() {
					assertPVCPlacementPopulated(vm.Spec.Volumes[0].PersistentVolumeClaim, 0, 0, vmopv1.VirtualControllerTypeSCSI)
				})
			})
		})

		When("VM has non-PVC volumes", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name:                       "non-pvc-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{},
				})
			})

			It("should skip non-PVC volumes", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.UnitNumber).ToNot(BeNil())
				Expect(vm.Spec.Volumes[2].PersistentVolumeClaim).To(BeNil())
			})
		})

		When("VM has volumes with UnmanagedVolumeClaim", func() {
			BeforeEach(func() {
				vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "unmanaged-volume",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{},
						},
					},
				})
			})

			It("should skip volumes with UnmanagedVolumeClaim", func() {
				pvc1 := vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc1.UnitNumber).ToNot(BeNil())

				unmanagedVolume := vm.Spec.Volumes[2].PersistentVolumeClaim
				Expect(unmanagedVolume.UnmanagedVolumeClaim).ToNot(BeNil())
				Expect(unmanagedVolume.UnitNumber).To(BeNil())
				Expect(unmanagedVolume.ControllerType).To(BeEmpty())
				Expect(unmanagedVolume.ControllerBusNumber).To(BeNil())
			})
		})

		// This test verifies that volumes attached to different controllers of the
		// same type are correctly backfilled with their respective bus numbers. This
		// ensures the backfill logic can distinguish between multiple SCSI controllers.
		Context("Multiple controllers of same type", func() {
			const (
				testControllerKey0 = 1000
				testControllerKey1 = 1001
			)

			var (
				scsiCtrl0 *vimtypes.ParaVirtualSCSIController
				scsiCtrl1 *vimtypes.ParaVirtualSCSIController
			)

			BeforeEach(func() {
				scsiCtrl0 = builder.DummySCSIController(testControllerKey0, 0)
				scsiCtrl1 = builder.DummySCSIController(testControllerKey1, 1)

				diskDevice1.ControllerKey = testControllerKey0
				diskDevice2.ControllerKey = testControllerKey1
				diskDevice2.UnitNumber = ptr.To(int32(0))

				vm.Status.Hardware.Controllers = []vmopv1.VirtualControllerStatus{
					createControllerStatus(testControllerKey0, 0, vmopv1.VirtualControllerTypeSCSI),
					createControllerStatus(testControllerKey1, 1, vmopv1.VirtualControllerTypeSCSI),
				}

				moVM.Config.Hardware.Device = []vimtypes.BaseVirtualDevice{
					scsiCtrl0,
					scsiCtrl1,
					diskDevice1,
					diskDevice2,
				}
			})

			When("VM has disks on multiple SCSI controller buses", func() {
				It("should backfill with correct bus numbers for each disk", func() {
					By("Verifying first volume is on controller bus 0")
					assertPVCPlacementPopulated(vm.Spec.Volumes[0].PersistentVolumeClaim, 0, 0, vmopv1.VirtualControllerTypeSCSI)

					By("Verifying second volume is on controller bus 1")
					assertPVCPlacementPopulated(vm.Spec.Volumes[1].PersistentVolumeClaim, 0, 1, vmopv1.VirtualControllerTypeSCSI)
				})
			})
		})
	})
})
