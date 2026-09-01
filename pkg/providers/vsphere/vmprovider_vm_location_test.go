// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmLocationTests() {
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
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()

		vmClass, vm = newVMTestObjects("test-vm")
	})

	JustBeforeEach(func() {
		ctx, vmProvider, nsInfo = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		pinVMToFirstZone(ctx, vm)
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	// callProviderOnce calls the provider exactly once and returns the raw error.
	// The caller is responsible for interpreting the error.
	callProviderOnce := func() error {
		opctx := ctxop.WithContext(ctx)
		return vmProvider.CreateOrUpdateVirtualMachine(opctx, vm)
	}

	When("VM is in the correct namespace RP and folder", func() {
		It("sets VirtualMachineLocationValid condition to True", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineLocationValid)).To(BeTrue())
		})
	})

	When("VM is in a direct child RP of the namespace RP", func() {
		It("sets VirtualMachineLocationValid condition to True", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Create a child RP under the namespace RP.
			rp, _ := ctx.CreateVirtualMachineSetResourcePolicy("child-rp-policy", nsInfo)
			Expect(rp).ToNot(BeNil())

			childRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", rp.Spec.ResourcePool.Name)
			Expect(childRP).ToNot(BeNil())

			// Move VM into the child RP, keep it in the namespace folder.
			task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(childRP.Reference()),
				Folder: ptr.To(nsInfo.Folder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineLocationValid)).To(BeTrue())
		})
	})

	When("VM is in a direct child folder of the namespace folder", func() {
		It("sets VirtualMachineLocationValid condition to True", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Create a child folder under the namespace folder.
			_, childFolder := ctx.CreateVirtualMachineSetResourcePolicy("child-folder-policy", nsInfo)
			Expect(childFolder).ToNot(BeNil())

			nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")

			// Move VM into the child folder, keep it in the namespace RP.
			task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(nsRP.Reference()),
				Folder: ptr.To(childFolder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineLocationValid)).To(BeTrue())
		})
	})

	When("VM is moved to an invalid resource pool", func() {
		It("sets VirtualMachineLocationValid condition to False and returns NoRequeueError", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Move VM to the cluster root RP — outside the namespace RP hierarchy.
			clusterRP, err := ctx.GetFirstClusterFromFirstZone().ResourcePool(ctx)
			Expect(err).ToNot(HaveOccurred())

			task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(clusterRP.Reference()),
				Folder: ptr.To(nsInfo.Folder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			err = callProviderOnce()
			Expect(err).To(HaveOccurred())
			var noRequeueErr pkgerr.NoRequeueError
			Expect(errors.As(err, &noRequeueErr)).To(BeTrue())

			cond := conditions.Get(vm, vmopv1.VirtualMachineLocationValid)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ResourcePoolMismatch"))
		})
	})

	When("VM is moved to an invalid folder", func() {
		It("sets VirtualMachineLocationValid condition to False and returns NoRequeueError", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Move VM to the datacenter VM folder — outside the namespace folder.
			dcFolder, err := ctx.Finder.DefaultFolder(ctx)
			Expect(err).ToNot(HaveOccurred())

			nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")
			task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(nsRP.Reference()),
				Folder: ptr.To(dcFolder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			err = callProviderOnce()
			Expect(err).To(HaveOccurred())
			var noRequeueErr pkgerr.NoRequeueError
			Expect(errors.As(err, &noRequeueErr)).To(BeTrue())

			cond := conditions.Get(vm, vmopv1.VirtualMachineLocationValid)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("FolderMismatch"))
		})
	})

	When("VM is moved to both an invalid resource pool and an invalid folder", func() {
		It("sets VirtualMachineLocationValid condition to False with ResourcePoolAndFolderMismatch reason", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Move VM to the cluster root RP (outside namespace RP) and to the
			// datacenter root folder (outside namespace folder) simultaneously.
			clusterRP, err := ctx.GetFirstClusterFromFirstZone().ResourcePool(ctx)
			Expect(err).ToNot(HaveOccurred())
			dcFolder, err := ctx.Finder.DefaultFolder(ctx)
			Expect(err).ToNot(HaveOccurred())

			task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(clusterRP.Reference()),
				Folder: ptr.To(dcFolder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			err = callProviderOnce()
			Expect(err).To(HaveOccurred())
			var noRequeueErr pkgerr.NoRequeueError
			Expect(errors.As(err, &noRequeueErr)).To(BeTrue())

			cond := conditions.Get(vm, vmopv1.VirtualMachineLocationValid)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ResourcePoolAndFolderMismatch"))
		})
	})

	When("VM is in an invalid location on consecutive reconciles", func() {
		Context("condition idempotency (False)", func() {
			It("does not change LastTransitionTime on the second reconcile", func() {
				vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
				Expect(err).ToNot(HaveOccurred())

				clusterRP, err := ctx.GetFirstClusterFromFirstZone().ResourcePool(ctx)
				Expect(err).ToNot(HaveOccurred())

				task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
					Pool:   ptr.To(clusterRP.Reference()),
					Folder: ptr.To(nsInfo.Folder.Reference()),
				}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
				Expect(err).ToNot(HaveOccurred())
				Expect(task.Wait(ctx)).To(Succeed())

				// First call sets condition to False.
				Expect(callProviderOnce()).Error().To(HaveOccurred())
				cond1 := conditions.Get(vm, vmopv1.VirtualMachineLocationValid)
				Expect(cond1).ToNot(BeNil())
				Expect(cond1.Status).To(Equal(metav1.ConditionFalse))
				ltt1 := cond1.LastTransitionTime

				// Second call: condition is already False — must not touch LastTransitionTime.
				Expect(callProviderOnce()).Error().To(HaveOccurred())
				cond2 := conditions.Get(vm, vmopv1.VirtualMachineLocationValid)
				Expect(cond2).ToNot(BeNil())
				Expect(cond2.LastTransitionTime).To(Equal(ltt1))
			})
		})

		Context("condition idempotency (True)", func() {
			It("does not change LastTransitionTime on the second reconcile", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

				cond1 := conditions.Get(vm, vmopv1.VirtualMachineLocationValid)
				Expect(cond1).ToNot(BeNil())
				Expect(cond1.Status).To(Equal(metav1.ConditionTrue))
				ltt1 := cond1.LastTransitionTime

				// Second full reconcile: condition is already True — must not touch LastTransitionTime.
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				cond2 := conditions.Get(vm, vmopv1.VirtualMachineLocationValid)
				Expect(cond2).ToNot(BeNil())
				Expect(cond2.LastTransitionTime).To(Equal(ltt1))
			})
		})
	})

	When("VM is moved back to the correct location after being in an invalid resource pool", func() {
		It("resets VirtualMachineLocationValid condition to True", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineLocationValid)).To(BeTrue())

			// Move VM to the cluster root RP — outside the namespace RP hierarchy.
			clusterRP, err := ctx.GetFirstClusterFromFirstZone().ResourcePool(ctx)
			Expect(err).ToNot(HaveOccurred())

			task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(clusterRP.Reference()),
				Folder: ptr.To(nsInfo.Folder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			// Condition should become False while VM is in an invalid location.
			Expect(callProviderOnce()).Error().To(HaveOccurred())
			Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineLocationValid)).To(BeTrue())

			// Move VM back to the namespace RP and folder.
			nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")
			Expect(nsRP).ToNot(BeNil())
			task, err = vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(nsRP.Reference()),
				Folder: ptr.To(nsInfo.Folder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			// Condition should be reset to True after the VM is back in the correct location.
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineLocationValid)).To(BeTrue())
		})
	})
}
