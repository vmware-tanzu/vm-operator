// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validatingwebhookconfiguration

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
)

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &admissionv1.ValidatingWebhookConfiguration{}
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

	c, err := controller.New(controllerNameShort, mgr, controller.Options{
		Reconciler:     r,
		LogConstructor: pkgutil.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
	})
	if err != nil {
		return err
	}

	cache, err := pkgmgr.NewNamespacedCacheForObject(mgr, &ctx.SyncPeriod, controlledType)
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(cache, controlledType, &handler.TypedEnqueueRequestForObject[*admissionv1.ValidatingWebhookConfiguration]{},
		predicate.TypedFuncs[*admissionv1.ValidatingWebhookConfiguration]{
			CreateFunc: func(e event.TypedCreateEvent[*admissionv1.ValidatingWebhookConfiguration]) bool {
				return e.Object.GetName() == spqutil.ValidatingWebhookConfigName
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*admissionv1.ValidatingWebhookConfiguration]) bool {
				return e.ObjectOld.GetName() == spqutil.ValidatingWebhookConfigName
			},
			DeleteFunc: func(e event.TypedDeleteEvent[*admissionv1.ValidatingWebhookConfiguration]) bool {
				return false
			},
			GenericFunc: func(e event.TypedGenericEvent[*admissionv1.ValidatingWebhookConfiguration]) bool {
				return false
			},
		},
		kubeutil.TypedResourceVersionChangedPredicate[*admissionv1.ValidatingWebhookConfiguration]{},
	))
}

func NewReconciler(ctx context.Context, client client.Client, logger logr.Logger, recorder record.Recorder) *Reconciler {

	return &Reconciler{
		Context:  ctx,
		Client:   client,
		Logger:   logger,
		Recorder: recorder,
	}
}

// Reconciler reconciles a ValidatingWebhookConfiguration object.
type Reconciler struct {
	client.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.Recorder
}

// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch
// +kubebuilder:rbac:groups=cns.vmware.com,resources=storagepolicyusages,verbs=list

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	if req.Name != spqutil.ValidatingWebhookConfigName {
		pkgutil.FromContextOrDefault(ctx).Error(nil, "Reconcile called for unexpected object")
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, r.ReconcileNormal(ctx, req)
}

func (r *Reconciler) ReconcileNormal(ctx context.Context, req ctrl.Request) error {
	logger := pkgutil.FromContextOrDefault(ctx)
	logger.Info("Reconciling validating webhook configuration")

	caBundle, err := spqutil.GetWebhookCABundle(ctx, r.Client)
	if err != nil {
		return err
	}

	spuList := &spqv1.StoragePolicyUsageList{}
	if err := r.Client.List(ctx, spuList); err != nil {
		return fmt.Errorf("unable to list StoragePolicyUsage objects: %w", err)
	}

	for _, spu := range spuList.Items {
		switch spu.Spec.ResourceExtensionName {
		case spqutil.StoragePolicyQuotaVMSnapshotExtensionName:
			if !pkgcfg.FromContext(ctx).Features.VMSnapshots {
				logger.V(4).Info(
					"Skipping VMSnapshot SPU due to disabled feature flag",
					"name", spu.Name, "namespace", spu.Namespace,
				)

				continue
			}
			// proceed to shared patch logic

		case spqutil.StoragePolicyQuotaVMExtensionName:
			// proceed to shared patch logic

		default:
			// Not a target SPU type; skip
			continue
		}
		// shared patch logic
		if !bytes.Equal(spu.Spec.CABundle, caBundle) {
			spuPatch := client.MergeFrom(spu.DeepCopy())
			spu.Spec.CABundle = caBundle

			if err := r.Client.Patch(ctx, &spu, spuPatch); err != nil {
				return fmt.Errorf("unable to patch StoragePolicyUsage object: %w", err)
			}
		}
	}

	return nil
}
