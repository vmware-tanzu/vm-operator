// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pbmmethods "github.com/vmware/govmomi/pbm/methods"
	pbmsim "github.com/vmware/govmomi/pbm/simulator"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// unmanagedVolumesTests provides comprehensive test coverage for the
// AllDisksArePVCs feature in the vSphere provider.
func unmanagedVolumesTests() {

	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		mockProfileResults []pbmtypes.PbmQueryProfileResult

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		parentCtx = vmconfig.WithContext(parentCtx)

		// Enable AllDisksArePVCs feature
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = true
			config.AsyncSignalEnabled = true
			config.Features.AllDisksArePVCs = true
		})

		testConfig = builder.VCSimTestConfig{
			WithContentLibrary: true,
		}

		vmClass = builder.DummyVirtualMachineClassGenName()
		vm = builder.DummyBasicVirtualMachine("test-vm", "")

		// Disable network for simplicity
		if vm.Spec.Network == nil {
			vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		vm.Spec.Network.Disabled = true
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSimWithParentContext(
			parentCtx, testConfig, initObjects...)
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.AllDisksArePVCs = true
			config.MaxDeployThreadsOnProvider = 1
		})

		// Create storage class with policy
		storageClass := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: ctx.StorageClassName,
			},
			Provisioner: "fake-provisioner",
			Parameters: map[string]string{
				"storagePolicyID": ctx.StorageProfileID,
			},
		}
		initObjects = append(initObjects, storageClass)

		// Update profile results to include the new disk
		mockProfileResults = append(mockProfileResults,
			pbmtypes.PbmQueryProfileResult{
				Object: pbmtypes.PbmServerObjectRef{
					Key: "vm-108:203",
				},
				ProfileId: []pbmtypes.PbmProfileId{
					{
						UniqueId: ctx.StorageProfileID,
					},
				},
			},
		)

		// Mock PBM service
		ctx.SimulatorContext().For("/pbm").Map.Handler = func(
			simCtx *simulator.Context,
			m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

			if m.Name == "PbmQueryAssociatedProfiles" {
				return &fakeProfileManager{
					ProfileManager: &pbmsim.ProfileManager{},
					Result:         mockProfileResults,
				}, nil
			}

			return nil, nil
		}

		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
		nsInfo = ctx.CreateWorkloadNamespace()

		vmClass.Namespace = nsInfo.Namespace
		Expect(ctx.Client.Create(ctx, vmClass)).To(Succeed())

		clusterVMImage := &vmopv1.ClusterVirtualMachineImage{}
		Expect(ctx.Client.Get(ctx, client.ObjectKey{Name: ctx.ContentLibraryImageName}, clusterVMImage)).To(Succeed())

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMImage.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMImage.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
	})

	AfterEach(func() {
		vmClass = nil
		vm = nil
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
		mockProfileResults = nil
	})

	When("VM has unmanaged disks from vSphere", func() {

		It("should backfill unmanaged disk into spec.volumes", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).
				To(MatchError(vsphere.ErrUnmanagedVolsRegister))
			Expect(conditions.IsTrue(
				vm, "VirtualMachineUnmanagedVolumesBackfill")).To(BeTrue())
			Expect(conditions.IsFalse(
				vm, "VirtualMachineUnmanagedVolumesRegister")).To(BeTrue())

			// Check that a volume was added for the unmanaged disk.
			var vol *vmopv1.VirtualMachineVolume
			for i, v := range vm.Spec.Volumes {
				if v.PersistentVolumeClaim != nil &&
					v.PersistentVolumeClaim.UnmanagedVolumeClaim != nil {
					vol = &vm.Spec.Volumes[i]
					break
				}
			}

			Expect(vol).ToNot(BeNil(), "should have added volume for unmanaged disk")
			Expect(vol.PersistentVolumeClaim.UnmanagedVolumeClaim.Type).To(Equal(vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM))
			Expect(vol.PersistentVolumeClaim.ClaimName).ToNot(BeEmpty())

			claimName := vol.PersistentVolumeClaim.ClaimName
			Expect(claimName).ToNot(BeEmpty())

			// Check that PVC was created.
			pvc := &corev1.PersistentVolumeClaim{}
			Expect(ctx.Client.Get(ctx, client.ObjectKey{
				Namespace: vm.Namespace,
				Name:      claimName,
			}, pvc)).To(Succeed())

			// Verify PVC properties.
			Expect(pvc.Spec.StorageClassName).ToNot(BeNil())
			Expect(*pvc.Spec.StorageClassName).To(Equal(ctx.StorageClassName))
			Expect(pvc.Spec.DataSourceRef).ToNot(BeNil())
			Expect(pvc.Spec.DataSourceRef.Kind).To(Equal("VirtualMachine"))
			Expect(pvc.Spec.DataSourceRef.Name).To(Equal(vm.Name))
			Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
			expectedStorage := *kubeutil.BytesToResource(31457280)
			Expect(pvc.Spec.Resources.Requests[corev1.ResourceStorage].Equal(expectedStorage)).To(BeTrue())

			// Verify OwnerReference.
			Expect(pvc.OwnerReferences).To(HaveLen(1))
			Expect(pvc.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
			Expect(pvc.OwnerReferences[0].Name).To(Equal(vm.Name))
			Expect(pvc.OwnerReferences[0].Controller).To(BeNil())

			// Check that CnsRegisterVolume was created.
			crv := &cnsv1alpha1.CnsRegisterVolume{}
			Expect(ctx.Client.Get(ctx, client.ObjectKey{
				Namespace: vm.Namespace,
				Name:      claimName,
			}, crv)).To(Succeed())

			// Verify CnsRegisterVolume properties.
			Expect(crv.Spec.PvcName).To(Equal(claimName))
			Expect(crv.Spec.DiskURLPath).ToNot(BeEmpty())
			Expect(crv.Spec.AccessMode).To(Equal(corev1.ReadWriteOnce))

			// Verify labels.
			Expect(crv.Labels["vmoperator.vmware.com/created-by"]).To(Equal(vm.Name))

			// Verify OwnerReference.
			Expect(crv.OwnerReferences).To(HaveLen(1))
			Expect(crv.OwnerReferences[0].Kind).To(Equal("VirtualMachine"))
			Expect(crv.OwnerReferences[0].Name).To(Equal(vm.Name))
			Expect(crv.OwnerReferences[0].Controller).ToNot(BeNil())
			Expect(*crv.OwnerReferences[0].Controller).To(BeTrue())

			// Simulate PVC becoming bound
			pvc.Status.Phase = corev1.ClaimBound
			Expect(ctx.Client.Status().Update(ctx, pvc)).To(Succeed())
			vm.Status.Volumes[0].Attached = true
			Expect(ctx.Client.Status().Update(ctx, vm)).To(Succeed())

			// Trigger another reconciliation
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())

			// Verify CnsRegisterVolume was deleted
			Expect(client.IgnoreNotFound(ctx.Client.Get(ctx, client.ObjectKey{
				Namespace: vm.Namespace,
				Name:      claimName,
			}, crv))).To(Succeed())

			Expect(conditions.IsTrue(
				vm, "VirtualMachineUnmanagedVolumesRegister")).To(BeTrue())
		})
	})
}

// fakeProfileManager is a mock PBM ProfileManager for testing.
type fakeProfileManager struct {
	*pbmsim.ProfileManager
	Result []pbmtypes.PbmQueryProfileResult
}

func (m *fakeProfileManager) PbmQueryAssociatedProfiles(
	req *pbmtypes.PbmQueryAssociatedProfiles) soap.HasFault {

	body := new(pbmmethods.PbmQueryAssociatedProfilesBody)
	body.Res = new(pbmtypes.PbmQueryAssociatedProfilesResponse)
	body.Res.Returnval = m.Result

	return body
}
