// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmUpgradeTests() {
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

	JustBeforeEach(func() {
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
			config.Features.AllDisksArePVCs = false
		})
	})
	JustBeforeEach(func() {
		// Create the VM.
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

		// Clear its annotations and update it in K8s.
		vm.Annotations = nil
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
	})

	It("should return ErrUpgradeSchema, then ErrUpgradeObject, then ErrBackup, then success", func() {
		Expect(vm.Annotations).To(HaveLen(0))

		// Update the VM and expect ErrUpgradeSchema.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			MatchError(vsphere.ErrUpgradeSchema))

		// Assert that the VM was schema upgraded.
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToBuildVersionAnnotationKey,
			pkgcfg.FromContext(ctx).BuildVersion))
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToSchemaVersionAnnotationKey,
			vmopv1.GroupVersion.Version))
		Expect(vm.Annotations).ToNot(HaveKey(
			pkgconst.UpgradedToFeatureVersionAnnotationKey))

		// Update the VM again and expect ErrUpgradeObject.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			MatchError(vsphere.ErrUpgradeObject))

		// Assert that the VM was object upgraded.
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToBuildVersionAnnotationKey,
			pkgcfg.FromContext(ctx).BuildVersion))
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToSchemaVersionAnnotationKey,
			vmopv1.GroupVersion.Version))
		Expect(vm.Annotations).To(HaveKeyWithValue(
			pkgconst.UpgradedToFeatureVersionAnnotationKey,
			vmopv1util.ActivatedFeatureVersion(ctx).String()))

		// Update the VM again and expect ErrBackup.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			MatchError(vsphere.ErrBackup))

		// Update the VM again and expect no error.
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(
			Succeed())
	})
}
