// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	testlabels "github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	dummyNamespace  = "dummy-ns"
	dummyKey        = "team"
	dummyValue      = "blue"
	dummyOtherValue = "red"

	// devOpsUser is an arbitrary, non-privileged, non-allow-listed username
	// used to exercise the "must be denied" side of the privileged check.
	devOpsUser = "sso:devops-user"

	// otherKubeSystemServiceAccount is a kube-system service account that is
	// NOT on the allow-list. It must still be denied.
	otherKubeSystemServiceAccount = "system:serviceaccount:kube-system:some-other-controller"

	allowListedGC        = "system:serviceaccount:kube-system:generic-garbage-collector"
	allowListedNamespace = "system:serviceaccount:kube-system:namespace-controller"
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
		unitTestsValidateTagCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateTagUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateTagDelete,
	)
}

// dummyTag returns a Tag whose metadata.name is the derived name for
// dummyKey/dummyValue, so it passes the derived-name check out of the box.
func dummyTag() *vspherepolv1.Tag {
	return &vspherepolv1.Tag{
		ObjectMeta: metav1.ObjectMeta{
			Name:      virtualmachine.TagResourceName(dummyKey, dummyValue),
			Namespace: dummyNamespace,
		},
		Spec: vspherepolv1.TagSpec{
			Key:   dummyKey,
			Value: dummyValue,
		},
	}
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook

	tag    *vspherepolv1.Tag
	tagOld *vspherepolv1.Tag
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	tag := dummyTag()
	obj, err := builder.ToUnstructured(tag)
	Expect(err).ToNot(HaveOccurred())

	var oldTag *vspherepolv1.Tag

	var oldObj *unstructured.Unstructured

	if isUpdate {
		oldTag = tag.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldTag)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		tag:                                 tag,
		tagOld:                              oldTag,
	}
}

func unitTestsValidateTagCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		invalidKey   bool
		invalidValue bool
		emptyValue   bool
		wrongName    bool
		isPrivileged bool
		username     string
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		var err error

		if args.isPrivileged {
			ctx.IsPrivilegedAccount = true
		}

		if args.username != "" {
			ctx.UserInfo.Username = args.username
		}

		if args.invalidKey {
			ctx.tag.Spec.Key = ""
		}

		if args.invalidValue {
			ctx.tag.Spec.Value = "not a valid value!"
		}

		if args.emptyValue {
			ctx.tag.Spec.Value = ""
			ctx.tag.Name = virtualmachine.TagResourceName(ctx.tag.Spec.Key, "")
		}

		if args.wrongName {
			ctx.tag.Name = "tag-wrong-name"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.tag)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("create table", validateCreate,
		Entry("privileged account, valid label key/value/derived-name should allow",
			createArgs{isPrivileged: true}, true, ""),
		Entry("privileged account, empty value is permitted should allow",
			createArgs{isPrivileged: true, emptyValue: true}, true, ""),
		Entry("privileged account, empty/invalid label key should deny",
			createArgs{isPrivileged: true, invalidKey: true}, false,
			"spec.key"),
		Entry("privileged account, invalid label value should deny",
			createArgs{isPrivileged: true, invalidValue: true}, false,
			"spec.value"),
		Entry("privileged account, name not matching derived name should deny",
			createArgs{isPrivileged: true, wrongName: true}, false,
			"metadata.name"),
		Entry("DevOps user, otherwise-valid spec should deny",
			createArgs{username: devOpsUser}, false,
			"only privileged users may CREATE a Tag"),
		Entry("DevOps user, invalid spec should still deny as unprivileged",
			createArgs{username: devOpsUser, invalidKey: true}, false,
			"only privileged users may CREATE a Tag"),
	)
}

func unitTestsValidateTagUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		changeKey    bool
		changeValue  bool
		invalidKey   bool
		invalidValue bool
		isPrivileged bool
		username     string
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string) {
		var err error

		if args.isPrivileged {
			ctx.IsPrivilegedAccount = true
		}

		if args.username != "" {
			ctx.UserInfo.Username = args.username
		}

		if args.changeKey {
			ctx.tag.Spec.Key = "other-key"
		}

		if args.changeValue {
			ctx.tag.Spec.Value = dummyOtherValue
		}

		if args.invalidKey {
			ctx.tag.Spec.Key = ""
		}

		if args.invalidValue {
			ctx.tag.Spec.Value = "not a valid value!"
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.tag)
		Expect(err).ToNot(HaveOccurred())

		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.tagOld)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})
	AfterEach(func() {
		ctx = nil
	})

	specKeyPath := field.NewPath("spec", "key")
	specValuePath := field.NewPath("spec", "value")

	DescribeTable("update table", validateUpdate,
		Entry("privileged account, label key/value unchanged should allow",
			updateArgs{isPrivileged: true}, true, ""),
		Entry("privileged account, label key changed should deny",
			updateArgs{isPrivileged: true, changeKey: true}, false,
			field.Forbidden(specKeyPath, "field is immutable").Error()),
		Entry("privileged account, value changed should deny",
			updateArgs{isPrivileged: true, changeValue: true}, false,
			field.Forbidden(specValuePath, "field is immutable").Error()),
		Entry("privileged account, invalid label key on update should deny",
			updateArgs{isPrivileged: true, invalidKey: true}, false,
			"spec.key"),
		Entry("privileged account, invalid value on update should deny",
			updateArgs{isPrivileged: true, invalidValue: true}, false,
			"spec.value"),
		Entry("DevOps user, otherwise-valid update should deny",
			updateArgs{username: devOpsUser}, false,
			"only privileged users may UPDATE a Tag"),
		Entry("generic-garbage-collector, otherwise-valid update should allow (allow-listed)",
			updateArgs{username: allowListedGC}, true, ""),
		Entry("namespace-controller, otherwise-valid update should allow (allow-listed)",
			updateArgs{username: allowListedNamespace}, true, ""),
		Entry("arbitrary other kube-system service account should still deny",
			updateArgs{username: otherKubeSystemServiceAccount}, false,
			"only privileged users may UPDATE a Tag"),
	)
}

func unitTestsValidateTagDelete() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type deleteArgs struct {
		isPrivileged bool
		username     string
	}

	validateDelete := func(args deleteArgs, expectedAllowed bool, expectedReason string) {
		var err error

		if args.isPrivileged {
			ctx.IsPrivilegedAccount = true
		}

		if args.username != "" {
			ctx.UserInfo.Username = args.username
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.tag)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateDelete(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	DescribeTable("delete table", validateDelete,
		Entry("privileged account should allow",
			deleteArgs{isPrivileged: true}, true, ""),
		Entry("DevOps user should deny",
			deleteArgs{username: devOpsUser}, false,
			"only privileged users may DELETE a Tag"),
		Entry("generic-garbage-collector should allow (allow-listed)",
			deleteArgs{username: allowListedGC}, true, ""),
		Entry("namespace-controller should allow (allow-listed)",
			deleteArgs{username: allowListedNamespace}, true, ""),
		Entry("arbitrary other kube-system service account should still deny",
			deleteArgs{username: otherKubeSystemServiceAccount}, false,
			"only privileged users may DELETE a Tag"),
	)
}
