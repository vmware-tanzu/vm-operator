// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware/govmomi/vapi/cluster"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmSetResourcePolicyTests() {
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

	var resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy

	JustBeforeEach(func() {
		resourcePolicyName := "test-policy"
		resourcePolicy = getVirtualMachineSetResourcePolicy(resourcePolicyName, nsInfo.Namespace)
		Expect(vmProvider.CreateOrUpdateVirtualMachineSetResourcePolicy(ctx, resourcePolicy)).To(Succeed())
		Expect(ctx.Client.Create(ctx, resourcePolicy)).To(Succeed())

		vm.Annotations["vsphere-cluster-module-group"] = resourcePolicy.Spec.ClusterModuleGroups[0]
		if vm.Spec.Reserved == nil {
			vm.Spec.Reserved = &vmopv1.VirtualMachineReservedSpec{}
		}
		vm.Spec.Reserved.ResourcePolicyName = resourcePolicy.Name
	})

	AfterEach(func() {
		resourcePolicy = nil
	})

	When("a cluster module is specified without resource policy", func() {
		JustBeforeEach(func() {
			vm.Spec.Reserved.ResourcePolicyName = ""
		})

		It("returns error", func() {
			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot set cluster module without resource policy"))
		})
	})

	It("VM is created in child Folder and ResourcePool", func() {
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		By("has expected condition", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady)).To(BeTrue())
		})

		By("has expected inventory path", func() {
			Expect(vcVM.InventoryPath).To(HaveSuffix(
				fmt.Sprintf("/%s/%s/%s", nsInfo.Namespace, resourcePolicy.Spec.Folder, vm.Name)))
		})

		By("has expected namespace resource pool", func() {
			rp, err := vcVM.ResourcePool(ctx)
			Expect(err).ToNot(HaveOccurred())
			childRP := ctx.GetResourcePoolForNamespace(
				nsInfo.Namespace,
				vm.Labels[corev1.LabelTopologyZone],
				resourcePolicy.Spec.ResourcePool.Name)
			Expect(childRP).ToNot(BeNil())
			Expect(rp.Reference().Value).To(Equal(childRP.Reference().Value))
		})
	})

	It("Cluster Modules", func() {
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var members []vimtypes.ManagedObjectReference
		for i := range resourcePolicy.Status.ClusterModules {
			m, err := cluster.NewManager(ctx.RestClient).ListModuleMembers(ctx, resourcePolicy.Status.ClusterModules[i].ModuleUuid)
			Expect(err).ToNot(HaveOccurred())
			members = append(m, members...)
		}

		Expect(members).To(ContainElements(vcVM.Reference()))
	})

	It("Returns error with non-existence cluster module", func() {
		clusterModName := "bogusClusterMod"
		vm.Annotations["vsphere-cluster-module-group"] = clusterModName
		err := createOrUpdateVM(ctx, vmProvider, vm)
		Expect(err).To(MatchError("VirtualMachineSetResourcePolicy cluster module is not ready"))
	})
}
