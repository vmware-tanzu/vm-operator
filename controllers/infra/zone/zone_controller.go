// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package zone

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/watcher"
)

// SkipNameValidation is used for testing to allow multiple controllers with the
// same name since Controller-Runtime has a global singleton registry to
// prevent controllers with the same name, even if attached to different
// managers.
var SkipNameValidation *bool

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &topologyv1.Zone{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			SkipNameValidation: SkipNameValidation,
			LogConstructor:     pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Finalizer is the finalizer placed on Zone objects by VM Operator.
const Finalizer = "vmoperator.vmware.com/zone-finalizer"

// Reconciler reconciles a StoragePolicyQuota object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones/status,verbs=get

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)
	ctx = watcher.JoinContext(ctx, r.Context)

	var obj topologyv1.Zone
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(&obj, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, &obj); err != nil {
			if reterr == nil {
				reterr = err
			} else {
				reterr = fmt.Errorf("%w,%w", err, reterr)
			}
		}
	}()

	if !obj.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(ctx, &obj)
	}

	return r.ReconcileNormal(ctx, &obj)
}

func (r *Reconciler) ReconcileDelete(
	ctx context.Context,
	obj *topologyv1.Zone) (ctrl.Result, error) {

	if val := obj.Spec.ManagedVMs.FolderMoID; val != "" {
		if err := watcher.Remove(
			ctx,
			vimtypes.ManagedObjectReference{
				Type:  "Folder",
				Value: val,
			},
			fmt.Sprintf("%s/%s", obj.Namespace, obj.Name)); err != nil {

			if !errors.Is(err, watcher.ErrAsyncSignalDisabled) {
				// We don't ignore watcher.ErrNoWatcher here to interlock with the vm watcher
				// service that is in the process of restarting the watcher. This does mean
				// that if watcher cannot start like because of invalid VC creds the finalizer
				// won't be removed.
				return ctrl.Result{}, err
			}
		}
	}
	controllerutil.RemoveFinalizer(obj, Finalizer)

	return ctrl.Result{}, nil
}

func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *topologyv1.Zone) (ctrl.Result, error) {

	if controllerutil.AddFinalizer(obj, Finalizer) {
		// Ensure the finalizer is present before watching this zone.
		return ctrl.Result{}, nil
	}

	if val := obj.Spec.ManagedVMs.FolderMoID; val != "" {
		if err := watcher.Add(
			ctx,
			vimtypes.ManagedObjectReference{
				Type:  "Folder",
				Value: val,
			},
			fmt.Sprintf("%s/%s", obj.Namespace, obj.Name)); err != nil {

			if !errors.Is(err, watcher.ErrAsyncSignalDisabled) {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}
