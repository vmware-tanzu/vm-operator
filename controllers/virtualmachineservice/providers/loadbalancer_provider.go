// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"
	"os"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	clientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"
	ncpclientset "gitlab.eng.vmware.com/guest-clusters/ncp-client/pkg/client/clientset/versioned"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/providers/simplelb"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
)

const (
	ServiceLoadBalancerHealthCheckNodePortTagKey = "ncp/healthCheckNodePort"
	NSXTLoadBalancer                             = "nsx-t-lb"
	SimpleLoadBalancer                           = "simple-lb"
	ClusterNameKey                               = "capw.vmware.com/cluster.name"
)

var LBProvider string

func SetLBProvider() {
	LBProvider = os.Getenv("LB_PROVIDER")
	if LBProvider == "" {
		vdsNetwork := os.Getenv("VSPHERE_NETWORKING")
		if vdsNetwork == "true" {
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

// LoadbalancerProvider sets up Loadbalancer for different type of Loadbalancer
type LoadbalancerProvider interface {
	EnsureLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) error

	// GetServiceAnnotations returns the annotations, if any, to place on a VM Service.
	GetVMServiceAnnotations(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (map[string]string, error)
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

func (noopLoadbalancerProvider) EnsureLoadBalancer(context.Context, *vmoperatorv1alpha1.VirtualMachineService) error {
	return nil
}

func (noopLoadbalancerProvider) GetVMServiceAnnotations(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
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

func (nl *nsxtLoadbalancerProvider) EnsureLoadBalancer(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) error {
	return nil
}

// GetVMServiceAnnotations provides the intended NSX-T specific annotations on
// VM Service. The responsibility is left to the caller to actually set them
func (nl *nsxtLoadbalancerProvider) GetVMServiceAnnotations(ctx context.Context, vmService *vmoperatorv1alpha1.VirtualMachineService) (map[string]string, error) {
	res := make(map[string]string)

	if healthCheckNodePortString, ok := vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey]; ok {
		res[ServiceLoadBalancerHealthCheckNodePortTagKey] = healthCheckNodePortString
	}

	return res, nil
}
