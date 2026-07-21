// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	netsetutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/networksettings"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	finalizerName           = "vmoperator.vmware.com/virtualmachineservice"
	deprecatedFinalizerName = "virtualmachineservice.vmoperator.vmware.com"

	OpCreate = "CreateK8sService"
	OpDelete = "DeleteK8sService"
	OpUpdate = "UpdateK8sService"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1.VirtualMachineService{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
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
			LogConstructor:          pkglog.ControllerLogConstructor(controllerNameShort, controlledType, mgr.GetScheme()),
		}).
		Watches(&corev1.Service{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &vmopv1.VirtualMachineService{})).
		Watches(&corev1.Endpoints{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &vmopv1.VirtualMachineService{})).
		Watches(&vmopv1.VirtualMachine{},
			r.virtualMachineToVirtualMachineServiceHandler()).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
) *ReconcileVirtualMachineService {
	return &ReconcileVirtualMachineService{
		Context:  ctx,
		Client:   client,
		log:      logger,
		recorder: recorder,
	}
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineService{}

// ReconcileVirtualMachineService reconciles a VirtualMachineService object.
type ReconcileVirtualMachineService struct {
	Context context.Context
	client.Client
	log      logr.Logger
	recorder record.Recorder
}

// Reconcile reads that state of the cluster for a VirtualMachineService object and makes changes based on the state read
// and what is in the VirtualMachineService.Spec
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;create;update;patch;delete

func (r *ReconcileVirtualMachineService) Reconcile(ctx context.Context, request reconcile.Request) (_ reconcile.Result, reterr error) {
	ctx = pkgcfg.JoinContext(ctx, r.Context)

	vmService := &vmopv1.VirtualMachineService{}
	if err := r.Get(ctx, request.NamespacedName, vmService); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	vmServiceCtx := &pkgctx.VirtualMachineServiceContext{
		Context:   ctx,
		Logger:    pkglog.FromContextOrDefault(ctx),
		VMService: vmService,
	}

	patchHelper, err := patch.NewHelper(vmService, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper for %s: %w", vmServiceCtx, err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, vmService); err != nil {
			if reterr == nil {
				reterr = err
			}
			vmServiceCtx.Logger.Error(err, "patch failed")
		}
	}()

	if !vmService.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, r.ReconcileDelete(vmServiceCtx)
	}

	return reconcile.Result{}, r.ReconcileNormal(vmServiceCtx)
}

func (r *ReconcileVirtualMachineService) ReconcileDelete(ctx *pkgctx.VirtualMachineServiceContext) error {
	if controllerutil.ContainsFinalizer(ctx.VMService, finalizerName) ||
		controllerutil.ContainsFinalizer(ctx.VMService, deprecatedFinalizerName) {
		objectMeta := metav1.ObjectMeta{
			Name:      ctx.VMService.Name,
			Namespace: ctx.VMService.Namespace,
		}

		endpoint := &corev1.Endpoints{ObjectMeta: objectMeta}
		if err := r.Client.Delete(ctx, endpoint); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete Endpoints: %w", err)
		}

		service := &corev1.Service{ObjectMeta: objectMeta}
		if err := r.Client.Delete(ctx, service); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete Service: %w", err)
		}

		ctx.Logger.Info("Delete VirtualMachineService")
		r.recorder.EmitEvent(ctx.VMService, OpDelete, nil, false)
		controllerutil.RemoveFinalizer(ctx.VMService, finalizerName)
		controllerutil.RemoveFinalizer(ctx.VMService, deprecatedFinalizerName)
	}

	return nil
}

func (r *ReconcileVirtualMachineService) ReconcileNormal(ctx *pkgctx.VirtualMachineServiceContext) error {
	if !controllerutil.ContainsFinalizer(ctx.VMService, finalizerName) {
		// If the object has the deprecated finalizer, remove it.
		if updated := controllerutil.RemoveFinalizer(ctx.VMService, deprecatedFinalizerName); updated {
			ctx.Logger.V(5).Info("Removed deprecated finalizer", "finalizerName", deprecatedFinalizerName)
		}

		controllerutil.AddFinalizer(ctx.VMService, finalizerName)
		// NOTE: The VirtualMachineService is set as the OwnerReference of the Service and Endpoints.
		// So while ReconcileDelete() does delete them when our finalizer is set, the k8s GC will
		// delete them if they still exist if the VirtualMachineService is deleted so we do not have
		// to return here. The explicit delete in ReconcileDelete() just speeds up the ultimate removal
		// of the service from the LB.
	}

	if err := r.reconcileVMService(ctx); err != nil {
		return fmt.Errorf("failed to reconcile VirtualMachineService: %w", err)
	}

	return nil
}

func (r *ReconcileVirtualMachineService) getLoadBalancerProvider(
	ctx context.Context,
	namespace string) (providers.LoadbalancerProvider, error) {

	lbProviderType := pkgcfg.FromContext(ctx).LoadBalancerProvider
	if lbProviderType == "" {
		providerType, err := netsetutil.GetProviderType(ctx, r.Client, namespace)
		if err != nil {
			return nil, err
		}

		switch providerType {
		case pkgcfg.NetworkProviderTypeNSXT, pkgcfg.NetworkProviderTypeVPC:
			// This LB provider dates back to the initial release which was NSXT only, but
			// we have no way to determine what the actual LB is. So continue to set these
			// based on network provider, but this is hack ideally this would be expressed
			// in another way like a LoadBalancerClass.
			lbProviderType = providers.NSXTLoadBalancer
		}
	}

	return providers.GetLoadbalancerProviderByType(r.Client, lbProviderType)
}

func (r *ReconcileVirtualMachineService) reconcileVMService(ctx *pkgctx.VirtualMachineServiceContext) error {
	ctx.Logger.Info("Reconcile VirtualMachineService")
	defer ctx.Logger.Info("Finished Reconcile VirtualMachineService")

	vmService := ctx.VMService

	if vmService.Spec.Type == vmopv1.VirtualMachineServiceTypeLoadBalancer {
		lbProvider, err := r.getLoadBalancerProvider(ctx, vmService.Namespace)
		if err != nil {
			return fmt.Errorf("failed to get load balancer provider: %w", err)
		}

		// Get LoadBalancer to attach
		err = lbProvider.EnsureLoadBalancer(ctx, vmService)
		if err != nil {
			return fmt.Errorf("failed to create or get load balancer for VM Service: %w", err)
		}

		// Get the provider specific annotations for service and add them to the VMService as
		// that's where Service inherits the values.
		annotations, err := lbProvider.GetServiceAnnotations(ctx, vmService)
		if err != nil {
			return fmt.Errorf("failed to get load balancer annotations for service: %w", err)
		}

		if vmService.Annotations == nil {
			vmService.Annotations = make(map[string]string)
		}

		for k, v := range annotations {
			if oldValue, ok := vmService.Annotations[k]; ok {
				ctx.Logger.V(5).Info("Replacing previous annotation value on VM Service",
					"key", k, "oldValue", oldValue, "newValue", v)
			}
			vmService.Annotations[k] = v
		}

		// Get the provider specific labels for Service and add them to the vm service
		// as that's where Service inherits the values.
		svcLabels, err := lbProvider.GetServiceLabels(ctx, vmService)
		if err != nil {
			return fmt.Errorf("failed to get load balancer labels for service: %w", err)
		}

		if vmService.Labels == nil {
			vmService.Labels = make(map[string]string)
		}

		for k, v := range svcLabels {
			if oldValue, ok := vmService.Labels[k]; ok {
				ctx.Logger.V(5).Info("Replacing previous label value on VM Service",
					"key", k, "oldValue", oldValue, "newValue", v)
			}
			vmService.Labels[k] = v
		}
	}

	service, err := r.createOrUpdateService(ctx)
	if err != nil {
		return fmt.Errorf("failed to update VirtualMachineService k8s Service: %w", err)
	}

	ctx.Logger.V(5).Info("Service spec.ipFamilies",
		"service", client.ObjectKeyFromObject(service).String(),
		"serviceSpecIPFamilies", service.Spec.IPFamilies)

	err = r.createOrUpdateEndpoints(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to update VirtualMachineService Endpoints: %w", err)
	}

	err = r.updateVMService(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to update VirtualMachineService Status: %w", err)
	}

	return nil
}

// virtualMachineToVirtualMachineServiceHandler returns an event handler that
// enqueues the VirtualMachineServices whose label selector matches a given
// VirtualMachine.
//
// On update, a VM's labels may have changed such that it now matches a
// different set of VirtualMachineServices than it did before. To ensure a VM is
// promptly removed from the Endpoints of a VirtualMachineService it no longer
// matches, the update handler enqueues the union of the VirtualMachineServices
// that matched the VM's previous labels and those that match its current
// labels. This mirrors how the core Kubernetes endpoints controller reacts to
// Pod updates (it computes the service memberships of both the old and new Pod
// and reconciles the combined set).
//
// Recomputing service memberships on every VirtualMachine event would be
// wasteful because a VM is updated for many reasons unrelated to its Endpoints.
// The update handler therefore skips the lookup entirely unless a change
// relevant to the Endpoints occurred; see virtualMachineEndpointsChanged.
func (r *ReconcileVirtualMachineService) virtualMachineToVirtualMachineServiceHandler() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if vm, ok := e.Object.(*vmopv1.VirtualMachine); ok {
				r.enqueueVirtualMachineServicesSelectingVM(ctx, vm, q)
			}
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			oldVM, ok := e.ObjectOld.(*vmopv1.VirtualMachine)
			if !ok {
				return
			}
			newVM, ok := e.ObjectNew.(*vmopv1.VirtualMachine)
			if !ok {
				return
			}

			changed, labelsChanged := virtualMachineEndpointsChanged(oldVM, newVM)
			if !changed {
				return
			}

			// When the labels have changed, reconcile the services that the VM
			// previously selected so the VM is removed from services it no longer
			// selects, in addition to the services it now selects.
			if labelsChanged {
				r.enqueueVirtualMachineServicesSelectingLabelSets(ctx, newVM.Namespace, q, oldVM.Labels, newVM.Labels)
			} else {
				r.enqueueVirtualMachineServicesSelectingVM(ctx, newVM, q)
			}
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if vm, ok := e.Object.(*vmopv1.VirtualMachine); ok {
				r.enqueueVirtualMachineServicesSelectingVM(ctx, vm, q)
			}
		},
		GenericFunc: func(ctx context.Context, e event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if vm, ok := e.Object.(*vmopv1.VirtualMachine); ok {
				r.enqueueVirtualMachineServicesSelectingVM(ctx, vm, q)
			}
		},
	}
}

// enqueueVirtualMachineServicesSelectingVM enqueues a reconcile request for
// every VirtualMachineService whose label selector matches the given
// VirtualMachine's labels.
func (r *ReconcileVirtualMachineService) enqueueVirtualMachineServicesSelectingVM(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	if len(vm.Labels) != 0 {
		r.enqueueVirtualMachineServicesSelectingLabelSets(ctx, vm.Namespace, q, vm.Labels)
	}
}

// enqueueVirtualMachineServicesSelectingLabelSets lists the
// VirtualMachineServices in ns once and enqueues a reconcile request for each
// whose selector matches any of the given label sets.
func (r *ReconcileVirtualMachineService) enqueueVirtualMachineServicesSelectingLabelSets(
	ctx context.Context,
	ns string,
	q workqueue.TypedRateLimitingInterface[reconcile.Request],
	labelSets ...map[string]string) {

	vmServiceList := &vmopv1.VirtualMachineServiceList{}
	if err := r.List(ctx, vmServiceList, client.InNamespace(ns)); err != nil {
		pkglog.FromContextOrDefault(ctx).Error(err, "Failed to list VirtualMachineServices")
		return
	}

	for _, vmService := range vmServiceList.Items {
		if len(vmService.Spec.Selector) == 0 {
			continue
		}

		selector := labels.SelectorFromValidatedSet(vmService.Spec.Selector)
		for _, l := range labelSets {
			if len(l) != 0 && selector.Matches(labels.Set(l)) {
				q.Add(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&vmService)})
				break
			}
		}
	}
}

// virtualMachineEndpointsChanged returns true if the difference between the old
// and new VirtualMachine could affect the Endpoints of any VirtualMachineService.
// The second bool return is true if the labels has changed, since that requires
// reconciling the services that might no longer select the VM.
func virtualMachineEndpointsChanged(oldVM, newVM *vmopv1.VirtualMachine) (bool, bool) {
	// VM's changed labels may select different services.
	if !maps.Equal(oldVM.Labels, newVM.Labels) {
		return true, true
	}

	// VM's being deleted are not included in the ready Endpoints.
	if oldVM.DeletionTimestamp.IsZero() != newVM.DeletionTimestamp.IsZero() {
		return true, false
	}

	// We don't have the Service at this point so we don't know which IP families
	// it cares about, but IPs changing is not an expected common occurrence so
	// reconcile if either changes.
	oldIP4, oldIP6 := virtualMachinePrimaryIPs(oldVM)
	newIP4, newIP6 := virtualMachinePrimaryIPs(newVM)
	if oldIP4 != newIP4 || oldIP6 != newIP6 {
		return true, false
	}

	// The Ready condition mirrors probe results but is not cleared when the
	// probe itself is removed, so a probe being added or removed must be
	// compared directly rather than relying solely on the condition.
	if virtualMachineHasReadinessProbe(oldVM) != virtualMachineHasReadinessProbe(newVM) {
		return true, false
	}

	// The VM's Readiness probe (if it had/has one) will update the ready condition.
	return virtualMachineReadyStatus(oldVM) != virtualMachineReadyStatus(newVM), false
}

// virtualMachineHasReadinessProbe returns whether the VM has a configured
// readiness probe that generateSubsetsForService uses to gate readiness.
func virtualMachineHasReadinessProbe(vm *vmopv1.VirtualMachine) bool {
	probe := vm.Spec.ReadinessProbe
	return probe != nil && (probe.TCPSocket != nil || probe.GuestHeartbeat != nil || len(probe.GuestInfo) != 0)
}

// virtualMachinePrimaryIPs returns the VM's primary IPv4 and IPv6 addresses.
func virtualMachinePrimaryIPs(vm *vmopv1.VirtualMachine) (ip4, ip6 string) {
	if vm.Status.Network != nil {
		ip4 = vm.Status.Network.PrimaryIP4
		ip6 = vm.Status.Network.PrimaryIP6
	}
	return ip4, ip6
}

// virtualMachineReadyStatus returns the status of the VM's Ready condition, or
// the empty string if the condition is not present.
func virtualMachineReadyStatus(vm *vmopv1.VirtualMachine) metav1.ConditionStatus {
	if c := pkgcond.Get(vm, vmopv1.ReadyConditionType); c != nil {
		return c.Status
	}
	return ""
}

// Set labels and annotations on the Service from the VirtualMachineService. Some load balancer
// providers (currently only NCP) need to filter or translate labels and annotations too.
func (r *ReconcileVirtualMachineService) setServiceAnnotationsAndLabels(
	ctx *pkgctx.VirtualMachineServiceContext,
	service *corev1.Service) error {
	vmService := ctx.VMService

	// TODO: Clean the provider interfaces here. This is way too verbose.

	if service.Annotations == nil {
		service.Annotations = make(map[string]string, len(vmService.Annotations))
		for k, v := range vmService.Annotations {
			service.Annotations[k] = v
		}
	} else {
		for k, v := range vmService.Annotations {
			if oldValue, ok := service.Annotations[k]; ok {
				ctx.Logger.V(5).Info("Replacing previous annotation value on Service",
					"key", k, "oldValue", oldValue, "newValue", v)
			}
			service.Annotations[k] = v
		}
	}

	// Explicitly remove provider specific annotations
	lbProvider, err := r.getLoadBalancerProvider(ctx, vmService.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get load balancer provider: %w", err)
	}

	annotationsToBeRemoved, err := lbProvider.GetToBeRemovedServiceAnnotations(ctx, ctx.VMService)
	if err != nil {
		return fmt.Errorf("failed to get load balancer specific annotations to remove from Service: %w", err)
	}
	for k := range annotationsToBeRemoved {
		ctx.Logger.V(5).Info("Removing annotation from service", "key", k)
		delete(service.Annotations, k)
	}

	if service.Labels == nil {
		service.Labels = make(map[string]string, len(vmService.Labels))
		for k, v := range vmService.Labels {
			service.Labels[k] = v
		}
	} else {
		for k, v := range vmService.Labels {
			if oldValue, ok := service.Labels[k]; ok {
				ctx.Logger.V(5).Info("Replacing previous label value on Service",
					"key", k, "oldValue", oldValue, "newValue", v)
			}
			service.Labels[k] = v
		}
	}

	// Explicitly remove provider specific labels
	labelsToBeRemoved, err := lbProvider.GetToBeRemovedServiceLabels(ctx, ctx.VMService)
	if err != nil {
		return fmt.Errorf("failed to get load balancer specific labels to remove from Service: %w", err)
	}
	for k := range labelsToBeRemoved {
		ctx.Logger.V(5).Info("Removing label from Service", "key", k)
		delete(service.Labels, k)
	}

	// Explicitly remove vm service managed annotations if needed
	for _, k := range []string{utils.AnnotationServiceExternalTrafficPolicyKey, utils.AnnotationServiceHealthCheckNodePortKey} {
		if _, exist := vmService.Annotations[k]; !exist {
			if v, exist := service.Annotations[k]; exist {
				ctx.Logger.V(5).Info("Removing annotation from Service", "key", k, "value", v)
				delete(service.Annotations, k)
			}
		}
	}

	return nil
}

func (r *ReconcileVirtualMachineService) createOrUpdateService(ctx *pkgctx.VirtualMachineServiceContext) (*corev1.Service, error) {
	ctx.Logger.V(5).Info("Reconciling k8s Service")
	defer ctx.Logger.V(5).Info("Finished reconciling k8s Service")

	vmService := ctx.VMService
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmService.Name,
			Namespace: vmService.Namespace,
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.Client, service, func() error {
		if err := controllerutil.SetControllerReference(vmService, service, r.Client.Scheme()); err != nil {
			return err
		}

		if err := r.setServiceAnnotationsAndLabels(ctx, service); err != nil {
			return err
		}

		service.Spec.Type = corev1.ServiceType(vmService.Spec.Type)
		service.Spec.ExternalName = vmService.Spec.ExternalName
		service.Spec.LoadBalancerIP = vmService.Spec.LoadBalancerIP
		service.Spec.LoadBalancerSourceRanges = vmService.Spec.LoadBalancerSourceRanges

		// IPFamilies and IPFamilyPolicy are dual-stack fields gated by the WorkloadIPv6 capability.
		// When the capability is disabled, existing values on the Service are left untouched so that
		// live dual-stack Services continue to function without unexpected changes.
		if pkgcfg.FromContext(ctx).Features.WorkloadIPv6 {
			// These fields only apply to ClusterIP and LoadBalancer types;
			// they are wiped when type is ExternalName.
			if vmService.Spec.Type != vmopv1.VirtualMachineServiceTypeExternalName {
				// Only overwrite when the VirtualMachineService specifies ipFamilies so we do not clear
				// values defaulted or stored by the apiserver when the CR omits them.
				if len(vmService.Spec.IPFamilies) > 0 {
					service.Spec.IPFamilies = vmService.Spec.IPFamilies
				}
				if vmService.Spec.IPFamilyPolicy != nil {
					service.Spec.IPFamilyPolicy = vmService.Spec.IPFamilyPolicy
				}
			} else {
				// Clear IPFamilies and IPFamilyPolicy for ExternalName services
				service.Spec.IPFamilies = nil
				service.Spec.IPFamilyPolicy = nil
			}
		}

		if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			service.Spec.AllocateLoadBalancerNodePorts = ptr.To(false)
		} else {
			service.Spec.AllocateLoadBalancerNodePorts = nil
		}

		// Parts of the Service.Spec can be updated by k8s after creation, and we need to
		// preserve those fields.
		if service.ResourceVersion == "" {
			// ClusterIP cannot be changed through update.
			service.Spec.ClusterIP = vmService.Spec.ClusterIP
		}

		// Maintain the existing mapping of ServicePort -> NodePort as un-setting it will cause
		// a new NodePort to be allocated.
		// BMV: Just the Name might not be a sufficient key here.
		nodePortMap := make(map[string]int32, len(service.Spec.Ports))
		for _, port := range service.Spec.Ports {
			nodePortMap[port.Name] = port.NodePort
		}
		servicePorts := make([]corev1.ServicePort, 0, len(vmService.Spec.Ports))
		for _, vmPort := range vmService.Spec.Ports {
			servicePort := corev1.ServicePort{
				Name:       vmPort.Name,
				Protocol:   corev1.Protocol(vmPort.Protocol),
				Port:       vmPort.Port,
				TargetPort: intstr.FromInt(int(vmPort.TargetPort)),
				NodePort:   nodePortMap[vmPort.Name],
			}
			servicePorts = append(servicePorts, servicePort)
		}
		service.Spec.Ports = servicePorts

		// This is the default that k8s would otherwise set (note that we don't really support NodePort).
		// The only real purpose of this is if the AnnotationServiceExternalTrafficPolicyKey annotation
		// below is removed, so that we switch the Service back to the default.
		if service.Spec.Type == corev1.ServiceTypeNodePort || service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
		}

		if externalTrafficPolicy, ok := service.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey]; ok {
			// Note that this annotation is only set (and makes sense) from the GC cloud provider.
			trafficPolicy := corev1.ServiceExternalTrafficPolicy(externalTrafficPolicy)
			switch trafficPolicy {
			case corev1.ServiceExternalTrafficPolicyTypeLocal, corev1.ServiceExternalTrafficPolicyTypeCluster:
				service.Spec.ExternalTrafficPolicy = trafficPolicy
			default:
				ctx.Logger.V(5).Info("Unknown externalTrafficPolicy VirtualMachineService annotation",
					"externalTrafficPolicy", externalTrafficPolicy)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	switch result {
	case controllerutil.OperationResultCreated:
		r.recorder.EmitEvent(ctx.VMService, OpCreate, nil, false)
	case controllerutil.OperationResultUpdated:
		r.recorder.EmitEvent(ctx.VMService, OpUpdate, nil, false)
	}

	return service, nil
}

func (r *ReconcileVirtualMachineService) getVirtualMachinesSelectedByVMService(
	ctx *pkgctx.VirtualMachineServiceContext) (*vmopv1.VirtualMachineList, error) {

	if len(ctx.VMService.Spec.Selector) == 0 {
		return nil, nil
	}

	vmList := &vmopv1.VirtualMachineList{}
	err := r.List(ctx, vmList, client.InNamespace(ctx.VMService.Namespace), client.MatchingLabels(ctx.VMService.Spec.Selector))
	return vmList, err
}

// getVMsReferencedByServiceEndpoints gets all VMs that are referenced by service endpoints.
func (r *ReconcileVirtualMachineService) getVMsReferencedByServiceEndpoints(
	ctx *pkgctx.VirtualMachineServiceContext,
	service *corev1.Service) map[types.UID]struct{} {
	endpoints := &corev1.Endpoints{}
	if err := r.Get(ctx, client.ObjectKey{Name: service.Name, Namespace: service.Namespace}, endpoints); err != nil {
		ctx.Logger.Error(err, "Failed to get Endpoints")
		return nil
	}

	vmToSubsetsMap := make(map[types.UID]struct{})
	for _, subset := range endpoints.Subsets {
		for _, epa := range subset.Addresses {
			if epa.TargetRef != nil {
				vmToSubsetsMap[epa.TargetRef.UID] = struct{}{}
			}
		}
	}
	return vmToSubsetsMap
}

// createOrUpdateEndpoints updates the Endpoints for VirtualMachineService.
func (r *ReconcileVirtualMachineService) createOrUpdateEndpoints(ctx *pkgctx.VirtualMachineServiceContext, service *corev1.Service) error {
	ctx.Logger.V(5).Info("Updating VirtualMachineService Endpoints")
	defer ctx.Logger.V(5).Info("Finished updating VirtualMachineService Endpoints")

	if len(ctx.VMService.Spec.Selector) == 0 {
		ctx.Logger.V(5).Info("Selectorless VirtualMachineService so skipping Endpoints reconciliation")
		return nil
	}

	unpackedSubsets, err := r.generateSubsetsForService(ctx, service)
	if err != nil {
		return err
	}
	subsets := utils.RepackSubsets(unpackedSubsets)

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.Client, endpoints, func() error {
		if err := controllerutil.SetControllerReference(ctx.VMService, endpoints, r.Client.Scheme()); err != nil {
			return err
		}

		// NCP apparently needs the same Labels as what is present on the Service, and I'm not aware
		// of anything else setting Labels, so just sync the Labels (and Annotations) with the Service.
		endpoints.Labels = service.Labels
		endpoints.Annotations = service.Annotations
		endpoints.Subsets = subsets
		return nil
	})

	if err != nil {
		return err
	}

	switch result {
	case controllerutil.OperationResultCreated:
		ctx.Logger.Info("Creating Service Endpoints", "endpoints", endpoints)
	case controllerutil.OperationResultUpdated:
		ctx.Logger.Info("Updating Service Endpoints", "endpoints", endpoints)
	}

	return nil
}

func findVMPortNum(_ *vmopv1.VirtualMachine, port intstr.IntOrString, _ corev1.Protocol) (int, error) {
	switch port.Type {
	case intstr.String:
		// Not supported.
	case intstr.Int:
		return port.IntValue(), nil
	}

	return 0, fmt.Errorf("no matching port on VM")
}

// generateSubsetsForService generates Endpoints subsets for a given Service.
func (r *ReconcileVirtualMachineService) generateSubsetsForService(
	ctx *pkgctx.VirtualMachineServiceContext,
	service *corev1.Service) ([]corev1.EndpointSubset, error) {

	vmList, err := r.getVirtualMachinesSelectedByVMService(ctx)
	if err != nil {
		return nil, err
	}

	// Include only VM addresses whose IP family is listed in Service spec.ipFamilies.
	allowedFamilies := determineAllowedIPFamilies(service)

	type addressInfo struct {
		addr  corev1.EndpointAddress
		ready bool
		ports []corev1.EndpointPort
	}
	var addressInfos []addressInfo
	var vmInSubsetsMap map[types.UID]struct{}

	for i := range vmList.Items {
		vm := vmList.Items[i]
		logger := ctx.Logger.WithValues("virtualMachine", vm.NamespacedName())

		// NOTE: Anything that is changed here may need to be reflected in
		// virtualMachineEndpointsChanged(), so that the service is timely
		// reconciled when the VM changes.

		if !vm.DeletionTimestamp.IsZero() {
			logger.Info("Skipping VM marked for deletion")
			continue
		}

		var vmIPs []string
		ipV4, ipV6 := virtualMachinePrimaryIPs(&vm)
		if ipV4 != "" && allowedFamilies[corev1.IPv4Protocol] {
			vmIPs = append(vmIPs, ipV4)
		}
		if ipV6 != "" && allowedFamilies[corev1.IPv6Protocol] {
			vmIPs = append(vmIPs, ipV6)
		}

		if len(vmIPs) == 0 {
			// The EndpointAddress must have a valid IP so we cannot include this VM in the
			// NotReadyAddresses.
			// TODO: When we more fully support multiple NICs, we'll need someway to select which IP.
			logger.Info("Skipping VM without primary IP assigned")
			continue
		}

		// If the VM has a ReadinessProbe and Ready condition, ready is a reflection of the condition
		// status. If the VM has a ReadinessProbe but no condition, we assume that the prober just
		// hasn't run against the VM yet, so infer the VM's readiness if it was previously in the EP;
		// this is to handle upgrade scenarios.
		// Otherwise, a VM that does not have a ReadinessProbe is implicitly ready.
		ready := true

		if virtualMachineHasReadinessProbe(&vm) {
			if condition := pkgcond.Get(&vm, vmopv1.ReadyConditionType); condition == nil {
				if vmInSubsetsMap == nil {
					vmInSubsetsMap = r.getVMsReferencedByServiceEndpoints(ctx, service)
				}

				// If this VM was previously in the EP subset, preserve its readiness until prober
				// updates the condition (the probe used to be done inline here before we had a
				// Ready condition).
				_, ready = vmInSubsetsMap[vm.UID]
			} else {
				ready = condition.Status == metav1.ConditionTrue
			}
		}

		// Build ports list once for reuse across all IPs
		var ports []corev1.EndpointPort
		// TODO: Headless support
		for _, servicePort := range service.Spec.Ports {
			portName := servicePort.Name
			portProto := servicePort.Protocol

			logger.V(5).Info("ServicePort for VirtualMachine",
				"port name", portName, "port proto", portProto)

			portNum, err := findVMPortNum(&vm, servicePort.TargetPort, portProto)
			if err != nil {
				logger.Info("Failed to find port for service",
					"name", portName, "protocol", portProto, "error", err)
				continue
			}

			ports = append(ports,
				corev1.EndpointPort{
					Name:     portName,
					Port:     int32(portNum), //nolint:gosec // disable G115
					Protocol: portProto,
				})
		}

		for _, ip := range vmIPs {
			epa := corev1.EndpointAddress{
				IP: ip,
				TargetRef: &corev1.ObjectReference{
					APIVersion: vm.APIVersion,
					Kind:       vm.Kind,
					Namespace:  vm.Namespace,
					Name:       vm.Name,
					UID:        vm.UID,
					// NOTE: This currently isn't set to limit downstream reconcile churn in things
					// watching these Endpoints but isn't ideal. We should be smarter and only update
					// this when something relevant to the service, e.g. the VM's IP, changes.
					// ResourceVersion: vm.ResourceVersion,
				},
			}

			addressInfos = append(addressInfos, addressInfo{
				addr:  epa,
				ready: ready,
				ports: ports,
			})
		}
	}

	subsets := make([]corev1.EndpointSubset, 0, len(addressInfos))
	for _, addrInfo := range addressInfos {
		var readyAddrs, notReadyAddrs []corev1.EndpointAddress
		if addrInfo.ready {
			readyAddrs = append(readyAddrs, addrInfo.addr)
		} else {
			notReadyAddrs = append(notReadyAddrs, addrInfo.addr)
		}
		subsets = append(subsets, corev1.EndpointSubset{
			Addresses:         readyAddrs,
			NotReadyAddresses: notReadyAddrs,
			Ports:             addrInfo.ports,
		})
	}

	return subsets, nil
}

// determineAllowedIPFamilies returns which IP families may appear on Endpoints for this Service.
// Only spec.ipFamilies is used (from the Service object after createOrUpdateService / apiserver merge).
func determineAllowedIPFamilies(service *corev1.Service) map[corev1.IPFamily]bool {
	allowedFamilies := make(map[corev1.IPFamily]bool)
	for _, family := range service.Spec.IPFamilies {
		allowedFamilies[family] = true
	}
	return allowedFamilies
}

// updateVMService syncs the VirtualMachineService Status from the Service status.
//
//nolint:unparam
func (r *ReconcileVirtualMachineService) updateVMService(ctx *pkgctx.VirtualMachineServiceContext, service *corev1.Service) error {
	vmService := ctx.VMService

	if vmService.Spec.Type == vmopv1.VirtualMachineServiceTypeLoadBalancer {
		vmService.Status.LoadBalancer.Ingress = make([]vmopv1.LoadBalancerIngress, len(service.Status.LoadBalancer.Ingress))
		for idx, ingress := range service.Status.LoadBalancer.Ingress {
			vmIngress := vmopv1.LoadBalancerIngress{
				IP:       ingress.IP,
				Hostname: ingress.Hostname,
			}
			vmService.Status.LoadBalancer.Ingress[idx] = vmIngress
		}
	} else {
		vmService.Status.LoadBalancer.Ingress = nil
	}

	syncServiceReadyCondition(ctx, service)

	return nil
}

// syncServiceReadyCondition checks for Ready condition in the underlying Service
// and reflects it on the VirtualMachineService.
func syncServiceReadyCondition(
	ctx *pkgctx.VirtualMachineServiceContext,
	service *corev1.Service) {

	var readyCond *metav1.Condition
	for _, condition := range service.Status.Conditions {
		if condition.Type == "Ready" {
			readyCond = &condition
			break
		}
	}

	if readyCond != nil {
		switch readyCond.Status {
		case metav1.ConditionTrue:
			pkgcond.MarkTrue(ctx.VMService, vmopv1.ServiceReadyConditionType)
		case metav1.ConditionFalse:
			reason := "ServiceNotReady"
			if readyCond.Reason != "" {
				reason = readyCond.Reason
			}
			message := "Underlying Service is not ready"
			if readyCond.Message != "" {
				message = readyCond.Message
			}
			pkgcond.MarkFalse(ctx.VMService, vmopv1.ServiceReadyConditionType, reason, "%s", message)
		case metav1.ConditionUnknown, "":
			reason := "ServiceReadinessUnknown"
			if readyCond.Reason != "" {
				reason = readyCond.Reason
			}
			message := "Underlying Service readiness is unknown"
			if readyCond.Message != "" {
				message = readyCond.Message
			}
			pkgcond.MarkUnknown(ctx.VMService, vmopv1.ServiceReadyConditionType, reason, "%s", message)
		}
	} else {
		// Most of our Service providers do not set any conditions at all. A few do but
		// don't use Ready. There is an argument for just not showing this condition on
		// the VirtualMachineService when the Service does not have Ready since just the
		// presence of any not-True condition could be seen as something is wrong. But
		// to try to give a consistent conditions in our status, report that as unknown
		// with a clear reason and message.
		pkgcond.MarkUnknown(ctx.VMService,
			vmopv1.ServiceReadyConditionType,
			"ServiceReadyNotPresent",
			"Underlying Service does not have Ready condition")
	}
}
