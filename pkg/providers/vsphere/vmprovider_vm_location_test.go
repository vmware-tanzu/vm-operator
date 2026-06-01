// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
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
		Expect(ctx.Client.Get(
			ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
			clusterVMI1)).To(Succeed())

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI1.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI1.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		zoneName := ctx.GetFirstZoneName()
		vm.Labels[corev1.LabelTopologyZone] = zoneName
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
	})

	AfterEach(func() {
		vmClass = nil
		vm = nil

		ctx.AfterEach()
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
		It("sets VirtualMachineInValidLocation condition to True", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineInValidLocation)).To(BeTrue())
		})
	})

	When("VM is in a child RP matching a VirtualMachineSetResourcePolicy", func() {
		It("sets VirtualMachineInValidLocation condition to True", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Create a resource policy which provisions a child RP and folder.
			resourcePolicy, _ := ctx.CreateVirtualMachineSetResourcePolicy("child-rp-policy", nsInfo)
			Expect(resourcePolicy).ToNot(BeNil())

			childRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", resourcePolicy.Spec.ResourcePool.Name)
			Expect(childRP).ToNot(BeNil())

			// Move VM into the child RP, keep it in the namespace folder.
			task, err := vcVM.Relocate(ctx, vimtypes.VirtualMachineRelocateSpec{
				Pool:   ptr.To(childRP.Reference()),
				Folder: ptr.To(nsInfo.Folder.Reference()),
			}, vimtypes.VirtualMachineMovePriorityDefaultPriority)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())

			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineInValidLocation)).To(BeTrue())
		})
	})

	When("VM is in a child folder matching a VirtualMachineSetResourcePolicy", func() {
		It("sets VirtualMachineInValidLocation condition to True", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			// Create a resource policy which provisions a child RP and folder.
			resourcePolicy, childFolder := ctx.CreateVirtualMachineSetResourcePolicy("child-folder-policy", nsInfo)
			Expect(resourcePolicy).ToNot(BeNil())
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
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineInValidLocation)).To(BeTrue())
		})
	})

	When("VM is moved to an invalid resource pool", func() {
		It("sets VirtualMachineInValidLocation condition to False and returns NoRequeueError", func() {
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

			Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineInValidLocation)).To(BeTrue())
			cond := conditions.Get(vm, vmopv1.VirtualMachineInValidLocation)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Reason).To(Equal("LocationMismatch"))
		})
	})

	When("VM is moved to an invalid folder", func() {
		It("sets VirtualMachineInValidLocation condition to False and returns NoRequeueError", func() {
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

			Expect(conditions.IsFalse(vm, vmopv1.VirtualMachineInValidLocation)).To(BeTrue())
			cond := conditions.Get(vm, vmopv1.VirtualMachineInValidLocation)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Reason).To(Equal("LocationMismatch"))
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
				cond1 := conditions.Get(vm, vmopv1.VirtualMachineInValidLocation)
				Expect(cond1).ToNot(BeNil())
				Expect(cond1.Status).To(Equal(metav1.ConditionFalse))
				ltt1 := cond1.LastTransitionTime

				// Second call: condition is already False — must not touch LastTransitionTime.
				Expect(callProviderOnce()).Error().To(HaveOccurred())
				cond2 := conditions.Get(vm, vmopv1.VirtualMachineInValidLocation)
				Expect(cond2).ToNot(BeNil())
				Expect(cond2.LastTransitionTime).To(Equal(ltt1))
			})
		})

		Context("condition idempotency (True)", func() {
			It("does not change LastTransitionTime on the second reconcile", func() {
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

				cond1 := conditions.Get(vm, vmopv1.VirtualMachineInValidLocation)
				Expect(cond1).ToNot(BeNil())
				Expect(cond1.Status).To(Equal(metav1.ConditionTrue))
				ltt1 := cond1.LastTransitionTime

				// Second full reconcile: condition is already True — must not touch LastTransitionTime.
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				cond2 := conditions.Get(vm, vmopv1.VirtualMachineInValidLocation)
				Expect(cond2).ToNot(BeNil())
				Expect(cond2.LastTransitionTime).To(Equal(ltt1))
			})
		})
	})
}
