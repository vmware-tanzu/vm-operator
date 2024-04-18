// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"context"

	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1/patch"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	DefaultExpiryTime = time.Second * 120
	UUIDLabelKey      = "vmoperator.vmware.com/webconsolerequest-uuid"

	ProxyAddrServiceName      = "kube-apiserver-lb-svc"
	ProxyAddrServiceNamespace = "kube-system"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1a1.WebConsoleRequest{}
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
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	provider providers.VirtualMachineProviderInterface) *Reconciler {

	return &Reconciler{
		Context:    ctx,
		Client:     client,
		Logger:     logger,
		Recorder:   recorder,
		VMProvider: provider,
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

// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=webconsolerequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=webconsolerequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	webconsolerequest := &vmopv1a1.WebConsoleRequest{}
	err := r.Get(ctx, req.NamespacedName, webconsolerequest)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	webConsoleRequestCtx := &pkgctx.WebConsoleRequestContext{
		Context:           ctx,
		Logger:            ctrl.Log.WithName("WebConsoleRequest").WithValues("name", req.NamespacedName),
		WebConsoleRequest: webconsolerequest,
		VM:                &vmopv1a1.VirtualMachine{},
	}

	done, err := r.ReconcileEarlyNormal(webConsoleRequestCtx)
	if err != nil {
		webConsoleRequestCtx.Logger.Error(err, "failed to expire WebConsoleRequest")
		return ctrl.Result{}, err
	}
	if done {
		return ctrl.Result{}, nil
	}

	err = r.Get(ctx, client.ObjectKey{Name: webconsolerequest.Spec.VirtualMachineName, Namespace: webconsolerequest.Namespace}, webConsoleRequestCtx.VM)
	if err != nil {
		r.Recorder.Warn(webConsoleRequestCtx.WebConsoleRequest, "VirtualMachine Not Found", "")
		webConsoleRequestCtx.Logger.Error(err, "failed to get subject vm %s", webconsolerequest.Spec.VirtualMachineName)
		return ctrl.Result{}, errors.Wrapf(err, "failed to get subject vm %s", webconsolerequest.Spec.VirtualMachineName)
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

func (r *Reconciler) ReconcileEarlyNormal(ctx *pkgctx.WebConsoleRequestContext) (bool, error) {
	expiryTime := ctx.WebConsoleRequest.Status.ExpiryTime
	nowTime := metav1.Now()
	if !expiryTime.IsZero() && !nowTime.Before(&expiryTime) {
		err := r.Delete(ctx, ctx.WebConsoleRequest)
		if client.IgnoreNotFound(err) != nil {
			return false, errors.Wrapf(err, "failed to delete webconsolerequest")
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

func (r *Reconciler) ReconcileNormal(ctx *pkgctx.WebConsoleRequestContext) error {
	ctx.Logger.Info("Reconciling WebConsoleRequest")
	defer func() {
		ctx.Logger.Info("Finished reconciling WebConsoleRequest")
	}()

	v1a3VM := &vmopv1.VirtualMachine{}
	if err := ctx.VM.ConvertTo(v1a3VM); err != nil {
		return errors.Wrapf(err, "failed to convert VM to v1a2")
	}

	ticket, err := r.VMProvider.GetVirtualMachineWebMKSTicket(ctx, v1a3VM, ctx.WebConsoleRequest.Spec.PublicKey)
	if err != nil {
		return errors.Wrapf(err, "failed to get webmksticket")
	}

	r.Recorder.EmitEvent(ctx.WebConsoleRequest, "Acquired Ticket", nil, false)

	ctx.WebConsoleRequest.Status.Response = ticket
	ctx.WebConsoleRequest.Status.ExpiryTime = metav1.NewTime(metav1.Now().Add(DefaultExpiryTime))

	// Retrieve the proxy address from the load balancer service ingress IP.
	proxySvc := &corev1.Service{}
	proxySvcObjectKey := client.ObjectKey{Name: ProxyAddrServiceName, Namespace: ProxyAddrServiceNamespace}
	err = r.Get(ctx, proxySvcObjectKey, proxySvc)
	if err != nil {
		return errors.Wrapf(err, "failed to get proxy address service  %s", proxySvcObjectKey)
	}
	if len(proxySvc.Status.LoadBalancer.Ingress) == 0 {
		return errors.Errorf("no ingress found for proxy address service %s", proxySvcObjectKey)
	}

	ctx.WebConsoleRequest.Status.ProxyAddr = proxySvc.Status.LoadBalancer.Ingress[0].IP

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

func (r *Reconciler) ReconcileOwnerReferences(ctx *pkgctx.WebConsoleRequestContext) error {
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
