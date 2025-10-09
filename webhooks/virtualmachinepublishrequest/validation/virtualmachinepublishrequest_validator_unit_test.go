// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
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
	vmPub    *vmopv1.VirtualMachinePublishRequest
	oldVMPub *vmopv1.VirtualMachinePublishRequest
	vm       *vmopv1.VirtualMachine
	cl       *imgregv1a1.ContentLibrary
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	vm := builder.DummyVirtualMachine()
	vm.Name = "dummy-vm"
	vm.Namespace = "dummy-ns"
	cl := builder.DummyContentLibrary("dummy-cl", "dummy-ns", "dummy-uuid")

	vmPub := builder.DummyVirtualMachinePublishRequest("dummy-vmpub", "dummy-ns", vm.Name,
		"dummy-item", cl.Name)
	obj, err := builder.ToUnstructured(vmPub)
	Expect(err).ToNot(HaveOccurred())

	var oldVMPub *vmopv1.VirtualMachinePublishRequest
	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldVMPub = vmPub.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldVMPub)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, vm, cl),
		vmPub:                               vmPub,
		oldVMPub:                            oldVMPub,
		vm:                                  vm,
		cl:                                  cl,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
		err error

		invalidAPIVersion = "vmoperator.vmware.com/v1"
	)

	type createArgs struct {
		invalidSourceAPIVersion         bool
		invalidSourceKind               bool
		invalidTargetLocationAPIVersion bool
		invalidTargetLocationKind       bool
		sourceNotFound                  bool
		defaultSourceNotFound           bool
		targetLocationNotWritable       bool
		targetLocationNameEmpty         bool
		targetLocationNotFound          bool
		targetItemAlreadyExists         bool
		managedByLabelExists            bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.managedByLabelExists {
			ctx.IsPrivilegedAccount = false
			ctx.vmPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ""}
		}

		if args.invalidSourceAPIVersion {
			ctx.vmPub.Spec.Source.APIVersion = invalidAPIVersion
		}

		if args.invalidSourceKind {
			ctx.vmPub.Spec.Source.Kind = "Machine"
		}

		if args.invalidTargetLocationAPIVersion {
			ctx.vmPub.Spec.Target.Location.APIVersion = invalidAPIVersion
		}

		if args.invalidTargetLocationKind {
			ctx.vmPub.Spec.Target.Location.Kind = "ClusterContentLibrary"
		}

		if args.sourceNotFound {
			Expect(ctx.Client.Delete(ctx, ctx.vm)).To(Succeed())
		}

		if args.defaultSourceNotFound {
			ctx.vmPub.Spec.Source.Name = ""
		}

		if args.targetLocationNotWritable {
			ctx.cl.Spec.Writable = false
			Expect(ctx.Client.Update(ctx, ctx.cl)).To(Succeed())
		}

		if args.targetLocationNameEmpty {
			ctx.vmPub.Spec.Target.Location.Name = ""
		}

		if args.targetLocationNotFound {
			Expect(ctx.Client.Delete(ctx, ctx.cl)).To(Succeed())
		}

		if args.targetItemAlreadyExists {
			clItem := utils.DummyContentLibraryItem("dummy-item", ctx.vmPub.Namespace)
			Expect(ctx.Client.Create(ctx, clItem)).To(Succeed())
			clItem.Status.Name = ctx.vmPub.Spec.Target.Item.Name
			Expect(ctx.Client.Status().Update(ctx, clItem)).To(Succeed())
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmPub)
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

	sourcePath := field.NewPath("spec").Child("source")
	targetLocationPath := field.NewPath("spec").Child("target", "location")
	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, nil, nil),
		Entry("should deny invalid source API version", createArgs{invalidSourceAPIVersion: true}, false,
			field.NotSupported(sourcePath.Child("apiVersion"), invalidAPIVersion,
				[]string{"vmoperator.vmware.com/v1alpha1", "vmoperator.vmware.com/v1alpha2", "vmoperator.vmware.com/v1alpha3", "vmoperator.vmware.com/v1alpha4", "vmoperator.vmware.com/v1alpha5", ""}).Error(), nil),
		Entry("should deny invalid source kind", createArgs{invalidSourceKind: true}, false,
			field.NotSupported(sourcePath.Child("kind"), "Machine",
				[]string{"VirtualMachine", ""}).Error(), nil),
		Entry("should deny invalid target location API version", createArgs{invalidTargetLocationAPIVersion: true}, false,
			field.NotSupported(targetLocationPath.Child("apiVersion"), invalidAPIVersion,
				[]string{"imageregistry.vmware.com/v1alpha1", "imageregistry.vmware.com/v1alpha2", ""}).Error(), nil),
		Entry("should deny invalid target location kind", createArgs{invalidTargetLocationKind: true}, false,
			field.NotSupported(targetLocationPath.Child("kind"), "ClusterContentLibrary",
				[]string{"ContentLibrary", ""}).Error(), nil),
		Entry("should deny if target location name is empty", createArgs{targetLocationNameEmpty: true}, false,
			field.Required(targetLocationPath.Child("name"), "").Error(), nil),
		Entry("should deny if managed-by label is set by a non privileged user", createArgs{managedByLabelExists: true}, false,
			"", fmt.Errorf("cannot add the %q label without a VirtualMachineGroupPublishRequest owner reference",
				vmopv1.VirtualMachinePublishRequestManagedByLabelKey)),
	)

	type createArgsAnnotations struct {
		annotationKey           string
		privilegedUser          bool
		supervisorProviderAdmin bool
		expectedReason          string
		expectedErr             error
	}

	validateCreateQuotaAnnotations := func(args createArgsAnnotations) {
		ctx.vmPub.Annotations = map[string]string{args.annotationKey: "doesn't matter"}
		ctx.IsPrivilegedAccount = args.privilegedUser

		if args.supervisorProviderAdmin {
			ctx.UserInfo.Groups = append(ctx.UserInfo.Groups, "sso:SupervisorProviderAdministrators@vsphere.local")
		}
		ctx.UserInfo.Groups = append(ctx.UserInfo.Groups, "sso:Administrators@vsphere.local")

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmPub)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(args.privilegedUser || args.supervisorProviderAdmin))
		if args.expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(args.expectedReason))
		}
		if args.expectedErr != nil {
			Expect(response.Result.Message).To(Equal(args.expectedErr.Error()))
		}
	}

	annotationPath := field.NewPath("metadata", "annotations")
	DescribeTable("create quota annotations table", validateCreateQuotaAnnotations,
		// vmware-system-vcfa-storage-quota
		Entry("adding the check annotation should be allowed for a privileged user",
			createArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaPerformCheckAnnotationKey,
				privilegedUser: true,
			}),
		Entry("adding the check annotation should be allowed for a user in the privileged group",
			createArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaPerformCheckAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("adding the check annotation should be denied for an unprivileged user",
			createArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaPerformCheckAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaPerformCheckAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),

		// vcfa.storage.quota.check.vmware.com/requested-capacity
		Entry("adding the requested capacity annotation should be allowed for a privileged user",
			createArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey,
				privilegedUser: true,
			}),
		Entry("adding the requested capacity annotation should be allowed for a user in the privileged group",
			createArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("adding the requested capacity annotation should be denied for an unprivileged user",
			createArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),

		// vcfa.storage.quota.check.vmware.com/status
		Entry("adding the status annotation should be allowed for a privileged user",
			createArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaCheckStatusAnnotationKey,
				privilegedUser: true,
			}),
		Entry("adding the status annotation should be allowed for a user in the privileged group",
			createArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaCheckStatusAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("adding the status annotation should be denied for an unprivileged user",
			createArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaCheckStatusAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaCheckStatusAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),

		// vcfa.storage.quota.check.vmware.com/message
		Entry("adding the message annotation should be allowed for a privileged user",
			createArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaCheckMessageAnnotationKey,
				privilegedUser: true,
			}),
		Entry("adding the message annotation should be allowed for a user in the privileged group",
			createArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaCheckMessageAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("adding the message annotation should be denied for an unprivileged user",
			createArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaCheckMessageAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaCheckMessageAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),
	)

}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
		err      error
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})

	AfterEach(func() {
		ctx = nil
	})

	When("a publish request is updated", func() {
		JustBeforeEach(func() {
			response = ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		})

		Context("Source/Target is updated", func() {
			BeforeEach(func() {
				ctx.vmPub.Spec.Source.Name = "updated-vm"
				ctx.vmPub.Spec.Target.Location.Name = "updated-cl"
				ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmPub)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should not allow the request", func() {
				Expect(response.Allowed).To(BeFalse())
				Expect(response.Result).ToNot(BeNil())
				Expect(string(response.Result.Reason)).To(ContainSubstring("field is immutable"))
			})
		})

		Context("Managed-by label is updated", func() {
			BeforeEach(func() {
				ctx.IsPrivilegedAccount = false
			})

			When("try to add managed-by label", func() {
				BeforeEach(func() {
					ctx.vmPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ""}
					ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vmPub)
					Expect(err).ToNot(HaveOccurred())
				})
				It("should not allow the request", func() {
					Expect(response.Allowed).To(BeFalse())
					Expect(response.Result.Message).To(Equal(
						fmt.Sprintf("cannot add the %q label without a VirtualMachineGroupPublishRequest owner reference",
							vmopv1.VirtualMachinePublishRequestManagedByLabelKey)))
				})
			})

			When("try to update when object has the managed-by label", func() {
				BeforeEach(func() {
					ctx.oldVMPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ""}
					ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMPub)
					Expect(err).ToNot(HaveOccurred())
				})
				It("should not allow the request", func() {
					Expect(response.Allowed).To(BeFalse())
					Expect(response.Result.Message).To(ContainSubstring("VirtualMachineGroupPublishRequest owned"))
				})
			})

			When("try to update when object has the managed-by label and user is privileged", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = true
					ctx.oldVMPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ""}
					ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMPub)
					Expect(err).ToNot(HaveOccurred())
				})
				It("should not allow the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})

			When("try to update when object has the managed-by label and user has privileged group", func() {
				BeforeEach(func() {
					ctx.IsPrivilegedAccount = false
					ctx.UserInfo.Groups = append(ctx.UserInfo.Groups, "sso:SupervisorProviderAdministrators@vsphere.local")
					ctx.oldVMPub.Labels = map[string]string{vmopv1.VirtualMachinePublishRequestManagedByLabelKey: ""}
					ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMPub)
					Expect(err).ToNot(HaveOccurred())
				})
				It("should not allow the request", func() {
					Expect(response.Allowed).To(BeTrue())
				})
			})
		})
	})

	type updateArgsAnnotations struct {
		annotationKey           string
		privilegedUser          bool
		supervisorProviderAdmin bool
		expectedReason          string
		expectedErr             error
	}

	validateUpdateQuotaAnnotations := func(args updateArgsAnnotations) {
		ctx.oldVMPub.Annotations = map[string]string{args.annotationKey: "doesn't matter"}
		ctx.IsPrivilegedAccount = args.privilegedUser

		if args.supervisorProviderAdmin {
			ctx.UserInfo.Groups = append(ctx.UserInfo.Groups, "sso:SupervisorProviderAdministrators@vsphere.local")
		}
		ctx.UserInfo.Groups = append(ctx.UserInfo.Groups, "sso:Administrators@vsphere.local")

		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVMPub)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(args.privilegedUser || args.supervisorProviderAdmin))
		if args.expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(args.expectedReason))
		}
		if args.expectedErr != nil {
			Expect(response.Result.Message).To(Equal(args.expectedErr.Error()))
		}
	}

	annotationPath := field.NewPath("metadata", "annotations")
	DescribeTable("update quota annotations table", validateUpdateQuotaAnnotations,
		// vmware-system-vcfa-storage-quota
		Entry("removing the check annotation should be allowed for a privileged user",
			updateArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaPerformCheckAnnotationKey,
				privilegedUser: true,
			}),
		Entry("removing the check annotation should be allowed for a user in the privileged group",
			updateArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaPerformCheckAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("removing the check annotation should be denied for an unprivileged user",
			updateArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaPerformCheckAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaPerformCheckAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),

		// vcfa.storage.quota.check.vmware.com/requested-capacity
		Entry("removing the requested capacity annotation should be allowed for a privileged user",
			updateArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey,
				privilegedUser: true,
			}),
		Entry("removing the requested capacity annotation should be allowed for a user in the privileged group",
			updateArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("removing the requested capacity annotation should be denied for an privileged user",
			updateArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaCheckRequestedCapacityAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),

		// vcfa.storage.quota.check.vmware.com/status
		Entry("removing the status annotation should be allowed for a privileged user",
			updateArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaCheckStatusAnnotationKey,
				privilegedUser: true,
			}),
		Entry("removing the status annotation should be allowed for a user in the privileged group",
			updateArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaCheckStatusAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("removing the status annotation should be denied for an unprivileged user",
			updateArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaCheckStatusAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaCheckStatusAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),

		// vcfa.storage.quota.check.vmware.com/message
		Entry("removing the message annotation should be allowed for a privileged user",
			updateArgsAnnotations{
				annotationKey:  pkgconst.AsyncQuotaCheckMessageAnnotationKey,
				privilegedUser: true,
			}),
		Entry("removing the message annotation should be allowed for a user in the privileged group",
			updateArgsAnnotations{
				annotationKey:           pkgconst.AsyncQuotaCheckMessageAnnotationKey,
				supervisorProviderAdmin: true,
			}),
		Entry("removing the message annotation should be denied for an privileged user",
			updateArgsAnnotations{
				annotationKey: pkgconst.AsyncQuotaCheckMessageAnnotationKey,
				expectedReason: field.Forbidden(annotationPath.Key(pkgconst.AsyncQuotaCheckMessageAnnotationKey),
					"modifying this annotation is not allowed for non-admin users").Error(),
			}),
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
