// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"context"
	"fmt"
	"reflect"
	"strings"

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
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	deleteFailedReason = "DeleteFailed"
)

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
		record.New(mgr.GetEventRecorder(controllerNameShort)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, ctx.MaxConcurrentReconciles),
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
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a Tag object. It owns the Tag object only — waking
// the VMs affected by a Tag's lifecycle is the VM controller's Tag watch, so
// this reconciler never lists or enqueues a VirtualMachine.
type Reconciler struct {
	ctrlclient.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
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

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		err := apierrorsutil.FilterOut(patchHelper.Patch(ctx, &obj), apierrors.IsNotFound)
		if err != nil {
			if reterr == nil {
				reterr = fmt.Errorf("failed to create patch helper: %w", err)
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

	if mirror, ok := obj.Labels[obj.Spec.Key]; !ok || mirror != obj.Spec.Value {
		if obj.Labels == nil {
			obj.Labels = make(map[string]string, 1)
		}
		obj.Labels[obj.Spec.Key] = obj.Spec.Value
	}

	if len(obj.OwnerReferences) == 0 {
		// Preconditioned on the ResourceVersion just read: if a VM
		// concurrently added an owner reference since, this fails with a
		// conflict instead of deleting a Tag that is no longer unowned, and
		// the next reconcile re-reads the fresh state.
		err := r.Delete(ctx, obj, ctrlclient.Preconditions{ResourceVersion: &obj.ResourceVersion})
		if err != nil && !apierrors.IsNotFound(err) {
			conditions.MarkError(obj, vspherepolv1.ReadyConditionType, deleteFailedReason, err)
			return ctrl.Result{}, fmt.Errorf("failed to delete Tag with no owners: %w", err)
		}

		conditions.MarkFalse(
			obj,
			vspherepolv1.ReadyConditionType,
			vspherepolv1.TagNoOwnersReason,
			"Tag has no owners and is being deleted")

		return ctrl.Result{}, nil
	}

	obj.Status.ObservedGeneration = obj.Generation

	conditions.Set(obj, &metav1.Condition{
		Type:   vspherepolv1.ReadyConditionType,
		Status: metav1.ConditionTrue,
		Reason: vspherepolv1.TagReadyReason,
	})

	return ctrl.Result{}, nil
}
