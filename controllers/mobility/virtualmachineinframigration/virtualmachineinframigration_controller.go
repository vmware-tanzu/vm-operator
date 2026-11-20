// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineinframigration

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/go-logr/logr"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	mobilityv1a4 "github.com/vmware-tanzu/vm-operator/external/mobility-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// migrationKind is the kind of the VirtualMachineInfraMigration that is
// referred to in the message of every condition mirrored from it.
const migrationKind = "VirtualMachineInfraMigration"

// maxConditionMessageLen is the most characters that the message of a condition
// can have. The API server rejects a status patch with a longer one.
const maxConditionMessageLen = 32768

// truncationMark ends a message that was truncated.
const truncationMark = "..."

// homeStoragePolicyIDField is the field index of a migration's
// spec.targetStorage.homeStoragePolicyID.
const homeStoragePolicyIDField = "spec.targetStorage.homeStoragePolicyID"

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &mobilityv1a4.VirtualMachineInfraMigration{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
	)

	// Index the migration's home storage policy ID, so that the migrations
	// that refer to a policy can be listed without listing every migration.
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		controlledType,
		homeStoragePolicyIDField,
		func(rawObj client.Object) []string {
			ts := rawObj.(*mobilityv1a4.VirtualMachineInfraMigration).
				Spec.TargetStorage
			if ts == nil || ts.HomeStoragePolicyID == "" {
				return nil
			}
			return []string{ts.HomeStoragePolicyID}
		}); err != nil {
		return err
	}

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName))

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		// A VirtualMachineInfraMigration may be reconciled before its
		// VirtualMachine exists, and the VirtualMachine's spec decides which
		// VolumeAttributesClass is recorded, so the VirtualMachine is watched
		// as well, but only for the events that can change what is written.
		Watches(
			&vmopv1.VirtualMachine{},
			handler.EnqueueRequestsFromMapFunc(
				vmToVirtualMachineInfraMigrationMapper(mgr.GetClient())),
			ctrlbuilder.WithPredicates(virtualMachinePredicate())).
		// A ready migration may refer to a storage policy that no
		// VolumeAttributesClass refers to yet, so a VolumeAttributesClass that
		// is created later reconciles the migrations that refer to its policy.
		Watches(
			&storagev1.VolumeAttributesClass{},
			handler.EnqueueRequestsFromMapFunc(
				vacToVirtualMachineInfraMigrationMapper(mgr.GetClient())),
			ctrlbuilder.WithPredicates(volumeAttributesClassPredicate())).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ctx.GetMaxConcurrentReconciles(controllerNameShort, 0),
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Complete(r)
}

// vmToVirtualMachineInfraMigrationMapper returns a mapper function that maps a
// VirtualMachine to the VirtualMachineInfraMigration with the same namespace
// and name. A VirtualMachine is only mapped if it has a migration, if that
// cannot be determined, so that the reconciler can retry, or if it still has the
// mirrored condition of a migration that no longer exists.
func vmToVirtualMachineInfraMigrationMapper(
	c client.Client) handler.MapFunc {

	return func(ctx context.Context, o client.Object) []reconcile.Request {
		key := client.ObjectKeyFromObject(o)

		// This is a cache read and avoids reconciling every VirtualMachine
		// event.
		if err := c.Get(
			ctx,
			key,
			&mobilityv1a4.VirtualMachineInfraMigration{}); err != nil {

			if !apierrors.IsNotFound(err) {
				// Enqueue the migration so that the reconciler, which reads
				// it again, retries instead of the event being dropped.
				pkglog.FromContextOrDefault(ctx).Error(err,
					"Failed to get infra migration for a virtual machine event",
					"namespace", key.Namespace, "name", key.Name)

				return []reconcile.Request{{NamespacedName: key}}
			}

			// The migration is gone, but the VirtualMachine may still have the
			// condition that it mirrored, for example if the migration was
			// deleted while VM Operator was not running, so no delete event
			// was seen. Reconciling removes the condition.
			if vm, ok := o.(*vmopv1.VirtualMachine); ok && conditions.Has(
				vm,
				vmopv1.VirtualMachineConditionInfraMigrationReady) {

				return []reconcile.Request{{NamespacedName: key}}
			}

			return nil
		}

		return []reconcile.Request{{NamespacedName: key}}
	}
}

// virtualMachinePredicate passes the VirtualMachine events that can change what
// this controller writes: a create, since the migration may already exist, a
// delete, a spec change, since the class that the spec names decides which
// VolumeAttributesClass is recorded, and a change to the mirrored Ready
// condition, so that it is restored if something else changes or removes it.
// Every other status write, including this controller's own patch and the ones
// made by the VirtualMachine controller, is dropped. A delete reconciles the
// migration, which finds no VirtualMachine and does nothing.
func virtualMachinePredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldVM, ok := e.ObjectOld.(*vmopv1.VirtualMachine)
			if !ok {
				return true
			}
			newVM, ok := e.ObjectNew.(*vmopv1.VirtualMachine)
			if !ok {
				return true
			}

			return oldVM.Generation != newVM.Generation ||
				!apiequality.Semantic.DeepEqual(
					conditions.Get(
						oldVM,
						vmopv1.VirtualMachineConditionInfraMigrationReady),
					conditions.Get(
						newVM,
						vmopv1.VirtualMachineConditionInfraMigrationReady))
		},
	}
}

// volumeAttributesClassPredicate passes only the creation of a
// VolumeAttributesClass. The parameters of a VolumeAttributesClass are
// immutable, so an update cannot change the storage policy it refers to, and
// the deletion of one only removes a candidate: the recorded class is never
// cleared, and the next VirtualMachine or migration event evaluates the rest.
func volumeAttributesClassPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

// vacToVirtualMachineInfraMigrationMapper returns a mapper function that maps a
// VolumeAttributesClass to the VirtualMachineInfraMigrations whose home storage
// policy is the one that the class refers to.
func vacToVirtualMachineInfraMigrationMapper(
	c client.Client) handler.MapFunc {

	return func(ctx context.Context, o client.Object) []reconcile.Request {
		vac, ok := o.(*storagev1.VolumeAttributesClass)
		if !ok {
			return nil
		}

		policyID, err := kubeutil.GetStoragePolicyIDFromVAC(*vac)
		if err != nil {
			// A class without a storage policy ID cannot be the one that a
			// migration's home storage policy refers to.
			return nil
		}

		var list mobilityv1a4.VirtualMachineInfraMigrationList
		if err := c.List(
			ctx,
			&list,
			client.MatchingFields{homeStoragePolicyIDField: policyID}); err != nil {

			pkglog.FromContextOrDefault(ctx).Error(err,
				"Failed to list infra migrations for a volume attributes "+
					"class event",
				"volumeAttributesClass", vac.Name,
				"storagePolicyID", policyID)

			return nil
		}

		requests := make([]reconcile.Request, len(list.Items))
		for i := range list.Items {
			requests[i] = reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&list.Items[i]),
			}
		}

		return requests
	}
}

// NewReconciler returns a reconciler for VirtualMachineInfraMigrations.
func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger) *Reconciler {

	return &Reconciler{
		Context: ctx,
		Client:  client,
		Logger:  logger,
	}
}

// Reconciler reconciles a VirtualMachineInfraMigration object.
type Reconciler struct {
	client.Client
	Context context.Context
	Logger  logr.Logger
}

// +kubebuilder:rbac:groups=mobility-operator.vmware.com,resources=virtualmachineinframigrations,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=volumeattributesclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list;watch
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines/status,verbs=get;update;patch

// Reconcile mirrors the VirtualMachineInfraMigration with the provided name onto
// the VirtualMachine with the same namespace and name, or, if the migration no
// longer exists, removes what was mirrored.
func (r *Reconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (_ ctrl.Result, reterr error) {

	ctx = pkgcfg.JoinContext(ctx, r.Context)

	var obj mobilityv1a4.VirtualMachineInfraMigration
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.ReconcileDelete(ctx, req.NamespacedName)
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.ReconcileNormal(ctx, &obj)
}

// ReconcileDelete removes the mirrored Ready condition from the VirtualMachine
// with the provided namespace and name, which are also the migration's, once the
// migration no longer exists. Otherwise the condition would go on describing a
// resource that is gone, and the failure of an abandoned migration would stay on
// the VirtualMachine forever. status.volumeAttributesClassName is not cleared,
// since it is the durable record of what the VirtualMachine is configured to.
func (r *Reconciler) ReconcileDelete(
	ctx context.Context,
	key client.ObjectKey) error {

	logger := pkglog.FromContextOrDefault(ctx)

	var vm vmopv1.VirtualMachine
	if err := r.Get(ctx, key, &vm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf(
			"failed to get virtual machine %s/%s: %w",
			key.Namespace, key.Name, err)
	}

	if !vm.DeletionTimestamp.IsZero() ||
		!conditions.Has(
			&vm,
			vmopv1.VirtualMachineConditionInfraMigrationReady) {

		return nil
	}

	statusBefore := mirroredReadyStatus(&vm)

	if err := r.patchVirtualMachine(ctx, &vm, func() {
		conditions.Delete(
			&vm, vmopv1.VirtualMachineConditionInfraMigrationReady)
	}); err != nil {
		return err
	}

	logger.Info("Infra migration status changed on virtual machine",
		"condition", fmt.Sprintf(
			"%s: %s -> %s",
			vmopv1.VirtualMachineConditionInfraMigrationReady,
			statusBefore, noStatus))

	return nil
}

// ReconcileNormal mirrors the Ready condition of a VirtualMachineInfraMigration
// onto the VirtualMachine with the same namespace and name, as described by
// VirtualMachineConditionInfraMigrationReady. The condition is only mirrored if
// it describes the migration's current generation. Otherwise it describes the
// previous spec, and the VirtualMachine does not keep the one it mirrored
// earlier.
//
// Only when the VirtualMachineInfraMigration's Ready condition is True for the
// migration's current generation is the VolumeAttributesClass that the
// VirtualMachine was migrated to recorded in status.volumeAttributesClassName.
func (r *Reconciler) ReconcileNormal(
	ctx context.Context,
	obj *mobilityv1a4.VirtualMachineInfraMigration) error {

	logger := pkglog.FromContextOrDefault(ctx)

	var vm vmopv1.VirtualMachine
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), &vm); err != nil {
		if apierrors.IsNotFound(err) {
			// The VirtualMachine's creation will trigger another reconcile.
			logger.V(4).Info("No virtual machine for infra migration")
			return nil
		}
		return fmt.Errorf(
			"failed to get virtual machine %s/%s: %w",
			obj.Namespace, obj.Name, err)
	}

	if !vm.DeletionTimestamp.IsZero() {
		return nil
	}

	var vacName string
	// A Ready condition whose observedGeneration is less than the
	// metadata.generation describes the previous spec, so it is not considered
	// until the current spec has been observed.
	if conditions.IsReady(obj, conditions.WithGeneration(obj.Generation)) {
		var err error
		vacName, err = r.getMigratedVAC(ctx, obj, &vm)
		if err != nil {
			return err
		}
	} else {
		logger.V(4).Info("Infra migration is not ready")
	}

	// A Ready condition that is behind the migration's generation describes the
	// previous spec, so mirroring it would make the VirtualMachine briefly
	// report an outcome that is out of date.
	ready := conditions.GetObservedAt(obj, obj.Generation).
		Get(conditions.ReadyConditionType)
	if ready == nil {
		logger.V(4).Info("Infra migration has no Ready condition that " +
			"describes its current generation")
	}

	var (
		// Log only when there is a status change.
		statusBefore = mirroredReadyStatus(&vm)
		statusAfter  string
		vacRecorded  = vacName != "" &&
			vacName != vm.Status.VolumeAttributesClassName
	)

	if err := r.patchVirtualMachine(ctx, &vm, func() {
		mirrorReady(&vm, obj, ready)

		if vacName != "" {
			vm.Status.VolumeAttributesClassName = vacName
		}

		statusAfter = mirroredReadyStatus(&vm)
	}); err != nil {
		return err
	}

	logger.V(4).Info("Mirrored infra migration condition to virtual machine",
		"namespace", vm.Namespace,
		"name", vm.Name)

	if statusBefore != statusAfter || vacRecorded {
		// The migration is the VirtualMachine's namesake, so the logger
		// already carries its name and namespace.
		var kvs []any
		if statusBefore != statusAfter {
			kvs = append(kvs, "condition", fmt.Sprintf(
				"%s: %s -> %s",
				vmopv1.VirtualMachineConditionInfraMigrationReady,
				statusBefore, statusAfter))
		}
		if vacRecorded {
			kvs = append(kvs, "volumeAttributesClassName", vacName)
		}
		logger.Info("Infra migration status changed on virtual machine", kvs...)
	}

	return nil
}

// noStatus is how the status of a condition that does not exist is described.
const noStatus = "<none>"

// addSuffixOnMsg returns the message of the VirtualMachineInfraMigration's
// condition followed by a reference to the VirtualMachineInfraMigration, in the
// form "<message> @ <kind>/<name>", so that a user reading the VirtualMachine
// can tell which resource the condition was mirrored from. If the condition has
// no message, the message is only the reference.
//
// The result is never longer than maxConditionMessageLen characters, since the
// API server would reject the whole status patch. If the migration's message
// does not leave room for the reference, it is truncated and ends with
// truncationMark, so that the reference is always there.
func addSuffixOnMsg(
	obj *mobilityv1a4.VirtualMachineInfraMigration,
	message string) string {

	ref := fmt.Sprintf("%s/%s", migrationKind, obj.Name)
	if message == "" {
		return ref
	}

	suffix := " @ " + ref

	// The limit is in characters, not bytes, so truncate on a character
	// boundary.
	room := maxConditionMessageLen - utf8.RuneCountInString(suffix)
	if runes := []rune(message); len(runes) > room {
		message = string(runes[:room-len(truncationMark)]) + truncationMark
	}

	return message + suffix
}

// mirroredReadyStatus returns the status of the VirtualMachine's mirrored Ready
// condition, or noStatus if it does not have one.
func mirroredReadyStatus(vm *vmopv1.VirtualMachine) string {
	if c := conditions.Get(
		vm,
		vmopv1.VirtualMachineConditionInfraMigrationReady); c != nil {

		return string(c.Status)
	}
	return noStatus
}

// mirrorReady sets the VirtualMachine's mirrored Ready condition from the
// VirtualMachineInfraMigration's Ready condition, or removes it if there is no
// Ready condition to mirror.
//
// The status and reason are copied, and so is the message, which is followed by
// a reference to the VirtualMachineInfraMigration. The observedGeneration is
// not, since it refers to the generation of the VirtualMachineInfraMigration
// rather than the VirtualMachine, and the last transition time is only moved
// when one of the copied fields changes.
func mirrorReady(
	vm *vmopv1.VirtualMachine,
	obj *mobilityv1a4.VirtualMachineInfraMigration,
	ready *metav1.Condition) {

	if ready == nil {
		conditions.Delete(
			vm, vmopv1.VirtualMachineConditionInfraMigrationReady)
		return
	}

	// The reason of a condition must not be empty.
	reason := ready.Reason
	if reason == "" {
		reason = string(ready.Status)
	}

	conditions.Set(vm, &metav1.Condition{
		Type:    vmopv1.VirtualMachineConditionInfraMigrationReady,
		Status:  ready.Status,
		Reason:  reason,
		Message: addSuffixOnMsg(obj, ready.Message),
	})
}

// getMigratedVAC returns the name of the
// VolumeAttributesClass that the VirtualMachine was migrated to, or an empty
// string if it cannot be determined.
//
// The migration reports the storage policy ID of the VirtualMachine's home
// directory rather than a VolumeAttributesClass name, so the name is looked up
// from the VolumeAttributesClass objects on the cluster that refer to that
// policy. The name is only used if it is also the VolumeAttributesClass in the
// VirtualMachine's spec. Otherwise the migration does not describe what the
// VirtualMachine currently requests, so nothing is recorded, and the reason is
// logged at verbosity 2.
func (r *Reconciler) getMigratedVAC(
	ctx context.Context,
	obj *mobilityv1a4.VirtualMachineInfraMigration,
	vm *vmopv1.VirtualMachine) (string, error) {

	ts := obj.Spec.TargetStorage
	if ts == nil || ts.HomeStoragePolicyID == "" {
		// The migration does not change the home storage policy.
		return "", nil
	}

	names, err := kubeutil.GetVACNamesForStoragePolicyID(
		ctx, r.Client, ts.HomeStoragePolicyID)
	if err != nil {
		return "", err
	}

	specName := vm.Spec.VolumeAttributesClassName
	if specName == "" || !slices.Contains(names, specName) {
		// This most likely means that the spec was edited again and the
		// mobility operator has not picked it up yet, which is not an error.
		// The next change to the spec, the migration, or the
		// VolumeAttributesClasses reconciles again.
		pkglog.FromContextOrDefault(ctx).V(2).Info(
			"Infra migration home storage policy is not referenced by the "+
				"virtual machine's volume attributes class",
			"storagePolicyID", ts.HomeStoragePolicyID,
			"volumeAttributesClasses", names,
			"specVolumeAttributesClassName", specName)
		return "", nil
	}

	return specName, nil
}

// patchVirtualMachine applies mutate to the VirtualMachine's status and
// patches the result. The patch helper only writes the changed conditions
// against the latest version of the VirtualMachine, retrying on conflict, so
// this does not clobber the conditions that the VirtualMachine controller
// owns. The mirrored Ready condition is the one this controller owns, so it is
// written even if it changed in the meantime.
func (r *Reconciler) patchVirtualMachine(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	mutate func()) error {

	patchHelper, err := patch.NewHelper(vm, r.Client)
	if err != nil {
		return fmt.Errorf(
			"failed to init patch helper for virtual machine %s/%s: %w",
			vm.Namespace, vm.Name, err)
	}

	mutate()

	if err := patchHelper.Patch(
		ctx,
		vm,
		patch.WithOwnedConditions{
			Conditions: []string{
				vmopv1.VirtualMachineConditionInfraMigrationReady,
			},
		}); err != nil {

		return fmt.Errorf(
			"failed to patch virtual machine %s/%s: %w",
			vm.Namespace, vm.Name, err)
	}

	return nil
}
