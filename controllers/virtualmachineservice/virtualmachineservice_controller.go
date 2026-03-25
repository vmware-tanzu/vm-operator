// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
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
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	lbProviderType := pkgcfg.FromContext(ctx).LoadBalancerProvider
	if lbProviderType == "" {
		if pkgcfg.FromContext(ctx).NetworkProviderType == pkgcfg.NetworkProviderTypeNSXT || pkgcfg.FromContext(ctx).NetworkProviderType == pkgcfg.NetworkProviderTypeVPC {
			lbProviderType = providers.NSXTLoadBalancer
		}
	}

	lbProvider, err := providers.GetLoadbalancerProviderByType(mgr, lbProviderType)
	if err != nil {
		return err
	}

	r := NewReconciler(
		ctx,
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		lbProvider,
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
			handler.EnqueueRequestsFromMapFunc(r.virtualMachineToVirtualMachineServiceMapper())).
		Complete(r)
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	lbProvider providers.LoadbalancerProvider,
) *ReconcileVirtualMachineService {
	return &ReconcileVirtualMachineService{
		Context:              ctx,
		Client:               client,
		log:                  logger,
		recorder:             recorder,
		loadbalancerProvider: lbProvider,
	}
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineService{}

// ReconcileVirtualMachineService reconciles a VirtualMachineService object.
type ReconcileVirtualMachineService struct {
	Context context.Context
	client.Client
	log                  logr.Logger
	recorder             record.Recorder
	loadbalancerProvider providers.LoadbalancerProvider
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
			ctx.Logger.Error(err, "Failed to delete Endpoints")
			return err
		}

		service := &corev1.Service{ObjectMeta: objectMeta}
		if err := r.Client.Delete(ctx, service); client.IgnoreNotFound(err) != nil {
			ctx.Logger.Error(err, "Failed to delete Service")
			return err
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
		ctx.Logger.Error(err, "Failed to reconcile VirtualMachineService")
		return err
	}

	return nil
}

func (r *ReconcileVirtualMachineService) reconcileVMService(ctx *pkgctx.VirtualMachineServiceContext) error {
	ctx.Logger.Info("Reconcile VirtualMachineService")
	defer ctx.Logger.Info("Finished Reconcile VirtualMachineService")

	vmService := ctx.VMService

	if vmService.Spec.Type == vmopv1.VirtualMachineServiceTypeLoadBalancer {
		// Get LoadBalancer to attach
		err := r.loadbalancerProvider.EnsureLoadBalancer(ctx, vmService)
		if err != nil {
			ctx.Logger.Error(err, "Failed to create or get load balancer for VM Service")
			return err
		}

		// Get the provider specific annotations for service and add them to the VMService as
		// that's where Service inherits the values.
		annotations, err := r.loadbalancerProvider.GetServiceAnnotations(ctx, vmService)
		if err != nil {
			ctx.Logger.Error(err, "Failed to get loadbalancer annotations for service")
			return err
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
		labels, err := r.loadbalancerProvider.GetServiceLabels(ctx, vmService)
		if err != nil {
			ctx.Logger.Error(err, "Failed to get loadbalancer labels for service")
			return err
		}

		if vmService.Labels == nil {
			vmService.Labels = make(map[string]string)
		}

		for k, v := range labels {
			if oldValue, ok := vmService.Labels[k]; ok {
				ctx.Logger.V(5).Info("Replacing previous label value on VM Service",
					"key", k, "oldValue", oldValue, "newValue", v)
			}
			vmService.Labels[k] = v
		}
	}

	service, err := r.createOrUpdateService(ctx)
	if err != nil {
		ctx.Logger.Error(err, "Failed to update VirtualMachineService k8s Service")
		return err
	}

	err = r.createOrUpdateEndpoints(ctx, service)
	if err != nil {
		ctx.Logger.Error(err, "Failed to update VirtualMachineService Endpoints")
		return err
	}

	err = r.updateVMService(ctx, service)
	if err != nil {
		ctx.Logger.Error(err, "Failed to update VirtualMachineService Status")
		return err
	}

	return nil
}

// virtualMachineToVirtualMachineServiceMapper returns a mapper function that returns reconcile requests for
// VirtualMachineServices that select a given VM via label selectors.
// TODO: The VM's labels could have been changed so this should also return VirtualMachineServices that the
// VM is currently an Endpoint for, because otherwise the VM won't be removed in a timely manner.
func (r *ReconcileVirtualMachineService) virtualMachineToVirtualMachineServiceMapper() func(_ context.Context, o client.Object) []reconcile.Request {
	return func(_ context.Context, o client.Object) []reconcile.Request {
		vm := o.(*vmopv1.VirtualMachine)

		reconcileRequests, err := r.getVirtualMachineServicesSelectingVirtualMachine(context.Background(), vm)
		if err != nil {
			return nil
		}

		if len(reconcileRequests) != 0 && r.log.V(4).Enabled() {
			logger := r.log.WithValues("VirtualMachine", client.ObjectKey{Namespace: vm.Namespace, Name: vm.Name})
			for _, r := range reconcileRequests {
				logger.V(4).Info("Generating reconcile request for VM Service due to event on VM", "VirtualMachineService", r.NamespacedName)
			}
		}

		return reconcileRequests
	}
}

// Set labels and annotations on the Service from the VirtualMachineService. Some loadbalancer providers (currently
// only NCP) need to filter or translate labels and annotations too.
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
	annotationsToBeRemoved, err := r.loadbalancerProvider.GetToBeRemovedServiceAnnotations(ctx, ctx.VMService)
	if err != nil {
		ctx.Logger.Error(err, "Failed to get loadbalancer specific annotations to remove from Service")
		return err
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
	labelsToBeRemoved, err := r.loadbalancerProvider.GetToBeRemovedServiceLabels(ctx, ctx.VMService)
	if err != nil {
		ctx.Logger.Error(err, "Failed to get loadbalancer specific labels to remove from Service")
		return err
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

		// Set IPFamilies and IPFamilyPolicy for dual-stack support
		// These fields only apply to ClusterIP, NodePort, and LoadBalancer types
		// They are wiped when type is ExternalName
		if vmService.Spec.Type != vmopv1.VirtualMachineServiceTypeExternalName {
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
			trafficPolicy := corev1.ServiceExternalTrafficPolicyType(externalTrafficPolicy)
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

func (r *ReconcileVirtualMachineService) getVirtualMachineServicesSelectingVirtualMachine(
	ctx context.Context,
	lookupVM *vmopv1.VirtualMachine) ([]reconcile.Request, error) {

	if len(lookupVM.Labels) == 0 {
		return nil, nil
	}

	vmServiceList := &vmopv1.VirtualMachineServiceList{}
	err := r.List(ctx, vmServiceList, client.InNamespace(lookupVM.Namespace))
	if err != nil {
		return nil, err
	}

	var matchingVMServices []reconcile.Request
	vmLabels := labels.Set(lookupVM.Labels)

	for _, vmService := range vmServiceList.Items {
		if len(vmService.Spec.Selector) != 0 {
			sel := labels.SelectorFromValidatedSet(vmService.Spec.Selector)
			if sel.Matches(vmLabels) {
				matchingVMServices = append(matchingVMServices, reconcile.Request{
					NamespacedName: client.ObjectKey{Namespace: vmService.Namespace, Name: vmService.Name},
				})
			}
		}
	}

	return matchingVMServices, nil
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
	// Split subsets by IP family to ensure IPv4 and IPv6 remain separate
	// Above RepackSubsets merges subsets with identical ports, which combines IPv4 and IPv6
	subsets = r.splitSubsetsByIPFamily(subsets)

	// Filter endpoints based on Service's IPFamilies and IPFamilyPolicy
	// Endpoints must match the Service's IP family - an IPv4 Service cannot route to IPv6 endpoints
	allowedFamilies := r.determineAllowedIPFamilies(service)
	if len(allowedFamilies) > 0 {
		subsets = r.filterSubsetsByIPFamilies(subsets, allowedFamilies)
	}

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

	// Group addresses by IP family to ensure IPv4 and IPv6 remain in separate subsets
	// after RepackSubsets merges subsets with identical ports
	type addressInfo struct {
		addr  corev1.EndpointAddress
		ready bool
		ports []corev1.EndpointPort
	}
	ipFamilyToAddresses := make(map[corev1.IPFamily][]addressInfo)
	var vmInSubsetsMap map[types.UID]struct{}

	for i := range vmList.Items {
		vm := vmList.Items[i]
		logger := ctx.Logger.WithValues("virtualMachine", vm.NamespacedName())

		if !vm.DeletionTimestamp.IsZero() {
			logger.Info("Skipping VM marked for deletion")
			continue
		}

		// Collect all IPs for this VM (both IPv4 and IPv6)
		var vmIPs []struct {
			ip     string
			family corev1.IPFamily
		}
		if vm.Status.Network != nil {
			if vm.Status.Network.PrimaryIP4 != "" {
				vmIPs = append(vmIPs, struct {
					ip     string
					family corev1.IPFamily
				}{vm.Status.Network.PrimaryIP4, corev1.IPv4Protocol})
			}
			if vm.Status.Network.PrimaryIP6 != "" {
				vmIPs = append(vmIPs, struct {
					ip     string
					family corev1.IPFamily
				}{vm.Status.Network.PrimaryIP6, corev1.IPv6Protocol})
			}
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

		if probe := vm.Spec.ReadinessProbe; probe != nil && (probe.TCPSocket != nil || probe.GuestHeartbeat != nil || len(probe.GuestInfo) != 0) {
			if condition := conditions.Get(&vm, vmopv1.ReadyConditionType); condition == nil {
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

		// Group addresses by IP family
		// This ensures IPv4 and IPv6 addresses remain in separate subsets
		for _, vmIPInfo := range vmIPs {
			epa := corev1.EndpointAddress{
				IP: vmIPInfo.ip,
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

			ipFamilyToAddresses[vmIPInfo.family] = append(ipFamilyToAddresses[vmIPInfo.family], addressInfo{
				addr:  epa,
				ready: ready,
				ports: ports,
			})
		}
	}

	// Create subsets grouped by IP family
	// This ensures IPv4 and IPv6 addresses remain in separate subsets even after RepackSubsets
	subsets := make([]corev1.EndpointSubset, 0, len(ipFamilyToAddresses))
	for _, addrInfos := range ipFamilyToAddresses {
		if len(addrInfos) == 0 {
			continue
		}

		var readyAddrs, notReadyAddrs []corev1.EndpointAddress
		for _, addrInfo := range addrInfos {
			if addrInfo.ready {
				readyAddrs = append(readyAddrs, addrInfo.addr)
			} else {
				notReadyAddrs = append(notReadyAddrs, addrInfo.addr)
			}
		}

		// All addresses in this group should have the same ports (they come from the same service)
		// Use the ports from the first address info
		ports := addrInfos[0].ports

		subset := corev1.EndpointSubset{
			Addresses:         readyAddrs,
			NotReadyAddresses: notReadyAddrs,
			Ports:             ports,
		}

		subsets = append(subsets, subset)
	}

	return subsets, nil
}

// splitSubsetsByIPFamily splits subsets that contain both IPv4 and IPv6 addresses
// into separate subsets, one for each IP family. This ensures dual-stack support
// even after RepackSubsets merges subsets with identical ports.
func (r *ReconcileVirtualMachineService) splitSubsetsByIPFamily(subsets []corev1.EndpointSubset) []corev1.EndpointSubset {
	var result []corev1.EndpointSubset

	for _, subset := range subsets {
		var ipv4Addrs, ipv6Addrs []corev1.EndpointAddress
		var ipv4NotReady, ipv6NotReady []corev1.EndpointAddress

		// Split ready addresses by IP family
		for _, addr := range subset.Addresses {
			// Use net.ParseIP to properly detect IPv4 vs IPv6
			// Skip invalid IPs (shouldn't happen with valid endpoint addresses)
			if ip := net.ParseIP(addr.IP); ip != nil {
				if ip.To4() != nil {
					ipv4Addrs = append(ipv4Addrs, addr)
				} else {
					ipv6Addrs = append(ipv6Addrs, addr)
				}
			}
			// Invalid IPs are silently skipped (shouldn't occur in practice)
		}

		// Split not-ready addresses by IP family
		for _, addr := range subset.NotReadyAddresses {
			// Use net.ParseIP to properly detect IPv4 vs IPv6
			// Skip invalid IPs (shouldn't happen with valid endpoint addresses)
			if ip := net.ParseIP(addr.IP); ip != nil {
				if ip.To4() != nil {
					ipv4NotReady = append(ipv4NotReady, addr)
				} else {
					ipv6NotReady = append(ipv6NotReady, addr)
				}
			}
			// Invalid IPs are silently skipped (shouldn't occur in practice)
		}

		// Create separate subsets for IPv4 and IPv6 if both exist
		if len(ipv4Addrs) > 0 || len(ipv4NotReady) > 0 {
			result = append(result, corev1.EndpointSubset{
				Addresses:         ipv4Addrs,
				NotReadyAddresses: ipv4NotReady,
				Ports:             subset.Ports,
			})
		}
		if len(ipv6Addrs) > 0 || len(ipv6NotReady) > 0 {
			result = append(result, corev1.EndpointSubset{
				Addresses:         ipv6Addrs,
				NotReadyAddresses: ipv6NotReady,
				Ports:             subset.Ports,
			})
		}
	}

	return result
}

// determineAllowedIPFamilies determines which IP families should be included
// in endpoints based on Service's IPFamilies and IPFamilyPolicy.
// For SingleStack policy, only the ClusterIP/IPFamilies family is allowed.
// Endpoints must match the Service's IP family - an IPv4 Service cannot route to IPv6 endpoints.
func (r *ReconcileVirtualMachineService) determineAllowedIPFamilies(service *corev1.Service) map[corev1.IPFamily]bool {
	allowedFamilies := make(map[corev1.IPFamily]bool)

	// If IPFamilies is explicitly set, use that (takes precedence for all policies)
	if len(service.Spec.IPFamilies) > 0 {
		for _, family := range service.Spec.IPFamilies {
			allowedFamilies[family] = true
		}
		return allowedFamilies
	}

	// For SingleStack policy without explicit IPFamilies, use ClusterIP family as primary
	if service.Spec.IPFamilyPolicy != nil && *service.Spec.IPFamilyPolicy == corev1.IPFamilyPolicySingleStack {
		// Determine primary family from IPFamilies or ClusterIP
		var primaryFamily corev1.IPFamily
		if len(service.Spec.IPFamilies) > 0 {
			primaryFamily = service.Spec.IPFamilies[0]
		} else {
			// Determine family from ClusterIP
			var firstIP string
			if len(service.Spec.ClusterIPs) > 0 {
				firstIP = service.Spec.ClusterIPs[0]
			} else if service.Spec.ClusterIP != "" {
				firstIP = service.Spec.ClusterIP
			}
			if firstIP != "" && firstIP != "None" {
				if ip := net.ParseIP(firstIP); ip != nil {
					if ip.To4() != nil {
						primaryFamily = corev1.IPv4Protocol
					} else {
						primaryFamily = corev1.IPv6Protocol
					}
				} else {
					primaryFamily = corev1.IPv4Protocol // Default
				}
			} else {
				primaryFamily = corev1.IPv4Protocol // Default
			}
		}

		// For SingleStack, only allow the primary family (ClusterIP/IPFamilies family)
		// This ensures that endpoints match the Service's IP family, as an IPv4 Service
		// cannot route to IPv6 endpoints and vice versa
		allowedFamilies[primaryFamily] = true
		return allowedFamilies
	}

	// For non-SingleStack policies, use IPFamilies if explicitly set
	if len(service.Spec.IPFamilies) > 0 {
		for _, family := range service.Spec.IPFamilies {
			allowedFamilies[family] = true
		}
		return allowedFamilies
	}

	// If IPFamilies is not set, use IPFamilyPolicy to determine behavior
	if service.Spec.IPFamilyPolicy != nil {
		switch *service.Spec.IPFamilyPolicy {
		case corev1.IPFamilyPolicyPreferDualStack, corev1.IPFamilyPolicyRequireDualStack:
			// PreferDualStack or RequireDualStack: Include both families
			allowedFamilies[corev1.IPv4Protocol] = true
			allowedFamilies[corev1.IPv6Protocol] = true
		}
	} else {
		// If neither IPFamilies nor IPFamilyPolicy is set, include all (backward compatibility)
		allowedFamilies[corev1.IPv4Protocol] = true
		allowedFamilies[corev1.IPv6Protocol] = true
	}

	return allowedFamilies
}

// filterSubsetsByIPFamilies filters endpoint subsets to only include addresses
// matching the allowed IP families.
func (r *ReconcileVirtualMachineService) filterSubsetsByIPFamilies(subsets []corev1.EndpointSubset, allowedFamilies map[corev1.IPFamily]bool) []corev1.EndpointSubset {
	var result []corev1.EndpointSubset
	for _, subset := range subsets {
		var filteredAddrs, filteredNotReady []corev1.EndpointAddress

		// Filter ready addresses
		for _, addr := range subset.Addresses {
			// Use net.ParseIP to properly detect IPv4 vs IPv6
			// Skip invalid IPs (shouldn't happen with valid endpoint addresses)
			if ip := net.ParseIP(addr.IP); ip != nil {
				var family corev1.IPFamily
				if ip.To4() != nil {
					family = corev1.IPv4Protocol
				} else {
					family = corev1.IPv6Protocol
				}
				if allowedFamilies[family] {
					filteredAddrs = append(filteredAddrs, addr)
				}
			}
			// Invalid IPs are silently skipped (shouldn't occur in practice)
		}

		// Filter not-ready addresses
		for _, addr := range subset.NotReadyAddresses {
			// Use net.ParseIP to properly detect IPv4 vs IPv6
			// Skip invalid IPs (shouldn't happen with valid endpoint addresses)
			if ip := net.ParseIP(addr.IP); ip != nil {
				var family corev1.IPFamily
				if ip.To4() != nil {
					family = corev1.IPv4Protocol
				} else {
					family = corev1.IPv6Protocol
				}
				if allowedFamilies[family] {
					filteredNotReady = append(filteredNotReady, addr)
				}
			}
			// Invalid IPs are silently skipped (shouldn't occur in practice)
		}

		// Only include subset if it has addresses
		if len(filteredAddrs) > 0 || len(filteredNotReady) > 0 {
			result = append(result, corev1.EndpointSubset{
				Addresses:         filteredAddrs,
				NotReadyAddresses: filteredNotReady,
				Ports:             subset.Ports,
			})
		}
	}
	return result
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
	}

	return nil
}
