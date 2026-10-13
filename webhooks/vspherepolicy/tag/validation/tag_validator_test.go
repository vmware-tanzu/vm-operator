// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	rbacvalidation "k8s.io/component-helpers/auth/rbac/validation"
	"sigs.k8s.io/yaml"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	testlabels "github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

const (
	dummyNamespace = "dummy-ns"
	dummyKey       = "team"
	dummyValue     = "blue"

	// devOpsUser is an arbitrary non-privileged username used to verify
	// that admission leaves authorization to RBAC.
	devOpsUser = "sso:devops-user"

	// otherKubeSystemServiceAccount is an arbitrary control-plane identity.
	otherKubeSystemServiceAccount = "system:serviceaccount:kube-system:some-other-controller"

	garbageCollector    = "system:serviceaccount:kube-system:generic-garbage-collector"
	namespaceController = "system:serviceaccount:kube-system:namespace-controller"
)

func unitTests() {
	Describe("RBAC manifests", Label(testlabels.API), unitTestsTagRBAC)
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
			Name:      virtualmachine.TagResourceName(dummyNamespace, dummyKey, dummyValue),
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
			ctx.tag.Name = virtualmachine.TagResourceName(ctx.tag.Namespace, ctx.tag.Spec.Key, "")
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
		Entry("valid label key/value/derived-name should allow",
			createArgs{isPrivileged: true}, true, ""),
		Entry("empty value is permitted should allow",
			createArgs{isPrivileged: true, emptyValue: true}, true, ""),
		Entry("empty/invalid label key should deny",
			createArgs{isPrivileged: true, invalidKey: true}, false,
			"spec.key"),
		Entry("invalid label value should deny",
			createArgs{isPrivileged: true, invalidValue: true}, false,
			"spec.value"),
		Entry("name not matching derived name should deny",
			createArgs{isPrivileged: true, wrongName: true}, false,
			"metadata.name"),
		Entry("generic-garbage-collector, valid spec is allowed by admission",
			createArgs{username: garbageCollector}, true, ""),
		Entry("namespace-controller, valid spec is allowed by admission",
			createArgs{username: namespaceController}, true, ""),
		Entry("other kube-system service account is allowed by admission",
			createArgs{username: otherKubeSystemServiceAccount}, true, ""),
		Entry("DevOps user, valid spec is allowed by admission",
			createArgs{username: devOpsUser}, true, ""),
		Entry("non-privileged account with an invalid spec is denied",
			createArgs{username: devOpsUser, invalidKey: true}, false,
			"spec.key"),
	)
}

func unitTestsValidateTagUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
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

	DescribeTable("update table", validateUpdate,
		Entry("label key/value unchanged should allow",
			updateArgs{isPrivileged: true}, true, ""),
		Entry("invalid label key on update should deny",
			updateArgs{isPrivileged: true, invalidKey: true}, false,
			"spec.key"),
		Entry("invalid value on update should deny",
			updateArgs{isPrivileged: true, invalidValue: true}, false,
			"spec.value"),
		Entry("DevOps user, valid update is allowed by admission",
			updateArgs{username: devOpsUser}, true, ""),
		Entry("generic-garbage-collector, valid update is allowed by admission",
			updateArgs{username: garbageCollector}, true, ""),
		Entry("namespace-controller, valid update is allowed by admission",
			updateArgs{username: namespaceController}, true, ""),
		Entry("other kube-system service account is allowed by admission",
			updateArgs{username: otherKubeSystemServiceAccount}, true, ""),
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
		Entry("DevOps user is allowed by admission",
			deleteArgs{username: devOpsUser}, true, ""),
		Entry("generic-garbage-collector is allowed by admission",
			deleteArgs{username: garbageCollector}, true, ""),
		Entry("namespace-controller is allowed by admission",
			deleteArgs{username: namespaceController}, true, ""),
		Entry("other kube-system service account is allowed by admission",
			deleteArgs{username: otherKubeSystemServiceAccount}, true, ""),
	)
}

func unitTestsTagRBAC() {
	It("grants Tag access to the bound manager role and excludes tenant roles", func() {
		rbacDir := filepath.Join(testutil.GetRootDirOrDie(), "config", "rbac")
		readRole := func(name string) rbacv1.ClusterRole {
			data, err := os.ReadFile(filepath.Join(rbacDir, name))
			Expect(err).ToNot(HaveOccurred())
			var role rbacv1.ClusterRole
			Expect(yaml.Unmarshal(data, &role)).To(Succeed())
			return role
		}

		managerRole := readRole("role.yaml")
		data, err := os.ReadFile(filepath.Join(rbacDir, "role_binding.yaml"))
		Expect(err).ToNot(HaveOccurred())
		var binding rbacv1.ClusterRoleBinding
		Expect(yaml.Unmarshal(data, &binding)).To(Succeed())
		Expect(binding.RoleRef).To(Equal(rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     managerRole.Name,
		}))
		Expect(binding.Subjects).To(ContainElement(rbacv1.Subject{
			Kind: "ServiceAccount", Name: "vmoperator-service-account", Namespace: "system",
		}))

		tenantPaths, err := filepath.Glob(filepath.Join(rbacDir, "*_editor_role.yaml"))
		Expect(err).ToNot(HaveOccurred())
		viewerPaths, err := filepath.Glob(filepath.Join(rbacDir, "*_viewer_role.yaml"))
		Expect(err).ToNot(HaveOccurred())
		tenantPaths = append(tenantPaths, viewerPaths...)
		Expect(tenantPaths).ToNot(BeEmpty())
		var tenantRules []rbacv1.PolicyRule
		for _, path := range tenantPaths {
			role := readRole(filepath.Base(path))
			tenantRules = append(tenantRules, role.Rules...)
		}

		for _, resource := range []string{"tags", "tags/status"} {
			for _, verb := range []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"} {
				rules := []rbacv1.PolicyRule{{
					APIGroups: []string{vspherepolv1.GroupVersion.Group},
					Resources: []string{resource},
					Verbs:     []string{verb},
				}}
				allowed, _ := rbacvalidation.Covers(tenantRules, rules)
				Expect(allowed).To(BeFalse(), "tenant roles must not grant %s on %s", verb, resource)

				if resource == "tags" && verb != "deletecollection" ||
					resource == "tags/status" && (verb == "get" || verb == "patch" || verb == "update") {
					allowed, _ = rbacvalidation.Covers(managerRole.Rules, rules)
					Expect(allowed).To(BeTrue(), "manager role must grant %s on %s", verb, resource)
				}
			}
		}
	})
}
