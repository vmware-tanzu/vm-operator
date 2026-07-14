// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	isoImageName    = "iso-image"
	nonISOImageName = "non-iso-image"
	isoCdromName    = "cdrom1"
	isoSecretName   = "my-vm-1-bootstrap-data"
)

func dummyISOTypeImage(name string, isoType bool) *vmopv1.VirtualMachineImage {
	imgType := string(imgregv1a1.ContentLibraryItemTypeOvf)
	if isoType {
		imgType = string(imgregv1a1.ContentLibraryItemTypeIso)
	}
	return &vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: dummyNamespaceName,
		},
		Status: vmopv1.VirtualMachineImageStatus{
			Type: imgType,
		},
	}
}

// newISOUnitTestContext builds a create/update validating-webhook context
// for a VM with an ISO cdrom already attached, mirroring
// newUnitTestContextForValidatingWebhook but adding image objects and
// allowing the caller to customize vm (and oldVM, for updates) before the
// request is serialized.
func newISOUnitTestContext(
	isUpdate bool,
	mutateVM func(vm *vmopv1.VirtualMachine),
	mutateOldVM func(oldVM *vmopv1.VirtualMachine)) *unitValidatingWebhookContext {

	vm := builder.DummyVirtualMachine()
	vm.Name = "dummy-vm"
	vm.Namespace = dummyNamespaceName
	vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
		{
			Name: isoCdromName,
			Image: vmopv1.VirtualMachineImageRef{
				Name: isoImageName,
				Kind: vmiKind,
			},
		},
	}
	vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
		ISO: &vmopv1.VirtualMachineBootstrapISOSpec{
			Commands: []string{"boot"},
		},
	}

	var oldVM *vmopv1.VirtualMachine
	var oldObj *unstructured.Unstructured
	if isUpdate {
		oldVM = vm.DeepCopy()
		if mutateOldVM != nil {
			mutateOldVM(oldVM)
		}
		o, err := builder.ToUnstructured(oldVM)
		Expect(err).ToNot(HaveOccurred())
		oldObj = o
	}

	if mutateVM != nil {
		mutateVM(vm)
	}

	obj, err := builder.ToUnstructured(vm)
	Expect(err).ToNot(HaveOccurred())

	az := builder.DummyAvailabilityZone()
	zone := builder.DummyZone(dummyNamespaceName)
	initObjects := []client.Object{
		az, zone,
		dummyISOTypeImage(isoImageName, true),
		dummyISOTypeImage(nonISOImageName, false),
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, initObjects...),
		vm:                                  vm,
		oldVM:                               oldVM,
	}
}

var _ = Describe("Validate ISO bootstrap", func() {
	Context("Create", func() {
		It("allows a VM with an ISO-type cdrom image", func() {
			ctx := newISOUnitTestContext(false, nil, nil)
			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})

		It("forbids ISO combined with CloudInit", func() {
			ctx := newISOUnitTestContext(false, func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Bootstrap.CloudInit = &vmopv1.VirtualMachineBootstrapCloudInitSpec{}
			}, nil)
			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring("ISO may not be used with any other bootstrap provider"))
		})

		It("requires at least one cdrom entry", func() {
			ctx := newISOUnitTestContext(false, func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Hardware.Cdrom = nil
			}, nil)
			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(
				"at least one CD-ROM referencing an ISO-type image is required"))
		})

		It("rejects a cdrom image that is not ISO-typed", func() {
			ctx := newISOUnitTestContext(false, func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Hardware.Cdrom[0].Image.Name = nonISOImageName
			}, nil)
			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(
				"at least one CD-ROM must reference an ISO-type image"))
		})

		It("rejects a cdrom image that does not exist", func() {
			ctx := newISOUnitTestContext(false, func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Hardware.Cdrom[0].Image.Name = "does-not-exist"
			}, nil)
			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring(
				"at least one CD-ROM must reference an ISO-type image"))
		})

		It("rejects an invalid asset secret name", func() {
			ctx := newISOUnitTestContext(false, func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Bootstrap.ISO.Assets = []vmopv1.VirtualMachineBootstrapISOAsset{
					{Name: "Not_A_Valid_Name", Key: "user-data"},
				}
			}, nil)
			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring("spec.bootstrap.iso.assets[0].name"))
		})

		It("allows a valid asset secret name", func() {
			ctx := newISOUnitTestContext(false, func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Bootstrap.ISO.Assets = []vmopv1.VirtualMachineBootstrapISOAsset{
					{Name: isoSecretName, Key: "user-data"},
				}
			}, nil)
			response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})
	})

	Context("Update", func() {
		It("allows changing commands when boot commands were never sent", func() {
			ctx := newISOUnitTestContext(true, func(vm *vmopv1.VirtualMachine) {
				vm.Spec.Bootstrap.ISO.Commands = []string{"boot", "<enter>"}
			}, nil)
			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})

		It("forbids changing commands after boot commands were sent while powered on", func() {
			ctx := newISOUnitTestContext(true,
				func(vm *vmopv1.VirtualMachine) {
					vm.Spec.Bootstrap.ISO.Commands = []string{"boot", "<enter>"}
				},
				func(oldVM *vmopv1.VirtualMachine) {
					oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					oldVM.Annotations[pkgconst.BootstrapISOHashAnnotationKey] = "some-hash"
				})
			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
			Expect(string(response.Result.Reason)).To(ContainSubstring("spec.bootstrap.iso.commands"))
		})

		It("allows an unchanged commands list after boot commands were sent while powered on", func() {
			ctx := newISOUnitTestContext(true,
				nil,
				func(oldVM *vmopv1.VirtualMachine) {
					oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
					oldVM.Annotations[pkgconst.BootstrapISOHashAnnotationKey] = "some-hash"
				})
			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})
	})
})
