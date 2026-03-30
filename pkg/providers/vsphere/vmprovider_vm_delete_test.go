// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware/govmomi/simulator"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmDeleteTests() {
	const zoneName = "az-1"

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
		// Explicitly place the VM into one of the zones that the test context will create.
		vm.Labels[corev1.LabelTopologyZone] = zoneName
	})

	JustBeforeEach(func() {
		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
	})

	Context("when the VM is off", func() {
		BeforeEach(func() {
			vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		})

		It("deletes the VM", func() {
			Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOff))

			uniqueID := vm.Status.UniqueID
			Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

			Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
			Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
		})
	})

	It("when the VM is on", func() {
		Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))

		uniqueID := vm.Status.UniqueID
		Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

		// This checks that we power off the VM prior to deletion.
		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
		Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
	})

	It("returns success when VM does not exist", func() {
		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
	})

	It("returns NotFound when VM does not exist", func() {
		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
		delete(vm.Labels, corev1.LabelTopologyZone)
		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
	})

	It("Deletes existing VM when zone info is missing", func() {
		_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		uniqueID := vm.Status.UniqueID
		Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

		Expect(vm.Labels).To(HaveKeyWithValue(corev1.LabelTopologyZone, zoneName))
		delete(vm.Labels, corev1.LabelTopologyZone)

		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())
		Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
	})

	It("Does not delete paused VM", func() {
		vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
		Expect(err).ToNot(HaveOccurred())

		uniqueID := vm.Status.UniqueID
		Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

		sctx := ctx.SimulatorContext()
		sctx.WithLock(
			vcVM.Reference(),
			func() {
				vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
				vm.Config.ExtraConfig = append(vm.Config.ExtraConfig,
					&vimtypes.OptionValue{
						Key:   vmopv1.PauseVMExtraConfigKey,
						Value: "True",
					})
			},
		)

		err = vmProvider.DeleteVirtualMachine(ctx, vm)
		Expect(err).To(HaveOccurred())
		var noRequeueErr pkgerr.NoRequeueError
		Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
		Expect(noRequeueErr.Message).To(Equal(constants.VMPausedByAdminError))
		Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())
	})

	Context("Fast Deploy is enabled", func() {
		JustBeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.FastDeploy = true
			})
		})

		It("return success", func() {
			// TODO: We don't have explicit promote tests in here so
			// punt on that. But with the feature enable, we'll get
			// all the VM's tasks.
			_, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			uniqueID := vm.Status.UniqueID
			Expect(ctx.GetVMFromMoID(uniqueID)).ToNot(BeNil())

			err = vmProvider.DeleteVirtualMachine(ctx, vm)
			Expect(err).ToNot(HaveOccurred())

			Expect(ctx.GetVMFromMoID(uniqueID)).To(BeNil())
		})
	})

	DescribeTable("VM is not connected",
		func(state vimtypes.VirtualMachineConnectionState) {
			vcVM, err := createOrUpdateAndGetVcVM(ctx, vmProvider, vm)
			Expect(err).ToNot(HaveOccurred())

			sctx := ctx.SimulatorContext()
			sctx.WithLock(
				vcVM.Reference(),
				func() {
					vm := sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine)
					vm.Summary.Runtime.ConnectionState = state
				})

			err = vmProvider.DeleteVirtualMachine(ctx, vm)

			if state == "" {
				Expect(err).ToNot(HaveOccurred())
				Expect(ctx.GetVMFromMoID(vm.Status.UniqueID)).To(BeNil())
			} else {
				Expect(err).To(HaveOccurred())
				var noRequeueErr pkgerr.NoRequeueError
				Expect(errors.As(err, &noRequeueErr)).To(BeTrue())
				Expect(noRequeueErr.Message).To(Equal(
					fmt.Sprintf("unsupported connection state: %s", state)))
				Expect(ctx.GetVMFromMoID(vm.Status.UniqueID)).ToNot(BeNil())
			}
		},
		Entry("empty", vimtypes.VirtualMachineConnectionState("")),
		Entry("disconnected", vimtypes.VirtualMachineConnectionStateDisconnected),
		Entry("inaccessible", vimtypes.VirtualMachineConnectionStateInaccessible),
		Entry("invalid", vimtypes.VirtualMachineConnectionStateInvalid),
		Entry("orphaned", vimtypes.VirtualMachineConnectionStateOrphaned),
	)
}
