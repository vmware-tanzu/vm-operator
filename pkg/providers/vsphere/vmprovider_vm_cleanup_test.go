// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func vmCleanupTests() {
	const zoneName = "az-1"

	var (
		parentCtx   context.Context
		initObjects []client.Object
		testConfig  builder.VCSimTestConfig
		ctx         *builder.TestContextForVCSim
		vmProvider  providers.VirtualMachineProviderInterface

		vm      *vmopv1.VirtualMachine
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		parentCtx = newVMTestParentContext()
		testConfig = newVMTestConfig()

		vmClass, vm = newVMTestObjects("test-vm")

		// Explicitly place the VM into one of the zones that the test context will create.
		vm.Labels[corev1.LabelTopologyZone] = zoneName
	})

	JustBeforeEach(func() {
		ctx, vmProvider, _ = setupVMTest(
			parentCtx, testConfig, vmClass, vm, initObjects...)

		Expect(createOrUpdateVM(ctx, vmProvider, vm)).To(Succeed())
	})

	AfterEach(func() {
		vmTestAfterEach(ctx, vm)

		vmClass = nil
		vm = nil

		ctx = nil
		initObjects = nil
		vmProvider = nil
	})

	It("successfully cleans up VM service state", func() {
		vcVM, err := ctx.Finder.VirtualMachine(ctx, vm.Name)
		Expect(err).NotTo(HaveOccurred())

		// Set up some VM Operator managed fields
		configSpec := vimtypes.VirtualMachineConfigSpec{
			ExtraConfig: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{
					Key:   constants.ExtraConfigVMServiceNamespacedName,
					Value: vm.Namespace + "/" + vm.Name,
				},
				&vimtypes.OptionValue{
					Key:   "guestinfo.userdata",
					Value: "test-data",
				},
			},
			ManagedBy: &vimtypes.ManagedByInfo{
				ExtensionKey: vmopv1.ManagedByExtensionKey,
				Type:         vmopv1.ManagedByExtensionType,
			},
		}

		task, err := vcVM.Reconfigure(ctx, configSpec)
		Expect(err).NotTo(HaveOccurred())
		Expect(task.Wait(ctx)).To(Succeed())

		// Verify fields are set before cleanup
		var moVMBefore mo.VirtualMachine
		err = vcVM.Properties(ctx, vcVM.Reference(), []string{"config"}, &moVMBefore)
		Expect(err).NotTo(HaveOccurred())
		Expect(moVMBefore.Config).ToNot(BeNil())
		Expect(moVMBefore.Config.ManagedBy).ToNot(BeNil())
		Expect(moVMBefore.Config.ManagedBy.ExtensionKey).To(Equal(vmopv1.ManagedByExtensionKey))

		// Run cleanup
		Expect(vmProvider.CleanupVirtualMachine(ctx, vm)).To(Succeed())

		// Verify VM Operator managed fields were removed
		var moVMAfter mo.VirtualMachine
		err = vcVM.Properties(ctx, vcVM.Reference(), []string{"config"}, &moVMAfter)
		Expect(err).NotTo(HaveOccurred())
		Expect(moVMAfter.Config).ToNot(BeNil())

		// Check ExtraConfig
		ecList := object.OptionValueList(moVMAfter.Config.ExtraConfig)
		val, ok := ecList.Get(constants.ExtraConfigVMServiceNamespacedName)
		Expect(!ok || val == "").To(BeTrue(), "Expected vmservice.namespacedName to be removed")

		val2, ok2 := ecList.Get("guestinfo.userdata")
		Expect(!ok2 || val2 == "").To(BeTrue(), "Expected guestinfo.userdata to be removed")

		// Check ManagedBy
		Expect(moVMAfter.Config.ManagedBy).To(BeNil())

		// Verify VM still exists in vCenter
		Expect(ctx.GetVMFromMoID(vm.Status.UniqueID)).ToNot(BeNil())
	})

	It("returns success when VM does not exist in vCenter", func() {
		// Delete the VM from vCenter first
		Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())

		// Cleanup should succeed even though VM doesn't exist
		Expect(vmProvider.CleanupVirtualMachine(ctx, vm)).To(Succeed())
	})

	Context("Affinity Tag ownership (TaggingAPI)", func() {
		var tag *vspherepolv1.Tag

		BeforeEach(func() {
			pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
				config.Features.TaggingAPI = true
			})
		})

		JustBeforeEach(func() {
			tag = &vspherepolv1.Tag{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "team-blue",
					Namespace: vm.Namespace,
				},
				Spec: vspherepolv1.TagSpec{
					Key:   "team",
					Value: "blue",
				},
			}
			Expect(controllerutil.SetOwnerReference(
				vm, tag, ctx.Client.Scheme())).To(Succeed())
			Expect(ctx.Client.Create(ctx, tag)).To(Succeed())
		})

		It("releases ownership on the skip-delete path", func() {
			Expect(vmProvider.CleanupVirtualMachine(ctx, vm)).To(Succeed())

			got := &vspherepolv1.Tag{}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(tag), got)).To(Succeed())
			Expect(got.OwnerReferences).To(BeEmpty())
		})

		It("releases ownership even when the VM has no vCenter VM", func() {
			Expect(vmProvider.DeleteVirtualMachine(ctx, vm)).To(Succeed())

			// Re-establish ownership now that the vCenter VM is gone, then
			// clean up again so CleanupVirtualMachine hits the vcVM == nil
			// early-return after releasing ownership.
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(tag), tag)).To(Succeed())
			Expect(controllerutil.SetOwnerReference(
				vm, tag, ctx.Client.Scheme())).To(Succeed())
			Expect(ctx.Client.Update(ctx, tag)).To(Succeed())

			Expect(vmProvider.CleanupVirtualMachine(ctx, vm)).To(Succeed())

			got := &vspherepolv1.Tag{}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(tag), got)).To(Succeed())
			Expect(got.OwnerReferences).To(BeEmpty())
		})

		It("propagates an error from releasing ownership", func() {
			injectedErr := errors.New("release ownership boom")

			fakeClient := builder.NewFakeClientWithInterceptors(
				interceptor.Funcs{
					Patch: func(
						ctx context.Context,
						c client.WithWatch,
						obj client.Object,
						patch client.Patch,
						opts ...client.PatchOption) error {

						if _, ok := obj.(*vspherepolv1.Tag); ok {
							return injectedErr
						}
						return c.Patch(ctx, obj, patch, opts...)
					},
				},
				tag,
			)

			p := vsphere.NewVSphereVMProviderFromClient(ctx, fakeClient, ctx.Recorder)
			Expect(p.CleanupVirtualMachine(ctx, vm)).To(MatchError(injectedErr))
		})
	})
}
