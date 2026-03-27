// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/vmware/govmomi/vim25/mo"

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

func vmDisksTests() {
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

	Context("VM has thin provisioning", func() {
		BeforeEach(func() {
			if vm.Spec.Advanced == nil {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
			}
			vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThin
		})

		It("Succeeds", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			_, backing := getVMHomeDisk(ctx, vcVM, o)
			Expect(backing.ThinProvisioned).To(PointTo(BeTrue()))
		})
	})

	XContext("VM has thick provisioning", func() {
		BeforeEach(func() {
			vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThick
		})

		It("Succeeds", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			/* vcsim CL deploy has "thick" but that isn't reflected for this disk. */
			_, backing := getVMHomeDisk(ctx, vcVM, o)
			Expect(backing.ThinProvisioned).To(PointTo(BeFalse()))
		})
	})

	XContext("VM has eager zero provisioning", func() {
		BeforeEach(func() {
			if vm.Spec.Advanced == nil {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
			}
			vm.Spec.Advanced.DefaultVolumeProvisioningMode = vmopv1.VolumeProvisioningModeThickEagerZero
		})

		It("Succeeds", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			/* vcsim CL deploy has "eagerZeroedThick" but that isn't reflected for this disk. */
			_, backing := getVMHomeDisk(ctx, vcVM, o)
			Expect(backing.EagerlyScrub).To(PointTo(BeTrue()))
		})
	})

	Context("Should resize root disk", func() {
		It("Succeeds", func() {
			newSize := resource.MustParse("4242Gi")

			if vm.Spec.Advanced == nil {
				vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
			}
			vm.Spec.Advanced.BootDiskCapacity = &newSize
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
			disk, _ := getVMHomeDisk(ctx, vcVM, o)
			Expect(disk.CapacityInBytes).To(BeEquivalentTo(newSize.Value()))
		})
	})
}
