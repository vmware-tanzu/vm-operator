// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	rbacvalidation "k8s.io/component-helpers/auth/rbac/validation"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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
			Name:      virtualmachine.ConcreteTagResourceName(dummyNamespace, dummyKey+":"+dummyValue),
			Namespace: dummyNamespace,
		},
		Spec: vspherepolv1.TagSpec{
			Key:        dummyKey,
			Value:      dummyValue,
			Name:       dummyKey + ":" + dummyValue,
			Kind:       vspherepolv1.TagKindOriginal,
			Membership: vspherepolv1.TagMembershipSourceLabel,
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
			ctx.tag.Spec.Name = ctx.tag.Spec.Key + ":"
			ctx.tag.Name = virtualmachine.ConcreteTagResourceName(ctx.tag.Namespace, ctx.tag.Spec.Name)
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
		Entry("generic-garbage-collector may not create",
			createArgs{username: garbageCollector}, false,
			"only privileged users may CREATE a Tag"),
		Entry("namespace-controller may not create",
			createArgs{username: namespaceController}, false,
			"only privileged users may CREATE a Tag"),
		Entry("other kube-system service account may not create",
			createArgs{username: otherKubeSystemServiceAccount}, false,
			"only privileged users may CREATE a Tag"),
		Entry("DevOps user, valid spec is denied",
			createArgs{username: devOpsUser}, false,
			"only privileged users may CREATE a Tag"),
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
		Entry("DevOps user, valid update is denied",
			updateArgs{username: devOpsUser}, false,
			"only privileged users may UPDATE a Tag"),
		Entry("generic-garbage-collector, valid update is allowed",
			updateArgs{username: garbageCollector}, true, ""),
		Entry("namespace-controller, valid update is denied",
			updateArgs{username: namespaceController}, false,
			"only privileged users may UPDATE a Tag"),
		Entry("other kube-system service account is denied",
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

var _ = Describe("VC Tag identity", Label(testlabels.API, testlabels.Validation, testlabels.Webhook), func() {
	policyHash := func(t virtualmachine.AffinityPolicyType, targets ...virtualmachine.LabelPair) virtualmachine.AffinityPolicyHash {
		h, err := virtualmachine.NewAffinityPolicyHash(
			t,
			vimtypes.VmPlacementPolicyVmPlacementPolicyStrictnessPreferredDuringPlacementPreferredDuringExecution,
			vimtypes.VmPlacementPolicyVmPlacementPolicyTopologyHost,
			targets)
		Expect(err).ToNot(HaveOccurred())
		return h
	}
	afHash := func() virtualmachine.AffinityPolicyHash {
		return policyHash(virtualmachine.AffinityPolicyTypeAffinity, virtualmachine.LabelPair{Key: "app", Value: "nginx"})
	}
	aafHash := func() virtualmachine.AffinityPolicyHash {
		return policyHash(virtualmachine.AffinityPolicyTypeGroupedAntiAffinity,
			virtualmachine.LabelPair{Key: "app", Value: "cache"},
			virtualmachine.LabelPair{Key: "app", Value: "database"})
	}
	derivedTag := func(self bool) *vspherepolv1.Tag {
		tag := dummyTag()
		tag.Spec.Kind = vspherepolv1.TagKindAffinityTarget
		tag.Spec.Policy = &vspherepolv1.TagPolicyIdentity{
			Type:       "VmVmAffinity",
			Strictness: "PreferredDuringPlacementPreferredDuringExecution",
			Topology:   "Host",
			Targets:    []vspherepolv1.TagLabelPair{{Key: "app", Value: "nginx"}},
		}
		tag.Spec.Key, tag.Spec.Value = "app", "nginx"
		tag.Spec.Name = virtualmachine.PrefixFreePolicyTagName(afHash())
		if self {
			tag.Spec.Kind = vspherepolv1.TagKindGroupedAntiAffinitySelf
			tag.Spec.Membership = vspherepolv1.TagMembershipPolicyRequesters
			tag.Spec.Key, tag.Spec.Value = "", ""
			tag.Spec.Name = virtualmachine.PrefixFreePolicyTagName(aafHash())
			tag.Spec.Policy.Type = "VmToVmGroupsAntiAffinity"
			tag.Spec.Policy.Targets = []vspherepolv1.TagLabelPair{{Key: "app", Value: "cache"}, {Key: "app", Value: "database"}}
		}
		tag.Name = virtualmachine.ConcreteTagResourceName(tag.Namespace, tag.Spec.Name)
		return tag
	}

	DescribeTable("validates canonical derived identities", func(self bool, mutate func(*vspherepolv1.Tag), reason string) {
		ctx := newUnitTestContextForValidatingWebhook(false)
		ctx.IsPrivilegedAccount = true
		tag := derivedTag(self)
		mutate(tag)
		var err error
		ctx.Obj, err = builder.ToUnstructured(tag)
		Expect(err).ToNot(HaveOccurred())
		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(reason == ""))
		if reason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(reason))
		}
	},
		Entry("valid affinity target", false, func(*vspherepolv1.Tag) {}, ""),
		Entry("valid grouped self", true, func(*vspherepolv1.Tag) {}, ""),
		Entry("missing policy", false, func(t *vspherepolv1.Tag) { t.Spec.Policy = nil }, "spec.policy"),
		Entry("wrong concrete hash", false, func(t *vspherepolv1.Tag) { t.Spec.Name = "af-pp-host-0000000000000000-app:nginx" }, "spec.name"),
	)

	It("allows ownership updates", func() {
		ctx := newUnitTestContextForValidatingWebhook(true)
		ctx.IsPrivilegedAccount = true
		tag := derivedTag(false)
		var err error
		ctx.OldObj, err = builder.ToUnstructured(tag)
		Expect(err).ToNot(HaveOccurred())
		tag.OwnerReferences = []metav1.OwnerReference{{APIVersion: "vmoperator.vmware.com/v1alpha6", Kind: "VirtualMachine", Name: "vm", UID: "vm-uid"}}
		ctx.Obj, err = builder.ToUnstructured(tag)
		Expect(err).ToNot(HaveOccurred())
		Expect(ctx.ValidateUpdate(&ctx.WebhookRequestContext).Allowed).To(BeTrue())
	})
})

func intgTests() {
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateUpdate,
	)
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateCreate,
	)
}

func intgTestsValidateCreate() {
	const sourceLabelRule = "source label is required for original/affinity tags and forbidden for self tags"

	var ctx *builder.IntegrationTestContext

	newTag := func(spec vspherepolv1.TagSpec) *vspherepolv1.Tag {
		return &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtualmachine.ConcreteTagResourceName(ctx.Namespace, spec.Name),
				Namespace: ctx.Namespace,
			},
			Spec: spec,
		}
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	// The source label rule is a CEL rule on the CRD, so like immutability it
	// can only be verified against a real API server.
	When("an original Tag has no source label", func() {
		It("should deny the request", func() {
			tag := newTag(vspherepolv1.TagSpec{
				Name:       "missing",
				Kind:       vspherepolv1.TagKindOriginal,
				Membership: vspherepolv1.TagMembershipSourceLabel,
			})
			err := ctx.Client.Create(ctx, tag)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(sourceLabelRule))
		})
	})

	When("a self Tag has a source label", func() {
		It("should deny the request", func() {
			tag := newTag(vspherepolv1.TagSpec{
				Name:       "self",
				Kind:       vspherepolv1.TagKindGroupedAntiAffinitySelf,
				Membership: vspherepolv1.TagMembershipPolicyRequesters,
				Key:        dummyKey,
				Value:      dummyValue,
			})
			err := ctx.Client.Create(ctx, tag)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(sourceLabelRule))
		})
	})

	When("an original Tag has an empty label value", func() {
		It("should allow the request and keep the identity", func() {
			name := virtualmachine.PrefixFreeTagName(dummyKey, "")
			tag := newTag(vspherepolv1.TagSpec{
				Name:       name,
				Kind:       vspherepolv1.TagKindOriginal,
				Membership: vspherepolv1.TagMembershipSourceLabel,
				Key:        dummyKey,
			})
			Expect(ctx.Client.Create(ctx, tag)).To(Succeed())

			got := &vspherepolv1.Tag{}
			Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKeyFromObject(tag), got)).To(Succeed())
			Expect(got.Spec.Key).To(Equal(dummyKey))
			Expect(got.Spec.Value).To(BeEmpty())
			Expect(got.Spec.Name).To(Equal(name))
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		ctx *builder.IntegrationTestContext
		tag *vspherepolv1.Tag
	)

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		name := dummyKey + ":" + dummyValue
		tag = &vspherepolv1.Tag{
			ObjectMeta: metav1.ObjectMeta{
				Name:      virtualmachine.ConcreteTagResourceName(ctx.Namespace, name),
				Namespace: ctx.Namespace,
			},
			Spec: vspherepolv1.TagSpec{
				Key:        dummyKey,
				Value:      dummyValue,
				Name:       name,
				Kind:       vspherepolv1.TagKindOriginal,
				Membership: vspherepolv1.TagMembershipSourceLabel,
			},
		}
		Expect(ctx.Client.Create(ctx, tag)).To(Succeed())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	// Spec immutability is enforced by the CEL transition rule on the CRD,
	// not by the webhook, so this can only be verified against a real API
	// server.
	When("spec is changed", func() {
		It("should deny the request", func() {
			tag.Spec.Value = "red"
			err := ctx.Client.Update(ctx, tag)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tag identity is immutable"))
		})
	})

	When("only metadata is changed", func() {
		It("should allow the request", func() {
			tag.Labels = map[string]string{"foo": "bar"}
			Expect(ctx.Client.Update(ctx, tag)).To(Succeed())
		})
	})
}
