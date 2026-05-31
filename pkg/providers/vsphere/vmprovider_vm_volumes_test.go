// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmVolumeOpsTests() {
	var (
		parentCtx  context.Context
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim
		vmProvider providers.VirtualMachineProviderInterface
		nsInfo     builder.WorkloadNamespaceInfo

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
			WithContentLibrary: false,
		}

		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm-volumes", "")
		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true
	})

	JustBeforeEach(func() {
		vsphere.SkipVMImageCLProviderCheck = true

		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx, testConfig)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.MaxDeployThreadsOnProvider = 1
		})
		vmProvider = vsphere.NewVSphereVMProviderFromClient(
			ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMI := builder.DummyClusterVirtualMachineImage("DC0_C0_RP0_VM0")
		Expect(ctx.Client.Create(ctx, clusterVMI)).To(Succeed())

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
	})

	AfterEach(func() {
		vsphere.SkipVMImageCLProviderCheck = false
		ctx.AfterEach()
		ctx = nil
		vmProvider = nil
		vm = nil
		vmClass = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	Describe("GetVirtualDiskByUUID", func() {
		It("should return nil when no disk with the given UUID is attached", func() {
			info, err := vmProvider.GetVirtualDiskByUUID(ctx, vm, "non-existent-uuid")
			Expect(err).ToNot(HaveOccurred())
			Expect(info).To(BeNil())
		})
	})

	Describe("RemoveDiskFromVM", func() {
		It("should treat a missing disk UUID as success (idempotent)", func() {
			Expect(vmProvider.RemoveDiskFromVM(ctx, vm, "non-existent-uuid")).To(Succeed())
		})
	})
}
