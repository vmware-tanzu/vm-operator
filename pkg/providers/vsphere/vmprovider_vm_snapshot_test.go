// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func vmSnapshotTests() {
	Describe("VCSim tests", func() {
		var (
			initObjects   []ctrlclient.Object
			ctx           *builder.TestContextForVCSim
			vmProvider    providers.VirtualMachineProviderInterface
			nsInfo        builder.WorkloadNamespaceInfo
			vmSnapshot    *vmopv1.VirtualMachineSnapshot
			vcVM          *object.VirtualMachine
			vm            *vmopv1.VirtualMachine
			vmCtx         pkgctx.VirtualMachineContext
			deleted       bool
			err           error
			dummySnapshot = "dummy-snapshot"
		)

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{}, initObjects...)
			vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
			nsInfo = ctx.CreateWorkloadNamespace()

			By("Creating VM")
			vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
			Expect(err).ToNot(HaveOccurred())
			Expect(vcVM).ToNot(BeNil())
			vm = builder.DummyBasicVirtualMachine(dummySnapshot, nsInfo.Namespace)
			vm.Status.UniqueID = vcVM.Reference().Value
			vmSnapshot = builder.DummyVirtualMachineSnapshot(nsInfo.Namespace, dummySnapshot, vcVM.Name())
			Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

			// TODO (lubron): Add FCD to the VM and test the snapshot size once
			// vcsim has support to show attached disk as device

			By("Creating snapshot")
			logger := testutil.GinkgoLogr(5)
			vmCtx = pkgctx.VirtualMachineContext{
				Context: logr.NewContext(ctx, logger),
				Logger:  logger.WithValues("vmName", vcVM.Name()),
				VM:      vm,
			}
			args := virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: *vmSnapshot,
				VcVM:       vcVM,
			}
			snapMo, err := virtualmachine.CreateSnapshot(args)
			Expect(err).To(BeNil())
			Expect(snapMo).ToNot(BeNil())
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			initObjects = nil
			vmProvider = nil
			vmSnapshot = nil
			vmCtx = pkgctx.VirtualMachineContext{}
			vm = nil
			nsInfo = builder.WorkloadNamespaceInfo{}
			deleted = false
		})

		Context("GetSnapshotSize", func() {
			It("should return the size of the snapshot", func() {
				size, err := vmProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
				Expect(err).ToNot(HaveOccurred())
				// since we only have one snapshot, the size should be same as the vm
				var moVM mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot", "layoutEx", "config.hardware.device"}, &moVM)).To(Succeed())

				var sum int64
				for _, file := range moVM.LayoutEx.File {
					if strings.HasSuffix(file.Name, ".vmdk") || strings.HasSuffix(file.Name, ".vmsn") || strings.HasSuffix(file.Name, ".vmem") {
						sum += file.Size
					}
				}
				Expect(size).To(Equal(sum))
			})

			When("there is issue finding vm", func() {
				BeforeEach(func() {
					vm.Status.UniqueID = ""
				})
				It("should return error", func() {
					size, err := vmProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
					Expect(err).To(HaveOccurred())
					Expect(size).To(BeZero())
				})
			})

			When("there is issue finding snapshot", func() {
				BeforeEach(func() {
					vmSnapshot.Name = ""
				})
				It("should return error", func() {
					size, err := vmProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
					Expect(err).To(HaveOccurred())
					Expect(size).To(BeZero())
				})
			})
		})

		Context("GetParentSnapshot", func() {
			var childSnapshot *vmopv1.VirtualMachineSnapshot

			BeforeEach(func() {
				By("Creating snapshot")
				childSnapshot = builder.DummyVirtualMachineSnapshot(nsInfo.Namespace, "child-snapshot", vcVM.Name())
				args := virtualmachine.SnapshotArgs{
					VMCtx:      vmCtx,
					VMSnapshot: *childSnapshot,
					VcVM:       vcVM,
				}
				snapMo, err := virtualmachine.CreateSnapshot(args)
				Expect(err).To(BeNil())
				Expect(snapMo).ToNot(BeNil())
			})

			It("should return the parent snapshot of the child snapshot", func() {
				parent, err := vmProvider.GetParentSnapshot(ctx, childSnapshot.Name, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(parent).ToNot(BeNil())
				Expect(parent.Name).To(Equal(vmSnapshot.Name))
			})

			When("there is issue finding vm", func() {
				BeforeEach(func() {
					vm.Status.UniqueID = ""
				})
				It("should return error", func() {
					parent, err := vmProvider.GetParentSnapshot(ctx, childSnapshot.Name, vm)
					Expect(err).To(HaveOccurred())
					Expect(parent).To(BeNil())
				})
			})

			When("there is no parent snapshot", func() {
				It("should return error", func() {
					parent, err := vmProvider.GetParentSnapshot(ctx, vmSnapshot.Name, vm)
					Expect(err).ToNot(HaveOccurred())
					Expect(parent).To(BeNil())
				})
			})

			When("snapshot doesn't exist", func() {
				BeforeEach(func() {
					childSnapshot.Name = ""
				})
				It("should return nil", func() {
					parent, err := vmProvider.GetParentSnapshot(ctx, childSnapshot.Name, vm)
					Expect(err).ToNot(HaveOccurred())
					Expect(parent).To(BeNil())
				})
			})
		})

		Context("DeleteSnapshot", func() {
			JustBeforeEach(func() {
				deleted, err = vmProvider.DeleteSnapshot(ctx, vmSnapshot, vm, true, nil)
			})

			It("should return false and no error", func() {
				Expect(deleted).To(BeFalse())
				Expect(err).NotTo(HaveOccurred())
				snapMoRef, err := vcVM.FindSnapshot(ctx, dummySnapshot)
				Expect(err).To(HaveOccurred())
				Expect(snapMoRef).To(BeNil())
			})

			Context("VM is not found", func() {
				BeforeEach(func() {
					vm.Status.UniqueID = ""
				})
				It("should return true and no error", func() {
					Expect(deleted).To(BeTrue())
					Expect(err).NotTo(HaveOccurred())
					snapMoRef, err := vcVM.FindSnapshot(ctx, dummySnapshot)
					Expect(err).NotTo(HaveOccurred())
					Expect(snapMoRef).NotTo(BeNil())
				})
			})

			Context("snapshot not found", func() {
				BeforeEach(func() {
					By("Deleting snapshot in advance")
					Expect(virtualmachine.DeleteSnapshot(virtualmachine.SnapshotArgs{
						VMCtx:      vmCtx,
						VMSnapshot: *vmSnapshot,
						VcVM:       vcVM,
					})).To(Succeed())
				})
				It("should return false and no error", func() {
					Expect(deleted).To(BeFalse())
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("SyncVMSnapshotTreeStatus", func() {
			It("should sync the VM's current and root snapshots status", func() {
				Expect(vmProvider.SyncVMSnapshotTreeStatus(ctx, vm)).To(Succeed())
				Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
				Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))
				Expect(vm.Status.RootSnapshots).To(HaveLen(1))
				Expect(vm.Status.RootSnapshots[0].Name).To(Equal(vmSnapshot.Name))
			})

			When("VM is not found", func() {
				BeforeEach(func() {
					vm.Status.UniqueID = ""
				})
				It("should return error", func() {
					Expect(vmProvider.SyncVMSnapshotTreeStatus(ctx, vm)).To(HaveOccurred())
				})
			})

			When("there is no snapshot", func() {
				BeforeEach(func() {
					Expect(virtualmachine.DeleteSnapshot(virtualmachine.SnapshotArgs{
						VMCtx:      vmCtx,
						VMSnapshot: *vmSnapshot,
						VcVM:       vcVM,
					})).To(Succeed())
				})
				It("should show expected current snapshot and root snapshots", func() {
					Expect(vmProvider.SyncVMSnapshotTreeStatus(ctx, vm)).To(Succeed())
					Expect(vm.Status.CurrentSnapshot).To(BeNil())
					Expect(vm.Status.RootSnapshots).To(BeNil())
				})
			})
		})
	})

	Describe("K8s tests", func() {
		var (
			k8sClient   ctrlclient.Client
			initObjects []ctrlclient.Object

			vmCtx pkgctx.VirtualMachineContext
		)

		BeforeEach(func() {
			vm := builder.DummyBasicVirtualMachine("test-vm", "dummy-ns")

			vmCtx = pkgctx.VirtualMachineContext{
				Context: pkgcfg.WithConfig(pkgcfg.Config{MaxDeployThreadsOnProvider: 16}),
				Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
				VM:      vm,
			}
		})

		JustBeforeEach(func() {
			k8sClient = builder.NewFakeClient(initObjects...)
		})

		AfterEach(func() {
			k8sClient = nil
			initObjects = nil
		})

		Context("PatchSnapshotStatus", func() {
			var (
				vmSnapshot *vmopv1.VirtualMachineSnapshot
				snapMoRef  *vimtypes.ManagedObjectReference
			)

			BeforeEach(func() {
				timeout, _ := time.ParseDuration("1h35m")
				vmSnapshot = &vmopv1.VirtualMachineSnapshot{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "vmoperator.vmware.com/v1alpha5",
						Kind:       "VirtualMachineSnapshot",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "snap-1",
						Namespace: vmCtx.VM.Namespace,
					},
					Spec: vmopv1.VirtualMachineSnapshotSpec{
						VMRef: &common.LocalObjectRef{
							APIVersion: vmCtx.VM.APIVersion,
							Kind:       vmCtx.VM.Kind,
							Name:       vmCtx.VM.Name,
						},
						Quiesce: &vmopv1.QuiesceSpec{
							Timeout: &metav1.Duration{Duration: timeout},
						},
					},
				}
			})

			When("snapshot patched with vm info and ready condition", func() {
				BeforeEach(func() {
					vmCtx.VM.Status = vmopv1.VirtualMachineStatus{
						UniqueID:   "dummyID",
						PowerState: vmopv1.VirtualMachinePowerStateOn,
					}

					snapMoRef = &vimtypes.ManagedObjectReference{
						Value: "snap-103",
					}

					initObjects = append(initObjects, vmSnapshot)
				})

				It("succeeds", func() {
					err := vsphere.PatchSnapshotSuccessStatus(vmCtx, k8sClient, vmSnapshot, snapMoRef)
					Expect(err).ToNot(HaveOccurred())

					snapObj := &vmopv1.VirtualMachineSnapshot{}
					Expect(k8sClient.Get(vmCtx, ctrlclient.ObjectKey{Name: vmSnapshot.Name, Namespace: vmSnapshot.Namespace}, snapObj)).To(Succeed())
					Expect(snapObj.Status.UniqueID).To(Equal(snapMoRef.Value))
					Expect(snapObj.Status.Quiesced).To(BeTrue())
					Expect(snapObj.Status.PowerState).To(Equal(vmCtx.VM.Status.PowerState))
					Expect(snapObj.Status.Conditions).To(HaveLen(1))
					Expect(snapObj.Status.Conditions[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReadyCondition))
					Expect(snapObj.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
				})
			})
		})

	})
}
