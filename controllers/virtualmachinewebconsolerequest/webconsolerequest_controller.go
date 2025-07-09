// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinewebconsolerequest

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/proxyaddr"
)

const (
	DefaultExpiryTime = time.Second * 120
	UUIDLabelKey      = "vmoperator.vmware.com/webconsolerequest-uuid"
)

// addToManager adds this package's controller to the provided manager.
func addToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineWebConsoleRequest{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		ctx.VMProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			LogConstructor:          pkgutil.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	vmProvider providers.VirtualMachineProviderInterface) *Reconciler {
	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: vmProvider,
	}
}

// Reconciler reconciles a WebConsoleRequest object.
type Reconciler struct {
	client.Client
	Context    context.Context
	Logger     logr.Logger
	Recorder   record.Recorder
	VMProvider providers.VirtualMachineProviderInterface
}

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinewebconsolerequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachinewebconsolerequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	webconsolerequest := &vmopv1.VirtualMachineWebConsoleRequest{}
	err := r.Get(ctx, req.NamespacedName, webconsolerequest)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	webConsoleRequestCtx := &pkgctx.WebConsoleRequestContextV1{
		Context:           ctx,
		Logger:            pkgutil.FromContextOrDefault(ctx),
		WebConsoleRequest: webconsolerequest,
		VM:                &vmopv1.VirtualMachine{},
	}

	done, err := r.ReconcileEarlyNormal(webConsoleRequestCtx)
	if err != nil {
		webConsoleRequestCtx.Logger.Error(err, "failed to expire WebConsoleRequest")
		return ctrl.Result{}, err
	}
	if done {
		return ctrl.Result{}, nil
	}

	err = r.Get(ctx, client.ObjectKey{Name: webconsolerequest.Spec.Name, Namespace: webconsolerequest.Namespace}, webConsoleRequestCtx.VM)
	if err != nil {
		r.Recorder.Warn(webConsoleRequestCtx.WebConsoleRequest, "VirtualMachine Not Found", "")
		webConsoleRequestCtx.Logger.Error(err, "failed to get subject vm %s", webconsolerequest.Spec.Name)
		return ctrl.Result{}, fmt.Errorf("failed to get subject vm %s: %w", webconsolerequest.Spec.Name, err)
	}

	patchHelper, err := patch.NewHelper(webconsolerequest, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", webConsoleRequestCtx, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, webconsolerequest); err != nil {
			if reterr == nil {
				reterr = err
			}
			webConsoleRequestCtx.Logger.Error(err, "patch failed")
		}
	}()

	if err := r.ReconcileNormal(webConsoleRequestCtx); err != nil {
		webConsoleRequestCtx.Logger.Error(err, "failed to reconcile WebConsoleRequest")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true, RequeueAfter: DefaultExpiryTime}, nil
}

func (r *Reconciler) ReconcileEarlyNormal(ctx *pkgctx.WebConsoleRequestContextV1) (bool, error) {
	expiryTime := ctx.WebConsoleRequest.Status.ExpiryTime
	nowTime := metav1.Now()
	if !expiryTime.IsZero() && !nowTime.Before(&expiryTime) {
		err := r.Delete(ctx, ctx.WebConsoleRequest)
		if client.IgnoreNotFound(err) != nil {
			return false, fmt.Errorf("failed to delete webconsolerequest: %w", err)
		}
		ctx.Logger.Info("Deleted expired WebConsoleRequest")
		return true, nil
	}

	if ctx.WebConsoleRequest.Status.Response != "" &&
		ctx.WebConsoleRequest.Status.ProxyAddr != "" {
		// If the response and proxy address are already set, no need to reconcile anymore
		ctx.Logger.Info("Response and proxy address already set, skip reconciling")
		return true, nil
	}

	return false, nil
}

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.WebConsoleRequestContextV1) error {
	ctx.Logger.Info("Reconciling WebConsoleRequest")
	defer func() {
		ctx.Logger.Info("Finished reconciling WebConsoleRequest")
	}()

	ticket, err := r.VMProvider.GetVirtualMachineWebMKSTicket(ctx, ctx.VM, ctx.WebConsoleRequest.Spec.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to get webmksticket: %w", err)
	}
	r.Recorder.EmitEvent(ctx.WebConsoleRequest, "Acquired Ticket", nil, false)

	ctx.WebConsoleRequest.Status.Response = ticket
	ctx.WebConsoleRequest.Status.ExpiryTime = metav1.NewTime(metav1.Now().Add(DefaultExpiryTime))

	proxyAddr, err := proxyaddr.ProxyAddress(ctx, r)
	if err != nil {
		return err
	}
	ctx.WebConsoleRequest.Status.ProxyAddr = proxyAddr

	// Add UUID as a Label to the current WebConsoleRequest resource after acquiring the ticket.
	// This will be used when validating the connection request from users to the web console URL.
	if ctx.WebConsoleRequest.Labels == nil {
		ctx.WebConsoleRequest.Labels = make(map[string]string)
	}
	ctx.WebConsoleRequest.Labels[UUIDLabelKey] = string(ctx.WebConsoleRequest.UID)

	err = r.ReconcileOwnerReferences(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) ReconcileOwnerReferences(ctx *pkgctx.WebConsoleRequestContextV1) error {
	isController := true
	ownerRef := metav1.OwnerReference{
		APIVersion: ctx.VM.APIVersion,
		Kind:       ctx.VM.Kind,
		Name:       ctx.VM.Name,
		UID:        ctx.VM.UID,
		Controller: &isController,
	}

	ctx.WebConsoleRequest.SetOwnerReferences([]metav1.OwnerReference{ownerRef})
	return nil
}
