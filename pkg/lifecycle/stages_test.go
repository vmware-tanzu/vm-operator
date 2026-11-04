// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package lifecycle_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	lifecyclev1 "github.com/vmware-tanzu/vm-operator/external/lifecycle/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/lifecycle"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("PublishStages", Label(testlabels.Controller), func() {
	var (
		objs  []ctrlclient.Object
		funcs interceptor.Funcs
		cnt   *counts
		c     ctrlclient.Client
		ctx   context.Context
	)

	getStages := func() *lifecyclev1.LifecycleStages {
		var s lifecyclev1.LifecycleStages
		ExpectWithOffset(1, c.Get(
			context.Background(),
			ctrlclient.ObjectKey{Name: lifecycle.StagesName},
			&s)).To(Succeed())
		return &s
	}

	BeforeEach(func() {
		objs = nil
		funcs = interceptor.Funcs{}
		cnt = &counts{}
		ctx = ctxOn()
	})

	JustBeforeEach(func() {
		c = builder.NewFakeClientWithInterceptors(countingFuncs(cnt, funcs), objs...)
		cnt.create.Store(0)
		cnt.patch.Store(0)
		cnt.get.Store(0)
	})

	It("makes no API call when the capability is off", func() {
		Expect(lifecycle.PublishStages(ctxOff(), c)).To(Succeed())
		Expect(cnt.get.Load()).To(BeZero())
		Expect(cnt.create.Load()).To(BeZero())
		Expect(cnt.patch.Load()).To(BeZero())
	})

	It("creates the instance with the four blocking, reentrant stages", func() {
		Expect(lifecycle.PublishStages(ctx, c)).To(Succeed())

		s := getStages()
		Expect(s.Spec.Target).To(Equal(lifecyclev1.LifecycleStagesTarget{
			Group: "vmoperator.vmware.com",
			Kind:  "VirtualMachine",
		}))
		Expect(s.Spec.Stages).To(HaveLen(4))

		names := []string{}
		for _, st := range s.Spec.Stages {
			names = append(names, st.Name)
			Expect(st.Type).To(Equal("Reentrant"))
			Expect(st.Blocking).To(BeTrue())
			Expect(st.ConditionName).To(Equal("LifecycleHookReady"))
		}
		Expect(names).To(ConsistOf(
			"Create", "PowerStateChange", "Delete", "ResourceDelete"))
	})

	When("the instance has drifted", func() {
		BeforeEach(func() {
			objs = append(objs, &lifecyclev1.LifecycleStages{
				ObjectMeta: metav1.ObjectMeta{Name: lifecycle.StagesName},
				Spec: lifecyclev1.LifecycleStagesSpec{
					Target: lifecyclev1.LifecycleStagesTarget{
						Group: "other.example.com", Kind: "Other"},
					Stages: []lifecyclev1.LifecycleStageDefinition{
						{Name: "Bogus"}},
				},
			})
		})

		It("overwrites its spec", func() {
			Expect(lifecycle.PublishStages(ctx, c)).To(Succeed())
			s := getStages()
			Expect(s.Spec.Target.Kind).To(Equal("VirtualMachine"))
			Expect(s.Spec.Stages).To(HaveLen(4))
			Expect(cnt.patch.Load()).To(Equal(int32(1)))
		})
	})

	It("does not write an unchanged instance", func() {
		Expect(lifecycle.PublishStages(ctx, c)).To(Succeed())
		cnt.create.Store(0)
		cnt.patch.Store(0)

		Expect(lifecycle.PublishStages(ctx, c)).To(Succeed())
		Expect(cnt.create.Load()).To(BeZero())
		Expect(cnt.patch.Load()).To(BeZero())
	})

	When("the API server keeps failing", func() {
		BeforeEach(func() {
			funcs.Get = func(
				_ context.Context,
				_ ctrlclient.WithWatch,
				_ ctrlclient.ObjectKey,
				_ ctrlclient.Object,
				_ ...ctrlclient.GetOption) error {

				return errors.New("boom")
			}
		})

		It("retries a bounded number of times, then returns the error", func() {
			err := lifecycle.PublishStages(ctx, c)
			Expect(err).To(MatchError(ContainSubstring("boom")))
			Expect(cnt.get.Load()).To(BeNumerically(">", 1))
			Expect(cnt.get.Load()).To(BeNumerically("<=", 10))
		})
	})

	When("a failure is transient", func() {
		BeforeEach(func() {
			var failed bool
			funcs.Get = func(
				ctx context.Context,
				cl ctrlclient.WithWatch,
				key ctrlclient.ObjectKey,
				obj ctrlclient.Object,
				opts ...ctrlclient.GetOption) error {

				if !failed {
					failed = true
					return errors.New("transient")
				}
				return cl.Get(ctx, key, obj, opts...)
			}
		})

		It("succeeds on retry", func() {
			Expect(lifecycle.PublishStages(ctx, c)).To(Succeed())
			Expect(getStages().Spec.Stages).To(HaveLen(4))
		})
	})
})
