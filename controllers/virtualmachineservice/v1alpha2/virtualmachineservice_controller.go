// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	goctx "context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/v1alpha2/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/v1alpha2/utils"
	conditions "github.com/vmware-tanzu/vm-operator/pkg/conditions2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	patch "github.com/vmware-tanzu/vm-operator/pkg/patch2"
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
		controlledType     = &vmopv1.VirtualMachineService{}
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
		record.New(mgr.GetEventRecorderFor(controllerNameLong)),
		lbProvider,
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(controlledType).
		WithOptions(controller.Options{MaxConcurrentReconciles: ctx.MaxConcurrentReconciles}).
		Watches(&source.Kind{Type: &corev1.Service{}},
			&handler.EnqueueRequestForOwner{OwnerType: &vmopv1.VirtualMachineService{}}).
		Watches(&source.Kind{Type: &corev1.Endpoints{}},
			&handler.EnqueueRequestForOwner{OwnerType: &vmopv1.VirtualMachineService{}}).
		Watches(&source.Kind{Type: &vmopv1.VirtualMachine{}},
			handler.EnqueueRequestsFromMapFunc(r.virtualMachineToVirtualMachineServiceMapper())).
		Complete(r)
}

func NewReconciler(
	client client.Client,
	logger logr.Logger,
	recorder record.Recorder,
	lbProvider providers.LoadbalancerProvider,
) *ReconcileVirtualMachineService {
	return &ReconcileVirtualMachineService{
		Client:               client,
		log:                  logger,
		recorder:             recorder,
		loadbalancerProvider: lbProvider,
	}
}

var _ reconcile.Reconciler = &ReconcileVirtualMachineService{}

// ReconcileVirtualMachineService reconciles a VirtualMachineService object.
type ReconcileVirtualMachineService struct {
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

func (r *ReconcileVirtualMachineService) Reconcile(ctx goctx.Context, request reconcile.Request) (_ reconcile.Result, reterr error) {
	vmService := &vmopv1.VirtualMachineService{}
	if err := r.Get(ctx, request.NamespacedName, vmService); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	vmServiceCtx := &context.VirtualMachineServiceContextA2{
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

func (r *ReconcileVirtualMachineService) ReconcileDelete(ctx *context.VirtualMachineServiceContextA2) error {
	if controllerutil.ContainsFinalizer(ctx.VMService, finalizerName) {
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
	}

	return nil
}

func (r *ReconcileVirtualMachineService) ReconcileNormal(ctx *context.VirtualMachineServiceContextA2) error {
	if !controllerutil.ContainsFinalizer(ctx.VMService, finalizerName) {
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

func (r *ReconcileVirtualMachineService) reconcileVMService(ctx *context.VirtualMachineServiceContextA2) error {
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
func (r *ReconcileVirtualMachineService) virtualMachineToVirtualMachineServiceMapper() func(o client.Object) []reconcile.Request {
	return func(o client.Object) []reconcile.Request {
		vm := o.(*vmopv1.VirtualMachine)

		reconcileRequests, err := r.getVirtualMachineServicesSelectingVirtualMachine(goctx.Background(), vm)
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
	ctx *context.VirtualMachineServiceContextA2,
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

func (r *ReconcileVirtualMachineService) createOrUpdateService(ctx *context.VirtualMachineServiceContextA2) (*corev1.Service, error) {
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
		if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
			service.Spec.AllocateLoadBalancerNodePorts = pointer.Bool(false)
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
	ctx *context.VirtualMachineServiceContextA2) (*vmopv1.VirtualMachineList, error) {

	if len(ctx.VMService.Spec.Selector) == 0 {
		return nil, nil
	}

	vmList := &vmopv1.VirtualMachineList{}
	err := r.List(ctx, vmList, client.InNamespace(ctx.VMService.Namespace), client.MatchingLabels(ctx.VMService.Spec.Selector))
	return vmList, err
}

// getVMsReferencedByServiceEndpoints gets all VMs that are referenced by service endpoints.
func (r *ReconcileVirtualMachineService) getVMsReferencedByServiceEndpoints(
	ctx *context.VirtualMachineServiceContextA2,
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
	ctx goctx.Context,
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
func (r *ReconcileVirtualMachineService) createOrUpdateEndpoints(ctx *context.VirtualMachineServiceContextA2, service *corev1.Service) error {
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
	ctx *context.VirtualMachineServiceContextA2,
	service *corev1.Service) ([]corev1.EndpointSubset, error) {

	vmList, err := r.getVirtualMachinesSelectedByVMService(ctx)
	if err != nil {
		return nil, err
	}

	var subsets = make([]corev1.EndpointSubset, 0, len(vmList.Items))
	var vmInSubsetsMap map[types.UID]struct{}

	for i := range vmList.Items {
		vm := vmList.Items[i]
		logger := ctx.Logger.WithValues("virtualMachine", vm.NamespacedName())

		if !vm.DeletionTimestamp.IsZero() {
			logger.Info("Skipping VM marked for deletion")
			continue
		}

		var vmIP string
		if vm.Status.Network != nil {
			vmIP = vm.Status.Network.PrimaryIP4
			if vmIP == "" {
				vmIP = vm.Status.Network.PrimaryIP6
			}
		}

		if vmIP == "" {
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

		if probe := vm.Spec.ReadinessProbe; probe.TCPSocket != nil || probe.GuestHeartbeat != nil || len(probe.GuestInfo) != 0 {
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

		epa := corev1.EndpointAddress{
			IP: vmIP,
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

		// Populate the EP subset for this VM. We create one subset for each VM, and then our
		// caller will repack the subsets that have identical ports.
		subset := corev1.EndpointSubset{}
		if ready {
			subset.Addresses = []corev1.EndpointAddress{epa}
		} else {
			subset.NotReadyAddresses = []corev1.EndpointAddress{epa}
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

			subset.Ports = append(subset.Ports,
				corev1.EndpointPort{Name: portName, Port: int32(portNum), Protocol: portProto})
		}

		subsets = append(subsets, subset)
	}

	return subsets, nil
}

// updateVMService syncs the VirtualMachineService Status from the Service status.
func (r *ReconcileVirtualMachineService) updateVMService(ctx *context.VirtualMachineServiceContextA2, service *corev1.Service) error {
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
