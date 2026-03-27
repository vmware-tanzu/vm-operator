// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmCreateTests() {
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

	It("Basic VM", func() {
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())

		By("has VC UUID annotation set", func() {
			Expect(vm.Annotations).Should(HaveKeyWithValue(vmopv1.ManagerID, ctx.VCClient.Client.ServiceContent.About.InstanceUuid))
		})

		By("has expected Status values", func() {
			Expect(vm.Status.PowerState).To(Equal(vm.Spec.PowerState))
			Expect(vm.Status.NodeName).ToNot(BeEmpty())
			Expect(vm.Status.InstanceUUID).To(And(Not(BeEmpty()), Equal(o.Config.InstanceUuid)))
			Expect(vm.Status.BiosUUID).To(And(Not(BeEmpty()), Equal(o.Config.Uuid)))

			Expect(vm.Status.Class).ToNot(BeNil())
			Expect(vm.Status.Class.Name).To(Equal(vm.Spec.ClassName))
			Expect(vm.Status.Class.APIVersion).To(Equal(vmopv1.GroupVersion.String()))

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

		vmClassRes := &vmClass.Spec.Policies.Resources

		By("has expected CpuAllocation", func() {
			Expect(o.Config.CpuAllocation).ToNot(BeNil())

			reservation := o.Config.CpuAllocation.Reservation
			Expect(reservation).ToNot(BeNil())
			Expect(*reservation).To(Equal(virtualmachine.CPUQuantityToMhz(vmClassRes.Requests.Cpu, vcsimCPUFreq)))
			limit := o.Config.CpuAllocation.Limit
			Expect(limit).ToNot(BeNil())
			Expect(*limit).To(Equal(virtualmachine.CPUQuantityToMhz(vmClassRes.Limits.Cpu, vcsimCPUFreq)))
		})

		By("has expected MemoryAllocation", func() {
			Expect(o.Config.MemoryAllocation).ToNot(BeNil())

			reservation := o.Config.MemoryAllocation.Reservation
			Expect(reservation).ToNot(BeNil())
			Expect(*reservation).To(Equal(virtualmachine.MemoryQuantityToMb(vmClassRes.Requests.Memory)))
			limit := o.Config.MemoryAllocation.Limit
			Expect(limit).ToNot(BeNil())
			Expect(*limit).To(Equal(virtualmachine.MemoryQuantityToMb(vmClassRes.Limits.Memory)))
		})

		By("has expected hardware config", func() {
			Expect(o.Summary.Config.NumCpu).To(BeEquivalentTo(vmClass.Spec.Hardware.Cpus))
			Expect(o.Summary.Config.MemorySizeMB).To(BeEquivalentTo(vmClass.Spec.Hardware.Memory.Value() / 1024 / 1024))
		})

		By("has expected backup ExtraConfig key", func() {
			Expect(o.Config.ExtraConfig).ToNot(BeNil())

			ecMap := pkgutil.OptionValues(o.Config.ExtraConfig).StringMap()
			Expect(ecMap).To(HaveKey(backupapi.VMResourceYAMLExtraConfigKey))
		})

		// TODO: More assertions!
	})

	When("using async create", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
				config.AsyncCreateEnabled = true
				config.AsyncSignalEnabled = true
			})
		})
		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.MaxDeployThreadsOnProvider = 16
			})
		})

		It("should succeed", func() {
			Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
			Expect(vm.Status.UniqueID).ToNot(BeEmpty())
		})

		When("there is an error getting the pre-reqs", func() {
			It("should not prevent a subsequent create attempt from going through", func() {
				imgName := vm.Spec.Image.Name
				vm.Spec.Image.Name = "does-not-exist"
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(
					"clustervirtualmachineimages.vmoperator.vmware.com \"does-not-exist\" not found: " +
						"clustervirtualmachineimages.vmoperator.vmware.com \"does-not-exist\" not found"))
				vm.Spec.Image.Name = imgName
				Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
				Expect(vm.Status.UniqueID).ToNot(BeEmpty())
			})
		})

		When("there is an error creating the VM", func() {
			JustBeforeEach(func() {
				ctx.SimulatorContext().Map.Handler = func(
					ctx *simulator.Context,
					m *simulator.Method) (mo.Reference, vimtypes.BaseMethodFault) {

					if m.Name == "ImportVApp" {
						return nil, &vimtypes.InvalidRequest{}
					}
					return nil, nil
				}
			})

			It("should fail to create the VM without an NPE", func() {
				err := createOrUpdateVM(ctx, vmProvider, vm)
				Expect(err).To(HaveOccurred())
				Eventually(func(g Gomega) {
					g.Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), vm)).To(Succeed())
					g.Expect(vm.Status.UniqueID).To(BeEmpty())
					c := conditions.Get(vm, vmopv1.VirtualMachineConditionCreated)
					g.Expect(c).ToNot(BeNil())
					g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(c.Reason).To(Equal("Error"))
					g.Expect(c.Message).To(Equal("deploy error: ServerFaultCode: InvalidRequest"))
				}).Should(Succeed())
			})
		})
	})

	Describe("FastDeploy", Label(testlabels.Create), vmFastDeployTests)

}
