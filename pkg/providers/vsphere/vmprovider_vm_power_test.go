// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha6/cloudinit"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func vmPowerStateTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
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

		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(
			ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMI1 := &vmopv1.ClusterVirtualMachineImage{}

		if testConfig.WithContentLibrary {
			Expect(ctx.Client.Get(
				ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
				clusterVMI1)).To(Succeed())
		} else {
			vsphere.SkipVMImageCLProviderCheck = true
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
	})

	AfterEach(func() {
		vsphere.SkipVMImageCLProviderCheck = false

		if vm != nil &&
			!pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
			By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
				Expect(vm.Status.Crypto).To(BeNil())
			})
		}

		vmClass = nil
		vm = nil

		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

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
}
