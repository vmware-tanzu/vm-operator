// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers/simplelb"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

const (
	LoadbalancerKind                             = "LoadBalancer"
	APIVersion                                   = "vmware.com/v1alpha1"
	ServiceLoadBalancerTagKey                    = "ncp/crd_lb"
	ServiceLoadBalancerHealthCheckNodePortTagKey = "ncp/healthCheckNodePort"
	NSXTLoadBalancer                             = "nsx-t-lb"
	SimpleLoadBalancer                           = "simple-lb"
	NoOpLoadBalancer                             = ""
	ClusterNameKey                               = "capw.vmware.com/cluster.name"
)

var LBProvider string

func SetLBProvider() {
	LBProvider = os.Getenv("LB_PROVIDER")
	if LBProvider == "" {
		vdsNetwork := os.Getenv("VSPHERE_NETWORKING")
		if vdsNetwork == "true" || lib.IsT1PerNamespaceEnabled() {
			// Use noopLoadbalancerProvider
			return
		}
		LBProvider = NSXTLoadBalancer
	}
}

func init() {
	SetLBProvider()
}

var log = logf.Log.WithName("loadbalancer")

//LoadbalancerProvider sets up Loadbalancer for different type of Loadbalancer
type LoadbalancerProvider interface {
	GetNetworkName(virtualMachines []vmoperatorv1alpha1.VirtualMachine, vmService *vmoperatorv1alpha1.VirtualMachineService) (string, error)

	EnsureLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, virtualNetworkName string) error
}

// Get Loadbalancer Provider By Type, currently only support nsxt provider, if provider type unknown, will return nil
func GetLoadbalancerProviderByType(mgr manager.Manager, providerType string) (LoadbalancerProvider, error) {
	if providerType == NSXTLoadBalancer {
		// TODO:  () Using static ncp client for now, replace it with runtime ncp client
		ncpClient, err := ncpclientset.NewForConfig(mgr.GetConfig())
		if err != nil {
			log.Error(err, "unable to get ncp clientset from config")
			return nil, err
		}
		return NsxtLoadBalancerProvider(ncpClient), nil
	}
	if providerType == SimpleLoadBalancer {
		return simplelb.New(mgr), nil
	}
	return noopLoadbalancerProvider{}, nil
}

type noopLoadbalancerProvider struct{}

func (noopLoadbalancerProvider) GetNetworkName([]vmoperatorv1alpha1.VirtualMachine, *vmoperatorv1alpha1.VirtualMachineService) (string, error) {
	return "", nil
}

func (noopLoadbalancerProvider) EnsureLoadBalancer(context.Context, *vmoperatorv1alpha1.VirtualMachineService, string) error {
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

func (nl *nsxtLoadbalancerProvider) EnsureLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, virtualNetworkName string) error {
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

			OwnerReferences: []metav1.OwnerReference{utils.MakeVMServiceOwnerRef(vmService)},
		},
		Spec: ncpv1alpha1.LoadBalancerSpec{
			Size:               ncpv1alpha1.SizeSmall,
			VirtualNetworkName: virtualNetworkName,
		},
	}
}

//Create or Update NSX-T Loadbalancer
func (nl *nsxtLoadbalancerProvider) ensureNSXTLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService, virtualNetworkName string) error {

	clusterName, ok := vmService.Spec.Selector[ClusterNameKey]
	if !ok {
		return fmt.Errorf("can't create loadbalancer without cluster name")
	}

	loadBalancerName := nl.getLoadbalancerName(vmService.Namespace, clusterName)

	log.V(5).Info("Get or create loadbalancer", "name", loadBalancerName)
	defer log.V(5).Info("Finished get or create Loadbalancer", "name", loadBalancerName)
	//Get current loadbalancer
	currentLoadbalancer, err := nl.client.VmwareV1alpha1().LoadBalancers(vmService.Namespace).Get(loadBalancerName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to get load balancer", "name", loadBalancerName)
			return err
		}
		//check if the virtual network exist
		_, err = nl.client.VmwareV1alpha1().VirtualNetworks(vmService.Namespace).Get(virtualNetworkName, metav1.GetOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to get virtual network", "name", virtualNetworkName)
			} else {
				log.Error(err, "Virtual Network does not exist, can't create loadbalancer", "name", virtualNetworkName)
			}
			return err
		}
		//no current loadbalancer, create a new loadbalancer
		//We only update Loadbalancer owner reference for now, other spec should be immutable .
		currentLoadbalancer = nl.createNSXTLoadBalancerSpec(ctx, vmService, virtualNetworkName)
		log.Info("Creating loadBalancer", "name", currentLoadbalancer.Name, "loadBalancer", currentLoadbalancer)
		currentLoadbalancer, err = nl.client.VmwareV1alpha1().LoadBalancers(currentLoadbalancer.Namespace).Create(currentLoadbalancer)
		if err != nil {
			log.Error(err, "Create loadbalancer error", "name", virtualNetworkName)
			return err
		}
	}
	//annotate which loadbalancer to attach
	addNcpAnnotations(currentLoadbalancer.Name, &vmService.ObjectMeta)
	return nil
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

	if healthCheckNodePortString, ok := annotations[utils.AnnotationServiceHealthCheckNodePortKey]; ok {
		annotations[ServiceLoadBalancerHealthCheckNodePortTagKey] = healthCheckNodePortString
	}

	objectMeta.SetAnnotations(annotations)
}
