// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice

import (
	goctx "context"
	"fmt"
	"net"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	endpointsv1 "k8s.io/kubernetes/pkg/api/v1/endpoints"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	ServiceOwnerRefKind    = "VirtualMachineService"
	ServiceOwnerRefVersion = "vmoperator.vmware.com/v1alpha1"

	finalizerName = "virtualmachineservice.vmoperator.vmware.com"

	OpCreate       = "CreateK8sService"
	OpDelete       = "DeleteK8sService"
	OpUpdate       = "UpdateK8sService"
	ControllerName = "virtualmachineservice-controller"

	defaultConnectTimeout = time.Second * 10

	probeFailureRequeueTime = time.Second * 10
)

// RequeueAfterError implements error interface and can be used to indicate the error should result in a requeue of
// the object under reconciliation after the specified duration of time.
type RequeueAfterError struct {
	RequeueAfter time.Duration
}

func (e *RequeueAfterError) Error() string {
	return fmt.Sprintf("requeue in: %s", e.RequeueAfter)
}

func (e *RequeueAfterError) GetRequeueAfter() time.Duration {
	return e.RequeueAfter
}

func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(ctx, mgr, r, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (*ReconcileVirtualMachineService, error) {
	provider, err := providers.GetLoadbalancerProviderByType(mgr, providers.LBProvider)
	if err != nil {
		return nil, err
	}
	return &ReconcileVirtualMachineService{
		Client:               mgr.GetClient(),
		log:                  ctrl.Log.WithName("controllers").WithName("VirtualMachineServices"),
		scheme:               mgr.GetScheme(),
		recorder:             record.New(mgr.GetEventRecorderFor("virtualmachineservices")),
		loadbalancerProvider: provider,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(ctx *context.ControllerManagerContext, mgr manager.Manager, r reconcile.Reconciler, rvms *ReconcileVirtualMachineService) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: ctx.MaxConcurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to VirtualMachineService
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachineService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch VirtualMachine resources so that VmServices can be updated in response to changes in VM IP status and VM
	// label configuration.
	//
	// TODO: Ensure that we have adequate tests for these IP and label updates.
	err = c.Watch(&source.Kind{Type: &vmoperatorv1alpha1.VirtualMachine{}},
		&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(rvms.virtualMachineToVirtualMachineServiceMapper)})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}},
		&handler.EnqueueRequestForOwner{OwnerType: &vmoperatorv1alpha1.VirtualMachineService{}, IsController: false})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Endpoints{}},
		&handler.EnqueueRequestForOwner{OwnerType: &vmoperatorv1alpha1.VirtualMachineService{}, IsController: false})
	if err != nil {
		return err
	}

	return nil
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
// +kubebuilder:rbac:groups=vmware.com,resources=loadbalancers;loadbalancers/status,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups=vmware.com,resources=virtualnetworks;virtualnetworks/status,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;create;update;patch;delete

func (r *ReconcileVirtualMachineService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := goctx.Background()

	instance := &vmoperatorv1alpha1.VirtualMachineService{}
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	ctx = goctx.WithValue(ctx, vimtypes.ID{}, "vmoperator-"+instance.Name+"-"+ControllerName)

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instance, finalizerName) {
			if err := r.deleteVmService(ctx, instance); err != nil {
				return reconcile.Result{}, err
			}

			controllerutil.RemoveFinalizer(instance, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return reconcile.Result{}, err
			}
		}

		return reconcile.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(instance, finalizerName) {
		controllerutil.AddFinalizer(instance, finalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	err = r.reconcileVmService(ctx, instance)
	if err != nil {
		if requeueErr, ok := err.(*RequeueAfterError); ok {
			return reconcile.Result{RequeueAfter: requeueErr.GetRequeueAfter()}, nil
		}
		r.log.Error(err, "Failed to reconcile VirtualMachineService", "service", instance)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVirtualMachineService) deleteVmService(ctx goctx.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (err error) {
	r.log.Info("Delete VirtualMachineService", "service", vmService)
	r.recorder.EmitEvent(vmService, OpDelete, err, false)
	return nil
}

func (r *ReconcileVirtualMachineService) reconcileVmService(ctx goctx.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (err error) {
	r.log.Info("Reconcile VirtualMachineService", "name", vmService.NamespacedName())
	defer r.log.Info("Finished Reconcile VirtualMachineService", "name", vmService.NamespacedName())

	if vmService.Spec.Type == vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer {
		// Get virtual network name from vm spec
		virtualNetworkName, err := r.getVirtualNetworkName(ctx, vmService)
		if err != nil {
			r.log.Error(err, "Failed to get virtual network from vm spec", "name", vmService.Name)
			return err
		}
		// Get LoadBalancer to attach
		err = r.loadbalancerProvider.EnsureLoadBalancer(ctx, vmService, virtualNetworkName)
		if err != nil {
			r.log.Error(err, "Failed to create or get load balancer for vm service", "name", vmService.Name)
			return err
		}
		annotations, err := r.loadbalancerProvider.GetVMServiceAnnotations(ctx, vmService)
		if err != nil {
			r.log.Error(err, "Failed to get loadbalancer annotations for vm service", "name", vmService.Name)
			return err
		}
		// Initialize VM Service Annotation when it is nil
		if vmService.Annotations == nil {
			vmService.Annotations = make(map[string]string)
		}
		for k, v := range annotations {
			if oldValue, ok := vmService.Annotations[k]; ok {
				r.log.V(5).Info("Replacing previous annotation value on vm service",
					"vmServiceName", vmService.NamespacedName,
					"key", k,
					"oldValue", oldValue,
					"newValue", v)
			}
			vmService.Annotations[k] = v
		}
	}

	// Reconcile k8s Service
	newService, err := r.createOrUpdateService(ctx, vmService)
	if err != nil {
		r.log.Error(err, "Failed to update k8s Service for VirtualMachineService", "vmServiceName", vmService.NamespacedName)
		return err
	}

	// Update VirtualMachineService endpoints
	err = r.updateEndpoints(ctx, vmService, newService)
	if err != nil {
		r.log.Error(err, "Failed to update VirtualMachineService endpoints", "name", vmService.NamespacedName())
		return err
	}

	// Update VirtualMachineService resource
	err = r.updateVmService(ctx, vmService, newService)
	if err != nil {
		r.log.Error(err, "Failed to update VirtualMachineService Status", "name", vmService.NamespacedName())
		return err
	}

	return nil
}

// For a given VM, determine which vmServices select that VM via label selector and return a set of reconcile requests
// for those selecting VMServices.
func (r *ReconcileVirtualMachineService) virtualMachineToVirtualMachineServiceMapper(o handler.MapObject) []reconcile.Request {
	var reconcileRequests []reconcile.Request

	vm := o.Object.(*vmoperatorv1alpha1.VirtualMachine)
	// Find all vm services that match this vm
	vmServiceList, err := r.getVirtualMachineServicesSelectingVirtualMachine(goctx.Background(), vm)
	if err != nil {
		return reconcileRequests
	}

	for _, vmService := range vmServiceList {
		r.log.V(4).Info("Generating reconcile request for vmService due to event on VMs",
			"VirtualMachineService", types.NamespacedName{Namespace: vmService.Namespace, Name: vmService.Name},
			"VirtualMachine", types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name})
		reconcileRequests = append(reconcileRequests,
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: vmService.Namespace, Name: vmService.Name}})
	}

	return reconcileRequests
}

// MakeObjectMeta returns an ObjectMeta struct with values filled in from input VirtualMachineService. Also sets the VM operator annotations.
func MakeObjectMeta(vmService *vmoperatorv1alpha1.VirtualMachineService) metav1.ObjectMeta {
	om := metav1.ObjectMeta{
		Namespace:   vmService.Namespace,
		Name:        vmService.Name,
		Labels:      vmService.Labels,
		Annotations: vmService.Annotations,
		OwnerReferences: []metav1.OwnerReference{
			{
				UID:                vmService.UID,
				Name:               vmService.Name,
				Controller:         pointer.BoolPtr(true),
				BlockOwnerDeletion: pointer.BoolPtr(true),
				Kind:               ServiceOwnerRefKind,
				APIVersion:         ServiceOwnerRefVersion,
			},
		},
	}
	pkg.AddAnnotations(&om)

	return om
}

func (r *ReconcileVirtualMachineService) makeEndpoints(vmService *vmoperatorv1alpha1.VirtualMachineService, currentEndpoints *corev1.Endpoints, subsets []corev1.EndpointSubset) *corev1.Endpoints {
	newEndpoints := currentEndpoints.DeepCopy()
	newEndpoints.ObjectMeta = MakeObjectMeta(vmService)
	newEndpoints.Subsets = subsets
	return newEndpoints
}

func (r *ReconcileVirtualMachineService) makeEndpointAddress(vm *vmoperatorv1alpha1.VirtualMachine) *corev1.EndpointAddress {
	return &corev1.EndpointAddress{
		IP: vm.Status.VmIp,
		TargetRef: &corev1.ObjectReference{
			APIVersion: vm.APIVersion,
			Kind:       vm.Kind,
			Namespace:  vm.Namespace,
			Name:       vm.Name,
			UID:        vm.UID,
		}}
}

//Get virtual network name from vm spec
func (r *ReconcileVirtualMachineService) getVirtualNetworkName(ctx goctx.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (string, error) {
	r.log.V(5).Info("Get Virtual Network Name", "vmservice", vmService.NamespacedName())
	defer r.log.V(5).Info("Finished Get Virtual Network Name", "vmservice", vmService.NamespacedName())

	vmList := &vmoperatorv1alpha1.VirtualMachineList{}
	err := r.List(ctx, vmList, client.InNamespace(vmService.Namespace), client.MatchingLabels(vmService.Spec.Selector))
	if err != nil {
		return "", err
	}

	return r.loadbalancerProvider.GetNetworkName(vmList.Items, vmService)
}

// vmServiceToService converts a VM Service to k8s Service.
func (r *ReconcileVirtualMachineService) vmServiceToService(vmService *vmoperatorv1alpha1.VirtualMachineService) *corev1.Service {
	servicePorts := make([]corev1.ServicePort, 0, len(vmService.Spec.Ports))
	for _, vmPort := range vmService.Spec.Ports {
		sport := corev1.ServicePort{
			Name:       vmPort.Name,
			Protocol:   corev1.Protocol(vmPort.Protocol),
			Port:       vmPort.Port,
			TargetPort: intstr.FromInt(int(vmPort.TargetPort)),
		}
		servicePorts = append(servicePorts, sport)
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "core/v1",
		},
		ObjectMeta: MakeObjectMeta(vmService),
		Spec: corev1.ServiceSpec{
			// Don't specify selector to keep endpoints controller from interfering
			Type:                     corev1.ServiceType(vmService.Spec.Type),
			Ports:                    servicePorts,
			ExternalName:             vmService.Spec.ExternalName,
			ClusterIP:                vmService.Spec.ClusterIP,
			LoadBalancerIP:           vmService.Spec.LoadBalancerIP,
			LoadBalancerSourceRanges: vmService.Spec.LoadBalancerSourceRanges,
		},
	}

	// When VirtualMachineService is created with annotation
	// labelServiceExternalTrafficPolicyKey, use its value for the
	// Service.Spec.ExternalTrafficPolicy
	if externalTrafficPolicy, ok := svc.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey]; ok {
		switch corev1.ServiceExternalTrafficPolicyType(externalTrafficPolicy) {
		// Only two valid values are accepted
		case corev1.ServiceExternalTrafficPolicyTypeLocal, corev1.ServiceExternalTrafficPolicyTypeCluster:
			{
				svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyType(externalTrafficPolicy)
			}
		default:
			r.log.V(5).Info("Unknown externalTrafficPolicy configured in VirtualMachineService label, skip it", "externalTrafficPolicy", externalTrafficPolicy)
		}
	}
	// When VirtualMachineService is created with annotation
	// labelServiceHealthCheckNodePortKey, use its value for the
	// Service.Spec.HealthCheckNodePort
	if healthCheckNodePortString, ok := svc.Annotations[utils.AnnotationServiceHealthCheckNodePortKey]; ok {
		//nolint:gosec ignore the overflow warning
		healthCheckNodePort, err := strconv.Atoi(healthCheckNodePortString)
		if err != nil {
			r.log.V(5).Info("Invalid healthCheckNodePort configured in VirtualMachineService label, skip it", "healthCheckNodePort", healthCheckNodePortString)
		} else {
			svc.Spec.HealthCheckNodePort = int32(healthCheckNodePort)
		}
	}

	return svc
}

func findPort(vm *vmoperatorv1alpha1.VirtualMachine, portName intstr.IntOrString, portProto corev1.Protocol) (int, error) {
	switch portName.Type {
	case intstr.String:
		name := portName.StrVal
		for _, port := range vm.Spec.Ports {
			if port.Name == name && port.Protocol == portProto {
				return port.Port, nil
			}
		}
	case intstr.Int:
		return portName.IntValue(), nil
	}

	return 0, fmt.Errorf("no suitable port for manifest: %s", vm.UID)
}

func addEndpointSubset(subsets []corev1.EndpointSubset, epa corev1.EndpointAddress, epp *corev1.EndpointPort) []corev1.EndpointSubset {
	var ports []corev1.EndpointPort
	if epp != nil {
		ports = append(ports, *epp)
	}

	subsets = append(subsets,
		corev1.EndpointSubset{
			Addresses: []corev1.EndpointAddress{epa},
			Ports:     ports,
		})

	return subsets
}

// createOrUpdateService reconciles the k8s Service corresponding to the VirtualMachineService by creating a new Service or updating an existing one. Returns the newly created or updated Service.
func (r *ReconcileVirtualMachineService) createOrUpdateService(ctx goctx.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (*corev1.Service, error) {
	// We can use vmService's namespace and name since Service and VirtualMachineService live in the same namespace.
	serviceKey := client.ObjectKey{Name: vmService.Name, Namespace: vmService.Namespace}

	r.log.V(5).Info("Reconciling k8s service", "serviceName", serviceKey)
	defer r.log.V(5).Info("Finished reconciling k8s Service", "serviceName", serviceKey)

	// Find the current Service.
	currentService := &corev1.Service{}
	err := r.Get(ctx, serviceKey, currentService)
	if err != nil {
		if !errors.IsNotFound(err) {
			r.log.Error(err, "Failed to get Service", "name", serviceKey)
			return nil, err
		}

		// Service does not exist, we will create it.
		r.log.V(5).Info("Service not found. Will attempt to create it", "name", serviceKey)
		currentService = r.vmServiceToService(vmService)
	}

	// Determine if VirtualMachineService needs any update by comparing current k8s to newService synthesized from
	// VirtualMachineService
	newService := currentService.DeepCopy()
	svcFromVmService := r.vmServiceToService(vmService)

	// Merge labels of the Service with the VirtualMachineService. VirtualMachineService wins in case of conflicts.
	// We can't just clobber the labels since other operators (Net Operator) might rely on them.
	if newService.Labels == nil {
		newService.Labels = vmService.Labels
	} else {
		for k, v := range vmService.Labels {
			if oldValue, ok := newService.Labels[k]; ok {
				r.log.V(5).Info("Replacing previous label value on service",
					"service", newService.Name,
					"namespace", newService.Namespace,
					"key", k,
					"oldValue", oldValue,
					"newValue", v)
			}
			newService.Labels[k] = v
		}
	}

	newService.Spec.Type = svcFromVmService.Spec.Type
	newService.Spec.ExternalName = svcFromVmService.Spec.ExternalName
	newService.Spec.Ports = svcFromVmService.Spec.Ports
	newService.Spec.LoadBalancerIP = svcFromVmService.Spec.LoadBalancerIP
	if svcFromVmService.Spec.LoadBalancerSourceRanges != nil {
		newService.Spec.LoadBalancerSourceRanges = svcFromVmService.Spec.LoadBalancerSourceRanges
	}
	if svcFromVmService.Spec.ExternalTrafficPolicy != "" {
		newService.Spec.ExternalTrafficPolicy = svcFromVmService.Spec.ExternalTrafficPolicy
	}
	if svcFromVmService.Spec.HealthCheckNodePort != 0 {
		newService.Spec.HealthCheckNodePort = svcFromVmService.Spec.HealthCheckNodePort
	}

	// Maintain the existing mapping of ServicePort -> NodePort
	// as un-setting it will cause the apiserver to allocate a
	// new NodePort on an Update.
	populateNodePorts(currentService, newService)

	// Add VM operator annotations.
	pkg.AddAnnotations(&newService.ObjectMeta)

	// Create or update or don't update Service.
	createService := len(currentService.ResourceVersion) == 0
	if createService {
		r.log.Info("Creating k8s Service", "name", serviceKey, "service", newService)
		err = r.Create(ctx, newService)
		r.recorder.EmitEvent(vmService, OpCreate, err, false)
	} else if !apiequality.Semantic.DeepEqual(currentService, newService) {
		r.log.Info("Updating k8s Service", "name", serviceKey, "service", newService)
		err = r.Update(ctx, newService)
		r.recorder.EmitEvent(vmService, OpUpdate, err, false)
	} else {
		r.log.V(5).Info("No need to update current K8s Service. Skipping Update",
			"vmServiceName", vmService.NamespacedName(), "currentService", currentService, "vmService", vmService)
	}

	return newService, err
}

func (r *ReconcileVirtualMachineService) getVirtualMachinesSelectedByVmService(ctx goctx.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (*vmoperatorv1alpha1.VirtualMachineList, error) {
	vmList := &vmoperatorv1alpha1.VirtualMachineList{}
	err := r.List(ctx, vmList, client.InNamespace(vmService.Namespace), client.MatchingLabels(vmService.Spec.Selector))
	return vmList, err
}

// TODO: This mapping function has the potential to be a performance and scaling issue.  Consider this as a candidate for profiling
func (r *ReconcileVirtualMachineService) getVirtualMachineServicesSelectingVirtualMachine(ctx goctx.Context, lookupVm *vmoperatorv1alpha1.VirtualMachine) ([]*vmoperatorv1alpha1.VirtualMachineService, error) {
	var matchingVmServices []*vmoperatorv1alpha1.VirtualMachineService

	matchFunc := func(vmService *vmoperatorv1alpha1.VirtualMachineService) error {
		vmList, err := r.getVirtualMachinesSelectedByVmService(ctx, vmService)
		if err != nil {
			return err
		}

		lookupVmKey := types.NamespacedName{Namespace: lookupVm.Namespace, Name: lookupVm.Name}
		for _, vm := range vmList.Items {
			vmKey := types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}

			if vmKey == lookupVmKey {
				matchingVmServices = append(matchingVmServices, vmService)
				// Only one match is needed to add vmService, so return now.
				return nil
			}
		}

		return nil
	}

	vmServiceList := &vmoperatorv1alpha1.VirtualMachineServiceList{}
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

// updateEndpoints updates the service endpoints for VirtaulMachineService. This is done by running the readiness probe against all the VMs and adding the successful VMs to the endpoint.
func (r *ReconcileVirtualMachineService) updateEndpoints(ctx goctx.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, service *corev1.Service) error {
	logger := r.log.WithValues("vmServiceName", vmService.NamespacedName())
	logger.V(5).Info("Updating VirtualMachineService endpoints")
	defer logger.V(5).Info("Finished updating VirtualMachineService endpoints")

	vmList, err := r.getVirtualMachinesSelectedByVmService(ctx, vmService)
	if err != nil {
		return err
	}

	var probeFailureCount int
	var updateErr error
	var subsets []corev1.EndpointSubset

	for i := range vmList.Items {
		vm := vmList.Items[i]
		logger := logger.WithValues("virtualmachine", vm.NamespacedName())
		logger.V(5).Info("Resolving ports for VirtualMachine")
		// Skip VM's marked for deletions
		if vm.DeletionTimestamp != nil {
			logger.Info("Skipping VM marked for deletion")
			continue
		}
		// Handle multiple VM interfaces
		if len(vm.Status.VmIp) == 0 {
			logger.Info("Failed to find an IP for VirtualMachine")
			continue
		}
		// Ignore if all required values aren't present
		if vm.Status.Host == "" {
			logger.Info("Skipping VirtualMachine due to empty host")
			continue
		}

		// Ignore VM's that fail the readiness check (only when probes are specified)
		// TODO: Move this out of the controller into a runnable that periodically probes a VM and manages the endpoints
		// out-of-band from the controller. We currently rely on the controller's periodic sync to invoke the readiness
		// probe.
		if err := runProbe(vmService, &vm, vm.Spec.ReadinessProbe); err != nil {
			logger.Info("Skipping VirtualMachine due to failed readiness probe check", "probeError", err)
			probeFailureCount++
			continue
		}

		epa := *r.makeEndpointAddress(&vm)

		// TODO: Headless support
		for _, servicePort := range service.Spec.Ports {
			portName := servicePort.Name
			portProto := servicePort.Protocol

			logger.V(5).Info("EndpointPort for VirtualMachine",
				"port name", portName, "port proto", portProto)

			portNum, err := findPort(&vm, servicePort.TargetPort, portProto)
			if err != nil {
				logger.Info("Failed to find port for service",
					"name", portName, "protocol", portProto, "error", err)
				continue
			}

			epp := &corev1.EndpointPort{Name: portName, Port: int32(portNum), Protocol: portProto}
			subsets = addEndpointSubset(subsets, epa, epp)
		}
	}

	// Repack subsets in canonical order so that comparison of two subsets gives consistent results.
	subsets = endpointsv1.RepackSubsets(subsets)

	// Until is fixed, if probe fails on all selected VM's, we will aggressively requeue until the probe
	// succeeds on one of them. Note: We don't immediately requeue to allow for updating the endpoint subsets.
	if probeFailureCount > 0 && probeFailureCount == len(vmList.Items) {
		updateErr = &RequeueAfterError{RequeueAfter: probeFailureRequeueTime}
	}

	// See if there's actually an update here.
	currentEndpoints := &corev1.Endpoints{}
	err = r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, currentEndpoints)
	if err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to list endpoints")
			return err
		}

		currentEndpoints = &corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:   service.Name,
				Labels: service.Labels,
			},
		}
	} else if apiequality.Semantic.DeepEqual(endpointsv1.RepackSubsets(currentEndpoints.Subsets), subsets) {
		// Ideally, we dont have to repack both the endpoints (current endpoints and the calculated one) before comparing since after the first update
		// we expect the endpoint subsets to always be in canonical order. However, some controller is re-ordering these endpoint subsets. We sort both
		// sides for a consistent comparison result. PR: 2623292

		logger.V(5).Info("No change, no need to update endpoints", "endpoints", currentEndpoints)
		return updateErr
	}

	newEndpoints := r.makeEndpoints(vmService, currentEndpoints, subsets)

	createEndpoints := len(currentEndpoints.ResourceVersion) == 0

	logger.V(5).Info("EndpointSubsets have diverged. Will trigger Endpoint Create or Update", "currentEndpoints", currentEndpoints, "newEndpoints", newEndpoints, "isCreate", createEndpoints)

	if createEndpoints {
		logger.Info("Creating service endpoints", "endpoints", newEndpoints)
		err = r.Create(ctx, newEndpoints)
	} else {
		logger.Info("Updating service endpoints", "endpoints", newEndpoints)
		err = r.Update(ctx, newEndpoints)
	}

	if err != nil {
		if createEndpoints && errors.IsForbidden(err) {
			// A request is forbidden primarily for two reasons:
			// 1. namespace is terminating, endpoint creation is not allowed by default.
			// 2. policy is mis-configured, in which case no service would function anywhere.
			// Given the frequency of 1, we logger at a lower level.
			logger.V(5).Info("Forbidden from creating endpoints", "error", err.Error())
		}
		return err
	}
	return updateErr
}

// updateVmServiceStatus updates the VirtualMachineService status, syncs external IP for loadbalancer type of service. Also ensures that VirtualMachineService contains the VM operator annotations.
func (r *ReconcileVirtualMachineService) updateVmService(ctx goctx.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, newService *corev1.Service) error {
	r.log.V(5).Info("Updating VirtualMachineService Status", "name", vmService.NamespacedName())
	defer r.log.V(5).Info("Finished updating VirtualMachineService Status", "name", vmService.NamespacedName())

	newVMService := vmService.DeepCopy()

	// Update the load balancer's external IP.
	if newVMService.Spec.Type == vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer {
		//copy service ingress array to vm service ingress array
		newVMService.Status.LoadBalancer.Ingress = make([]vmoperatorv1alpha1.LoadBalancerIngress, len(newService.Status.LoadBalancer.Ingress))
		for idx, ingress := range newService.Status.LoadBalancer.Ingress {
			vmIngress := vmoperatorv1alpha1.LoadBalancerIngress{
				IP:       ingress.IP,
				Hostname: ingress.Hostname,
			}
			newVMService.Status.LoadBalancer.Ingress[idx] = vmIngress
		}
	}

	// We need to compare Status subresource separately since updating the object will not update the subresource itself.
	if !apiequality.Semantic.DeepEqual(vmService.Status, newVMService.Status) {
		r.log.V(5).Info("Updating the vmService status", "vmService", newVMService)
		if err := r.Status().Update(ctx, newVMService); err != nil {
			r.log.Error(err, "Error in updating the vmService status", "vmService", newVMService)
			return err
		}
	}

	pkg.AddAnnotations(&newVMService.ObjectMeta)

	// Update the resource to reflect the Annotations.
	if !apiequality.Semantic.DeepEqual(vmService.ObjectMeta, newVMService.ObjectMeta) {
		r.log.V(5).Info("Updating the vmService resource", "vmService", newVMService)
		if err := r.Update(ctx, newVMService); err != nil {
			r.log.Error(err, "Error in updating vmService resource", "vmService", newVMService)
			return err
		}
	}

	return nil
}

func runProbe(vmService *vmoperatorv1alpha1.VirtualMachineService, vm *vmoperatorv1alpha1.VirtualMachine, p *vmoperatorv1alpha1.Probe) error {
	var log = logf.Log.WithName(ControllerName)

	logger := log.WithValues("serviceName", vmService.NamespacedName(), "vm", vm.NamespacedName())
	if p == nil {
		logger.V(5).Info("Readiness probe not specified")
		return nil
	}
	if p.TCPSocket != nil {
		portProto := corev1.ProtocolTCP
		portNum, err := findPort(vm, p.TCPSocket.Port, portProto)
		if err != nil {
			return err
		}

		var host string
		if p.TCPSocket.Host != "" {
			host = p.TCPSocket.Host
		} else {
			logger.V(5).Info("TCPSocket Host not specified, using VM IP")
			host = vm.Status.VmIp
		}

		var timeout time.Duration
		if p.TimeoutSeconds <= 0 {
			timeout = defaultConnectTimeout
		} else {
			timeout = time.Duration(p.TimeoutSeconds) * time.Second
		}

		if err := checkConnection("tcp", host, strconv.Itoa(portNum), timeout); err != nil {
			return err
		}
		logger.V(5).Info("Readiness probe succeeded")
		return nil
	}
	return fmt.Errorf("unknown action specified for probe in VirtualMachine %s", vm.NamespacedName())
}

func checkConnection(proto, host, port string, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout(proto, address, timeout)
	if err != nil {
		return err
	}
	if err := conn.Close(); err != nil {
		return err
	}
	return nil
}

func populateNodePorts(oldService *corev1.Service, targetService *corev1.Service) {
	nodePortMap := map[string]int32{}
	for _, port := range oldService.Spec.Ports {
		nodePortMap[port.Name] = port.NodePort
	}
	for i := 0; i < len(targetService.Spec.Ports); i++ {
		if val, ok := nodePortMap[targetService.Spec.Ports[i].Name]; ok {
			targetService.Spec.Ports[i].NodePort = val
		}
	}
}
