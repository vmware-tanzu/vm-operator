// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/mo"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmHostLocalStorageTests() {

	const (
		hostLocalStorageClassName = "host-local-storage-class"
		hostLocalNodeName         = "esx-host-1.example.com"
		volumeName                = "hostlocal-volume-1"
		pvcName                   = "hostlocal-pvc-1"
	)

	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface
		nsInfo      builder.WorkloadNamespaceInfo

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass

		hostMoID string
	)

	BeforeEach(func() {
		parentCtx = pkgcfg.NewContextWithDefaultConfig()
		parentCtx = ctxop.WithContext(parentCtx)
		parentCtx = ovfcache.WithContext(parentCtx)
		parentCtx = cource.WithContext(parentCtx)
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
			config.Features.HostLocalStorage = true
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
		// These tests only assert placement (which host the VM lands on);
		// powering on would additionally require the host-local volume to
		// be attached, which is outside this feature's scope.
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
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

		// Resolve a real host from vcsim so the test can assert the created
		// VM actually landed there, and create the Node object VM Operator
		// resolves the host-local hostname through.
		zoneName := ctx.GetFirstZoneName()
		ccrs := ctx.GetAZClusterComputes(zoneName)
		Expect(ccrs).ToNot(BeEmpty())
		hosts, err := ccrs[0].Hosts(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).ToNot(BeEmpty())
		hostMoID = hosts[0].Reference().Value

		Expect(ctx.Client.Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        hostLocalNodeName,
				Annotations: map[string]string{"vmware-system-esxi-node-moid": hostMoID},
				Labels:      map[string]string{corev1.LabelTopologyZone: zoneName},
			},
		})).To(Succeed())

		Expect(ctx.Client.Create(ctx, &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostLocalStorageClassName,
				Annotations: map[string]string{
					constants.HostLocalPolicyStorageClassAnnotationKey: "true",
				},
			},
			Provisioner:       "csi.vsphere.vmware.com",
			VolumeBindingMode: ptr.To(storagev1.VolumeBindingWaitForFirstConsumer),
			Parameters: map[string]string{
				"storagePolicyID": ctx.StorageProfileID,
			},
		})).To(Succeed())
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

	addHostLocalVolume := func(pvc *corev1.PersistentVolumeClaim) {
		Expect(ctx.Client.Create(ctx, pvc)).To(Succeed())
		vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
			Name: volumeName,
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Name,
					},
				},
			},
		})
	}

	It("creates a VM pinned to a Bound host-local PVC's accessible host", func() {
		addHostLocalVolume(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: nsInfo.Namespace,
				Annotations: map[string]string{
					"csi.vsphere.volume-accessible-topology": `[{"kubernetes.io/hostname":"` + hostLocalNodeName + `"}]`,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(hostLocalStorageClassName),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		})

		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		Expect(vm.Annotations).To(HaveKeyWithValue(constants.HostLocalSelectedNodeMOIDAnnotationKey, hostMoID))
		Expect(vm.Annotations).To(HaveKeyWithValue(constants.HostLocalSelectedNodeAnnotationKey, hostLocalNodeName))

		var moVM mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"runtime.host"}, &moVM)).To(Succeed())
		Expect(moVM.Runtime.Host).ToNot(BeNil())
		Expect(moVM.Runtime.Host.Value).To(Equal(hostMoID))
	})

	It("creates a VM pinned to the explicit host-local override annotation", func() {
		if vm.Annotations == nil {
			vm.Annotations = map[string]string{}
		}
		vm.Annotations[constants.HostLocalSelectedNodeAnnotationKey] = hostLocalNodeName

		addHostLocalVolume(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: nsInfo.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(hostLocalStorageClassName),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimPending,
			},
		})

		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		Expect(vm.Annotations).To(HaveKeyWithValue(constants.HostLocalSelectedNodeMOIDAnnotationKey, hostMoID))

		var moVM mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"runtime.host"}, &moVM)).To(Succeed())
		Expect(moVM.Runtime.Host).ToNot(BeNil())
		Expect(moVM.Runtime.Host.Value).To(Equal(hostMoID))
	})

	It("fails when host-local PVCs are bound to conflicting hosts", func() {
		addHostLocalVolume(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: nsInfo.Namespace,
				Annotations: map[string]string{
					"csi.vsphere.volume-accessible-topology": `[{"kubernetes.io/hostname":"` + hostLocalNodeName + `"}]`,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(hostLocalStorageClassName),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		})

		vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
			Name: "hostlocal-volume-2",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "hostlocal-pvc-2",
					},
				},
			},
		})
		Expect(ctx.Client.Create(ctx, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hostlocal-pvc-2",
				Namespace: nsInfo.Namespace,
				Annotations: map[string]string{
					"csi.vsphere.volume-accessible-topology": `[{"kubernetes.io/hostname":"some-other-host.example.com"}]`,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(hostLocalStorageClassName),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		})).To(Succeed())

		err := createOrUpdateVM(ctx, vmProvider, vm)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("conflicting hosts"))
	})

	It("does not pin the VM when the HostLocalStorage feature is disabled", func() {
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.Features.HostLocalStorage = false
		})

		addHostLocalVolume(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: nsInfo.Namespace,
				Annotations: map[string]string{
					"csi.vsphere.volume-accessible-topology": `[{"kubernetes.io/hostname":"` + hostLocalNodeName + `"}]`,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(hostLocalStorageClassName),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		})

		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		Expect(vm.Annotations).ToNot(HaveKey(constants.HostLocalSelectedNodeMOIDAnnotationKey))
		Expect(vm.Annotations).ToNot(HaveKey(constants.HostLocalSelectedNodeAnnotationKey))
	})
}
