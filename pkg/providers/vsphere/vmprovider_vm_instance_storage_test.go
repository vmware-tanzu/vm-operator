// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmInstanceStorageTests() {
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

	BeforeEach(func() {
		testConfig.WithInstanceStorage = true
	})

	expectInstanceStorageVolumes := func(
		vm *vmopv1.VirtualMachine,
		isStorage vmopv1.InstanceStorage) {

		ExpectWithOffset(1, isStorage.Volumes).ToNot(BeEmpty())
		isVolumes := vmopv1util.FilterInstanceStorageVolumes(vm)
		ExpectWithOffset(1, isVolumes).To(HaveLen(len(isStorage.Volumes)))

		for _, isVol := range isStorage.Volumes {
			found := false

			for idx, vol := range isVolumes {
				claim := vol.PersistentVolumeClaim.InstanceVolumeClaim
				if claim.StorageClass == isStorage.StorageClass && claim.Size == isVol.Size {
					isVolumes = append(isVolumes[:idx], isVolumes[idx+1:]...)
					found = true
					break
				}
			}

			ExpectWithOffset(1, found).To(BeTrue(), "failed to find instance storage volume for %v", isVol)
		}
	}

	It("creates VM without instance storage", func() {
		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())
	})

	It("create VM with instance storage", func() {
		Expect(vm.Spec.Volumes).To(BeEmpty())

		vmClass.Spec.Hardware.InstanceStorage = vmopv1.InstanceStorage{
			StorageClass: vm.Spec.StorageClass,
			Volumes: []vmopv1.InstanceStorageVolume{
				{
					Size: resource.MustParse("256Gi"),
				},
				{
					Size: resource.MustParse("512Gi"),
				},
			},
		}
		Expect(ctx.Client.Update(ctx, vmClass)).To(Succeed())

		Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeFalse())

		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).To(MatchError(vsphere.ErrAddedInstanceStorageVols))

		By("Instance storage volumes should be added to VM", func() {
			Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeTrue())
			expectInstanceStorageVolumes(vm, vmClass.Spec.Hardware.InstanceStorage)
		})

		_, err = createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).To(MatchError("instance storage PVCs are not bound yet"))
		Expect(vmopv1util.IsInstanceStoragePresent(vm)).To(BeTrue())
		Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeFalse())

		By("Placement should have been done", func() {
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady)).To(BeTrue())
			Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeAnnotationKey))
			Expect(vm.Annotations).To(HaveKey(constants.InstanceStorageSelectedNodeMOIDAnnotationKey))
		})

		isVol0 := vm.Spec.Volumes[0]
		Expect(isVol0.PersistentVolumeClaim.InstanceVolumeClaim).ToNot(BeNil())

		By("simulate volume controller workflow", func() {
			// Simulate what would be set by volume controller.
			vm.Annotations[constants.InstanceStoragePVCsBoundAnnotationKey] = ""

			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("one or more persistent volumes is pending"))
			Expect(err.Error()).To(ContainSubstring(isVol0.Name))

			// Simulate what would be set by the volume controller.
			for _, vol := range vm.Spec.Volumes {
				vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
					Name:     vol.Name,
					Attached: true,
				})
			}
		})

		By("VM is now created", func() {
			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())
		})
	})
}
