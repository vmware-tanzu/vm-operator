// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package diskpromo_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/diskpromo"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", Label(testlabels.V1Alpha4), func() {
	It("should return a reconciler", func() {
		Expect(diskpromo.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", Label(testlabels.V1Alpha4), func() {
	It("should return diskpromo", func() {
		Expect(diskpromo.New().Name()).To(Equal("diskpromo"))
	})
})

var _ = Describe("Reconcile", Label(testlabels.V1Alpha4), func() {
	var (
		r          vmconfig.Reconciler
		ctx        context.Context
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		moVM       mo.VirtualMachine
		vcVM       *object.VirtualMachine
		vm         *vmopv1.VirtualMachine
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		configSpec *vimtypes.VirtualMachineConfigSpec

		moVMProps = []string{
			"config",
			"guest",
			"layoutEx",
			"recentTask",
			"runtime",
			"summary",
		}
	)

	BeforeEach(func() {
		r = diskpromo.New()

		vcsimCtx := builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()), builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = vmconfig.WithContext(ctx)
		ctx = logr.NewContext(
			ctx,
			textlogger.NewLogger(textlogger.NewConfig(
				textlogger.Verbosity(5),
				textlogger.Output(GinkgoWriter),
			)))
		vimClient = vcsimCtx.VCClient.Client

		var err error
		vcVM, err = vcsimCtx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).NotTo(HaveOccurred())

		dskName := "[LocalDS_0] base-" + vcVM.Name() + ".vmdk"
		dskMgr := object.NewVirtualDiskManager(vimClient)
		task, err := dskMgr.CreateVirtualDisk(ctx, dskName, vcsimCtx.Datacenter, &vimtypes.FileBackedVirtualDiskSpec{
			CapacityKb: 10 * 1024,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(task.Wait(ctx)).NotTo(HaveOccurred())

		device, err := vcVM.Device(ctx)
		Expect(err).NotTo(HaveOccurred())

		controller, err := device.FindDiskController("")
		Expect(err).NotTo(HaveOccurred())

		disk := device.CreateDisk(controller, vimtypes.ManagedObjectReference{}, dskName)
		disk = device.ChildDisk(disk)
		err = vcVM.AddDevice(ctx, disk)
		Expect(err).NotTo(HaveOccurred())

		Expect(vcVM.Properties(ctx, vcVM.Reference(), moVMProps, &moVM)).To(Succeed())
		moVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
		moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn

		// moVM = mo.VirtualMachine{
		// 	Config: &vimtypes.VirtualMachineConfigInfo{
		// 		Hardware: vimtypes.VirtualHardware{
		// 			Device: device,
		// 		},
		// 	},
		// 	Runtime: vimtypes.VirtualMachineRuntimeInfo{
		// 		PowerState: vimtypes.VirtualMachinePowerStatePoweredOn,
		// 	},
		// 	Summary: vimtypes.VirtualMachineSummary{
		// 		Runtime: vimtypes.VirtualMachineRuntimeInfo{
		// 			PowerState: vimtypes.VirtualMachinePowerStatePoweredOn,
		// 		},
		// 	},
		// }

		// moVM.Self = vcVM.Reference()

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{},
			Status: vmopv1.VirtualMachineStatus{
				UniqueID: moVM.Self.Value,
			},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{vm}
	})
	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	When("it should panic", func() {
		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
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
			BeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})

	})

	getPromoTaskRef := func() *vimtypes.ManagedObjectReference {
		ExpectWithOffset(1, vcVM.Properties(ctx, vcVM.Reference(), moVMProps, &moVM)).To(Succeed())
		pc := property.DefaultCollector(vimClient)
		for i := range moVM.RecentTask {
			taskRef := moVM.RecentTask[i]
			var moT mo.Task
			ExpectWithOffset(1, pc.RetrieveOne(ctx, taskRef, []string{"info"}, &moT)).To(Succeed())
			if moT.Info.DescriptionId == diskpromo.PromoteDisksTaskKey {
				return &taskRef
			}
		}
		return nil
	}

	getPromoTaskInfo := func(g Gomega, ref vimtypes.ManagedObjectReference) vimtypes.TaskInfo {
		pc := property.DefaultCollector(vimClient)
		var moT mo.Task
		g.ExpectWithOffset(1, pc.RetrieveOne(ctx, ref, []string{"info"}, &moT)).To(Succeed())
		return moT.Info
	}

	When("it should not panic", func() {
		var (
			err error
		)

		JustBeforeEach(func() {
			err = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
		})

		When("creating a new vm", func() {

			When("spec.powerState is PoweredOn", func() {
				When("promoteDisksMode is Disabled", func() {
					BeforeEach(func() {
						vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeDisabled
					})
					It("should not promote disks", func() {
						Expect(err).ToNot(HaveOccurred())
						c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
						Expect(c).To(BeNil())

						childDisks := 0

						device, err := vcVM.Device(ctx)
						Expect(err).NotTo(HaveOccurred())

						for _, d := range device.SelectByType(&vimtypes.VirtualDisk{}) {
							disk := d.(*vimtypes.VirtualDisk)
							b, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
							Expect(ok).To(BeTrue())
							if b.Parent != nil {
								childDisks++
							}
						}

						Expect(childDisks).To(Equal(1))
					})
				})

				When("promoteDisksMode is Offline", func() {
					BeforeEach(func() {
						vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOffline
					})
					It("should not promote disks", func() {
						Expect(err).ToNot(HaveOccurred())
						c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(diskpromo.ReasonPending))
					})
				})

				When("promoteDiskMode is Online", func() {
					BeforeEach(func() {
						vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
					})

					It("should promote disks", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(Equal(diskpromo.ErrPromoteDisks))

						promoRef := getPromoTaskRef()
						Expect(promoRef).ToNot(BeNil())

						// Simulator promoteDisks will delay the task based on
						// disk capacity, so we need > 1 Reconcile here.
						Eventually(func(g Gomega) {
							g.Expect(r.Reconcile(
								pkgctx.WithVMRecentTasks(
									ctx, []vimtypes.TaskInfo{
										getPromoTaskInfo(g, *promoRef),
									}),
								k8sClient,
								vimClient,
								vm,
								moVM,
								configSpec)).ToNot(HaveOccurred())
							c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
							g.Expect(c).ToNot(BeNil())
							g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
						}).Should(Succeed(), "waiting for promoteDisks task to complete")

						// VM should not have any child disks after disk promo.
						device, err := vcVM.Device(ctx)
						Expect(err).NotTo(HaveOccurred())
						for _, d := range device.SelectByType(&vimtypes.VirtualDisk{}) {
							disk := d.(*vimtypes.VirtualDisk)
							b, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
							Expect(ok).To(BeTrue())
							Expect(b.Parent).To(BeNil())
						}

						// Expect no promote when no child disks.
						Expect(vcVM.Properties(ctx, vcVM.Reference(), moVMProps, &moVM)).To(Succeed())
						moVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
						moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
						Expect(r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)).To(Succeed())
					})

					When("VM has a snapshot after disks were promoted", func() {
						It("should not mark the disk promotion synced as false since disks with snapshots are ignored", func() {
							Expect(err).To(HaveOccurred())
							Expect(err).To(Equal(diskpromo.ErrPromoteDisks))

							promoRef := getPromoTaskRef()
							Expect(promoRef).ToNot(BeNil())

							// Simulator promoteDisks will delay the task based on
							// disk capacity, so we need > 1 Reconcile here.
							Eventually(func(g Gomega) {
								g.Expect(r.Reconcile(
									pkgctx.WithVMRecentTasks(
										ctx, []vimtypes.TaskInfo{
											getPromoTaskInfo(g, *promoRef),
										}),
									k8sClient,
									vimClient,
									vm,
									moVM,
									configSpec)).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
								g.Expect(c).ToNot(BeNil())
								g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
							}).Should(Succeed(), "waiting for promoteDisks task to complete")

							// VM should not have any child disks after disk
							// promotion.
							device, err := vcVM.Device(ctx)
							Expect(err).NotTo(HaveOccurred())
							for _, d := range device.SelectByType(&vimtypes.VirtualDisk{}) {
								disk := d.(*vimtypes.VirtualDisk)
								b, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
								Expect(ok).To(BeTrue())
								Expect(b.Parent).To(BeNil())
							}

							// Take a VM snapshot.
							t, err := vcVM.CreateSnapshot(ctx, "root", "", false, false)
							Expect(err).ToNot(HaveOccurred())
							Expect(t.Wait(ctx)).To(Succeed())

							Expect(vcVM.Properties(ctx, vcVM.Reference(), moVMProps, &moVM)).To(Succeed())
							moVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
							moVM.Summary.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn

							// vC Sim does not properly add parent backing to
							// disks post-snapshot, so do that manually.
							for i := range moVM.Config.Hardware.Device {
								if d, ok := moVM.Config.Hardware.Device[i].(*vimtypes.VirtualDisk); ok {
									if b, ok := d.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
										b.Parent = &vimtypes.VirtualDiskFlatVer2BackingInfo{}
									}
								}

							}

							// Expect no task when no child disks that are not
							// snapshots.
							err = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
							Expect(err).ToNot(HaveOccurred())

							// Expect the disk promotion condition to still be
							// true.
							Expect(conditions.IsTrue(
								vm,
								vmopv1.VirtualMachineDiskPromotionSynced)).To(BeTrue())
						})
					})

					Context("Guest customization", func() {
						BeforeEach(func() {
							if moVM.Guest == nil {
								moVM.Guest = &vimtypes.GuestInfo{}
							}
							if moVM.Guest.CustomizationInfo == nil {
								moVM.Guest.CustomizationInfo = &vimtypes.GuestInfoCustomizationInfo{}
							}
						})
						When("Pending", func() {
							BeforeEach(func() {
								moVM.Guest.CustomizationInfo.CustomizationStatus = string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING)
							})
							It("should not promote the disks", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal(diskpromo.ReasonPending))
								Expect(c.Message).To(Equal("Pending guest customization"))
							})
						})

						When("Running", func() {
							BeforeEach(func() {
								moVM.Guest.CustomizationInfo.CustomizationStatus = string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING)
							})
							It("should not promote the disks", func() {
								Expect(err).ToNot(HaveOccurred())
								c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
								Expect(c).ToNot(BeNil())
								Expect(c.Status).To(Equal(metav1.ConditionFalse))
								Expect(c.Reason).To(Equal(diskpromo.ReasonPending))
								Expect(c.Message).To(Equal("Pending guest customization"))
							})
						})

						When("Not pending or running", func() {
							BeforeEach(func() {
								moVM.Guest.CustomizationInfo.CustomizationStatus = "fake"
							})
							It("should promote the disks", func() {
								Expect(err).To(HaveOccurred())
								Expect(err).To(Equal(diskpromo.ErrPromoteDisks))

								promoRef := getPromoTaskRef()
								Expect(promoRef).ToNot(BeNil())

								// Simulator promoteDisks will delay the task based on
								// disk capacity, so we need > 1 Reconcile here.
								Eventually(func(g Gomega) {
									g.Expect(r.Reconcile(
										pkgctx.WithVMRecentTasks(
											ctx, []vimtypes.TaskInfo{
												getPromoTaskInfo(g, *promoRef),
											}),
										k8sClient,
										vimClient,
										vm,
										moVM,
										configSpec)).ToNot(HaveOccurred())
									c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
									g.Expect(c).ToNot(BeNil())
									g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
								}).Should(Succeed(), "waiting for promoteDisks task to complete")
							})
						})
					})
				})

				When("vm is not created", func() {
					BeforeEach(func() {
						moVM.Config = nil
					})
					It("should not fail", func() {
						Expect(err).ToNot(HaveOccurred())
						c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
						Expect(c).To(BeNil())
					})
				})

				When("there is already a task running", func() {
					BeforeEach(func() {
						ctx = pkgctx.WithVMRecentTasks(ctx, []vimtypes.TaskInfo{
							{
								State:         vimtypes.TaskInfoStateRunning,
								DescriptionId: "fake.task.1",
							},
						})
					})
					It("should not fail", func() {
						Expect(err).ToNot(HaveOccurred())
						c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(diskpromo.ReasonPending))
						Expect(c.Message).To(Equal("Cannot promote disks when VM has running task"))
					})
				})

				When("vm has snapshots", func() {
					BeforeEach(func() {
						vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
						moVM.Snapshot = getSnapshotInfoWithLinearChain()
					})
					It("should not sync", func() {
						Expect(err).ToNot(HaveOccurred())
						c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(diskpromo.ReasonPending))
					})
				})

				When("invalid VirtualDisk has invalid Key", func() {
					BeforeEach(func() {
						vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline

						device := object.VirtualDeviceList(moVM.Config.Hardware.Device)
						for _, d := range device.SelectByType(&vimtypes.VirtualDisk{}) {
							disk := d.(*vimtypes.VirtualDisk)
							if b, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
								if b.Parent != nil {
									d.GetVirtualDevice().Key = -d.GetVirtualDevice().Key
								}
							}
						}
					})

					It("should fail the task", func() {
						Expect(err).To(HaveOccurred())
						Expect(err).To(Equal(diskpromo.ErrPromoteDisks))

						promoRef := getPromoTaskRef()
						Expect(promoRef).ToNot(BeNil())

						err = r.Reconcile(
							pkgctx.WithVMRecentTasks(
								ctx, []vimtypes.TaskInfo{
									getPromoTaskInfo(Default, *promoRef),
								}),
							k8sClient,
							vimClient,
							vm,
							moVM,
							configSpec)
						Expect(err).ToNot(HaveOccurred())
						c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
						Expect(c).ToNot(BeNil())
						Expect(c.Status).To(Equal(metav1.ConditionFalse))
						Expect(c.Reason).To(Equal(diskpromo.ReasonTaskError))
					})
				})
			})
		})

		When("spec.powerState is PoweredOn", func() {
			BeforeEach(func() {
				moVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOff
			})
			When("promoteDiskMode is Online", func() {
				BeforeEach(func() {
					vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
				})

				It("should not sync", func() {
					Expect(err).ToNot(HaveOccurred())

					c := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
					Expect(c).ToNot(BeNil())
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal(diskpromo.ReasonPending))
				})
			})
		})
	})
})

func getSnapshotInfoWithLinearChain() *vimtypes.VirtualMachineSnapshotInfo {
	return &vimtypes.VirtualMachineSnapshotInfo{
		CurrentSnapshot: &vimtypes.ManagedObjectReference{},
		RootSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
			{
				Name: "1",
				ChildSnapshotList: []vimtypes.VirtualMachineSnapshotTree{
					{
						Name: "1a",
					},
				},
			},
		},
	}
}
