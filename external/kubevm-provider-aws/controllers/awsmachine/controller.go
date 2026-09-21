// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package awsmachine reconciles an AWSMachine into an EC2 instance.
//
// The reconcile has six concerns with genuinely different failure modes, and
// they live in six files rather than one so the refusal paths are findable:
// this file claims and dispatches, create.go launches, observe.go reports,
// power.go starts and stops, publicip.go adds or removes the public address,
// delete.go terminates.
package awsmachine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	awsv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/intent"
)

// bootRequeue is how long to wait while a machine is between steady states.
//
// Ten seconds, matching what VM Operator uses for the analogous "powered on,
// no address yet" case. Adopted rather than invented: no evidence was gathered
// for a better number, and copying a value somebody else already tuned beats
// making one up.
const bootRequeue = 10 * time.Second

// propagationGrace is how long a just-created instance may be invisible to
// DescribeInstances before its absence is believed.
//
// RunInstances returns an id before that id has reached every endpoint, so an
// immediate lookup can answer InvalidInstanceID.NotFound for a machine that is
// running. Reporting "gone" in that window tells every reader a live machine
// was destroyed, and the vendor-neutral controller can freeze that answer onto
// the portable object indefinitely.
//
// Two minutes is generous against a gap measured in seconds. The cost of
// waiting too long is a slightly late error on a machine that really did
// vanish; the cost of not waiting is declaring a healthy machine dead. Those
// are not symmetric.
const propagationGrace = 2 * time.Minute

// Reconciler turns an AWSMachine into an EC2 instance.
type Reconciler struct {
	client.Client

	// EC2 is the narrow interface, so a test can substitute a fake and
	// "no test reaches AWS" stays provable by construction.
	EC2 ec2.Client

	// PropagationGrace overrides how long a just-created instance may be
	// invisible to DescribeInstances before its absence is believed. Zero
	// means propagationGrace.
	//
	// Injectable only so a test can reach BOTH sides of that window. Without
	// it the terminal "gone" path is unreachable in a test, because an object
	// the API server just created is always inside the grace period -- and an
	// unreachable path is one nobody notices breaking.
	PropagationGrace time.Duration

	// Now returns the current time, and is replaced by tests that need a
	// launch to be older than it is. Nil means time.Now.
	Now func() time.Time
}

// now returns the current time from this reconciler's clock.
func (r *Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// grace is the propagation window this reconciler uses.
func (r *Reconciler) grace() time.Duration {
	if r.PropagationGrace > 0 {
		return r.PropagationGrace
	}
	return propagationGrace
}

// Every verb here has a caller. delete on awsmachines is the hosted core's:
// it deletes this provider object when the portable one goes. There is no
// update anywhere, because every write in this module is a merge patch, and
// no awsmachines/finalizers grant, because nothing owns an AWSMachine.
//
// +kubebuilder:rbac:groups=infrastructure.kube-vm.io,resources=awsmachines,verbs=get;list;watch;patch;delete
// +kubebuilder:rbac:groups=infrastructure.kube-vm.io,resources=awsmachines/status,verbs=get;patch
// +kubebuilder:rbac:groups=kube-vm.io,resources=virtualmachines,verbs=get;list;watch
//
// Leader election and events. Without them the manager authenticates to AWS
// and then waits forever for its lease, and no envtest can catch it: its
// client is an administrator, so ClusterRoles are never consulted. The lease
// lock takes get, create and update; the event recorder takes create and
// patch.
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile drives one AWSMachine towards the state its parent asks for.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (
	ctrl.Result, error) {

	machine := &awsv1a1.AWSMachine{}
	if err := r.Get(ctx, req.NamespacedName, machine); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// The instance id on every line, once it is known. An object's conditions
	// say what is happening now; the log is the only account that survives
	// the object, and the delete path is where the expensive failure lives.
	if id := instanceIDFrom(machine.Status.ProviderID); id != "" {
		ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues(
			"instance", id))
	}

	if !machine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, machine)
	}
	return r.reconcileNormal(ctx, machine)
}

// reconcileNormal claims the object, then drives it towards its parent.
func (r *Reconciler) reconcileNormal(ctx context.Context,
	machine *awsv1a1.AWSMachine) (ctrl.Result, error) {

	// Every refusal below makes no AWS call at all -- not "no mutation", no
	// CALL. An object nobody has linked may be somebody else's, and reaching
	// into an account to ask about infrastructure we have not been given is
	// itself wrong.
	named := machine.GetAnnotations()[intent.AnnotationKey]
	parent, err := intent.ParentOf(ctx, r.Client, machine)
	switch {
	case errors.Is(err, intent.ErrNotLinked):
		return ctrl.Result{}, r.markNotReady(ctx, machine,
			awsv1a1.ReasonNotAdopted,
			fmt.Sprintf("no %s annotation: add one naming the "+
				"VirtualMachine this object belongs to", intent.AnnotationKey))

	case errors.Is(err, intent.ErrParentMissing):
		// Not a refusal: the VirtualMachine may simply not exist yet. The
		// watch on VirtualMachine wakes this object when it appears, which is
		// what keeps apply order free.
		return ctrl.Result{}, r.markNotReady(ctx, machine,
			awsv1a1.ReasonNotAdopted,
			fmt.Sprintf("VirtualMachine %q, named by the %s annotation, does "+
				"not exist yet", named, intent.AnnotationKey))

	case errors.Is(err, intent.ErrNotMutual):
		return ctrl.Result{}, r.markNotReady(ctx, machine,
			awsv1a1.ReasonNotAdopted,
			fmt.Sprintf("VirtualMachine %q does not name this AWSMachine in "+
				"spec.infrastructureRef; a link needs both sides", named))

	case err != nil:
		return ctrl.Result{}, err
	}

	// Two returns, because a refusal and a failure to record one are
	// different outcomes. Collapsed into a single error they are
	// indistinguishable -- a refusal whose status write succeeded reads as
	// "carry on", and the reconcile launches anyway.
	refused, err := r.refuseIfOwnedByAnother(ctx, machine, parent)
	if err != nil {
		return ctrl.Result{}, err
	}
	if refused {
		return ctrl.Result{}, nil
	}

	if err := r.claim(ctx, machine); err != nil {
		return ctrl.Result{}, err
	}

	// Before the launch, a missing instance type or image is refused: EC2
	// would silently substitute a default, producing a machine at a size or
	// from an image nobody chose. After it, the instance keeps what it was
	// launched with, so a field that disappears is reported as not applied
	// while power and status go on being reconciled.
	var want *intent.Intent
	if machine.Status.LaunchRequested != nil {
		want = intent.ReadLaunched(parent)
	} else {
		read, err := intent.Read(parent)
		if err != nil {
			return ctrl.Result{}, r.markNotReady(ctx, machine,
				awsv1a1.ReasonInvalidConfiguration, err.Error())
		}
		want = read
	}

	if err := r.persistResolved(ctx, machine, want); err != nil {
		// A rejected write is reported on the object, not only returned.
		// The schema constrains these fields, and a value the API server
		// refuses would otherwise leave the machine silently stuck with no
		// condition to read and nothing but a log line to go on.
		if apierrors.IsInvalid(err) {
			return ctrl.Result{}, r.markNotReady(ctx, machine,
				awsv1a1.ReasonInvalidConfiguration, err.Error())
		}
		return ctrl.Result{}, err
	}
	noteDelegatedDrift(machine, want)

	return r.reconcileInstance(ctx, machine, want)
}

// noteDelegatedDrift records any create-time value the portable object now
// asks for that differs from the one already resolved.
//
// Reported rather than applied. All three are settled when the instance is
// launched and cannot be changed afterwards, so the honest answer is to say
// so and keep reconciling everything else -- not to fail, which stops power
// and status too, and not to stay silent, which leaves UpToDate claiming
// everything was applied when it was not.
//
// Routed through Unsupported so it reaches the same UpToDate condition as
// every other unhonoured field. Two mechanisms writing one condition is how
// the last one to run wins and the first one's message disappears.
func noteDelegatedDrift(machine *awsv1a1.AWSMachine, want *intent.Intent) {
	// Only "the portable object now asks for nothing" is skipped: a field it
	// stopped naming is reported by intent.ReadAfterLaunch instead. An empty
	// value HERE is not skipped -- a machine launched without a subnet,
	// because nobody named one, and a parent that names one afterwards is
	// exactly the drift this exists to report.
	drift := func(field, have, wanted, why string) {
		if wanted == "" || have == wanted {
			return
		}
		if have == "" {
			want.Unsupported = append(want.Unsupported, fmt.Sprintf(
				"spec.%s cannot become %q: the instance was launched "+
					"without one, and %s", field, wanted, why))
			return
		}
		want.Unsupported = append(want.Unsupported, fmt.Sprintf(
			"spec.%s is %q and cannot become %q: %s", field, have, wanted,
			why))
	}

	drift("instanceType", machine.Spec.InstanceType, want.InstanceType,
		"an instance keeps the type it launched with, and this provider "+
			"does not resize")
	drift("imageID", machine.Spec.ImageID, want.ImageID,
		"an instance keeps the image it booted from")
	drift("subnet", machine.Spec.Subnet, want.Subnet,
		"a subnet pins the availability zone, and moving a running "+
			"instance between zones is not an edit")
}

// refuseIfOwnedByAnother reports whether this machine belongs to a different
// portable object, and records why if so.
//
// Two returns rather than one, because "refused" and "failed to record the
// refusal" are different outcomes and collapsing them is how a refusal becomes
// a launch.
func (r *Reconciler) refuseIfOwnedByAnother(ctx context.Context,
	machine *awsv1a1.AWSMachine, parent *kubevmv1a1.VirtualMachine) (
	bool, error) {

	for _, ref := range machine.GetOwnerReferences() {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		// Group as well as kind: vmoperator.vmware.com serves a VirtualMachine
		// too, and in a cluster running both, matching on the kind alone would
		// read somebody else's owner as this object's parent.
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil || gv.Group != kubevmv1a1.GroupName ||
			ref.Kind != "VirtualMachine" {
			continue
		}
		// UID, not name. A parent deleted and recreated under the same name is
		// a different object, and the core writes the UID for exactly this.
		if ref.UID == parent.UID {
			continue
		}
		return true, r.markNotReady(ctx, machine,
			awsv1a1.ReasonAlreadyOwned,
			fmt.Sprintf("already owned by VirtualMachine %q; refusing to "+
				"take it over", ref.Name))
	}
	return false, nil
}

// claim writes the finalizer that makes this object ours.
//
// Only the finalizer. The back-reference annotation is the user's to write --
// it is their consent to this object being claimed -- so by the time this
// runs, ParentOf has already confirmed both sides of the link.
//
// Ordered before any AWS call: an instance launched while the finalizer is
// missing is leaked outright. Deleting the object would then succeed
// immediately, and the instance runs on, billing, with nothing left in the
// cluster that knows it exists. The patch below completes before the caller
// proceeds, so the finalizer is durable by the time anything reaches EC2 -- no
// requeue needed to make it so.
func (r *Reconciler) claim(ctx context.Context,
	machine *awsv1a1.AWSMachine) error {

	base := machine.DeepCopy()
	controllerutil.AddFinalizer(machine, awsv1a1.Finalizer)
	return r.patchIfChanged(ctx, machine, base, "claiming the object")
}

// patchIfChanged writes the object only when it actually differs from base.
//
// controller-runtime's typedClient.Patch has no empty-body short circuit: it
// computes the body and issues the request either way. A settled machine does
// not poll (see observe.go -- only transitional states requeue), so the saving
// is small and nothing about correctness rests on it. It is here to keep
// no-op writes out of the audit log and the apiserver's request metrics, and
// because one shared guard is cheaper to read than the per-field bookkeeping
// it replaced. Upstream's patch helper short-circuits the same way, in three
// separate places.
func (r *Reconciler) patchIfChanged(ctx context.Context,
	machine, base *awsv1a1.AWSMachine, what string) error {

	if equality.Semantic.DeepEqual(base, machine) {
		return nil
	}
	// Optimistic: finalizers and owner references are lists, which a merge
	// patch replaces whole, and others write them too -- the core sets its
	// owner reference here, and a user or the API server may add a
	// finalizer. A stale write fails with a conflict and is retried.
	patch := client.MergeFromWithOptions(base,
		client.MergeFromWithOptimisticLock{})
	if err := r.Patch(ctx, machine, patch); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// persistResolved writes what was resolved into the machine's own spec.
//
// `kubectl get -o yaml` has to show what the machine will
// actually do, because other controllers read this object and GitOps diffing,
// backup, restore and audit all assume the spec is truthful. Resolving into a
// local variable would leave the object lying about itself.
func (r *Reconciler) persistResolved(ctx context.Context,
	machine *awsv1a1.AWSMachine, want *intent.Intent) error {

	base := machine.DeepCopy()

	// Kept in step with the portable object until the launch is requested,
	// then never rewritten. All three are create-time facts: an instance
	// cannot change its AMI, a subnet pins the availability zone, and this
	// provider does not implement resize. An object-level CEL rule refuses
	// any change to them once status.launchRequested is set -- a refusal
	// that is permanent, not transient, which is why they are not reasserted
	// after it.
	//
	// Until then they are copied every pass, so a value written in by hand
	// before the launch reverts, and an edit to the portable object after EC2
	// refused a launch is simply the new request. The porting guide's two
	// branches under "Field handling": these are the first; divergence after
	// the launch
	// is reported by noteDelegatedDrift, not applied.
	if machine.Status.LaunchRequested == nil {
		machine.Spec.ImageID = want.ImageID
		machine.Spec.Subnet = want.Subnet
		machine.Spec.InstanceType = want.InstanceType
	}

	// Reasserted every reconcile: the guide's second branch. The portable
	// object always wins here, which is what makes a hand-edit of either
	// field revert rather than take effect. Power state is the one thing
	// about a machine meant to change. The public address is changeable too:
	// EC2 adds or removes it on a running instance's primary interface.
	// Copied exactly, nil included: nil means the portable object states no
	// preference, and the subnet's default stays in charge.
	machine.Spec.PowerState = string(want.PowerState)
	machine.Spec.PublicIP = nil
	if want.PublicIP != nil {
		v := *want.PublicIP
		machine.Spec.PublicIP = &v
	}

	return r.patchIfChanged(ctx, machine, base,
		"persisting resolved values into spec")
}

// patchStatusIfChanged writes the status only when it differs from base.
//
// A write that changes nothing still wakes every watcher of this object,
// this controller included, so skipping it is what lets a settled machine
// stay quiet.
//
// Optimistically locked, like the spec patch above. Status carries two lists,
// conditions and addresses, and a merge patch replaces a list wholesale: with
// a basis read from a cache that may be behind, an unlocked patch can drop
// what somebody else just wrote. A conflict is not a problem here, because
// the reconcile is idempotent and the next pass rebuilds the same status.
//
// A NotFound is NOT swallowed. The create path writes status before it calls
// RunInstances, precisely so a launch whose reply is lost still leaves a
// trace; reporting that write as successful when the object has gone would
// hand back the guarantee it exists for.
func (r *Reconciler) patchStatusIfChanged(ctx context.Context,
	machine, base *awsv1a1.AWSMachine) error {

	if equality.Semantic.DeepEqual(base.Status, machine.Status) {
		return nil
	}
	patch := client.MergeFromWithOptions(base,
		client.MergeFromWithOptimisticLock{})
	if err := r.Status().Patch(ctx, machine, patch); err != nil {
		return fmt.Errorf("writing status: %w", err)
	}
	return nil
}

// SetupWithManager registers the controller, and the watch that makes an
// edit to the portable object reach this one.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1a1.AWSMachine{}).
		// Without this watch, editing the portable object triggers nothing:
		// the user changes power state or instance type and the machine
		// simply never follows.
		Watches(&kubevmv1a1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(machineForVirtualMachine)).
		Complete(r)
}

// machineForVirtualMachine maps a portable object to the provider object it
// names.
func machineForVirtualMachine(
	_ context.Context, o client.Object,
) []reconcile.Request {

	vm, ok := o.(*kubevmv1a1.VirtualMachine)
	if !ok {
		return nil
	}
	ref := vm.Spec.InfrastructureRef
	if ref.APIGroup != awsv1a1.GroupName || ref.Kind != "AWSMachine" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: vm.Namespace,
			Name:      ref.Name,
		},
	}}
}
