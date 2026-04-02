// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"fmt"
	"math/rand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmStorageTests() {
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

	Context("Without Storage Class", func() {
		BeforeEach(func() {
			testConfig.WithoutStorageClass = true
		})

		It("Fails to create VM", func() {
			Expect(vm.Spec.StorageClass).To(BeEmpty())

			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).To(HaveOccurred())

		})
	})

	Context("Without Content Library", func() {
		BeforeEach(func() {
			testConfig.WithContentLibrary = false
		})

		// TODO: Dedupe this with "Basic VM" above
		It("Clones VM", func() {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			var o mo.VirtualMachine
			Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

			By("has expected Status values", func() {
				Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))
				Expect(vm.Status.NodeName).ToNot(BeEmpty())
				Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
				Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))

				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionClassReady)).To(BeTrue())
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionImageReady)).To(BeTrue())
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionStorageReady)).To(BeTrue())
				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineConditionCreated)).To(BeTrue())

				By("did not have VMSetResourcePool", func() {
					Expect(vm.Spec.Reserved).To(BeNil())
					Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionVMSetResourcePolicyReady)).To(BeFalse())
				})
				By("did not have Bootstrap", func() {
					Expect(vm.Spec.Bootstrap).To(BeNil())
					Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionBootstrapReady)).To(BeFalse())
				})
				By("did not have Network", func() {
					Expect(vm.Spec.Network.Disabled).To(BeTrue())
					Expect(conditions.Has(vm, vmopv1.VirtualMachineConditionNetworkReady)).To(BeFalse())
				})
			})

			By("has expected inventory path", func() {
				Expect(vcVM.InventoryPath).To(HaveSuffix(fmt.Sprintf("/%s/%s", nsInfo.Namespace, vm.Name)))
			})

			By("has expected namespace resource pool", func() {
				rp, err := vcVM.ResourcePool(ctx)
				Expect(err).ToNot(HaveOccurred())
				nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, "", "")
				Expect(nsRP).ToNot(BeNil())
				Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
			})

			By("has expected power state", func() {
				Expect(o.Summary.Runtime.PowerState).To(Equal(vimtypes.VirtualMachinePowerStatePoweredOn))
			})

			By("has expected hardware config", func() {
				// TODO: Fix vcsim behavior: NumCPU is correct "2" in the CloneSpec.Config but ends up
				// with 1 CPU from source VM. Ditto for MemorySize. These assertions are only working
				// because the state is on so we reconfigure the VM after it is created.

				// TODO: These assertions are excluded right now because
				// of the aforementioned vcsim behavior. The referenced
				// loophole is no longer in place because the FSS for
				// VM Class as Config was removed, and we now rely on
				// the deploy call to set the correct CPU/memory.
				// Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
				// Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
			})

			// TODO: More assertions!
		})
	})

	// BMV: I don't think this is actually supported.
	XIt("Create VM from VMTX in ContentLibrary", func() {
		imageName := "test-vm-vmtx"

		ctx.ContentLibraryItemTemplate("DC0_C0_RP0_VM0", imageName)
		vm.Spec.ImageName = imageName

		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())
	})

	When("vm has explicit zone", func() {
		JustBeforeEach(func() {
			delete(vm.Labels, corev1.LabelTopologyZone)
		})

		It("creates VM in placement selected zone", func() {
			Expect(vm.Labels).ToNot(HaveKey(corev1.LabelTopologyZone))
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			azName, ok := vm.Labels[corev1.LabelTopologyZone]
			Expect(ok).To(BeTrue())
			Expect(azName).To(BeElementOf(ctx.ZoneNames))

			By("VM is created in the zone's ResourcePool", func() {
				rp, err := vcVM.ResourcePool(ctx)
				Expect(err).ToNot(HaveOccurred())
				nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, azName, "")
				Expect(nsRP).ToNot(BeNil())
				Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
			})
		})
	})

	It("creates VM in assigned zone", func() {
		Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
		azName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]
		vm.Labels[corev1.LabelTopologyZone] = azName

		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		By("VM is created in the zone's ResourcePool", func() {
			rp, err := vcVM.ResourcePool(ctx)
			Expect(err).ToNot(HaveOccurred())
			nsRP := ctx.GetResourcePoolForNamespace(nsInfo.Namespace, azName, "")
			Expect(nsRP).ToNot(BeNil())
			Expect(rp.Reference().Value).To(Equal(nsRP.Reference().Value))
		})
	})

	When("VM zone is constrained by PVC", func() {
		BeforeEach(func() {
			// Need to create the PVC before creating the VM.

			vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "dummy-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-claim-1",
							},
						},
					},
				},
			}

		})

		It("creates VM in allowed zone", func() {
			Expect(len(ctx.ZoneNames)).To(BeNumerically(">", 1))
			azName := ctx.ZoneNames[rand.Intn(len(ctx.ZoneNames))]

			// Make sure we do placement.
			delete(vm.Labels, corev1.LabelTopologyZone)

			pvc1 := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-claim-1",
					Namespace: vm.Namespace,
					Annotations: map[string]string{
						"csi.vsphere.volume-accessible-topology": fmt.Sprintf(`[{"topology.kubernetes.io/zone":"%s"}]`, azName),
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					StorageClassName: ptr.To(ctx.StorageClassName),
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			}
			Expect(ctx.Client.Create(ctx, pvc1)).To(Succeed())
			Expect(ctx.Client.Status().Update(ctx, pvc1)).To(Succeed())

			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(vm.Status.Zone).To(Equal(azName))
		})
	})
}
