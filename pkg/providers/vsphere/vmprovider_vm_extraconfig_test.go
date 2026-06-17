// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	vmconfextraconfig "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/extraconfig"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmExtraConfigTests() {
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
		// Set up vmconfig context so OnResult can be dispatched via the
		// vmconfig framework when TelcoVMServiceAPI is enabled.
		parentCtx = vmconfig.WithContext(parentCtx)
		parentCtx = vmconfig.Register(parentCtx, vmconfextraconfig.New())

		pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
			config.AsyncCreateEnabled = false
			config.AsyncSignalEnabled = false
			config.Features.TelcoVMServiceAPI = true
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
		Expect(ctx.Client.Get(
			ctx, client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
			clusterVMI1)).To(Succeed())

		vm.Namespace = nsInfo.Namespace
		vm.Spec.ClassName = vmClass.Name
		vm.Spec.ImageName = clusterVMI1.Name
		vm.Spec.Image.Kind = cvmiKind
		vm.Spec.Image.Name = clusterVMI1.Name
		vm.Spec.StorageClass = ctx.StorageClassName

		Expect(ctx.Client.Create(ctx, vm)).To(Succeed())

		zoneName := ctx.GetFirstZoneName()
		vm.Labels[corev1.LabelTopologyZone] = zoneName
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())
	})

	AfterEach(func() {
		vsphere.SkipVMImageCLProviderCheck = false

		vmClass = nil
		vm = nil
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmProvider = nil
		nsInfo = builder.WorkloadNamespaceInfo{}
	})

	// getECVal returns the string value of a vSphere ExtraConfig key, or "" if
	// not present.
	getECVal := func(ec []vimtypes.BaseOptionValue, key string) string {
		for _, bov := range ec {
			if kv := bov.GetOptionValue(); kv != nil && kv.Key == key {
				v, _ := kv.Value.(string)
				return v
			}
		}
		return ""
	}

	// createAndUpdate creates the VM then updates it so that the ExtraConfig
	// reconciler has a live moVM.Config to work with on the update pass.
	createAndUpdate := func() *mo.VirtualMachine {
		GinkgoHelper()
		// First pass: creates the VM in vSphere.
		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		// Second pass: updates the VM; ExtraConfig reconciler now has a live
		// moVM.Config and can apply spec.advanced fields.
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
		return &o
	}

	It("applies a first-class spec.advanced field to vSphere ExtraConfig", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			PreferHTEnabled: ptr.To(true),
		}
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		o := createAndUpdate()
		Expect(getECVal(o.Config.ExtraConfig, "numa.vcpu.preferHT")).To(Equal("TRUE"))
	})

	It("applies a bag key and writes the managed-keys sentinel to vSphere ExtraConfig", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{
				{Key: "telco.test.key", Value: "hello"},
			},
		}
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		o := createAndUpdate()
		Expect(getECVal(o.Config.ExtraConfig, "telco.test.key")).To(Equal("hello"))
		Expect(getECVal(o.Config.ExtraConfig, vsphereconst.ExtraConfigManagedKeysKey)).
			To(Equal("telco.test.key"))
	})

	It("clears a bag key removed from spec on the subsequent update", func() {
		vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{
			ExtraConfig: []vmopv1common.KeyValuePair{
				{Key: "telco.test.key", Value: "hello"},
			},
		}
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		// Apply the key first.
		createAndUpdate()

		// Remove the key from spec and trigger another update.
		vm.Spec.Advanced.ExtraConfig = nil
		Expect(ctx.Client.Update(ctx, vm)).To(Succeed())

		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		var o mo.VirtualMachine
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, &o)).To(Succeed())
		// vSphere removes a key when its value is set to "".
		Expect(getECVal(o.Config.ExtraConfig, "telco.test.key")).To(BeEmpty())
	})
}
