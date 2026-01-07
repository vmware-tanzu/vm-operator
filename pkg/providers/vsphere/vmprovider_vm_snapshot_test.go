// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vmconfunmanagedvolsfil "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/backfill"
	vmconfunmanagedvolsreg "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/register"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

func vmSnapshotTests() {
	const (
		dummySnapshot = "dummy-snapshot"
	)

	var (
		initObjects []ctrlclient.Object
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo
		vmSnapshot  *vmopv1.VirtualMachineSnapshot
		vcVM        *object.VirtualMachine
		vm          *vmopv1.VirtualMachine
		vmCtx       pkgctx.VirtualMachineContext
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{}, initObjects...)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())
		Expect(vcVM).ToNot(BeNil())

		By("Creating VM CR")
		vm = builder.DummyBasicVirtualMachine(dummySnapshot, nsInfo.Namespace)
		vm.Status.UniqueID = vcVM.Reference().Value
		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		By("Creating snapshot CR")
		vmSnapshot = builder.DummyVirtualMachineSnapshot(nsInfo.Namespace, dummySnapshot, vcVM.Name())
		Expect(ctx.Client.Create(ctx, vmSnapshot)).To(Succeed())

		// TODO (lubron): Add FCD to the VM and test the snapshot size once
		// vcsim has support to show attached disk as device

		By("Creating snapshot on vSphere")
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
		Expect(err).ToNot(HaveOccurred())
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
	})

	Context("GetSnapshotSize", func() {
		It("should return the size of the snapshot", func() {
			size, err := vmProvider.GetSnapshotSize(ctx, vmSnapshot.Name, vm)
			Expect(err).ToNot(HaveOccurred())

			// Since we only have one snapshot, the size should be same as the vm
			var moVM mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"layoutEx"}, &moVM)).To(Succeed())
			var total int64
			for _, file := range moVM.LayoutEx.File {
				switch filepath.Ext(file.Name) {
				case ".vmdk", ".vmsn", ".vmem":
					total += file.Size
				}
			}
			Expect(size).To(Equal(total))
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

	Context("DeleteSnapshot", func() {
		var (
			deleted bool
			err     error
		)

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
			Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))
			Expect(vm.Status.RootSnapshots).To(HaveLen(1))
			Expect(vm.Status.RootSnapshots[0].Name).To(Equal(vmSnapshot.Name))
			Expect(vm.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
		})

		When("VM is not found", func() {
			BeforeEach(func() {
				vm.Status.UniqueID = ""
			})
			It("should return error", func() {
				Expect(vmProvider.SyncVMSnapshotTreeStatus(ctx, vm)).NotTo(Succeed())
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

	Context("ReconcileCurrentSnapshot", func() {
		var (
			snapshot1 *vmopv1.VirtualMachineSnapshot
			snapshot2 *vmopv1.VirtualMachineSnapshot

			verifyK8sVMSnapshot = func(name, namespace string, isCreated bool) {
				GinkgoHelper()
				vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{
					Name:      name,
					Namespace: namespace,
				}, vmSnapshot)).To(Succeed())
				Expect(conditions.IsTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotCreatedCondition)).To(Equal(isCreated))
			}

			verifyNoVcVMSnapshot = func() {
				GinkgoHelper()
				var moVM mo.VirtualMachine
				Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVM)).To(Succeed())
				Expect(moVM.Snapshot).To(BeNil())
			}
		)

		BeforeEach(func() {
			By("Deleting the snapshot on vSphere created in outer BeforeEach")
			Expect(virtualmachine.DeleteSnapshot(virtualmachine.SnapshotArgs{
				VMCtx:      vmCtx,
				VMSnapshot: *vmSnapshot,
				VcVM:       vcVM,
			})).To(Succeed())

			By("Deleting the snapshot CR created in outer BeforeEach")
			Expect(ctx.Client.Delete(ctx, vmSnapshot)).To(Succeed())
		})

		AfterEach(func() {
			snapshot1 = nil
			snapshot2 = nil
		})

		When("no snapshots exist", func() {
			It("should complete without error", func() {
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())
				verifyNoVcVMSnapshot()
			})
		})

		When("one snapshot exists and is not created", func() {
			JustBeforeEach(func() {
				// Create snapshot1 CR with owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())
			})

			It("should process the snapshot", func() {
				// Reconcile the current snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Verify snapshot is created.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, true)

				// Verify snapshot status.
				updatedSnapshot := &vmopv1.VirtualMachineSnapshot{}
				Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{
					Name:      snapshot1.Name,
					Namespace: snapshot1.Namespace,
				}, updatedSnapshot)).To(Succeed())
				Expect(updatedSnapshot.Status.Quiesced).To(BeTrue())
				// Snapshot should be powered off since memory is not included in the snapshot.
				Expect(updatedSnapshot.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))
			})
		})

		When("multiple snapshots exist", func() {
			It("should process snapshots in order (oldest first)", func() {
				// Create snapshot1 CR with owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				creationTimeStamp := metav1.NewTime(time.Now())
				snapshot1.CreationTimestamp = creationTimeStamp
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Create snapshot2 CR with a later time and owner reference set to the VM.
				later := metav1.NewTime(time.Now().Add(1 * time.Second))
				snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
				snapshot2.CreationTimestamp = later
				Expect(controllerutil.SetOwnerReference(vm, snapshot2, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

				// First reconcile should process snapshot1, and requeue to process snapshot2.
				err := vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)
				Expect(err).To(HaveOccurred())
				Expect(pkgerr.IsRequeueError(err)).To(BeTrue())
				Expect(err.Error()).To(ContainSubstring("requeuing to process 1 remaining snapshots"))

				// Check that snapshot1 is created.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, true)

				// Check that snapshot2 is NOT created.
				verifyK8sVMSnapshot(snapshot2.Name, snapshot2.Namespace, false)

				// Second reconcile should process snapshot2.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Check that snapshot2 is now created.
				verifyK8sVMSnapshot(snapshot2.Name, snapshot2.Namespace, true)

				// Note: The Children status is populated by SyncVMSnapshotTreeStatus,
				// not by ReconcileCurrentSnapshot, which is tested separately above.
			})
		})

		When("one snapshot is already in progress", func() {
			It("should process the in-progress snapshot and requeue for the next", func() {
				// Create snapshot1 CR with in progress condition and owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				conditions.MarkFalse(snapshot1,
					vmopv1.VirtualMachineSnapshotCreatedCondition,
					vmopv1.VirtualMachineSnapshotCreationInProgressReason,
					"in progress",
				)
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Create snapshot2 CR with owner reference set to the VM.
				snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot2, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

				// Reconcile the current snapshot and expect a requeue error.
				err := vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)
				Expect(err).To(HaveOccurred())
				Expect(pkgerr.IsRequeueError(err)).To(BeTrue())

				// First snapshot should be created.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, true)

				// Second snapshot should NOT be created.
				verifyK8sVMSnapshot(snapshot2.Name, snapshot2.Namespace, false)
			})
		})

		When("snapshot is being deleted", func() {
			It("should skip all snapshot creation due to vSphere constraint", func() {
				// Create snapshot1 CR with owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				// Set a finalizer so we can delete the snapshot CR without it being removed from cluster.
				snapshot1.ObjectMeta.Finalizers = []string{"dummy-finalizer"}
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Create snapshot2 CR with owner reference set to the VM.
				snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot2, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

				// Delete snapshot1 CR.
				Expect(ctx.Client.Delete(ctx, snapshot1)).To(Succeed())

				// Reconcile the current snapshot and expect a requeue error.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// snapshot1 should NOT be created.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, false)

				// snapshot2 should NOT be created.
				verifyK8sVMSnapshot(snapshot2.Name, snapshot2.Namespace, false)
			})
		})

		When("snapshot already exists and has created condition", func() {
			It("should skip ready snapshot and process the next one", func() {
				// Create snapshot1 CR with created condition and owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				conditions.MarkTrue(snapshot1, vmopv1.VirtualMachineSnapshotCreatedCondition)
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Create snapshot2 CR with owner reference set to the VM.
				snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot2, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

				// Reconcile the current snapshot and expect no error.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// snapshot1 should remain created.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, true)

				// snapshot2 should be processed and marked as created.
				verifyK8sVMSnapshot(snapshot2.Name, snapshot2.Namespace, true)
			})
		})

		When("snapshot has empty VM name", func() {
			It("should skip snapshot with empty VM name and process the next one", func() {
				// Create snapshot1 CR with spec.VMName set to empty and owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				snapshot1.Spec.VMName = ""
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Create snapshot2 with owner reference set to the VM.
				snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot2, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

				// Reconcile the current snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// snapshot1 should not be processed (empty VMName).
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, false)

				// snapshot2 should be processed and marked as created.
				verifyK8sVMSnapshot(snapshot2.Name, snapshot2.Namespace, true)
			})
		})

		When("snapshot references different VM", func() {
			It("should skip snapshot for different VM and process the next one", func() {
				// Create snapshot1 CR with spec.VMName set to a different VM name than owner reference VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				snapshot1.Spec.VMName = "different-vm"
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Create snapshot2 CR with owner reference set to the VM.
				snapshot2 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-2", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot2, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot2)).To(Succeed())

				// Reconcile the current snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// snapshot1 should not be processed (different VM).
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, false)

				// snapshot2 should be processed and marked as created.
				verifyK8sVMSnapshot(snapshot2.Name, snapshot2.Namespace, true)
			})
		})

		When("VM is a VKS/TKG node", func() {
			It("should skip snapshot processing for VKS/TKG nodes", func() {
				// Add CAPI labels to mark VM as VKS/TKG node.
				vm.Labels = map[string]string{
					kubeutil.CAPWClusterRoleLabelKey: "worker",
				}
				Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

				// Create snapshot1 CR with owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Reconcile the current snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Snapshot should not be processed.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, false)
				verifyNoVcVMSnapshot()
			})
		})

		When("disk promotion sync is enabled but not ready", func() {
			JustBeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.FastDeploy = true
				})
			})

			It("should create snapshot after disk promotion sync is ready", func() {
				// Set the VM's promote disks mode to not disabled and disk promotion sync condition to false.
				vm.Spec.PromoteDisksMode = vmopv1.VirtualMachinePromoteDisksModeOnline
				conditions.MarkFalse(vm, vmopv1.VirtualMachineDiskPromotionSynced, "", "")
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())

				// Create a snapshot CR with owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Reconcile the snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Snapshot should not be processed.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, false)
				verifyNoVcVMSnapshot()

				// Update the VM's VirtualMachineDiskPromotionSynced condition to true.
				conditions.MarkTrue(vm, vmopv1.VirtualMachineDiskPromotionSynced)
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())

				// Reconcile the snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Snapshot should be created.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, true)
			})
		})

		When("AllDisksArePVCs is enabled but disks are not registered", func() {
			JustBeforeEach(func() {
				pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
					config.Features.AllDisksArePVCs = true
				})
			})

			It("should create snapshot after disk registration is ready", func() {
				// Set the VM's disk backfill condition to false.
				conditions.MarkFalse(vm, vmconfunmanagedvolsfil.Condition, "", "")
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())

				// Create a snapshot CR with owner reference set to the VM.
				snapshot1 = builder.DummyVirtualMachineSnapshot(vm.Namespace, "snapshot-1", vm.Name)
				Expect(controllerutil.SetOwnerReference(vm, snapshot1, ctx.Scheme)).To(Succeed())
				Expect(ctx.Client.Create(ctx, snapshot1)).To(Succeed())

				// Reconcile the snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Snapshot should not be processed.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, false)
				verifyNoVcVMSnapshot()

				// Update the VM's disk backfill condition to true.
				conditions.MarkTrue(vm, vmconfunmanagedvolsfil.Condition)
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())

				// Reconcile the snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Snapshot should NOT be created (pending disk registration).
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, false)
				verifyNoVcVMSnapshot()

				// Update the VM's disk registration condition to true.
				conditions.MarkTrue(vm, vmconfunmanagedvolsreg.Condition)
				Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())

				// Reconcile the snapshot.
				Expect(vsphere.ReconcileCurrentSnapshot(vmCtx, ctx.Client, vcVM)).To(Succeed())

				// Snapshot should be created.
				verifyK8sVMSnapshot(snapshot1.Name, snapshot1.Namespace, true)
			})
		})
	})
}
