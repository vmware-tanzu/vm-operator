// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// policyCounter ensures each dummyConfigPolicy/dummyZone has a unique name,
// since the integration tests share a single envtest API server across
// parallel specs.
var policyCounter atomic.Int64

func dummyZone(namespace string) *topologyv1.Zone {
	return &topologyv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("zone-%d", policyCounter.Add(1)),
			Namespace: namespace,
		},
	}
}

func dummyConfigPolicy(namespace, zoneName string) *vimv1.VirtualMachineConfigPolicy {
	return &vimv1.VirtualMachineConfigPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("policy-%d", policyCounter.Add(1)),
			Namespace: namespace,
		},
		Spec: vimv1.VirtualMachineConfigPolicySpec{
			Zone: zoneName,
		},
	}
}

// -----------------------------------------------------------------------
// Unit tests
// -----------------------------------------------------------------------

func unitTests() {
	Describe(
		"Create",
		Label(testlabels.Create, testlabels.API, testlabels.Validation, testlabels.Webhook),
		unitTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(testlabels.Update, testlabels.API, testlabels.Validation, testlabels.Webhook),
		unitTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(testlabels.Delete, testlabels.API, testlabels.Validation, testlabels.Webhook),
		unitTestsValidateDelete,
	)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook

	zone         *topologyv1.Zone
	configPolicy *vimv1.VirtualMachineConfigPolicy
}

// newUnitTestContextForValidatingWebhook builds a policy whose spec.zone
// references a freshly-generated Zone. When seedZone is true, that Zone is
// seeded into the fake client so the webhook's live lookup finds it;
// otherwise the lookup is expected to fail with not-found.
func newUnitTestContextForValidatingWebhook(isUpdate, seedZone bool) *unitValidatingWebhookContext {
	const namespace = "dummy-ns"

	zone := dummyZone(namespace)
	configPolicy := dummyConfigPolicy(namespace, zone.Name)

	obj, err := builder.ToUnstructured(configPolicy)
	Expect(err).ToNot(HaveOccurred())

	var oldObj *unstructured.Unstructured
	if isUpdate {
		oldObj, err = builder.ToUnstructured(configPolicy.DeepCopy())
		Expect(err).ToNot(HaveOccurred())
	}

	var initObjects []ctrlclient.Object
	if seedZone {
		initObjects = append(initObjects, zone)
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, initObjects...),
		zone:                                zone,
		configPolicy:                        configPolicy,
	}
}

func unitTestsValidateCreate() {
	type createArgs struct {
		seedZone       bool
		vmOperatorUser bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		ctx := newUnitTestContextForValidatingWebhook(false, args.seedZone)
		ctx.IsVMOperatorAccount = args.vmOperatorUser

		var err error

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.configPolicy)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	DescribeTable("create table", validateCreate,
		Entry("should allow when spec.zone references an existing Zone",
			createArgs{seedZone: true}, true, ""),
		Entry("should deny when spec.zone references a non-existent Zone",
			createArgs{}, false, "Not found"),
		Entry("should allow the VM Operator service account even when spec.zone references a non-existent Zone",
			createArgs{vmOperatorUser: true}, true, ""),
	)
}

func unitTestsValidateUpdate() {
	When("the update keeps a valid spec.zone", func() {
		It("should allow the request", func() {
			ctx := newUnitTestContextForValidatingWebhook(true, true)

			var err error

			ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.configPolicy)
			Expect(err).ToNot(HaveOccurred())

			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeTrue())
		})
	})

	When("the update changes spec.zone to a non-existent Zone", func() {
		It("should deny the request", func() {
			ctx := newUnitTestContextForValidatingWebhook(true, true)
			ctx.configPolicy.Spec.Zone = "some-other-zone"

			var err error

			ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.configPolicy)
			Expect(err).ToNot(HaveOccurred())

			response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
			Expect(response.Allowed).To(BeFalse())
		})
	})
}

func unitTestsValidateDelete() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false, true)
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

// -----------------------------------------------------------------------
// Integration (envtest) tests
// -----------------------------------------------------------------------

func intgTests() {
	Describe(
		"Create",
		Label(testlabels.Create, testlabels.EnvTest, testlabels.API, testlabels.Validation, testlabels.Webhook),
		intgTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(testlabels.Update, testlabels.EnvTest, testlabels.API, testlabels.Validation, testlabels.Webhook),
		intgTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(testlabels.Delete, testlabels.EnvTest, testlabels.API, testlabels.Validation, testlabels.Webhook),
		intgTestsValidateDelete,
	)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext

	zone         *topologyv1.Zone
	configPolicy *vimv1.VirtualMachineConfigPolicy
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.zone = dummyZone(ctx.Namespace)
	ctx.configPolicy = dummyConfigPolicy(ctx.Namespace, ctx.zone.Name)

	return ctx
}

func intgTestsValidateCreate() {
	var ctx *intgValidatingWebhookContext

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})
	AfterEach(func() {
		Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, ctx.configPolicy))).To(Succeed())
		Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, ctx.zone))).To(Succeed())
		ctx = nil
	})

	When("spec.zone references an existing Zone", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, ctx.zone)).To(Succeed())
		})

		It("should allow the request", func() {
			Expect(ctx.Client.Create(ctx, ctx.configPolicy)).To(Succeed())
		})
	})

	When("spec.zone references a non-existent Zone", func() {
		It("should deny the request", func() {
			err := ctx.Client.Create(ctx, ctx.configPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Not found"))
		})
	})

	When("an extraConfig entry has an empty key", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, ctx.zone)).To(Succeed())
			ctx.configPolicy.Spec.ExtraConfig = &vimv1.VirtualMachineConfigPolicyExtraConfigSpec{
				Allowed: []vimv1.VirtualMachineConfigPolicyExtraConfigKey{
					{Type: vimv1.MatchTypeFixed, Key: ""},
				},
			}
		})

		// This is rejected by the CRD's OpenAPI schema
		// (+kubebuilder:validation:MinLength=1 on
		// VirtualMachineConfigPolicyExtraConfigKey.Key) before the request
		// ever reaches the webhook, so this is a real envtest apiserver
		// admission rejection, not a webhook response.
		It("should deny the request with a schema validation error", func() {
			err := ctx.Client.Create(ctx, ctx.configPolicy)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("spec.extraConfig.allowed[0].key"))
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.zone)).To(Succeed())
		Expect(ctx.Client.Create(ctx, ctx.configPolicy)).To(Succeed())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.configPolicy)
	})
	AfterEach(func() {
		Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, ctx.configPolicy))).To(Succeed())
		Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, ctx.zone))).To(Succeed())

		err = nil
		ctx = nil
	})

	When("update keeps spec.zone unchanged", func() {
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("update repoints spec.zone at a non-existent Zone", func() {
		BeforeEach(func() {
			ctx.configPolicy.Spec.Zone = "some-other-zone"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Not found"))
		})
	})
}

func intgTestsValidateDelete() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.zone)).To(Succeed())
		Expect(ctx.Client.Create(ctx, ctx.configPolicy)).To(Succeed())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.configPolicy)
	})
	AfterEach(func() {
		Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, ctx.zone))).To(Succeed())

		err = nil
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
