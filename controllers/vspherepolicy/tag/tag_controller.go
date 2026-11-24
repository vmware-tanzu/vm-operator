// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
)

const (
	// resolveMinBackoff is the first delay before retrying a vCenter tag
	// that does not exist yet.
	resolveMinBackoff = 5 * time.Second

	// resolveMaxBackoff caps the delay between retries.
	resolveMaxBackoff = 5 * time.Minute
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vspherepolv1.Tag{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf(
			"%s-controller", strings.ToLower(controlledTypeName))
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, ctx.MaxConcurrentReconciles),
			SkipNameValidation:      SkipNameValidation,
			LogConstructor: pkglog.ControllerLogConstructor(
				controllerNameShort,
				controlledType,
				mgr.GetScheme()),
		}).
		Complete(r)
}

// NewReconciler returns a new Reconciler for the Tag resource.
func NewReconciler(
	ctx context.Context,
	client ctrlclient.Client,
	logger logr.Logger,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		VMProvider: vmProvider,
	}
}

// Reconciler reconciles a Tag object. It owns the Tag object only — waking
// the VMs affected by a Tag's lifecycle is the VM controller's Tag watch, so
// this reconciler never lists or enqueues a VirtualMachine.
type Reconciler struct {
	ctrlclient.Client
	Context    context.Context
	Logger     logr.Logger
	VMProvider providers.VirtualMachineProviderInterface
}

// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=tags,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vsphere.policy.vmware.com,resources=tags/status,verbs=get;update;patch

// Reconcile fetches the Tag identified by req and reconciles its observed
// state toward the desired state described by its Spec.
func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj vspherepolv1.Tag
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, ctrlclient.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		// This controller owns no finalizer. Leave terminating Tags untouched
		// while garbage collection or another finalizer owner completes deletion.
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf(
			"failed to init patch helper for Tag %s: %w", req.NamespacedName, err)
	}
	defer func() {
		err := apierrorsutil.FilterOut(patchHelper.Patch(ctx, &obj), apierrors.IsNotFound)
		if err != nil {
			if reterr == nil {
				reterr = fmt.Errorf("failed to patch Tag: %w", err)
			} else {
				reterr = fmt.Errorf("%w,%w", err, reterr)
			}
		}
	}()

	return r.ReconcileNormal(ctx, &obj)
}

// ReconcileNormal mirrors the Tag's Key/Value onto its labels and, once it
// has no owners left, deletes the Tag outright with no terminating window.
func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *vspherepolv1.Tag) (ctrl.Result, error) {

	if len(obj.OwnerReferences) == 0 {
		// Preconditioned on the ResourceVersion just read: if a VM
		// concurrently added an owner reference since, this fails with a
		// conflict instead of deleting a Tag that is no longer unowned, and
		// the next reconcile re-reads the fresh state.
		err := r.Delete(ctx, obj, ctrlclient.Preconditions{ResourceVersion: &obj.ResourceVersion})
		if err != nil && !apierrors.IsNotFound(err) {
			conditions.MarkError(obj, vspherepolv1.ReadyConditionType, vspherepolv1.TagDeleteFailedReason, err)
			return ctrl.Result{}, fmt.Errorf("failed to delete Tag with no owners: %w", err)
		}

		// The Tag is gone, so leave obj unmodified: the deferred patch then
		// has no diff and sends no request.
		return ctrl.Result{}, nil
	}

	if obj.Spec.Membership == vspherepolv1.TagMembershipSourceLabel {
		if mirror, ok := obj.Labels[obj.Spec.Key]; !ok || mirror != obj.Spec.Value {
			if obj.Labels == nil {
				obj.Labels = make(map[string]string, 1)
			}
			// Overwrite any stale label value for this key with the value from spec.
			obj.Labels[obj.Spec.Key] = obj.Spec.Value
		}
	}

	obj.Status.ObservedGeneration = obj.Generation

	// The Tag has an owner and any source-label mirror matches its spec.
	conditions.MarkTrue(obj, vspherepolv1.ReadyConditionType)

	// Resolving the vCenter IDs never affects Ready.
	return r.reconcileVCenterTag(ctx, obj)
}

// reconcileVCenterTag records the vCenter tag and category UUIDs in the Tag's
// status. While the vCenter tag does not exist yet, it requeues with an
// exponential backoff.
func (r *Reconciler) reconcileVCenterTag(
	ctx context.Context,
	obj *vspherepolv1.Tag) (ctrl.Result, error) {

	if !virtualmachine.AffinityStatusEnabled(ctx) || obj.Status.ID != "" {
		return ctrl.Result{}, nil
	}

	name := virtualmachine.AddAffinityTagPrefix(obj.Spec.Name)

	categoryID, ids, err := r.VMProvider.ResolveTagIDs(
		ctx,
		obj.Namespace,
		[]string{name})
	if err != nil {
		conditions.MarkError(
			obj,
			vspherepolv1.TagConditionVCenterTagResolved,
			vspherepolv1.TagVCenterTagResolveFailedReason,
			err)
		return ctrl.Result{}, fmt.Errorf(
			"failed to resolve vCenter tag %q: %w", name, err)
	}

	id, ok := ids[name]
	if !ok {
		delay := notFoundBackoff(obj, time.Now())
		conditions.MarkFalse(
			obj,
			vspherepolv1.TagConditionVCenterTagResolved,
			vspherepolv1.TagVCenterTagNotFoundReason,
			"vCenter tag %q does not exist yet", name)
		return ctrl.Result{RequeueAfter: delay}, nil
	}

	obj.Status.ID = id
	obj.Status.CategoryID = categoryID
	conditions.MarkTrue(obj, vspherepolv1.TagConditionVCenterTagResolved)

	return ctrl.Result{}, nil
}

// notFoundBackoff returns the next retry delay for a tag that is not found.
// It is derived from how long the condition has been False/NotFound, which
// roughly doubles the delay on each attempt without any in-memory state. The
// result is clamped to [resolveMinBackoff, resolveMaxBackoff].
func notFoundBackoff(obj *vspherepolv1.Tag, now time.Time) time.Duration {
	c := conditions.Get(obj, vspherepolv1.TagConditionVCenterTagResolved)
	if c == nil ||
		c.Status != metav1.ConditionFalse ||
		c.Reason != vspherepolv1.TagVCenterTagNotFoundReason {

		return resolveMinBackoff
	}
	d := now.Sub(c.LastTransitionTime.Time)
	return min(max(d, resolveMinBackoff), resolveMaxBackoff)
}
