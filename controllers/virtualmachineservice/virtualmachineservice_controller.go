// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice

import (
	goctx "context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	finalizerName = "virtualmachineservice.vmoperator.vmware.com"

	OpCreate = "CreateK8sService"
	OpDelete = "DeleteK8sService"
	OpUpdate = "UpdateK8sService"
)

func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	var (
		controlledType     = &vmopv1alpha1.VirtualMachineService{}
		controlledTypeName = reflect.TypeOf(controlledType).Elem().Name()

		controllerNameShort = fmt.Sprintf("%s-controller", strings.ToLower(controlledTypeName))
		controllerNameLong  = fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, controllerNameShort)
	)

	lbProviderType := os.Getenv("LB_PROVIDER")
	if lbProviderType == "" {
		vdsNetwork := os.Getenv("VSPHERE_NETWORKING")
		if vdsNetwork != "true" {
			lbProviderType = providers.NSXTLoadBalancer
		}
	}

	lbProvider, err := providers.GetLoadbalancerProviderByType(mgr, lbProviderType)
	if err != nil {
		return err
	}

	r := NewReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName(controlledTypeName),
		mgr.GetScheme(),
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		lbProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Watches(&source.Kind{Type: &corev1.Service{}},
			&handler.EnqueueRequestForOwner{OwnerType: &vmopv1alpha1.VirtualMachineService{}}).
		Watches(&source.Kind{Type: &corev1.Endpoints{}},
			&handler.EnqueueRequestForOwner{OwnerType: &vmopv1alpha1.VirtualMachineService{}}).
		Watches(&source.Kind{Type: &vmopv1alpha1.VirtualMachine{}},
			handler.EnqueueRequestsFromMapFunc(r.virtualMachineToVirtualMachineServiceMapper())).
		Complete(r)
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	scheme *runtime.Scheme,
	recorder record.Recorder,
	lbProvider providers.LoadbalancerProvider,
) *ReconcileVirtualMachineService {
	return &ReconcileVirtualMachineService{
		Client:               client,
		log:                  logger,
		scheme:               scheme,
		recorder:             recorder,
		loadbalancerProvider: lbProvider,
	}
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineService{}

// ReconcileVirtualMachineService reconciles a VirtualMachineService object
type ReconcileVirtualMachineService struct {
	client.Client
	log                  logr.Logger
	scheme               *runtime.Scheme
	recorder             record.Recorder
	loadbalancerProvider providers.LoadbalancerProvider
}

// Reconcile reads that state of the cluster for a VirtualMachineService object and makes changes based on the state read
// and what is in the VirtualMachineService.Spec
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;create;update;patch;delete

func (r *ReconcileVirtualMachineService) Reconcile(ctx goctx.Context, request reconcile.Request) (_ reconcile.Result, reterr error) {
	vmService := &vmopv1alpha1.VirtualMachineService{}
	if err := r.Get(ctx, request.NamespacedName, vmService); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	vmServiceCtx := &context.VirtualMachineServiceContext{
		Context:   ctx,
		Logger:    ctrl.Log.WithName("VirtualMachineService").WithValues("name", vmService.NamespacedName()),
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

func (r *ReconcileVirtualMachineService) ReconcileDelete(ctx *context.VirtualMachineServiceContext) error {
	if controllerutil.ContainsFinalizer(ctx.VMService, finalizerName) {
		ctx.Logger.Info("Delete VirtualMachineService")
		r.recorder.EmitEvent(ctx.VMService, OpDelete, nil, false)
		// NOTE: For now, we let k8s GC delete the Service and Endpoints.
		controllerutil.RemoveFinalizer(ctx.VMService, finalizerName)
	}

	return nil
}

func (r *ReconcileVirtualMachineService) ReconcileNormal(ctx *context.VirtualMachineServiceContext) error {
	if !controllerutil.ContainsFinalizer(ctx.VMService, finalizerName) {
		controllerutil.AddFinalizer(ctx.VMService, finalizerName)
		// NOTE: The VirtualMachineService is set as the OwnerReference of the Service and Endpoints,
		// and we let k8s GC delete those so we don't _have_ to return here to let the patch helper
		// update the VM Service finalizer before proceeding.
	}

	if err := r.reconcileVMService(ctx); err != nil {
		ctx.Logger.Error(err, "Failed to reconcile VirtualMachineService")
		return err
	}

	return nil
}

func (r *ReconcileVirtualMachineService) reconcileVMService(ctx *context.VirtualMachineServiceContext) error {
	ctx.Logger.Info("Reconcile VirtualMachineService")
	defer ctx.Logger.Info("Finished Reconcile VirtualMachineService")

	vmService := ctx.VMService

	if vmService.Spec.Type == vmopv1alpha1.VirtualMachineServiceTypeLoadBalancer {
		// Get LoadBalancer to attach
		err := r.loadbalancerProvider.EnsureLoadBalancer(ctx, vmService)
		if err != nil {
			ctx.Logger.Error(err, "Failed to create or get load balancer for VM Service")
			return err
		}

		// Get the provider specific annotations for service and add them to the VMService as
		// that's where Service inherits the values.
		annotations, err := r.loadbalancerProvider.GetServiceAnnotations(ctx, ctx.VMService)
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
		labels, err := r.loadbalancerProvider.GetServiceLabels(ctx, ctx.VMService)
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

	err = r.updateVmService(ctx, service)
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
func (r *ReconcileVirtualMachineService) virtualMachineToVirtualMachineServiceMapper() func(o client.Object) []reconcile.Request {
	return func(o client.Object) []reconcile.Request {
		vm := o.(*vmopv1alpha1.VirtualMachine)

		// Find all VMServices that match this VM.
		vmServiceList, err := r.getVirtualMachineServicesSelectingVirtualMachine(goctx.Background(), vm)
		if err != nil {
			return nil
		}

		logger := r.log.WithValues("VirtualMachine", types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name})

		var reconcileRequests []reconcile.Request
		for _, vmService := range vmServiceList {
			key := types.NamespacedName{Namespace: vmService.Namespace, Name: vmService.Name}
			logger.V(4).Info("Generating reconcile request for VM Service due to event on VMs", "VirtualMachineService", key)
			reconcileRequests = append(reconcileRequests, reconcile.Request{NamespacedName: key})
		}

		return reconcileRequests
	}
}

// Set labels and annotations on the Service from the VirtualMachineService. Some loadbalancer providers (currently
// only NCP) need to filter or translate labels and annotations too.
func (r *ReconcileVirtualMachineService) setServiceAnnotationsAndLabels(
	ctx *context.VirtualMachineServiceContext,
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

func (r *ReconcileVirtualMachineService) createOrUpdateService(ctx *context.VirtualMachineServiceContext) (*corev1.Service, error) {
	ctx.Logger.V(5).Info("Reconciling k8s Service")
	defer ctx.Logger.V(5).Info("Finished reconciling k8s Service")

	vmService := ctx.VMService
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmService.Name,
			Namespace: vmService.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		if err := controllerutil.SetControllerReference(vmService, service, r.scheme); err != nil {
			return err
		}

		if err := r.setServiceAnnotationsAndLabels(ctx, service); err != nil {
			return err
		}

		service.Spec.Type = corev1.ServiceType(vmService.Spec.Type)
		service.Spec.ExternalName = vmService.Spec.ExternalName
		service.Spec.LoadBalancerIP = vmService.Spec.LoadBalancerIP
		service.Spec.LoadBalancerSourceRanges = vmService.Spec.LoadBalancerSourceRanges

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

func (r *ReconcileVirtualMachineService) getVirtualMachinesSelectedByVmService(
	ctx goctx.Context,
	vmService *vmopv1alpha1.VirtualMachineService) (*vmopv1alpha1.VirtualMachineList, error) {

	vmList := &vmopv1alpha1.VirtualMachineList{}
	err := r.List(ctx, vmList, client.InNamespace(vmService.Namespace), client.MatchingLabels(vmService.Spec.Selector))
	return vmList, err
}

// getVMsReferencedByServiceEndpoints gets all VMs that are referenced by service endpoints
func (r *ReconcileVirtualMachineService) getVMsReferencedByServiceEndpoints(
	ctx *context.VirtualMachineServiceContext,
	service *corev1.Service) map[types.UID]struct{} {

	endpoints := &corev1.Endpoints{}
	if err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, endpoints); err != nil {
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

// TODO: This mapping function has the potential to be a performance and scaling issue.
// Consider this as a candidate for profiling. SPOILER: It already is an issue.
func (r *ReconcileVirtualMachineService) getVirtualMachineServicesSelectingVirtualMachine(
	ctx goctx.Context,
	lookupVm *vmopv1alpha1.VirtualMachine) ([]*vmopv1alpha1.VirtualMachineService, error) {

	var matchingVmServices []*vmopv1alpha1.VirtualMachineService

	matchFunc := func(vmService *vmopv1alpha1.VirtualMachineService) error {
		vmList, err := r.getVirtualMachinesSelectedByVmService(ctx, vmService)
		if err != nil {
			return err
		}

		for _, vm := range vmList.Items {
			if vm.Name == lookupVm.Name && vm.Namespace == lookupVm.Namespace {
				matchingVmServices = append(matchingVmServices, vmService)
				// Only one match is needed to add vmService, so return now.
				return nil
			}
		}

		return nil
	}

	vmServiceList := &vmopv1alpha1.VirtualMachineServiceList{}
	err := r.List(ctx, vmServiceList, client.InNamespace(lookupVm.Namespace))
	if err != nil {
		return nil, err
	}

	for _, vmService := range vmServiceList.Items {
		vms := vmService
		err := matchFunc(&vms)
		if err != nil {
			return nil, err
		}
	}

	return matchingVmServices, nil
}

// createOrUpdateEndpoints updates the Endpoints for VirtualMachineService.
func (r *ReconcileVirtualMachineService) createOrUpdateEndpoints(ctx *context.VirtualMachineServiceContext, service *corev1.Service) error {
	ctx.Logger.V(5).Info("Updating VirtualMachineService Endpoints")
	defer ctx.Logger.V(5).Info("Finished updating VirtualMachineService Endpoints")

	subsets, err := r.generateSubsetsForService(ctx, service)
	if err != nil {
		return err
	}

	endpoints := &corev1.Endpoints{}
	endpointsKey := types.NamespacedName{Name: service.Name, Namespace: service.Namespace}

	// The kube apiserver sorts the Endpoint subsets before saving, so the order that we create or
	// update with isn't necessarily the order we'll get back. Basically unroll controller-runtime's
	// CreateOrUpdate() here to handle resorting the subsets before the DeepEqual() comparison.

	mutateFn := func(epSubsets []corev1.EndpointSubset) error {
		endpoints.Name = service.Name
		endpoints.Namespace = service.Namespace
		// NCP apparently needs the same Labels as what is present on the Service, and I'm not aware
		// of anything else setting Labels, so just sync the Labels (and Annotations) with the Service.
		endpoints.Labels = service.Labels
		endpoints.Annotations = service.Annotations
		endpoints.Subsets = epSubsets
		return controllerutil.SetControllerReference(ctx.VMService, endpoints, r.scheme)
	}

	if err := r.Get(ctx, endpointsKey, endpoints); err != nil {
		if !apiErrors.IsNotFound(err) {
			ctx.Logger.Error(err, "Failed to get Endpoints")
			return err
		}

		if err := mutateFn(subsets); err != nil {
			return err
		}

		ctx.Logger.Info("Creating Service Endpoints", "endpoints", endpoints)
		return r.Create(ctx, endpoints)
	}

	// Copy the Endpoints and resort the subsets so that DeepEquals() doesn't return false when the
	// subsets are just sorted differently, causing the Endpoints to churn.
	existingEndpoints := endpoints.DeepCopy()
	existingEndpoints.Subsets = utils.RepackSubsets(existingEndpoints.Subsets)

	if err := mutateFn(utils.RepackSubsets(subsets)); err != nil {
		return err
	}

	if !apiequality.Semantic.DeepEqual(endpoints, existingEndpoints) {
		ctx.Logger.Info("Updating Service Endpoints", "endpoints", endpoints)
		if err := r.Update(ctx, endpoints); err != nil {
			return err
		}
	}

	return nil
}

func findVMPortNum(vm *vmopv1alpha1.VirtualMachine, port intstr.IntOrString, portProto corev1.Protocol) (int, error) {
	switch port.Type {
	case intstr.String:
		// NOTE: The VM Spec.Ports is deprecated.
		for _, vmPort := range vm.Spec.Ports {
			if vmPort.Name == port.StrVal && vmPort.Protocol == portProto {
				return vmPort.Port, nil
			}
		}
	case intstr.Int:
		return port.IntValue(), nil
	}

	return 0, fmt.Errorf("no matching port on VM")
}

// generateSubsetsForService generates Endpoints subsets for a given Service.
func (r *ReconcileVirtualMachineService) generateSubsetsForService(
	ctx *context.VirtualMachineServiceContext, service *corev1.Service) ([]corev1.EndpointSubset, error) {

	vmList, err := r.getVirtualMachinesSelectedByVmService(ctx, ctx.VMService)
	if err != nil {
		return nil, err
	}

	var subsets []corev1.EndpointSubset
	var vmInSubsetsMap map[types.UID]struct{}

	for i := range vmList.Items {
		vm := vmList.Items[i]
		logger := ctx.Logger.WithValues("virtualMachine", vm.NamespacedName())

		if !vm.DeletionTimestamp.IsZero() {
			logger.Info("Skipping VM marked for deletion")
			continue
		}

		// TODO: Handle multiple VM interfaces.
		if vm.Status.VmIp == "" {
			logger.Info("Skipping VM that does not have an IP")
			continue
		}

		// Ignore VMs that fail the readiness check (only when probes are specified)
		// If VM has Ready condition in the status and the value is true, we add the VM to the subset.
		// If this condition is missing but the probe spec is set, which may happen right after upgrade when it is first released,
		// we check the service's backing endpoints and determine whether this VM needs to be added to the subset.
		// If this VM isn't in the old endpoint subsets, we skip it.
		// Otherwise, we consider this VM as ready and add it to the subset.
		if vm.Spec.ReadinessProbe != nil {
			if condition := conditions.Get(&vm, vmopv1alpha1.ReadyCondition); condition == nil {
				if vmInSubsetsMap == nil {
					vmInSubsetsMap = r.getVMsReferencedByServiceEndpoints(ctx, service)
				}

				if _, ok := vmInSubsetsMap[vm.UID]; !ok {
					// This VM was not previously in the Service's Endpoints so don't include it yet.
					logger.Info("Skipping VM due to missing Ready condition")
					continue
				}
			} else if condition.Status != corev1.ConditionTrue {
				logger.Info("Skipping VM due to false Ready condition",
					"conditionReason", condition.Reason, "conditionMessage", condition.Message)
				continue
			}
		}

		epa := corev1.EndpointAddress{
			IP: vm.Status.VmIp,
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

			epp := corev1.EndpointPort{Name: portName, Port: int32(portNum), Protocol: portProto}
			subsets = append(subsets, corev1.EndpointSubset{
				Addresses: []corev1.EndpointAddress{epa},
				Ports:     []corev1.EndpointPort{epp},
			})
		}
	}

	return subsets, nil
}

// updateVmService syncs the VirtualMachineService Status from the Service status.
func (r *ReconcileVirtualMachineService) updateVmService(ctx *context.VirtualMachineServiceContext, service *corev1.Service) error {
	vmService := ctx.VMService

	if vmService.Spec.Type == vmopv1alpha1.VirtualMachineServiceTypeLoadBalancer {
		vmService.Status.LoadBalancer.Ingress = make([]vmopv1alpha1.LoadBalancerIngress, len(service.Status.LoadBalancer.Ingress))
		for idx, ingress := range service.Status.LoadBalancer.Ingress {
			vmIngress := vmopv1alpha1.LoadBalancerIngress{
				IP:       ingress.IP,
				Hostname: ingress.Hostname,
			}
			vmService.Status.LoadBalancer.Ingress[idx] = vmIngress
		}
	}

	return nil
}
