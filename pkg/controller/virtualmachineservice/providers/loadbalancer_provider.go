/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package providers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ptr "github.com/kubernetes/utils/pointer"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
)

type loadBalancerProviderType string

const (
	LoadbalancerKind                                   = "LoadBalancer"
	APIVersion                                         = "vmware.com/v1alpha1"
	ServiceLoadBalancerTagKey                          = "ncp/crd_lb"
	ServiceOwnerRefKind                                = "VirtualMachineService"
	ServiceOwnerRefVersion                             = "vmoperator.vmware.com/v1alpha1"
	NSXTLoadBalancer          loadBalancerProviderType = "nsx-t-lb"
	ClusterNameKey                                     = "capw.vmware.com/cluster.name"
)

var log = klogr.New()

// patchOperation represents a RFC6902 JSON patch operation.
type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

//LoadbalancerProvider sets up Loadbalancer for different type of Loadbalancer
type LoadbalancerProvider interface {
	GetNetworkName(virtualMachines []vmoperatorv1alpha1.VirtualMachine, vmService *vmoperatorv1alpha1.VirtualMachineService) (string, error)

	EnsureLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, virtualNetworkName string) (string, error)

	UpdateLoadBalancerOwnerReference(ctx context.Context, loadBalancerName string, vmService *vmoperatorv1alpha1.VirtualMachineService) error
}

// Get Loadbalancer Provider By Type, currently only support nsxt provider, if provider type unknown, will return nil
func GetLoadbalancerProviderByType(providerType loadBalancerProviderType) LoadbalancerProvider {
	if providerType == NSXTLoadBalancer {
		//TODO:  () Using static ncp client for now, replace it with runtime ncp client
		cfg, err := config.GetConfig()
		if err != nil {
			log.Error(err, "unable to set up client config")
			return nil
		}
		ncpClient, err := ncpclientset.NewForConfig(cfg)
		if err != nil {
			log.Error(err, "unable to get ncp clientset from config")
			return nil
		}
		return NsxtLoadBalancerProvider(ncpClient)
	}
	return nil
}

type nsxtLoadbalancerProvider struct {
	client clientset.Interface
}

//NsxLoadbalancerProvider returns a nsxLoadbalancerProvider instance
func NsxtLoadBalancerProvider(client clientset.Interface) *nsxtLoadbalancerProvider {
	return &nsxtLoadbalancerProvider{
		client: client,
	}
}

func (nl *nsxtLoadbalancerProvider) EnsureLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, virtualNetworkName string) (string, error) {
	return nl.ensureNSXTLoadBalancer(ctx, vmService, virtualNetworkName)
}

//Loadbalancer name formatting
func (nl *nsxtLoadbalancerProvider) getLoadbalancerName(namespace, clusterName string) string {
	return fmt.Sprintf("%s-%s-lb", namespace, clusterName)
}

//Create NSX-T Loadbalancer configuration
func (nl *nsxtLoadbalancerProvider) createNSXTLoadBalancerSpec(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, virtualNetworkName string) *ncpv1alpha1.LoadBalancer {
	return &ncpv1alpha1.LoadBalancer{
		TypeMeta: metav1.TypeMeta{
			Kind:       LoadbalancerKind,
			APIVersion: APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      nl.getLoadbalancerName(vmService.Namespace, vmService.Spec.Selector[ClusterNameKey]),
			Namespace: vmService.GetNamespace(),
		},
		Spec: ncpv1alpha1.LoadBalancerSpec{
			Size:               ncpv1alpha1.SizeSmall,
			VirtualNetworkName: virtualNetworkName,
		},
	}
}

//Create or Update NSX-T Loadbalancer
func (nl *nsxtLoadbalancerProvider) ensureNSXTLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, virtualNetworkName string) (string, error) {
	loadBalancerName := nl.getLoadbalancerName(vmService.Namespace, vmService.ClusterName)

	log.V(4).Info("Get or create loadbalancer", "name", loadBalancerName)
	defer log.V(4).Info("Finished get or create Loadbalancer", "name", loadBalancerName)
	//Get current loadbalancer
	currentLoadbalancer, err := nl.client.VmwareV1alpha1().LoadBalancers(vmService.Namespace).Get(loadBalancerName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get load balancer", "name", loadBalancerName)
			return "", err
		}
		//check if the virtual network exist
		_, err = nl.client.VmwareV1alpha1().VirtualNetworks(vmService.Namespace).Get(virtualNetworkName, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to get virtual network", "name", virtualNetworkName)
			} else {
				log.Error(err, "Virtual Network does not exist, can't create loadbalancer", "name", virtualNetworkName)
			}
			return "", err
		}
		//no current loadbalancer, create a new loadbalancer
		//We only update Loadbalancer owner reference for now, other spec should be immutable .
		currentLoadbalancer = nl.createNSXTLoadBalancerSpec(ctx, vmService, virtualNetworkName)
		log.Info("Creating loadBalancer", "name", currentLoadbalancer.Name, "loadBalancer", currentLoadbalancer)
		currentLoadbalancer, err = nl.client.VmwareV1alpha1().LoadBalancers(currentLoadbalancer.Namespace).Create(currentLoadbalancer)
		if err != nil {
			log.Error(err, "Create loadbalancer error", "name", virtualNetworkName)
			return "", err
		}
	}
	//annotate which loadbalancer to attach
	addNcpAnnotations(currentLoadbalancer.Name, &vmService.ObjectMeta)
	return currentLoadbalancer.Name, nil
}

//Check virtual network name from vm spec
func (nl *nsxtLoadbalancerProvider) GetNetworkName(virtualMachines []vmoperatorv1alpha1.VirtualMachine, vmService *vmoperatorv1alpha1.VirtualMachineService) (string, error) {
	if vmService != nil && vmService.Annotations != nil {
		if vnet := vmService.Annotations["ncp.vmware.com/virtual-network-name"]; vnet != "" {
			return vnet, nil
		}
	}

	if len(virtualMachines) <= 0 {
		return "", fmt.Errorf("no virtual machine matched selector")
	}

	networkName := ""
	//Check if there is only one NSX-T virtual network in cluster
	for _, vm := range virtualMachines {
		hasNSXTNetwork := false
		for _, networkInterface := range vm.Spec.NetworkInterfaces {
			if networkInterface.NetworkType == vsphere.NsxtNetworkType {
				if hasNSXTNetwork {
					return "", fmt.Errorf("virtual machine %q can't connect to two NST-X virtual network ", vm.NamespacedName())
				}
				hasNSXTNetwork = true
				if networkName == "" {
					networkName = networkInterface.NetworkName
				} else if networkName != networkInterface.NetworkName {
					return "", fmt.Errorf("virtual machine %q has different virtual network with previous vms", vm.NamespacedName())
				}
			}
		}
		if !hasNSXTNetwork {
			return "", fmt.Errorf("virtual machine %q doesn't have nsx-t virtual network", vm.NamespacedName())
		}
	}
	return networkName, nil
}

// VM service add annotations
func addNcpAnnotations(loadBalancerName string, objectMeta *metav1.ObjectMeta) {
	annotations := objectMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[ServiceLoadBalancerTagKey] = loadBalancerName
	objectMeta.SetAnnotations(annotations)
}

// UpdateLoadBalancerOwnerReference: Update load balancer owner reference to load balancer type of vm service
func (nl *nsxtLoadbalancerProvider) UpdateLoadBalancerOwnerReference(ctx context.Context, loadBalancerName string, vmService *vmoperatorv1alpha1.VirtualMachineService) error {
	loadBalancer, err := nl.client.VmwareV1alpha1().LoadBalancers(vmService.Namespace).Get(loadBalancerName, metav1.GetOptions{})
	if err != nil {
		log.Error(err, "Load Balancer does not exist, can't set owner reference for vm service", "vm service", vmService.NamespacedName())
		return err
	}
	patchOpStr, _ := nl.PrepareLoadBalancerOwnerRefPatchOperation(loadBalancer, vmService)
	_, err = nl.client.VmwareV1alpha1().LoadBalancers(loadBalancer.Namespace).Patch(loadBalancerName, types.JSONPatchType, patchOpStr)
	return err
}

// PrepareLoadBalancerOwnerRefPatchOperation Prepare patch operation for owner reference patch update
func (nl *nsxtLoadbalancerProvider) PrepareLoadBalancerOwnerRefPatchOperation(loadBalancer *ncpv1alpha1.LoadBalancer, vmService *vmoperatorv1alpha1.VirtualMachineService) ([]byte, error) {
	path := "/metadata/ownerReferences"
	var value interface{}
	newOwner := metav1.OwnerReference{
		UID:                vmService.UID,
		Name:               vmService.Name,
		Controller:         ptr.BoolPtr(false),
		BlockOwnerDeletion: ptr.BoolPtr(true),
		Kind:               ServiceOwnerRefKind,
		APIVersion:         ServiceOwnerRefVersion,
	}

	if len(loadBalancer.OwnerReferences) == 0 {
		value = []metav1.OwnerReference{newOwner}
	} else {
		path += "/-"
		value = newOwner
	}

	patchOp := []patchOperation{
		{
			Op:    "add",
			Path:  path,
			Value: value,
		},
	}
	return json.Marshal(patchOp)
}
