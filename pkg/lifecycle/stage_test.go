// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package lifecycle_test

import (
	"context"
	"errors"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"

	lifecyclev1 "github.com/vmware-tanzu/vm-operator/external/lifecycle/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/lifecycle"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

const (
	finalizer = "lifecycle.vcfa.vmware.com/vm-operator-state"
	otherFin  = "other.example.com/keep"
	ns        = "ns"
	vmName    = "vm1"
	vmUID     = types.UID("vm-uid")

	hooksReady    = lifecycle.HooksReadyConditionType
	reasonAllOK   = "AllHooksSucceeded"
	reasonFailed  = lifecycle.HooksReadyReasonHookFailed
	reasonPending = "HooksPending"
)

var (
	stateKey = ctrlclient.ObjectKey{
		Namespace: ns,
		Name:      lifecyclev1.LifecycleStateName("VirtualMachine", vmUID),
	}

	allStages = []string{
		lifecycle.StageCreate,
		lifecycle.StagePowerStateChange,
		lifecycle.StageDelete,
		lifecycle.StageResourceDelete,
	}
)

func newVM() *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: vmName, UID: vmUID},
	}
}

func stage(name string, paused, resumed bool) lifecyclev1.StageSpec {
	return lifecyclev1.StageSpec{
		Name:            name,
		WorkflowPaused:  paused,
		WorkflowResumed: resumed,
	}
}

func newState(stages ...lifecyclev1.StageSpec) *lifecyclev1.LifecycleState {
	isController := true
	return &lifecyclev1.LifecycleState{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  stateKey.Namespace,
			Name:       stateKey.Name,
			Finalizers: []string{finalizer, otherFin},
			Labels: map[string]string{
				lifecycle.TargetGroupLabel: lifecycle.TargetGroup,
				lifecycle.TargetKindLabel:  lifecycle.TargetKind,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: vmopv1.GroupVersion.String(),
				Kind:       "VirtualMachine",
				Name:       vmName,
				UID:        vmUID,
				Controller: &isController,
			}},
		},
		Spec: lifecyclev1.LifecycleStateSpec{Stages: stages},
	}
}

func withReady(
	s *lifecyclev1.LifecycleState,
	stageName string,
	status metav1.ConditionStatus,
	reason, msg string) *lifecyclev1.LifecycleState {

	s.Status.Stages = append(s.Status.Stages, lifecyclev1.StageStatus{
		Name: stageName,
		Conditions: []metav1.Condition{{
			Type:    hooksReady,
			Status:  status,
			Reason:  reason,
			Message: msg,
		}},
	})
	return s
}

// withEntry adds a status entry with no HooksReady condition.
func withEntry(
	s *lifecyclev1.LifecycleState,
	stageName string) *lifecyclev1.LifecycleState {

	s.Status.Stages = append(
		s.Status.Stages, lifecyclev1.StageStatus{Name: stageName})
	return s
}

func subscribed(stages ...string) *lifecyclev1.LifecycleSubscribedStages {
	return &lifecyclev1.LifecycleSubscribedStages{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name: lifecyclev1.SubscribedStagesName(
				lifecycle.TargetGroup, lifecycle.TargetKind),
		},
		Spec: lifecyclev1.LifecycleSubscribedStagesSpec{Stages: stages},
	}
}

func ctxOn() context.Context {
	return pkgcfg.UpdateContext(
		pkgcfg.NewContextWithDefaultConfig(),
		func(c *pkgcfg.Config) { c.Features.LifecycleHooks = true })
}

func ctxOff() context.Context {
	return pkgcfg.UpdateContext(
		pkgcfg.NewContextWithDefaultConfig(),
		func(c *pkgcfg.Config) { c.Features.LifecycleHooks = false })
}

func getState(c ctrlclient.Client) *lifecyclev1.LifecycleState {
	var s lifecyclev1.LifecycleState
	ExpectWithOffset(1, c.Get(context.Background(), stateKey, &s)).To(Succeed())
	return &s
}

func specOf(s *lifecyclev1.LifecycleState, name string) lifecyclev1.StageSpec {
	for _, st := range s.Spec.Stages {
		if st.Name == name {
			return st
		}
	}
	Fail("stage " + name + " not found")
	return lifecyclev1.StageSpec{}
}

func cond(vm *vmopv1.VirtualMachine) *metav1.Condition {
	return apimeta.FindStatusCondition(
		vm.Status.Conditions,
		vmopv1.VirtualMachineConditionLifecycleHookReady)
}

type counts struct {
	get, create, patch atomic.Int32
}

func countingFuncs(c *counts, base interceptor.Funcs) interceptor.Funcs {
	base.Get = wrapGet(c, base.Get)
	base.Create = wrapCreate(c, base.Create)
	base.Patch = wrapPatch(c, base.Patch)
	return base
}

func wrapGet(
	c *counts,
	next func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectKey, ctrlclient.Object, ...ctrlclient.GetOption) error,
) func(context.Context, ctrlclient.WithWatch, ctrlclient.ObjectKey, ctrlclient.Object, ...ctrlclient.GetOption) error {

	return func(
		ctx context.Context,
		cl ctrlclient.WithWatch,
		key ctrlclient.ObjectKey,
		obj ctrlclient.Object,
		opts ...ctrlclient.GetOption) error {

		c.get.Add(1)
		if next != nil {
			return next(ctx, cl, key, obj, opts...)
		}
		return cl.Get(ctx, key, obj, opts...)
	}
}

func wrapCreate(
	c *counts,
	next func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ...ctrlclient.CreateOption) error,
) func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ...ctrlclient.CreateOption) error {

	return func(
		ctx context.Context,
		cl ctrlclient.WithWatch,
		obj ctrlclient.Object,
		opts ...ctrlclient.CreateOption) error {

		c.create.Add(1)
		if next != nil {
			return next(ctx, cl, obj, opts...)
		}
		return cl.Create(ctx, obj, opts...)
	}
}

func wrapPatch(
	c *counts,
	next func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ctrlclient.Patch, ...ctrlclient.PatchOption) error,
) func(context.Context, ctrlclient.WithWatch, ctrlclient.Object, ctrlclient.Patch, ...ctrlclient.PatchOption) error {

	return func(
		ctx context.Context,
		cl ctrlclient.WithWatch,
		obj ctrlclient.Object,
		patch ctrlclient.Patch,
		opts ...ctrlclient.PatchOption) error {

		c.patch.Add(1)
		if next != nil {
			return next(ctx, cl, obj, patch, opts...)
		}
		return cl.Patch(ctx, obj, patch, opts...)
	}
}

func conflictOnPatch() interceptor.Funcs {
	return interceptor.Funcs{
		Patch: func(
			_ context.Context,
			_ ctrlclient.WithWatch,
			obj ctrlclient.Object,
			_ ctrlclient.Patch,
			_ ...ctrlclient.PatchOption) error {

			return apierrors.NewConflict(
				schema.GroupResource{Resource: "lifecyclestates"},
				obj.GetName(),
				errors.New("the object has been modified"))
		},
	}
}

var _ = Describe("GetOrCreateLifecycleState", Label(testlabels.Controller), func() {
	var (
		vm    *vmopv1.VirtualMachine
		objs  []ctrlclient.Object
		funcs interceptor.Funcs
		cnt   *counts
		c     ctrlclient.Client
		ctx   context.Context

		existing *lifecyclev1.LifecycleState
	)

	BeforeEach(func() {
		vm = newVM()
		objs = nil
		existing = nil
		funcs = interceptor.Funcs{}
		cnt = &counts{}
		ctx = ctxOn()
	})

	JustBeforeEach(func() {
		if existing != nil {
			objs = append(objs, existing)
		}
		c = builder.NewFakeClientWithInterceptors(countingFuncs(cnt, funcs), objs...)
	})

	It("returns nil without any API call when the capability is off", func() {
		state, err := lifecycle.GetOrCreateLifecycleState(ctxOff(), c, vm)
		Expect(err).ToNot(HaveOccurred())
		Expect(state).To(BeNil())
		Expect(cnt.get.Load()).To(BeZero())
		Expect(cnt.create.Load()).To(BeZero())
	})

	When("there is no LifecycleState and no LifecycleSubscribedStages", func() {
		It("returns nil and creates nothing", func() {
			state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(BeNil())
			Expect(cnt.create.Load()).To(BeZero())
		})
	})

	When("LifecycleSubscribedStages lists no stage", func() {
		BeforeEach(func() { objs = append(objs, subscribed()) })

		It("returns nil and creates nothing", func() {
			state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(state).To(BeNil())
			Expect(cnt.create.Load()).To(BeZero())
		})
	})

	When("LifecycleSubscribedStages lists hooked stages", func() {
		BeforeEach(func() {
			objs = append(objs, subscribed(
				lifecycle.StageCreate, lifecycle.StageDelete))
		})

		It("creates an owner-referenced LifecycleState, then reuses it", func() {
			state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(state).ToNot(BeNil())
			Expect(state.Name).To(Equal(stateKey.Name))
			Expect(state.Finalizers).To(ConsistOf(finalizer))
			Expect(state.Labels).To(HaveKeyWithValue(
				lifecycle.TargetGroupLabel, lifecycle.TargetGroup))
			Expect(state.Labels).To(HaveKeyWithValue(
				lifecycle.TargetKindLabel, lifecycle.TargetKind))
			Expect(state.OwnerReferences).To(HaveLen(1))
			Expect(*state.OwnerReferences[0].Controller).To(BeTrue())
			Expect(*state.OwnerReferences[0].BlockOwnerDeletion).To(BeFalse())
			Expect(state.OwnerReferences[0].UID).To(Equal(vmUID))
			Expect(state.Spec.Stages).To(ConsistOf(
				stage(lifecycle.StageCreate, false, false),
				stage(lifecycle.StageDelete, false, false)))

			again, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(again.Name).To(Equal(state.Name))
			Expect(cnt.create.Load()).To(Equal(int32(1)))
			Expect(cnt.patch.Load()).To(BeZero())
		})

		When("Create returns AlreadyExists", func() {
			BeforeEach(func() {
				funcs.Create = func(
					_ context.Context,
					_ ctrlclient.WithWatch,
					obj ctrlclient.Object,
					_ ...ctrlclient.CreateOption) error {

					return apierrors.NewAlreadyExists(
						schema.GroupResource{Resource: "lifecyclestates"},
						obj.GetName())
				}
			})

			It("returns a RequeueError, never nil", func() {
				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(state).To(BeNil())
				var rerr pkgerr.RequeueError
				Expect(errors.As(err, &rerr)).To(BeTrue())
			})
		})

		When("Create is rejected because the namespace is terminating", func() {
			BeforeEach(func() {
				funcs.Create = func(
					_ context.Context,
					_ ctrlclient.WithWatch,
					_ ctrlclient.Object,
					_ ...ctrlclient.CreateOption) error {

					return &apierrors.StatusError{ErrStatus: metav1.Status{
						Status: metav1.StatusFailure,
						Reason: metav1.StatusReasonForbidden,
						Details: &metav1.StatusDetails{
							Causes: []metav1.StatusCause{{
								Type: corev1.NamespaceTerminatingCause,
							}},
						},
					}}
				}
			})

			It("returns nil", func() {
				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(state).To(BeNil())
			})
		})

		When("Create fails for another reason", func() {
			BeforeEach(func() {
				funcs.Create = func(
					_ context.Context,
					_ ctrlclient.WithWatch,
					_ ctrlclient.Object,
					_ ...ctrlclient.CreateOption) error {

					return errors.New("boom")
				}
			})

			It("returns the error", func() {
				_, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(err).To(MatchError(ContainSubstring("boom")))
			})
		})
	})

	When("a LifecycleState exists", func() {
		BeforeEach(func() {
			existing = newState(stage(lifecycle.StageCreate, false, false))
		})

		When("it is bound to the VM", func() {
			It("returns it with no write", func() {
				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(state.Spec.Stages).To(HaveLen(1))
				Expect(cnt.patch.Load()).To(BeZero())
				Expect(cnt.create.Load()).To(BeZero())
			})
		})

		When("it lacks VM Operator's finalizer", func() {
			BeforeEach(func() { existing.Finalizers = []string{otherFin} })

			It("adopts it, keeping other finalizers", func() {
				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(state.Finalizers).To(ConsistOf(finalizer, otherFin))
				Expect(getState(c).Finalizers).To(ConsistOf(finalizer, otherFin))
			})

			When("a concurrent writer conflicts", func() {
				BeforeEach(func() { funcs = conflictOnPatch() })

				It("returns the conflict", func() {
					_, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
					Expect(apierrors.IsConflict(err)).To(BeTrue())
				})
			})
		})

		DescribeTable("it is not bound to the VM",
			func(mutate func(*lifecyclev1.LifecycleState)) {
				mutate(existing)
				c = builder.NewFakeClient(existing)

				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(state).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring("not bound")))
			},
			Entry("wrong group label", func(s *lifecyclev1.LifecycleState) {
				s.Labels[lifecycle.TargetGroupLabel] = "other.example.com"
			}),
			Entry("wrong kind label", func(s *lifecyclev1.LifecycleState) {
				s.Labels[lifecycle.TargetKindLabel] = "Other"
			}),
			Entry("wrong owner UID", func(s *lifecyclev1.LifecycleState) {
				s.OwnerReferences[0].UID = "other"
			}),
			Entry("wrong owner kind", func(s *lifecyclev1.LifecycleState) {
				s.OwnerReferences[0].Kind = "Other"
			}),
			Entry("wrong owner group", func(s *lifecyclev1.LifecycleState) {
				s.OwnerReferences[0].APIVersion = "other.example.com/v1"
			}),
			Entry("owner is not the controller", func(s *lifecyclev1.LifecycleState) {
				s.OwnerReferences[0].Controller = nil
			}),
		)

		When("it is terminating without VM Operator's finalizer", func() {
			BeforeEach(func() {
				now := metav1.Now()
				existing.DeletionTimestamp = &now
				existing.Finalizers = []string{otherFin}
			})

			It("returns an error", func() {
				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(state).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring("terminating")))
			})
		})

		When("it is terminating with VM Operator's finalizer", func() {
			BeforeEach(func() {
				now := metav1.Now()
				existing.DeletionTimestamp = &now
			})

			It("returns it with no write", func() {
				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(state).ToNot(BeNil())
				Expect(cnt.patch.Load()).To(BeZero())
			})
		})

		When("the same name was a different VM with a recreated UID", func() {
			It("derives a different name, so the VM gets its own State", func() {
				vm2 := newVM()
				vm2.UID = "vm-uid-2"
				state, err := lifecycle.GetOrCreateLifecycleState(ctx, c, vm2)
				Expect(err).ToNot(HaveOccurred())
				Expect(state).To(BeNil()) // no subscribed stages, no State
			})
		})
	})
})

var _ = Describe("ReconcileStage", Label(testlabels.Controller), func() {
	var (
		vm    *vmopv1.VirtualMachine
		state *lifecyclev1.LifecycleState
		funcs interceptor.Funcs
		cnt   *counts
		c     ctrlclient.Client
		ctx   context.Context
	)

	BeforeEach(func() {
		vm = newVM()
		state = nil
		funcs = interceptor.Funcs{}
		cnt = &counts{}
		ctx = ctxOn()
	})

	JustBeforeEach(func() {
		var objs []ctrlclient.Object
		if state != nil {
			objs = append(objs, state)
		}
		c = builder.NewFakeClientWithInterceptors(countingFuncs(cnt, funcs), objs...)
		if state != nil {
			state = getState(c)
		}
		cnt.get.Store(0)
		cnt.patch.Store(0)
	})

	It("proceeds with no API call and an untouched condition when the capability is off", func() {
		state = newState(stage(lifecycle.StageCreate, true, false))
		conditions.MarkFalse(vm, vmopv1.VirtualMachineConditionLifecycleHookReady,
			"DeleteStage", "left over")

		res, err := lifecycle.ReconcileStage(
			ctxOff(), c, vm, nil, lifecycle.StageCreate)
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Proceed).To(BeTrue())
		Expect(cond(vm).Reason).To(Equal("DeleteStage"))
		Expect(cnt.get.Load()).To(BeZero())
		Expect(cnt.patch.Load()).To(BeZero())
	})

	It("proceeds with no condition write when the state is nil", func() {
		res, err := lifecycle.ReconcileStage(ctx, c, vm, nil, lifecycle.StageCreate)
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Proceed).To(BeTrue())
		Expect(cond(vm)).To(BeNil())
	})

	When("the stage has no entry", func() {
		BeforeEach(func() {
			state = newState(stage(lifecycle.StageDelete, false, false))
		})

		It("proceeds with no write and leaves an earlier condition alone", func() {
			conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionLifecycleHookReady)
			before := *cond(vm)

			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeTrue())
			Expect(*cond(vm)).To(Equal(before))
			Expect(cnt.patch.Load()).To(BeZero())
		})
	})

	for _, name := range allStages {
		When("the "+name+" stage is reached with no previous hold", func() {
			BeforeEach(func() {
				state = newState(
					stage(lifecycle.StageCreate, false, false),
					stage(lifecycle.StagePowerStateChange, false, false),
					stage(lifecycle.StageDelete, false, false),
					stage(lifecycle.StageResourceDelete, false, false))
			})

			It("pauses to (true, false) and waits", func() {
				res, err := lifecycle.ReconcileStage(ctx, c, vm, state, name)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Proceed).To(BeFalse())
				Expect(specOf(getState(c), name)).To(Equal(stage(name, true, false)))
				Expect(specOf(state, name)).To(Equal(stage(name, true, false)))
				Expect(cond(vm).Status).To(Equal(metav1.ConditionFalse))
				Expect(cond(vm).Reason).To(Equal(name + "Stage"))
				Expect(cond(vm).Message).To(Equal("Waiting for " + name + " hooks"))
			})
		})
	}

	When("the stage was previously released, (false, true), with no status entry", func() {
		BeforeEach(func() {
			state = newState(stage(lifecycle.StageCreate, false, true))
		})

		It("pauses again to (true, false)", func() {
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeFalse())
			Expect(specOf(getState(c), lifecycle.StageCreate)).To(
				Equal(stage(lifecycle.StageCreate, true, false)))
		})
	})

	When("the previous hold is still being cleared", func() {
		BeforeEach(func() {
			state = withReady(
				newState(stage(lifecycle.StageCreate, false, true)),
				lifecycle.StageCreate, metav1.ConditionTrue, reasonAllOK, "done")
		})

		It("does not pause or patch, and waits", func() {
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeFalse())
			Expect(cnt.patch.Load()).To(BeZero())
			Expect(cond(vm).Status).To(Equal(metav1.ConditionFalse))
			Expect(cond(vm).Message).To(Equal("Waiting for Create hooks"))
		})
	})

	When("the previous hold ended with a failed hook and is still being cleared", func() {
		BeforeEach(func() {
			state = withReady(
				newState(stage(lifecycle.StageCreate, false, true)),
				lifecycle.StageCreate, metav1.ConditionTrue, reasonFailed, "boom")
		})

		It("keeps the hook-failed message and does not patch", func() {
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeFalse())
			Expect(cnt.patch.Load()).To(BeZero())
			Expect(cond(vm).Status).To(Equal(metav1.ConditionFalse))
			Expect(cond(vm).Message).To(Equal("Create hook failed, retrying: boom"))
		})
	})

	When("the stage is paused", func() {
		DescribeTable("and hooks are not ready",
			func(build func() *lifecyclev1.LifecycleState, msg string) {
				c = builder.NewFakeClientWithInterceptors(
					countingFuncs(cnt, interceptor.Funcs{}), build())
				state = getState(c)
				cnt.patch.Store(0)

				res, err := lifecycle.ReconcileStage(
					ctx, c, vm, state, lifecycle.StageCreate)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.Proceed).To(BeFalse())
				Expect(cnt.patch.Load()).To(BeZero())
				Expect(cond(vm).Status).To(Equal(metav1.ConditionFalse))
				Expect(cond(vm).Reason).To(Equal("CreateStage"))
				Expect(cond(vm).Message).To(Equal(msg))
			},
			Entry("no status entry", func() *lifecyclev1.LifecycleState {
				return newState(stage(lifecycle.StageCreate, true, false))
			}, "Waiting for Create hooks"),
			Entry("status entry without HooksReady", func() *lifecyclev1.LifecycleState {
				return withEntry(
					newState(stage(lifecycle.StageCreate, true, false)),
					lifecycle.StageCreate)
			}, "Waiting for Create hooks"),
			Entry("HooksReady=False/HooksPending", func() *lifecyclev1.LifecycleState {
				return withReady(
					newState(stage(lifecycle.StageCreate, true, false)),
					lifecycle.StageCreate, metav1.ConditionFalse, reasonPending,
					"0 of 1 hooks completed")
			}, "Waiting for Create hooks: 0 of 1 hooks completed"),
			Entry("HooksReady=False/HookFailed", func() *lifecyclev1.LifecycleState {
				return withReady(
					newState(stage(lifecycle.StageCreate, true, false)),
					lifecycle.StageCreate, metav1.ConditionFalse, reasonFailed,
					"boom")
			}, "Create hook failed, waiting for remaining hooks: boom"),
		)
	})

	When("hooks are ready", func() {
		BeforeEach(func() {
			state = withReady(
				newState(
					stage(lifecycle.StageCreate, true, false),
					stage(lifecycle.StageDelete, true, false)),
				lifecycle.StageCreate, metav1.ConditionTrue, reasonAllOK, "all done")
		})

		It("releases to (false, true), sets True, and proceeds", func() {
			rv := state.ResourceVersion

			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeTrue())
			Expect(specOf(getState(c), lifecycle.StageCreate)).To(
				Equal(stage(lifecycle.StageCreate, false, true)))
			Expect(state.ResourceVersion).ToNot(Equal(rv))
			Expect(cond(vm).Status).To(Equal(metav1.ConditionTrue))
			Expect(cond(vm).Reason).To(Equal("CreateStage"))
			Expect(cond(vm).Message).To(Equal("all done"))
			// Other stages are not touched, and the finalizer stays.
			Expect(specOf(getState(c), lifecycle.StageDelete)).To(
				Equal(stage(lifecycle.StageDelete, true, false)))
			Expect(getState(c).Finalizers).To(ConsistOf(finalizer, otherFin))
		})

		It("does not retry a conflict internally and writes no condition", func() {
			funcs = conflictOnPatch()
			c = builder.NewFakeClientWithInterceptors(funcs, state.DeepCopy())

			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(apierrors.IsConflict(err)).To(BeTrue())
			Expect(res.Proceed).To(BeFalse())
			Expect(cond(vm)).To(BeNil())
		})
	})

	When("hooks finished and at least one failed", func() {
		BeforeEach(func() {
			state = withReady(
				newState(stage(lifecycle.StageCreate, true, false)),
				lifecycle.StageCreate, metav1.ConditionTrue, reasonFailed, "boom")
		})

		It("releases the hold but does not proceed", func() {
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeFalse())
			Expect(specOf(getState(c), lifecycle.StageCreate)).To(
				Equal(stage(lifecycle.StageCreate, false, true)))
			Expect(cond(vm).Status).To(Equal(metav1.ConditionFalse))
			Expect(cond(vm).Reason).To(Equal("CreateStage"))
			Expect(cond(vm).Message).To(Equal("Create hook failed, retrying: boom"))
		})

		It("loops: re-pauses once the Lifecycle Operator clears the entry", func() {
			_, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())

			// The entry lingers: wait, no pause.
			state = getState(c)
			cnt.patch.Store(0)
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeFalse())
			Expect(cnt.patch.Load()).To(BeZero())
			Expect(cond(vm).Message).To(ContainSubstring("hook failed, retrying"))

			// The Lifecycle Operator clears the status entry.
			state.Status.Stages = nil
			Expect(c.Status().Update(context.Background(), state)).To(Succeed())

			state = getState(c)
			res, err = lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeFalse())
			Expect(specOf(getState(c), lifecycle.StageCreate)).To(
				Equal(stage(lifecycle.StageCreate, true, false)))
			Expect(cond(vm).Message).To(Equal("Waiting for Create hooks"))
		})
	})

	When("ResourceDelete hooks are ready", func() {
		BeforeEach(func() {
			state = withReady(
				newState(
					stage(lifecycle.StageDelete, true, false),
					stage(lifecycle.StageResourceDelete, true, false)),
				lifecycle.StageResourceDelete,
				metav1.ConditionTrue, reasonAllOK, "ok")
		})

		It("removes only VM Operator's finalizer and releases other paused stages in one write", func() {
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageResourceDelete)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeTrue())
			Expect(cnt.patch.Load()).To(Equal(int32(1)))

			got := getState(c)
			Expect(got.Finalizers).To(ConsistOf(otherFin))
			Expect(specOf(got, lifecycle.StageResourceDelete)).To(
				Equal(stage(lifecycle.StageResourceDelete, false, true)))
			Expect(specOf(got, lifecycle.StageDelete)).To(
				Equal(stage(lifecycle.StageDelete, false, true)))
		})

		It("releases an object without the finalizer normally", func() {
			state.Finalizers = []string{otherFin}
			Expect(c.Update(context.Background(), state)).To(Succeed())

			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageResourceDelete)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeTrue())
			Expect(getState(c).Finalizers).To(ConsistOf(otherFin))
		})
	})

	When("ResourceDelete hooks finished and one failed", func() {
		BeforeEach(func() {
			state = withReady(
				newState(
					stage(lifecycle.StageDelete, true, false),
					stage(lifecycle.StageResourceDelete, true, false)),
				lifecycle.StageResourceDelete,
				metav1.ConditionTrue, reasonFailed, "boom")
		})

		It("releases only that stage, keeps the finalizer, and does not proceed", func() {
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageResourceDelete)
			Expect(err).ToNot(HaveOccurred())
			Expect(res.Proceed).To(BeFalse())

			got := getState(c)
			Expect(got.Finalizers).To(ConsistOf(finalizer, otherFin))
			Expect(specOf(got, lifecycle.StageResourceDelete)).To(
				Equal(stage(lifecycle.StageResourceDelete, false, true)))
			Expect(specOf(got, lifecycle.StageDelete)).To(
				Equal(stage(lifecycle.StageDelete, true, false)))
			Expect(cond(vm).Status).To(Equal(metav1.ConditionFalse))
		})
	})

	When("a pause conflicts with a concurrent writer", func() {
		BeforeEach(func() {
			state = newState(stage(lifecycle.StageCreate, false, false))
			funcs = conflictOnPatch()
		})

		It("returns the conflict and writes no condition", func() {
			res, err := lifecycle.ReconcileStage(
				ctx, c, vm, state, lifecycle.StageCreate)
			Expect(apierrors.IsConflict(err)).To(BeTrue())
			Expect(res.Proceed).To(BeFalse())
			Expect(cond(vm)).To(BeNil())
		})
	})
})

var _ = Describe("AbandonStage", Label(testlabels.Controller), func() {
	var (
		vm    *vmopv1.VirtualMachine
		state *lifecyclev1.LifecycleState
		cnt   *counts
		c     ctrlclient.Client
	)

	BeforeEach(func() {
		vm = newVM()
		cnt = &counts{}
	})

	setup := func(s *lifecyclev1.LifecycleState) {
		c = builder.NewFakeClientWithInterceptors(
			countingFuncs(cnt, interceptor.Funcs{}), s)
		state = getState(c)
		cnt.patch.Store(0)
	}

	DescribeTable("a paused stage",
		func(build func() *lifecyclev1.LifecycleState) {
			setup(build())
			Expect(lifecycle.AbandonStage(
				ctxOn(), c, vm, state, lifecycle.StagePowerStateChange)).To(Succeed())
			Expect(specOf(getState(c), lifecycle.StagePowerStateChange)).To(
				Equal(stage(lifecycle.StagePowerStateChange, false, true)))
			Expect(cond(vm).Status).To(Equal(metav1.ConditionTrue))
			Expect(cond(vm).Reason).To(Equal("PowerStateChangeStage"))
		},
		Entry("with HooksReady absent", func() *lifecyclev1.LifecycleState {
			return newState(stage(lifecycle.StagePowerStateChange, true, false))
		}),
		Entry("with HooksReady False", func() *lifecyclev1.LifecycleState {
			return withReady(
				newState(stage(lifecycle.StagePowerStateChange, true, false)),
				lifecycle.StagePowerStateChange,
				metav1.ConditionFalse, reasonPending, "")
		}),
		Entry("with HooksReady True", func() *lifecyclev1.LifecycleState {
			return withReady(
				newState(stage(lifecycle.StagePowerStateChange, true, false)),
				lifecycle.StagePowerStateChange,
				metav1.ConditionTrue, reasonFailed, "")
		}),
	)

	It("is a no-op when the stage is not paused", func() {
		setup(newState(stage(lifecycle.StagePowerStateChange, false, false)))
		Expect(lifecycle.AbandonStage(
			ctxOn(), c, vm, state, lifecycle.StagePowerStateChange)).To(Succeed())
		Expect(cnt.patch.Load()).To(BeZero())
		Expect(cond(vm)).To(BeNil())
	})

	It("is a no-op when the stage has no entry", func() {
		setup(newState(stage(lifecycle.StageCreate, true, false)))
		Expect(lifecycle.AbandonStage(
			ctxOn(), c, vm, state, lifecycle.StagePowerStateChange)).To(Succeed())
		Expect(cnt.patch.Load()).To(BeZero())
	})

	It("is a no-op when the state is nil or the capability is off", func() {
		setup(newState(stage(lifecycle.StagePowerStateChange, true, false)))
		Expect(lifecycle.AbandonStage(
			ctxOn(), c, vm, nil, lifecycle.StagePowerStateChange)).To(Succeed())
		Expect(lifecycle.AbandonStage(
			ctxOff(), c, vm, state, lifecycle.StagePowerStateChange)).To(Succeed())
		Expect(cnt.patch.Load()).To(BeZero())
		Expect(cond(vm)).To(BeNil())
	})

	It("returns a conflict", func() {
		c = builder.NewFakeClientWithInterceptors(
			conflictOnPatch(),
			newState(stage(lifecycle.StagePowerStateChange, true, false)))
		state = getState(c)
		err := lifecycle.AbandonStage(
			ctxOn(), c, vm, state, lifecycle.StagePowerStateChange)
		Expect(apierrors.IsConflict(err)).To(BeTrue())
		Expect(cond(vm)).To(BeNil())
	})
})

var _ = Describe("MarkUnknown", Label(testlabels.Controller), func() {
	It("sets Unknown with the stage reason and error message", func() {
		vm := newVM()
		lifecycle.MarkUnknown(vm, lifecycle.StageDelete, errors.New("boom"))
		Expect(cond(vm).Status).To(Equal(metav1.ConditionUnknown))
		Expect(cond(vm).Reason).To(Equal("DeleteStage"))
		Expect(cond(vm).Message).To(Equal("Could not evaluate Delete hooks: boom"))
	})

	It("writes nothing for a conflict or a nil error", func() {
		vm := newVM()
		lifecycle.MarkUnknown(vm, lifecycle.StageDelete, apierrors.NewConflict(
			schema.GroupResource{Resource: "lifecyclestates"}, "x", errors.New("c")))
		lifecycle.MarkUnknown(vm, lifecycle.StageDelete, nil)
		Expect(cond(vm)).To(BeNil())
	})
})

var _ = Describe("ReleaseLifecycleState", Label(testlabels.Controller), func() {
	var (
		vm  *vmopv1.VirtualMachine
		cnt *counts
		c   ctrlclient.Client
	)

	BeforeEach(func() {
		vm = newVM()
		cnt = &counts{}
	})

	It("removes only VM Operator's finalizer and releases paused stages", func() {
		c = builder.NewFakeClientWithInterceptors(
			countingFuncs(cnt, interceptor.Funcs{}),
			newState(
				stage(lifecycle.StageDelete, true, false),
				stage(lifecycle.StageCreate, false, true)))
		state := getState(c)

		Expect(lifecycle.ReleaseLifecycleState(ctxOn(), c, c, vm, state)).To(Succeed())

		got := getState(c)
		Expect(got.Finalizers).To(ConsistOf(otherFin))
		Expect(specOf(got, lifecycle.StageDelete)).To(
			Equal(stage(lifecycle.StageDelete, false, true)))
		Expect(specOf(got, lifecycle.StageCreate)).To(
			Equal(stage(lifecycle.StageCreate, false, true)))
	})

	It("is a no-op when the state is nil or the finalizer is absent", func() {
		s := newState()
		s.Finalizers = []string{otherFin}
		c = builder.NewFakeClientWithInterceptors(
			countingFuncs(cnt, interceptor.Funcs{}), s)

		Expect(lifecycle.ReleaseLifecycleState(ctxOn(), c, c, vm, nil)).To(Succeed())
		Expect(lifecycle.ReleaseLifecycleState(ctxOn(), c, c, vm, getState(c))).To(Succeed())
		Expect(cnt.patch.Load()).To(BeZero())
	})

	It("returns a conflict and keeps the other writer's finalizer", func() {
		c = builder.NewFakeClientWithInterceptors(
			conflictOnPatch(), newState())
		err := lifecycle.ReleaseLifecycleState(ctxOn(), c, c, vm, getState(c))
		Expect(apierrors.IsConflict(err)).To(BeTrue())
		Expect(getState(c).Finalizers).To(ConsistOf(finalizer, otherFin))
	})

	When("the capability is off", func() {
		It("reads through the API reader and removes the finalizer", func() {
			c = builder.NewFakeClientWithInterceptors(
				countingFuncs(cnt, interceptor.Funcs{}), newState())
			reader := &counts{}
			apiReader := builder.NewFakeClientWithInterceptors(
				countingFuncs(reader, interceptor.Funcs{}), newState())

			// Pass a nil state: with the capability off the object must come
			// from the API reader.
			Expect(lifecycle.ReleaseLifecycleState(
				ctxOff(), c, apiReader, vm, nil)).To(Succeed())
			Expect(reader.get.Load()).To(Equal(int32(1)))
			Expect(cnt.get.Load()).To(BeZero())
			Expect(getState(c).Finalizers).To(ConsistOf(otherFin))
		})

		It("is a no-op when the object is not found", func() {
			c = builder.NewFakeClient()
			Expect(lifecycle.ReleaseLifecycleState(
				ctxOff(), c, c, vm, nil)).To(Succeed())
		})

		It("is a no-op when the CRD is not installed", func() {
			c = builder.NewFakeClient()
			noMatch := builder.NewFakeClientWithInterceptors(interceptor.Funcs{
				Get: func(
					_ context.Context,
					_ ctrlclient.WithWatch,
					_ ctrlclient.ObjectKey,
					_ ctrlclient.Object,
					_ ...ctrlclient.GetOption) error {

					return &apimeta.NoKindMatchError{
						GroupKind: schema.GroupKind{
							Group: lifecycle.TargetGroup, Kind: "LifecycleState"},
					}
				},
			})
			Expect(lifecycle.ReleaseLifecycleState(
				ctxOff(), c, noMatch, vm, nil)).To(Succeed())
		})
	})
})

var _ = Describe("StatusChangedPredicate", Label(testlabels.Controller), func() {
	var p lifecycle.StatusChangedPredicate

	withoutFinalizer := func() ctrlclient.Object {
		s := newState()
		s.Finalizers = []string{otherFin}
		return s
	}

	It("passes Create only without VM Operator's finalizer", func() {
		Expect(p.Create(event.TypedCreateEvent[ctrlclient.Object]{
			Object: newState()})).To(BeFalse())
		Expect(p.Create(event.TypedCreateEvent[ctrlclient.Object]{
			Object: withoutFinalizer()})).To(BeTrue())
	})

	It("filters Generic and passes Delete", func() {
		Expect(p.Generic(event.TypedGenericEvent[ctrlclient.Object]{
			Object: newState()})).To(BeFalse())
		Expect(p.Delete(event.TypedDeleteEvent[ctrlclient.Object]{
			Object: newState()})).To(BeTrue())
	})

	It("passes Update only when status differs", func() {
		oldS := newState(stage(lifecycle.StageCreate, false, false))
		specOnly := newState(stage(lifecycle.StageCreate, true, false))
		statusChanged := withReady(
			newState(stage(lifecycle.StageCreate, false, false)),
			lifecycle.StageCreate, metav1.ConditionTrue, reasonAllOK, "")

		Expect(p.Update(event.TypedUpdateEvent[ctrlclient.Object]{
			ObjectOld: oldS, ObjectNew: specOnly})).To(BeFalse())
		Expect(p.Update(event.TypedUpdateEvent[ctrlclient.Object]{
			ObjectOld: oldS, ObjectNew: statusChanged})).To(BeTrue())
		Expect(p.Update(event.TypedUpdateEvent[ctrlclient.Object]{
			ObjectOld: oldS, ObjectNew: &corev1.Pod{}})).To(BeFalse())
	})
})
