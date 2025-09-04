// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateDelete,
	)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	vmSnapshot    *vmopv1.VirtualMachineSnapshot
	oldVMSnapshot *vmopv1.VirtualMachineSnapshot
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vmSnapshot := builder.DummyVirtualMachineSnapshot("dummy-ns", "dummy-vm-snapshot", "dummy-vm")
	obj, err := builder.ToUnstructured(vmSnapshot)
	Expect(err).ToNot(HaveOccurred())

	var oldVMSnapshot *vmopv1.VirtualMachineSnapshot
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMSnapshot = vmSnapshot.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMSnapshot)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		vmSnapshot:                          vmSnapshot,
		oldVMSnapshot:                       oldVMSnapshot,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
		err error
	)

	type createArgs struct {
		invalidVMRef                bool
		invalidVMRefKind            bool
		invalidVMRefAPIVersion      bool
		invalidVMRefAPIVersionGroup bool
		emptyVMRefName              bool
		createVKSNode               bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.invalidVMRef {
			ctx.vmSnapshot.Spec.VMRef = nil
		}

		if args.invalidVMRefKind {
			ctx.vmSnapshot.Spec.VMRef.Kind = "VMVM"
		}

		if args.invalidVMRefAPIVersion {
			ctx.vmSnapshot.Spec.VMRef.APIVersion = "foobar.com/v1/v2"
		}

		if args.invalidVMRefAPIVersionGroup {
			ctx.vmSnapshot.Spec.VMRef.APIVersion = "foobar.com/v99"
		}

		if args.emptyVMRefName {
			ctx.vmSnapshot.Spec.VMRef.Name = ""
		}

		if args.createVKSNode {
			// Create a VM with CAPI labels to simulate a VKS/TKG node
			vm := builder.DummyBasicVirtualMachine(ctx.vmSnapshot.Spec.VMRef.Name, ctx.vmSnapshot.Namespace)
			vm.Labels = map[string]string{
				kubeutil.CAPWClusterRoleLabelKey: "worker",
			}
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmSnapshot)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	vmRefField := field.NewPath("spec", "vmRef")

	DescribeTable("create table", validateCreate,
		Entry("should allow valid",
			createArgs{},
			true,
			nil,
			nil,
		),
		Entry("should deny nil vmRef",
			createArgs{invalidVMRef: true},
			false,
			field.Required(vmRefField, "vmRef must be provided").Error(),
			nil,
		),
		Entry("should deny a non-empty vmRef, with Kind other than VirtualMachine",
			createArgs{invalidVMRefKind: true},
			false,
			field.Invalid(vmRefField.Child("kind"), "VMVM", "must be \"VirtualMachine\"").Error(),
			nil,
		),
		Entry("should deny a non-empty vmRef, with invalid APIVersion",
			createArgs{invalidVMRefAPIVersion: true},
			false,
			field.Invalid(vmRefField.Child("apiVersion"), "foobar.com/v1/v2", "must be valid group version").Error(),
			nil,
		),
		Entry("should deny a non-empty vmRef, with invalid APIVersion Group",
			createArgs{invalidVMRefAPIVersionGroup: true},
			false,
			field.Invalid(vmRefField.Child("apiVersion"), "foobar.com/v99", fmt.Sprintf("group must be %q", vmopv1.GroupName)).Error(),
			nil,
		),
		Entry("should deny a non-empty vmRef, with no Name specified",
			createArgs{emptyVMRefName: true},
			false,
			field.Required(vmRefField.Child("name"), "name must be provided").Error(),
			nil,
		),
		Entry("should deny snapshot for VKS/TKG node",
			createArgs{createVKSNode: true},
			false,
			field.Forbidden(vmRefField, "snapshots are not allowed for VKS/TKG nodes").Error(),
			nil,
		),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx     *unitValidatingWebhookContext
		dummyVM = "dummy-vm"
	)

	type updateArgs struct {
		updateMemory      bool
		updateQuiesce     bool
		updateVMRef       bool
		updateVMNameLabel bool
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		ctx.vmSnapshot.Spec.Description = "a new description"

		if args.updateMemory {
			ctx.vmSnapshot.Spec.Memory = !ctx.vmSnapshot.Spec.Memory
		}

		if args.updateQuiesce {
			ctx.vmSnapshot.Spec.Quiesce = &vmopv1.QuiesceSpec{Timeout: &metav1.Duration{Duration: 120 * time.Second}}
		}

		if args.updateVMRef {
			ctx.vmSnapshot.Spec.VMRef = &common.LocalObjectRef{
				Name:       "another-vm",
				Kind:       "VirtualMachine",
				APIVersion: "vmoperator.vmware.com/v1alpha5",
			}
		}

		if args.updateVMNameLabel {
			// Try to change the VM name label to a different value
			metav1.SetMetaDataLabel(&ctx.vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, "different-vm-name")
		}

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmSnapshot)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMSnapshot)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))
		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(HaveSuffix(expectedReason))
		}
		if expectedErr != nil {
			Expect(response.Result.Message).To(Equal(expectedErr.Error()))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)

		// Set up both snapshots with the VM name label set by mutation webhook
		metav1.SetMetaDataLabel(&ctx.oldVMSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, dummyVM)

		// New snapshot should also have the label by default (simulating no change)
		metav1.SetMetaDataLabel(&ctx.vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, dummyVM)
	})

	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("update table", validateUpdate,
		Entry("should allow updating description", updateArgs{}, true, "", nil),
		Entry("should not allow updating memory",
			updateArgs{updateMemory: true},
			false,
			"field is immutable",
			nil,
		),
		Entry("should not allow updating quiesce",
			updateArgs{updateQuiesce: true},
			false,
			"field is immutable",
			nil,
		),
		Entry("should not allow updating vmRef",
			updateArgs{updateVMRef: true},
			false,
			"field is immutable",
			nil,
		),
		Entry("should not allow updating VM name label",
			updateArgs{updateVMNameLabel: true},
			false,
			"field is immutable",
			nil,
		),
	)
}

func unitTestsValidateDelete() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("the delete is performed", func() {
		JustBeforeEach(func() {
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
