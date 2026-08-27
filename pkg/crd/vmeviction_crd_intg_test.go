// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package crd_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgcrd "github.com/vmware-tanzu/vm-operator/pkg/crd"
)

// These tests apply actual AutomaticVMEvictionPolicy/BestEffortRestartPolicy
// resources to a real kube-apiserver to verify the generated CRDs' kubebuilder
// markers (required fields, string length bounds, defaulting, status
// subresource) are enforced. The fake client used elsewhere skips OpenAPI
// schema validation entirely, so only a real apiserver can catch a marker
// that was dropped or miscopied during code generation.
var _ = Describe(
	"AutomaticVMEvictionPolicy and BestEffortRestartPolicy schema",
	Label(testlabels.EnvTest),
	func() {

		var (
			ctx       context.Context
			client    ctrlclient.Client
			namespace string
		)

		BeforeEach(func() {
			ctx = pkgcfg.WithConfig(pkgcfg.Config{
				CRDCleanupEnabled: true,
				Features: pkgcfg.FeatureStates{
					VSpherePolicies: true,
					VMEviction:      true,
				},
			})
			Expect(pkgcrd.Install(ctx, envTestClient, nil)).To(Succeed())

			// The apiserver establishes a newly created CRD's REST endpoint
			// asynchronously. The typed client built below defaults to a
			// dynamic RESTMapper that discovers GVK-to-resource mappings
			// from the apiserver, so building it before the CRD is
			// Established can race ahead and miss the new resource. This
			// is unrelated to AddToScheme below, which only registers Go
			// types in-memory and never talks to the apiserver.
			for _, crdName := range []string{
				"automaticvmevictionpolicies.vsphere.policy.vmware.com",
				"besteffortrestartpolicies.vsphere.policy.vmware.com",
			} {
				Eventually(func(g Gomega) {
					crd := &apiextensionsv1.CustomResourceDefinition{}
					g.Expect(envTestClient.Get(
						ctx,
						ctrlclient.ObjectKey{Name: crdName},
						crd)).To(Succeed())

					established := false
					for _, cond := range crd.Status.Conditions {
						if cond.Type == apiextensionsv1.Established &&
							cond.Status == apiextensionsv1.ConditionTrue {
							established = true
						}
					}
					g.Expect(established).To(BeTrue())
				}).Should(Succeed())
			}

			scheme := runtime.NewScheme()
			Expect(vspherepolv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			var err error
			client, err = ctrlclient.New(envTestEnv.Config, ctrlclient.Options{Scheme: scheme})
			Expect(err).ToNot(HaveOccurred())

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "vmevacuation-crd-test-"},
			}
			Expect(client.Create(ctx, ns)).To(Succeed())
			namespace = ns.Name
		})

		newAutomaticVMEvictionPolicy := func(policyID string) *vspherepolv1.AutomaticVMEvictionPolicy {
			return &vspherepolv1.AutomaticVMEvictionPolicy{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "auto-host-evac-",
					Namespace:    namespace,
				},
				Spec: vspherepolv1.AutomaticVMEvictionPolicySpec{
					PolicyID: policyID,
				},
			}
		}

		newBestEffortRestartPolicy := func(policyID string) *vspherepolv1.BestEffortRestartPolicy {
			return &vspherepolv1.BestEffortRestartPolicy{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "best-effort-restart-",
					Namespace:    namespace,
				},
				Spec: vspherepolv1.BestEffortRestartPolicySpec{
					PolicyID: policyID,
				},
			}
		}

		It("should accept a valid AutomaticVMEvictionPolicy and default enforcementMode", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())
			Expect(obj.Spec.EnforcementMode).To(Equal(vspherepolv1.PolicyEnforcementModeMandatory))
		})

		It("should accept a valid BestEffortRestartPolicy and default enforcementMode", func() {
			obj := newBestEffortRestartPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())
			Expect(obj.Spec.EnforcementMode).To(Equal(vspherepolv1.PolicyEnforcementModeMandatory))
		})

		It("should reject an AutomaticVMEvictionPolicy missing policyID", func() {
			obj := newAutomaticVMEvictionPolicy("")
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject an AutomaticVMEvictionPolicy with a policyID over 64 characters", func() {
			obj := newAutomaticVMEvictionPolicy(strings.Repeat("a", 65))
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject a BestEffortRestartPolicy with a policyID over 64 characters", func() {
			obj := newBestEffortRestartPolicy(strings.Repeat("a", 65))
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject an AutomaticVMEvictionPolicy with a description over 1024 characters", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			obj.Spec.Description = strings.Repeat("a", 1025)
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject an invalid enforcementMode value", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			obj.Spec.EnforcementMode = "NotAValidMode"
			err := client.Create(ctx, obj)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should ignore status updates submitted through the main endpoint", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())

			obj.Status.ObservedGeneration = 42
			Expect(client.Update(ctx, obj)).To(Succeed())

			fetched := &vspherepolv1.AutomaticVMEvictionPolicy{}
			Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), fetched)).To(Succeed())
			Expect(fetched.Status.ObservedGeneration).To(BeZero())
		})

		It("should persist status conditions written through the status subresource", func() {
			obj := newAutomaticVMEvictionPolicy("policy-1")
			Expect(client.Create(ctx, obj)).To(Succeed())

			obj.Status.Conditions = []metav1.Condition{
				{
					Type:               vspherepolv1.ReadyConditionType,
					Status:             metav1.ConditionTrue,
					Reason:             "Ready",
					Message:            "",
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: obj.Generation,
				},
			}
			Expect(client.Status().Update(ctx, obj)).To(Succeed())

			fetched := &vspherepolv1.AutomaticVMEvictionPolicy{}
			Expect(client.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), fetched)).To(Succeed())
			Expect(fetched.Status.Conditions).To(HaveLen(1))
			Expect(fetched.Status.Conditions[0].Type).To(Equal(vspherepolv1.ReadyConditionType))
		})
	},
)
