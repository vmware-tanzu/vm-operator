// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/kubevm"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

func kubevmTests() {
	Describe(
		"ResolveKubeVMParentOnCreate",
		Label(testlabels.Create, testlabels.API, testlabels.Mutation, testlabels.Webhook),
		unitTestsResolveKubeVMParentOnCreate,
	)
}

func unitTestsResolveKubeVMParentOnCreate() {
	const (
		genericName = "my-generic-vm"
		namespace   = "default"
	)

	var (
		ctx     *unitMutationWebhookContext
		vm      *vmopv1.VirtualMachine
		generic *kubevmv1a1.VirtualMachine
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
		vm = ctx.vm.DeepCopy()
		vm.Namespace = namespace
		vm.Annotations = map[string]string{
			kubevm.AnnotationKey: genericName,
		}
		// DummyVirtualMachine sets these; clear them so the delegated-field
		// resolution below is exercised against an otherwise-empty spec, as
		// the demo's provider object is created.
		vm.Spec.ClassName = ""
		vm.Spec.Image = nil
		vm.Spec.StorageClass = ""
		vm.Spec.PowerState = ""

		generic = &kubevmv1a1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      genericName,
				Namespace: namespace,
			},
			Spec: kubevmv1a1.VirtualMachineSpec{
				InfrastructureRef: kubevmv1a1.ObjectReference{
					APIGroup: vmopv1.GroupName,
					Kind:     "VirtualMachine",
					Name:     vm.Name,
				},
				InstanceType: &kubevmv1a1.InstanceTypeSpec{Name: "best-effort-2xlarge"},
				BootDisk: &kubevmv1a1.BootDiskSpec{
					Source: kubevmv1a1.DiskSource{
						Image: &kubevmv1a1.ObjectReference{
							APIGroup: vmopv1.GroupName,
							Kind:     "VirtualMachineImage",
							Name:     "vmi-0123456789abcdef0",
						},
					},
					StorageClassName: "my-storage-class",
				},
				PowerState: kubevmv1a1.PowerStateOn,
			},
		}

		pkgcfg.UpdateContext(ctx.Context, func(config *pkgcfg.Config) {
			config.Features.KubeVMProvider = true
		})
	})

	When("the feature gate is off", func() {
		It("does nothing, even with the annotation present", func() {
			pkgcfg.UpdateContext(ctx.Context, func(config *pkgcfg.Config) {
				config.Features.KubeVMProvider = false
			})
			Expect(ctx.Client.Create(ctx, generic)).To(Succeed())

			wasMutated, err := mutation.ResolveKubeVMParentOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeFalse())
			Expect(vm.Spec.ClassName).To(BeEmpty())
		})
	})

	When("the annotation is absent", func() {
		It("does nothing", func() {
			vm.Annotations = nil

			wasMutated, err := mutation.ResolveKubeVMParentOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeFalse())
			Expect(vm.Spec.ClassName).To(BeEmpty())
		})
	})

	When("the named generic object does not exist", func() {
		It("returns an error and mutates nothing", func() {
			_, err := mutation.ResolveKubeVMParentOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
			Expect(err).To(HaveOccurred())
			Expect(vm.Spec.ClassName).To(BeEmpty())
		})
	})

	When("the reference is one-sided: the generic object's infrastructureRef names a different VM", func() {
		It("returns an error and mutates nothing", func() {
			generic.Spec.InfrastructureRef.Name = "some-other-vm"
			Expect(ctx.Client.Create(ctx, generic)).To(Succeed())

			_, err := mutation.ResolveKubeVMParentOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
			Expect(err).To(HaveOccurred())
			Expect(vm.Spec.ClassName).To(BeEmpty())
		})
	})

	When("the generic object mutually names this VM", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, generic)).To(Succeed())
		})

		It("fills every empty delegated field", func() {
			wasMutated, err := mutation.ResolveKubeVMParentOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeTrue())

			Expect(vm.Spec.ClassName).To(Equal("best-effort-2xlarge"))
			Expect(vm.Spec.Image).To(Equal(&vmopv1.VirtualMachineImageRef{
				Kind: "VirtualMachineImage",
				Name: "vmi-0123456789abcdef0",
			}))
			Expect(vm.Spec.StorageClass).To(Equal("my-storage-class"))
			Expect(vm.Spec.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn))
		})

		It("does not overwrite a field the user already set", func() {
			vm.Spec.ClassName = "user-set-class"

			wasMutated, err := mutation.ResolveKubeVMParentOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeTrue())

			Expect(vm.Spec.ClassName).To(Equal("user-set-class"))
			// Fields not explicitly set by the user are still resolved.
			Expect(vm.Spec.StorageClass).To(Equal("my-storage-class"))
		})

		When("the generic object's fields are partly empty", func() {
			It("only fills the fields the generic object has set", func() {
				generic.Spec.InstanceType = nil
				Expect(ctx.Client.Update(ctx, generic)).To(Succeed())

				wasMutated, err := mutation.ResolveKubeVMParentOnCreate(&ctx.WebhookRequestContext, ctx.Client, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())

				Expect(vm.Spec.ClassName).To(BeEmpty())
				Expect(vm.Spec.StorageClass).To(Equal("my-storage-class"))
			})
		})
	})
}
