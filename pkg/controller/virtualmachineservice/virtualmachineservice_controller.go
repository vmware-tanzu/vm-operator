/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common/record"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachineservice/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

const (
	// TODO(bryanv) Get these from mgr.GetScheme().ObjectKinds(...) to exactly match EnqueueRequestForOwner behavior.
	ServiceOwnerRefKind    = "VirtualMachineService"
	ServiceOwnerRefVersion = "vmoperator.vmware.com/v1alpha1"

	OpCreate       = "CreateVMService"
	OpDelete       = "DeleteVMService"
	OpUpdate       = "UpdateVMService"
	ControllerName = "virtualmachineservice-controller"

	defaultConnectTimeout = time.Second * 10
)

var log = logf.Log.WithName(ControllerName)

// Add creates a new VirtualMachineService Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := newReconciler(mgr)
	rvms := r.(*ReconcileVirtualMachineService)
	return add(mgr, r, rvms)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileVirtualMachineService{
		Client:               mgr.GetClient(),
		scheme:               mgr.GetScheme(),
		loadbalancerProvider: providers.GetLoadbalancerProviderByType(mgr.GetConfig(), providers.NSXTLoadBalancer),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, rvms *ReconcileVirtualMachineService) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: common.GetMaxReconcileNum()})
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
	scheme               *runtime.Scheme
	loadbalancerProvider providers.LoadbalancerProvider
}

// Reconcile reads that state of the cluster for a VirtualMachineService object and makes changes based on the state read
// and what is in the VirtualMachineService.Spec
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachineservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmware.com,resources=loadbalancers;loadbalancers/status,verbs=create;get;list;patch;delete;watch;update
// +kubebuilder:rbac:groups=vmware.com,resources=virtualnetworks;virtualnetworks/status,verbs=create;get;list;patch;delete;watch;update
func (r *ReconcileVirtualMachineService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()

	// Fetch the VirtualMachineService instance
	instance := &vmoperatorv1alpha1.VirtualMachineService{}
	err := r.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	ctx = context.WithValue(ctx, vimtypes.ID{}, "vmoperator-"+instance.Name+"-"+ControllerName)

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		const finalizerName = vmoperator.VirtualMachineServiceFinalizer

		if lib.ContainsString(instance.ObjectMeta.Finalizers, finalizerName) {
			if err := r.deleteVmService(ctx, instance); err != nil {
				// return with error so that it can be retried
				return reconcile.Result{}, err
			}

			// remove our finalizer from the list and update it.
			instance.ObjectMeta.Finalizers = lib.RemoveString(instance.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Our finalizer has finished, so the reconciler can do nothing.
		return reconcile.Result{}, nil
	}

	err = r.reconcileVmService(ctx, instance)
	if err != nil {
		log.Error(err, "Failed to reconcile VirtualMachineService", "service", instance)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileVirtualMachineService) deleteVmService(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (err error) {
	log.Info("Delete VirtualMachineService", "service", vmService)
	defer record.EmitEvent(vmService, OpDelete, &err, false)

	return nil
}

func (r *ReconcileVirtualMachineService) reconcileVmService(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (err error) {
	log.Info("Reconcile VirtualMachineService", "name", vmService.NamespacedName())
	defer log.Info("Finished Reconcile VirtualMachineService", "name", vmService.NamespacedName())

	loadBalancerName := ""
	if vmService.Spec.Type == vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer {
		if r.loadbalancerProvider == nil {
			log.Error(err, "Failed to get Load balancer provider for vm service", "name", vmService.Name)
			return err
		}
		//Get virtual network name from vm spec
		virtualNetworkName, err := r.getVirtualNetworkName(ctx, vmService)
		if err != nil {
			log.Error(err, "Failed to get virtual network from vm spec", "name", vmService.Name)
			return err
		}
		// Get LoadBalancer to attach
		loadBalancerName, err = r.loadbalancerProvider.EnsureLoadBalancer(ctx, vmService, virtualNetworkName)
		if err != nil {
			log.Error(err, "Failed to create or get load balancer for vm service", "name", vmService.Name)
			return err
		}
	}

	// Translate vm service to service
	service := r.vmServiceToService(vmService)
	log.V(5).Info("Translate VM Service to K8S Service", "k8s service", service)
	// Update k8s Service
	newService, err := r.createOrUpdateService(ctx, vmService, service, loadBalancerName)
	if err != nil {
		log.Error(err, "Failed to update k8s services", "k8s services", service)
		return err
	}
	// Update endpoints
	err = r.updateEndpoints(ctx, vmService, newService)
	if err != nil {
		log.Error(err, "Failed to update VirtualMachineService endpoints", "name", vmService.NamespacedName())
		return err
	}
	// Update vm service
	newVMService, err := r.updateVmServiceStatus(ctx, vmService, newService)
	log.V(5).Info("Updated new vm services", "new vm service", newVMService)
	return err
}

// For a given VM, determine which vmServices select that VM via label selector and return a set of reconcile requests
// for those selecting VMServices.
func (r *ReconcileVirtualMachineService) virtualMachineToVirtualMachineServiceMapper(o handler.MapObject) []reconcile.Request {
	var reconcileRequests []reconcile.Request

	vm := o.Object.(*vmoperatorv1alpha1.VirtualMachine)
	// Find all vm services that match this vm
	vmServiceList, err := r.getVirtualMachineServicesSelectingVirtualMachine(context.Background(), vm)
	if err != nil {
		return reconcileRequests
	}

	for _, vmService := range vmServiceList {
		log.V(4).Info("Generating reconcile request for vmService due to event on VMs",
			"VirtualMachineService", types.NamespacedName{Namespace: vmService.Namespace, Name: vmService.Name},
			"VirtualMachine", types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name})
		reconcileRequests = append(reconcileRequests,
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: vmService.Namespace, Name: vmService.Name}})
	}

	return reconcileRequests
}

func (r *ReconcileVirtualMachineService) makeObjectMeta(vmService *vmoperatorv1alpha1.VirtualMachineService) *metav1.ObjectMeta {
	t := true
	om := &metav1.ObjectMeta{
		Namespace:   vmService.Namespace,
		Name:        vmService.Name,
		Labels:      vmService.Labels,
		Annotations: vmService.Annotations,
		OwnerReferences: []metav1.OwnerReference{
			{
				UID:                vmService.UID,
				Name:               vmService.Name,
				Controller:         &t,
				BlockOwnerDeletion: &t,
				Kind:               ServiceOwnerRefKind,
				APIVersion:         ServiceOwnerRefVersion,
			},
		},
	}
	pkg.AddAnnotations(om)

	return om
}

func (r *ReconcileVirtualMachineService) makeEndpoints(vmService *vmoperatorv1alpha1.VirtualMachineService, currentEndpoints *corev1.Endpoints, subsets []corev1.EndpointSubset) *corev1.Endpoints {
	newEndpoints := currentEndpoints.DeepCopy()
	newEndpoints.ObjectMeta = *r.makeObjectMeta(vmService)
	newEndpoints.Subsets = subsets
	return newEndpoints
}

func (r *ReconcileVirtualMachineService) makeEndpointAddress(vmService *vmoperatorv1alpha1.VirtualMachineService, vm *vmoperatorv1alpha1.VirtualMachine) *corev1.EndpointAddress {
	return &corev1.EndpointAddress{
		IP:       vm.Status.VmIp,
		NodeName: &vm.Status.Host,
		TargetRef: &corev1.ObjectReference{
			Kind:            vmService.Kind,
			Namespace:       vmService.Namespace,
			Name:            vmService.Name,
			UID:             vmService.UID,
			ResourceVersion: vmService.ResourceVersion,
		}}
}

//Get virtual network name from vm spec
func (r *ReconcileVirtualMachineService) getVirtualNetworkName(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (string, error) {
	log.V(5).Info("Get Virtual Network Name", "vmservice", vmService.NamespacedName())
	defer log.V(5).Info("Finished Get Virtual Network Name", "vmservice", vmService.NamespacedName())

	vmList := &vmoperatorv1alpha1.VirtualMachineList{}
	listOptions := (&client.ListOptions{}).InNamespace(vmService.Namespace).MatchingLabels(vmService.Spec.Selector)
	err := r.List(ctx, listOptions, vmList)
	if err != nil {
		return "", err
	}

	return r.loadbalancerProvider.GetNetworkName(vmList.Items, vmService)
}

//Convert vm service to k8s service
func (r *ReconcileVirtualMachineService) vmServiceToService(vmService *vmoperatorv1alpha1.VirtualMachineService) *corev1.Service {
	om := r.makeObjectMeta(vmService)

	servicePorts := make([]corev1.ServicePort, 0, len(vmService.Spec.Ports))
	for _, vmPort := range vmService.Spec.Ports {
		sport := corev1.ServicePort{
			// No node port field in vm service, cant't translate that one
			Name:       vmPort.Name,
			Protocol:   corev1.Protocol(vmPort.Protocol),
			Port:       vmPort.Port,
			TargetPort: intstr.FromInt(int(vmPort.TargetPort)),
		}
		servicePorts = append(servicePorts, sport)
	}

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "core/v1",
		},
		ObjectMeta: *om,
		Spec: corev1.ServiceSpec{
			// Don't specify selector to keep endpoints controller from interfering
			Type:         corev1.ServiceType(vmService.Spec.Type),
			Ports:        servicePorts,
			ExternalName: vmService.Spec.ExternalName,
			ClusterIP:    vmService.Spec.ClusterIP,
		},
	}
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

func addEndpointSubset(subsets []corev1.EndpointSubset, vm *vmoperatorv1alpha1.VirtualMachine, epa corev1.EndpointAddress, epp *corev1.EndpointPort) []corev1.EndpointSubset {
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

// Create or update k8s service
func (r *ReconcileVirtualMachineService) createOrUpdateService(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, service *corev1.Service, loadBalancerName string) (*corev1.Service, error) {
	serviceKey := client.ObjectKey{Name: service.Name, Namespace: service.Namespace}

	log.V(5).Info("Updating k8s service", "k8s service name", serviceKey)
	defer log.V(5).Info("Finished updating k8s service", "k8s service name", serviceKey)

	// find current service
	currentService := &corev1.Service{}
	err := r.Get(ctx, serviceKey, currentService)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get service", "name", serviceKey)
			return nil, err
		}
		// not exist, need to create one
		log.V(5).Info("No k8s service in this name, creating one", "name", serviceKey)
		currentService = service
	} else {
		ok, err := utils.VMServiceCompareToLastApplied(vmService, currentService.GetAnnotations()[corev1.LastAppliedConfigAnnotation])
		if err != nil {
			log.V(5).Info("Unmarshal last applied service config failed, no last applied vm svc configuration ", "current service", currentService)
		}
		if ok {
			log.V(5).Info("No change, no need to update service", "name", vmService.NamespacedName(), "service", service)
			return currentService, nil
		}
	}

	// just update possible changed service fields
	newService := currentService.DeepCopy()
	newService.Labels = service.Labels
	newService.Spec.Type = service.Spec.Type
	newService.Spec.ExternalName = service.Spec.ExternalName

	newService.Spec.Ports = service.Spec.Ports
	// can't simply copy everything, need to keep node port unchanged
	portNodePortMap := make(map[int32]int32)
	for _, servicePort := range currentService.Spec.Ports {
		portNodePortMap[servicePort.Port] = servicePort.NodePort
	}
	for idx, servicePort := range newService.Spec.Ports {
		if nodePort, ok := portNodePortMap[servicePort.Port]; ok {
			newService.Spec.Ports[idx].NodePort = nodePort
		}
	}

	pkg.AddAnnotations(&newService.ObjectMeta)

	// create or update service
	createService := len(currentService.ResourceVersion) == 0
	if createService {
		//every time create a load balancer type vm service, need to append this vm service to load balancer owner reference list
		if vmService.Spec.Type == vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer {
			err = r.loadbalancerProvider.UpdateLoadBalancerOwnerReference(ctx, loadBalancerName, vmService)
			if err != nil {
				log.Error(err, "Update LoadBalancer Owner Reference Error", "load balancer", loadBalancerName)
				return nil, err
			}
		}
		log.Info("Creating k8s service", "name", serviceKey, "service", newService)
		err = r.Create(ctx, newService)
		defer record.EmitEvent(vmService, OpCreate, &err, false)
	} else {
		log.Info("Updating k8s service", "name", serviceKey, "service", newService)
		err = r.Update(ctx, newService)
		//defer record.EmitEvent(vmService, OpUpdate, &err, false) ???
	}

	return newService, err
}

func (r *ReconcileVirtualMachineService) getVirtualMachinesSelectedByVmService(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (*vmoperatorv1alpha1.VirtualMachineList, error) {
	vmList := &vmoperatorv1alpha1.VirtualMachineList{}
	listOptions := (&client.ListOptions{}).MatchingLabels(vmService.Spec.Selector).InNamespace(vmService.Namespace)
	err := r.List(ctx, listOptions, vmList)
	return vmList, err
}

// TODO: This mapping function has the potential to be a performance and scaling issue.  Consider this as a candidate for profiling
func (r *ReconcileVirtualMachineService) getVirtualMachineServicesSelectingVirtualMachine(ctx context.Context, lookupVm *vmoperatorv1alpha1.VirtualMachine) ([]*vmoperatorv1alpha1.VirtualMachineService, error) {

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
	err := r.List(ctx, client.InNamespace(lookupVm.Namespace), vmServiceList)
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

func (r *ReconcileVirtualMachineService) updateEndpoints(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, service *corev1.Service) error {
	logger := log.WithValues("serviceName", vmService.NamespacedName())
	logger.V(5).Info("Updating VirtualMachineService endpoints")
	defer logger.V(5).Info("Finished updating VirtualMachineService endpoints")

	vmList, err := r.getVirtualMachinesSelectedByVmService(ctx, vmService)
	if err != nil {
		return err
	}

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
			continue
		}

		epa := *r.makeEndpointAddress(vmService, &vm)

		// TODO: Headless support
		for i := range service.Spec.Ports {
			servicePort := &service.Spec.Ports[i]
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
			subsets = addEndpointSubset(subsets, &vm, epa, epp)
		}
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
	} else if apiequality.Semantic.DeepEqual(currentEndpoints.Subsets, subsets) {
		logger.V(5).Info("No change, no need to update endpoints", "endpoints", currentEndpoints)
		return nil
	}
	newEndpoints := r.makeEndpoints(vmService, currentEndpoints, subsets)

	createEndpoints := len(currentEndpoints.ResourceVersion) == 0
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
	return nil
}

// updateVmServiceStatus update vmservice status, sync external ip for loadbalancer type of service
func (r *ReconcileVirtualMachineService) updateVmServiceStatus(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, newService *corev1.Service) (*vmoperatorv1alpha1.VirtualMachineService, error) {
	log.V(5).Info("Updating VirtualMachineService", "name", vmService.NamespacedName())
	defer log.V(5).Info("Finished updating VirtualMachineService", "name", vmService.NamespacedName())
	// if could update loadbalancer external IP
	if vmService.Spec.Type == vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer && len(newService.Status.LoadBalancer.Ingress) > 0 {
		vmServiceStatusStr, _ := json.Marshal(vmService.Status)
		serviceStatusStr, _ := json.Marshal(newService.Status)
		if string(vmServiceStatusStr) != string(serviceStatusStr) {
			//copy service ingress array to vm service ingress array
			vmService.Status.LoadBalancer.Ingress = make([]vmoperatorv1alpha1.LoadBalancerIngress, len(newService.Status.LoadBalancer.Ingress))
			for idx, ingress := range newService.Status.LoadBalancer.Ingress {
				vmIngress := vmoperatorv1alpha1.LoadBalancerIngress{
					IP:       ingress.IP,
					Hostname: ingress.Hostname,
				}
				vmService.Status.LoadBalancer.Ingress[idx] = vmIngress
			}
			if err := r.Status().Update(ctx, vmService); err != nil {
				log.Error(err, "Failed to update VirtualMachineService Status", "name", vmService.NamespacedName())
				return nil, err
			}
		}
	}
	// BMV: newVMService isn't saved after this point so annotations are missing
	pkg.AddAnnotations(&vmService.ObjectMeta)
	return vmService, nil
}

func runProbe(vmService *vmoperatorv1alpha1.VirtualMachineService, vm *vmoperatorv1alpha1.VirtualMachine, p *vmoperatorv1alpha1.Probe) error {
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
		if p.TimeoutSeconds == 0 {
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
