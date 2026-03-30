// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmSnapshotTests() {
	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass

		zoneName string
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

		zoneName = ctx.GetFirstZoneName()
		vm.Labels[corev1.LabelTopologyZone] = zoneName
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
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

			vm.Spec.CurrentSnapshotName = vmSnapshot.Name

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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name
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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name
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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name
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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name

			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

			// Verify VM status reflects current snapshot.
			Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
			Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))

			// Verify the status has root snapshots.
			Expect(vm.Status.RootSnapshots).ToNot(BeNil())
			Expect(vm.Status.RootSnapshots).To(HaveLen(1))
			Expect(vm.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.RootSnapshots[0].Name).To(Equal(vmSnapshot.Name))
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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name

			By("First reconcile should return ErrSnapshotRevert", func() {
				_, createErr := vmProvider.CreateOrUpdateVirtualMachineAsync(ctx, vm)
				Expect(createErr).To(HaveOccurred())
				Expect(errors.Is(createErr, vsphere.ErrSnapshotRevert))
				Expect(pkgerr.IsNoRequeueError(createErr)).To(BeTrue(), "Should return NoRequeueError")
			})

			err = createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Verify VM status reflects the reverted snapshot
			Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
			Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))

			// Verify the spec.currentSnapshot is cleared.
			Expect(vm.Spec.CurrentSnapshotName).To(BeEmpty())

			// Verify the status has root snapshots.
			Expect(vm.Status.RootSnapshots).ToNot(BeNil())
			Expect(vm.Status.RootSnapshots).To(HaveLen(1))
			Expect(vm.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.RootSnapshots[0].Name).To(Equal(vmSnapshot.Name))

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
				vm.Spec.CurrentSnapshotName = vmSnapshot.Name

				// Revert to the first snapshot
				err = createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				// Verify VM status reflects the reverted snapshot
				Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
				Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
				Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))

				// Verify the spec.currentSnapshot is cleared.
				Expect(vm.Spec.CurrentSnapshotName).To(BeEmpty())

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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name

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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name

			err = createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Verify VM status reflects the reverted snapshot
			Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
			Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))

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
			vm.Spec.CurrentSnapshotName = ""

			err = createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Status should reflect the actual current snapshot
			Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
			Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))

			// Verify the status has root snapshots.
			Expect(vm.Status.RootSnapshots).ToNot(BeNil())
			Expect(vm.Status.RootSnapshots).To(HaveLen(1))
			Expect(vm.Status.RootSnapshots[0].Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.RootSnapshots[0].Name).To(Equal(vmSnapshot.Name))
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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name

			err = createOrUpdateVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// VM status should still point to first snapshot because revert was skipped
			Expect(vm.Status.CurrentSnapshot).ToNot(BeNil())
			Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineSnapshotRevertSucceeded)).To(BeTrue())
			Expect(conditions.GetReason(vm, vmopv1.VirtualMachineSnapshotRevertSucceeded)).To(Equal(vmopv1.VirtualMachineSnapshotRevertSkippedReason))
			Expect(vm.Status.CurrentSnapshot.Type).To(Equal(vmopv1.VirtualMachineSnapshotReferenceTypeManaged))
			Expect(vm.Status.CurrentSnapshot.Name).To(Equal(vmSnapshot.Name))

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
			vm.Spec.CurrentSnapshotName = vmSnapshot.Name

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

			vm.Spec.CurrentSnapshotName = ""
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

			Expect(conditions.Get(vm,
				vmopv1.VirtualMachineSnapshotRevertSucceeded),
			).To(BeNil())
		})
	})
}
