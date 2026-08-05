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
	storagehelpers "k8s.io/component-helpers/storage/volume"

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
		hostLocalStorageClassName          = "host-local-storage-class"
		hostLocalImmediateStorageClassName = "host-local-immediate-storage-class"
		hostLocalNodeName                  = "esx-host-1.example.com"
		volumeName                         = "hostlocal-volume-1"
		pvcName                            = "hostlocal-pvc-1"
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

		hostMoID        string
		nodeNameForHost map[string]string
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

		hostMoID = ""
		nodeNameForHost = nil

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

		// Resolve the real hosts from vcsim so the tests can assert which one
		// the created VM landed on, and create the Node objects VM Operator
		// maps between host MoIDs and Supervisor node names through. Every
		// host needs a Node, since a VM with no host resolved up front may be
		// placed on any of them.
		firstZoneName := ctx.GetFirstZoneName()
		Expect(ctx.ZoneNames).ToNot(BeEmpty())

		nodeNameForHost = map[string]string{}
		for _, zoneName := range ctx.ZoneNames {
			for _, ccr := range ctx.GetAZClusterComputes(zoneName) {
				hosts, err := ccr.Hosts(ctx)
				Expect(err).ToNot(HaveOccurred())

				for _, host := range hosts {
					moID := host.Reference().Value

					// The first host of the first zone gets the well-known name
					// that the tests pinning a specific host refer to.
					nodeName := "esx-" + moID + ".example.com"
					if hostMoID == "" && zoneName == firstZoneName {
						hostMoID = moID
						nodeName = hostLocalNodeName
					}
					nodeNameForHost[moID] = nodeName

					Expect(ctx.Client.Create(ctx, &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name:        nodeName,
							Annotations: map[string]string{"vmware-system-esxi-node-moid": moID},
							Labels:      map[string]string{corev1.LabelTopologyZone: zoneName},
						},
					})).To(Succeed())
				}
			}
		}
		Expect(hostMoID).ToNot(BeEmpty())

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

		// The same host-local policy, but provisioned immediately rather than
		// waiting for a consumer.
		Expect(ctx.Client.Create(ctx, &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostLocalImmediateStorageClassName,
				Annotations: map[string]string{
					constants.HostLocalPolicyStorageClassAnnotationKey: "true",
				},
			},
			Provisioner:       "csi.vsphere.vmware.com",
			VolumeBindingMode: ptr.To(storagev1.VolumeBindingImmediate),
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

		// The host is derived from the PVC on every reconcile, so no host-local
		// annotation is recorded on the VM.
		for k := range vm.Annotations {
			Expect(k).ToNot(ContainSubstring("hostlocal"))
		}

		var moVM mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"runtime.host"}, &moVM)).To(Succeed())
		Expect(moVM.Runtime.Host).ToNot(BeNil())
		Expect(moVM.Runtime.Host.Value).To(Equal(hostMoID))
	})

	It("creates a VM pinned to a PVC that already names a selected node", func() {
		// This is how a host chosen by an earlier placement is remembered:
		// the PVC's own selected node, rather than anything on the VM.
		addHostLocalVolume(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: nsInfo.Namespace,
				Annotations: map[string]string{
					storagehelpers.AnnSelectedNode:               hostLocalNodeName,
					constants.CNSSelectedNodeIsZoneAnnotationKey: "false",
				},
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

	It("does not publish a host to the PVC when the HostLocalStorage feature is disabled", func() {
		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.Features.HostLocalStorage = false
		})

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

		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var pvc corev1.PersistentVolumeClaim
		Expect(ctx.Client.Get(
			ctx,
			client.ObjectKey{Namespace: nsInfo.Namespace, Name: pvcName},
			&pvc)).To(Succeed())
		Expect(pvc.Annotations).ToNot(HaveKey(storagehelpers.AnnSelectedNode))
	})

	It("publishes the host the VM was created on to its pending host-local PVC", func() {
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

		// The first pass creates the VM; the host is published on the
		// following reconcile, once the VM actually exists on a host.
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())

		var moVM mo.VirtualMachine
		Expect(vcVM.Properties(
			ctx, vcVM.Reference(), []string{"runtime.host"}, &moVM)).To(Succeed())
		Expect(moVM.Runtime.Host).ToNot(BeNil())

		// Whichever host DRS picked, that is the host the PVC must name.
		expectedNodeName := nodeNameForHost[moVM.Runtime.Host.Value]
		Expect(expectedNodeName).ToNot(BeEmpty())

		var pvc corev1.PersistentVolumeClaim
		Expect(ctx.Client.Get(
			ctx,
			client.ObjectKey{Namespace: nsInfo.Namespace, Name: pvcName},
			&pvc)).To(Succeed())
		Expect(pvc.Annotations).To(HaveKeyWithValue(
			storagehelpers.AnnSelectedNode, expectedNodeName))
		Expect(pvc.Annotations).To(HaveKeyWithValue(
			constants.CNSSelectedNodeIsZoneAnnotationKey, "false"))
	})

	It("does not publish a host to a pending Immediate host-local PVC", func() {
		// CNS provisions an Immediate volume without waiting to be told a
		// node, so it picks its own host and there is nothing to publish. The
		// VM cannot even be placed while such a PVC is unbound, since zone
		// constraints cannot be derived from it, so the VM waits for CNS
		// rather than the other way round.
		addHostLocalVolume(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: nsInfo.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(hostLocalImmediateStorageClassName),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimPending,
			},
		})

		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).To(MatchError(ContainSubstring("is not bound")))

		var pvc corev1.PersistentVolumeClaim
		Expect(ctx.Client.Get(
			ctx,
			client.ObjectKey{Namespace: nsInfo.Namespace, Name: pvcName},
			&pvc)).To(Succeed())
		Expect(pvc.Annotations).ToNot(HaveKey(storagehelpers.AnnSelectedNode))
	})

	It("does not publish a host to a PVC whose data source is the VM itself", func() {
		// Such a PVC describes one of the VM's own disks, which already exists
		// wherever the VM does, so there is no host to hand off.
		addHostLocalVolume(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: nsInfo.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: ptr.To(hostLocalStorageClassName),
				DataSourceRef: &corev1.TypedObjectReference{
					APIGroup: ptr.To(vmopv1.GroupVersion.Group),
					Kind:     "VirtualMachine",
					Name:     vm.Name,
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimPending,
			},
		})

		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())
		Expect(vmProvider.CreateOrUpdateVirtualMachine(ctx, vm)).To(Succeed())

		var pvc corev1.PersistentVolumeClaim
		Expect(ctx.Client.Get(
			ctx,
			client.ObjectKey{Namespace: nsInfo.Namespace, Name: pvcName},
			&pvc)).To(Succeed())
		Expect(pvc.Annotations).ToNot(HaveKey(storagehelpers.AnnSelectedNode))
	})
}
