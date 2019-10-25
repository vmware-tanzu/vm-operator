/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package virtualmachineservice

import (
	"context"
	"fmt"

	"github.com/vmware-tanzu/vm-operator/pkg/controller/virtualmachineservice/providers"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/controller/common"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	corev1 "k8s.io/api/core/v1"
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

	"github.com/vmware-tanzu/vm-operator/pkg/controller/common/record"
)

const (
	// TODO(bryanv) Get these from mgr.GetScheme().ObjectKinds(...) to exactly match EnqueueRequestForOwner behavior.
	ServiceOwnerRefKind    = "VirtualMachineService"
	ServiceOwnerRefVersion = "vmoperator.vmware.com/v1alpha1"

	OpCreate       = "CreateVMService"
	OpDelete       = "DeleteVMService"
	OpUpdate       = "UpdateVMService"
	ControllerName = "virtualmachineservice-controller"
)

var log = logf.Log.WithName(ControllerName)

// Add creates a new VirtualMachineService Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileVirtualMachineService{
		Client:               mgr.GetClient(),
		scheme:               mgr.GetScheme(),
		loadbalancerProvider: providers.GetLoadbalancerProviderByType(providers.NSXTLoadBalancer),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
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
	log.V(4).Info("Translate VM Service to K8S Service", "k8s service", service)
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
	newVMService, err := r.updateVmService(ctx, vmService, newService)
	log.Info("Updated new vm services", "new vm service", newVMService)
	return err
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
	log.V(4).Info("Get Virtual Network Name", "vmservice", vmService.NamespacedName())
	defer log.V(4).Info("Finished Get Virtual Network Name", "vmservice", vmService.NamespacedName())

	vmList := &vmoperatorv1alpha1.VirtualMachineList{}
	err := r.List(ctx, client.MatchingLabels(vmService.Spec.Selector), vmList)
	if err != nil {
		return "", err
	}

	return r.loadbalancerProvider.GetNetworkName(vmList.Items, vmService)
}

//Convert vm service to k8s service
func (r *ReconcileVirtualMachineService) vmServiceToService(vmService *vmoperatorv1alpha1.VirtualMachineService) *corev1.Service {
	om := r.makeObjectMeta(vmService)

	var servicePorts []corev1.ServicePort
	for _, vmPort := range vmService.Spec.Ports {
		sport := corev1.ServicePort{
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
			Type:  corev1.ServiceType(vmService.Spec.Type),
			Ports: servicePorts,
		},
	}
}

func findPort(vm *vmoperatorv1alpha1.VirtualMachine, svcPort *corev1.ServicePort) (int, error) {
	portName := svcPort.TargetPort
	switch portName.Type {
	case intstr.String:
		name := portName.StrVal
		for _, port := range vm.Spec.Ports {
			if port.Name == name && port.Protocol == svcPort.Protocol {
				return int(port.Port), nil
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

	log.Info("Updating k8s service", "k8s service name", serviceKey)
	defer log.Info("Finished updating k8s service", "k8s service name", serviceKey)

	// find current service
	currentService := &corev1.Service{}
	err := r.Get(ctx, serviceKey, currentService)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get service", "name", serviceKey)
			return nil, err
		}
		// not exist, need to create one
		log.Info("No k8s service in this name, creating one", "name", serviceKey)
		currentService = service
	}

	// update service labels and ports
	newService := currentService.DeepCopy()
	newService.Labels = service.Labels
	newService.Spec.Ports = service.Spec.Ports
	pkg.AddAnnotations(&newService.ObjectMeta)

	// create or update service
	createService := len(currentService.ResourceVersion) == 0
	if createService {
		log.Info("Creating k8s service", "name", serviceKey, "service", newService)
		//every time create a load balancer type vm service, need to append this vm service to load balancer owner reference list
		if vmService.Spec.Type == vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer {
			err = r.loadbalancerProvider.UpdateLoadBalancerOwnerReference(ctx, loadBalancerName, vmService)
			if err != nil {
				log.Error(err, "Update LoadBalancer Owner Reference Error", "load balancer", loadBalancerName)
				return nil, err
			}
		}
		err = r.Create(ctx, newService)
		defer record.EmitEvent(vmService, OpCreate, &err, false)
	} else {
		log.Info("Updating k8s service", "name", serviceKey, "service", newService)
		err = r.Update(ctx, newService)
		//defer record.EmitEvent(vmService, OpUpdate, &err, false) ???
	}

	return newService, err
}

func (r *ReconcileVirtualMachineService) getVMServiceSelectedVirtualMachines(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (*vmoperatorv1alpha1.VirtualMachineList, error) {
	vmList := &vmoperatorv1alpha1.VirtualMachineList{}
	err := r.List(ctx, client.MatchingLabels(vmService.Spec.Selector), vmList)
	return vmList, err
}

func (r *ReconcileVirtualMachineService) updateEndpoints(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, service *corev1.Service) error {
	log.Info("Updating VirtualMachineService endpoints", "name", vmService.NamespacedName())
	defer log.Info("Finished updating VirtualMachineService endpoints", "name", vmService.NamespacedName())

	vmList, err := r.getVMServiceSelectedVirtualMachines(ctx, vmService)
	if err != nil {
		return err
	}

	// Determine if any endpoints match
	var subsets []corev1.EndpointSubset

	for _, vm := range vmList.Items {
		log.Info("Resolving ports for VirtualMachine", "name", vm.NamespacedName())
		// Handle multiple VM interfaces
		if len(vm.Status.VmIp) == 0 {
			log.Info("Failed to find an IP for VirtualMachine", "name", vm.NamespacedName())
			continue
		}
		// Ignore if all required values aren't present
		if vm.Status.Host == "" {
			log.Info("Skipping VirtualMachine due to empty host", "name", vm.NamespacedName())
			continue
		}

		epa := *r.makeEndpointAddress(vmService, &vm)

		// TODO: Headless support
		for i := range service.Spec.Ports {
			servicePort := &service.Spec.Ports[i]
			portName := servicePort.Name
			portProto := servicePort.Protocol

			log.Info("VirtualMachine port", "name", vm.NamespacedName(),
				"port name", portName, "port proto", portProto)

			portNum, err := findPort(&vm, servicePort)
			if err != nil {
				log.Error(err, "Failed to find port for service",
					"namespace", service.Namespace, "name", service.Name)
				continue
			}

			epp := &corev1.EndpointPort{Name: portName, Port: int32(portNum), Protocol: portProto}
			subsets = addEndpointSubset(subsets, &vm, epa, epp)
		}

		// See if there's actually an update here.
		currentEndpoints := &corev1.Endpoints{}
		err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, currentEndpoints)
		if err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to list services")
				return err
			}

			currentEndpoints = &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:   service.Name,
					Labels: service.Labels,
				},
			}
		}

		newEndpoints := r.makeEndpoints(vmService, currentEndpoints, subsets)

		createEndpoints := len(currentEndpoints.ResourceVersion) == 0
		if createEndpoints {
			log.Info("Creating service endpoints",
				"service name", vmService.NamespacedName(), "endpoints", newEndpoints)
			err = r.Create(ctx, newEndpoints)
		} else {
			log.Info("Updating service endpoints", "service name", vmService.NamespacedName(), "endpoints", newEndpoints)
			err = r.Update(ctx, newEndpoints)
		}

		if err != nil {
			if createEndpoints && errors.IsForbidden(err) {
				// A request is forbidden primarily for two reasons:
				// 1. namespace is terminating, endpoint creation is not allowed by default.
				// 2. policy is mis-configured, in which case no service would function anywhere.
				// Given the frequency of 1, we log at a lower level.
				log.V(5).Info("Forbidden from creating endpoints", "error", err.Error())
			}
			return err
		}
	}

	return nil
}

// Create or update vm service
func (r *ReconcileVirtualMachineService) updateVmService(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, newService *corev1.Service) (*vmoperatorv1alpha1.VirtualMachineService, error) {
	log.Info("Updating VirtualMachineService", "name", vmService.NamespacedName())
	defer log.Info("Finished updating VirtualMachineService", "name", vmService.NamespacedName())

	// find current vm service
	// BMV: vmService is from r.Get() at the start of Reconcile so needed to do it again?
	currentVMService := &vmoperatorv1alpha1.VirtualMachineService{}
	err := r.Get(ctx, types.NamespacedName{Namespace: vmService.Namespace, Name: vmService.Name}, currentVMService)
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get VirtualMachineService", "name", vmService.NamespacedName())
			return nil, err
		}
		currentVMService = vmService
	}
	// update vm service
	newVMService := currentVMService.DeepCopy()
	// if could update loadbalancer external IP
	if vmService.Spec.Type == vmoperatorv1alpha1.VirtualMachineServiceTypeLoadBalancer && len(newService.Status.LoadBalancer.Ingress) > 0 {
		//copy service ingress array to vm service ingress array
		newVMService.Status.LoadBalancer.Ingress = make([]vmoperatorv1alpha1.LoadBalancerIngress, len(newService.Status.LoadBalancer.Ingress))
		for idx, ingress := range newService.Status.LoadBalancer.Ingress {
			vmIngress := vmoperatorv1alpha1.LoadBalancerIngress{
				IP:       ingress.IP,
				Hostname: ingress.Hostname,
			}
			newVMService.Status.LoadBalancer.Ingress[idx] = vmIngress
		}
		if err = r.Status().Update(ctx, newVMService); err != nil {
			log.Error(err, "Failed to update VirtualMachineService Status", "name", newVMService.NamespacedName())
			return nil, err
		}
	}

	// BMV: newVMService isn't saved after this point so annotations are missing
	pkg.AddAnnotations(&newVMService.ObjectMeta)
	return newVMService, nil
}
