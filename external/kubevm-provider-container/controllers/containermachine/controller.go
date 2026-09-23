// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package containermachine reconciles a ContainerMachine into a Docker or
// Podman container.
//
// Kept as one file, unlike external/kubevm-provider-aws's six-file split:
// EC2 has genuinely distinct failure modes for launch, observation, power,
// public-address management and deletion, each worth its own file. A local
// container engine does not — create, inspect, start, stop and remove are
// all synchronous CLI calls with no eventual consistency and no billing risk
// on a leak, so splitting this the same way would add navigation cost with
// nothing to show for it. See implementing-a-provider.md, "How much of the
// AWS provider's depth do you need", for the general version of this call.
package containermachine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	containerv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/internal/container"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/internal/link"
)

// pollRequeueDelay is how long to wait while a container is between steady
// states. Matches the core's own pollRequeueDelay
// (external/kubevm/controller/controllers/virtualmachine), for the same
// reason external/kubevm-provider-aws's bootRequeue does: no evidence was
// gathered for a better number here either, and copying one somebody already
// picked beats inventing a new one.
const pollRequeueDelay = 10 * time.Second

// Reconciler turns a ContainerMachine into a container.
type Reconciler struct {
	client.Client

	// Engine drives the container runtime. Nil means container.Client with
	// its default ExecRunner, which shells out to a real docker/podman
	// binary — set to a fake Runner in tests so no test needs one installed.
	Engine func(runtime containerv1a1.ContainerRuntime) container.Client
}

func (r *Reconciler) engine(rt containerv1a1.ContainerRuntime) container.Client {
	if r.Engine != nil {
		return r.Engine(rt)
	}
	name := string(rt)
	if name == "" {
		name = string(containerv1a1.ContainerRuntimeDocker)
	}
	return container.Client{Runtime: name}
}

// containerName is the deterministic name this provider creates containers
// under: <namespace>-<name>, prefixed so a `docker ps` on a shared host does
// not collide with something unrelated.
//
// Deterministic rather than random or UID-derived: it is what makes the
// by-name idempotency in internal/container.Client.Create sound. A random
// name would defeat it, because a retry after a lost reply would create a
// second container instead of finding the first.
func containerName(machine *containerv1a1.ContainerMachine) string {
	return fmt.Sprintf("kubevm-%s-%s", machine.Namespace, machine.Name)
}

// +kubebuilder:rbac:groups=infrastructure.kube-vm.io,resources=containermachines,verbs=get;list;watch;patch;delete
// +kubebuilder:rbac:groups=infrastructure.kube-vm.io,resources=containermachines/status,verbs=get;patch
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines,verbs=get;list;watch

// Reconcile drives one ContainerMachine towards the state its parent asks
// for.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (
	ctrl.Result, error) {

	machine := &containerv1a1.ContainerMachine{}
	if err := r.Get(ctx, req.NamespacedName, machine); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !machine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machine)
	}
	return r.reconcileNormal(ctx, machine)
}

// reconcileNormal claims the object, resolves what its parent asks for, and
// drives the container towards it.
func (r *Reconciler) reconcileNormal(ctx context.Context,
	machine *containerv1a1.ContainerMachine) (ctrl.Result, error) {

	// No CLI call happens before the link is confirmed mutual: an object
	// nobody has linked yet may belong to someone else, and starting a
	// container for it anyway is the hijack the two-sided check exists to
	// prevent. See internal/link's package comment.
	named := machine.GetAnnotations()[containerv1a1.AnnotationKey]
	parent, err := link.ParentOf(ctx, r.Client, machine)
	switch {
	case errors.Is(err, link.ErrNotLinked):
		return ctrl.Result{}, r.markNotReady(ctx, machine,
			containerv1a1.ReasonNotAdopted,
			fmt.Sprintf("no %s annotation: add one naming the VirtualMachine "+
				"this object belongs to", containerv1a1.AnnotationKey))

	case errors.Is(err, link.ErrParentMissing):
		return ctrl.Result{}, r.markNotReady(ctx, machine,
			containerv1a1.ReasonNotAdopted,
			fmt.Sprintf("VirtualMachine %q, named by the %s annotation, does "+
				"not exist yet", named, containerv1a1.AnnotationKey))

	case errors.Is(err, link.ErrNotMutual):
		return ctrl.Result{}, r.markNotReady(ctx, machine,
			containerv1a1.ReasonNotAdopted,
			fmt.Sprintf("VirtualMachine %q does not name this ContainerMachine "+
				"in spec.infrastructureRef; a link needs both sides", named))

	case err != nil:
		return ctrl.Result{}, err
	}

	if err := r.claim(ctx, machine); err != nil {
		return ctrl.Result{}, err
	}

	image := parent.Spec.BootDisk
	if machine.Status.ContainerID == "" {
		if image == nil || image.Source.Image == nil || image.Source.Image.Name == "" {
			return ctrl.Result{}, r.markNotReady(ctx, machine,
				containerv1a1.ReasonInvalidConfiguration,
				"no boot image named in spec.bootDisk.source.image.name, "+
					"and this provider requires one to create a container")
		}
	}

	if err := r.persistResolved(ctx, machine, parent); err != nil {
		if apierrors.IsInvalid(err) {
			return ctrl.Result{}, r.markNotReady(ctx, machine,
				containerv1a1.ReasonInvalidConfiguration, err.Error())
		}
		return ctrl.Result{}, err
	}

	return r.reconcileContainer(ctx, machine)
}

// claim writes the finalizer that makes this object ours.
//
// Ordered before any engine call: a container created while the finalizer is
// missing is leaked outright if the object is deleted immediately after.
func (r *Reconciler) claim(ctx context.Context,
	machine *containerv1a1.ContainerMachine) error {

	base := machine.DeepCopy()
	controllerutil.AddFinalizer(machine, containerv1a1.Finalizer)
	return r.patchIfChanged(ctx, machine, base)
}

// persistResolved writes what was resolved from the parent into this
// object's own spec, so `kubectl get -o yaml` shows what the container will
// actually run — the same rule external/kubevm-provider-aws follows for the
// same reason.
func (r *Reconciler) persistResolved(ctx context.Context,
	machine *containerv1a1.ContainerMachine,
	parent *kubevmv1a1.VirtualMachine) error {

	base := machine.DeepCopy()

	// Image is fixed once the container exists: a running container keeps
	// the image it was created from, and this provider does not implement
	// an in-place image swap.
	if machine.Status.ContainerID == "" &&
		parent.Spec.BootDisk != nil && parent.Spec.BootDisk.Source.Image != nil {
		machine.Spec.Image = parent.Spec.BootDisk.Source.Image.Name
	}

	// PowerState is reasserted every reconcile: it is the one thing about a
	// machine meant to change after creation.
	machine.Spec.PowerState = string(parent.Spec.PowerState)

	return r.patchIfChanged(ctx, machine, base)
}

// patchIfChanged writes the object only when it actually differs from base,
// so a settled machine's reconcile does not itself wake this controller
// again.
func (r *Reconciler) patchIfChanged(ctx context.Context,
	machine, base *containerv1a1.ContainerMachine) error {

	if equality.Semantic.DeepEqual(base, machine) {
		return nil
	}
	patch := client.MergeFromWithOptions(base,
		client.MergeFromWithOptimisticLock{})
	if err := r.Patch(ctx, machine, patch); err != nil {
		return fmt.Errorf("patching %s/%s: %w",
			machine.Namespace, machine.Name, err)
	}
	return nil
}

// patchStatusIfChanged writes the status only when it differs from base.
func (r *Reconciler) patchStatusIfChanged(ctx context.Context,
	machine, base *containerv1a1.ContainerMachine) error {

	if equality.Semantic.DeepEqual(base.Status, machine.Status) {
		return nil
	}
	patch := client.MergeFromWithOptions(base,
		client.MergeFromWithOptimisticLock{})
	if err := r.Status().Patch(ctx, machine, patch); err != nil {
		return fmt.Errorf("patching status of %s/%s: %w",
			machine.Namespace, machine.Name, err)
	}
	return nil
}

// markNotReady records why this object is not adopted or not configurable,
// without touching the engine.
func (r *Reconciler) markNotReady(ctx context.Context,
	machine *containerv1a1.ContainerMachine, reason, message string) error {

	base := machine.DeepCopy()
	setCondition(machine, metav1.Condition{
		Type:    containerv1a1.ConditionInfrastructureReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	return r.patchStatusIfChanged(ctx, machine, base)
}

// setCondition upserts one condition by type, bumping ObservedGeneration to
// this object's current generation.
func setCondition(machine *containerv1a1.ContainerMachine, c metav1.Condition) {
	c.ObservedGeneration = machine.Generation
	machine.Status.ObservedGeneration = machine.Generation

	meta := &machine.Status.Conditions
	for i := range *meta {
		if (*meta)[i].Type == c.Type {
			if (*meta)[i].Status != c.Status {
				c.LastTransitionTime = metav1.Now()
			} else {
				c.LastTransitionTime = (*meta)[i].LastTransitionTime
			}
			(*meta)[i] = c
			return
		}
	}
	if c.LastTransitionTime.IsZero() {
		c.LastTransitionTime = metav1.Now()
	}
	*meta = append(*meta, c)
}

// reconcileDelete stops and removes the container, then releases the
// finalizer.
func (r *Reconciler) reconcileDelete(ctx context.Context,
	machine *containerv1a1.ContainerMachine) (ctrl.Result, error) {

	if !controllerutil.ContainsFinalizer(machine, containerv1a1.Finalizer) {
		return ctrl.Result{}, nil
	}

	eng := r.engine(machine.Spec.Runtime)
	if err := eng.Remove(ctx, containerName(machine)); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing container: %w", err)
	}

	base := machine.DeepCopy()
	controllerutil.RemoveFinalizer(machine, containerv1a1.Finalizer)
	return ctrl.Result{}, r.patchIfChanged(ctx, machine, base)
}

// SetupWithManager registers the controller, and the watch that makes an
// edit to the portable object reach this one.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&containerv1a1.ContainerMachine{}).
		Watches(&kubevmv1a1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(machineForVirtualMachine)).
		Complete(r)
}

// machineForVirtualMachine maps a portable object to the provider object it
// names, so a power-state edit on the VirtualMachine wakes this controller
// without waiting for the core's own sync period.
func machineForVirtualMachine(
	_ context.Context, o client.Object,
) []reconcile.Request {

	vm, ok := o.(*kubevmv1a1.VirtualMachine)
	if !ok {
		return nil
	}
	ref := vm.Spec.InfrastructureRef
	if ref.APIGroup != containerv1a1.GroupName || ref.Kind != "ContainerMachine" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: vm.Namespace,
			Name:      ref.Name,
		},
	}}
}
