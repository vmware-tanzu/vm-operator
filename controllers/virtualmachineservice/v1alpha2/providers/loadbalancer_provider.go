// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/v1alpha2/providers/simplelb"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/v1alpha2/utils"
)

const (
	NSXTLoadBalancer   = "nsx-t-lb"
	SimpleLoadBalancer = "simple-lb"

	ServiceLoadBalancerHealthCheckNodePortTagKey = "ncp/healthCheckNodePort"
	NSXTServiceProxy                             = "nsx-t"
	// LabelServiceProxyName indicates that an alternative service proxy will implement
	// this Service. Copied from kubernetes pkg/proxy/apis/well_known_labels.go to avoid
	// k8s dependency.
	LabelServiceProxyName = "service.kubernetes.io/service-proxy-name"
)

// LoadbalancerProvider sets up Loadbalancer for different type of Loadbalancer.
type LoadbalancerProvider interface {
	EnsureLoadBalancer(ctx context.Context, vmService *vmopv1.VirtualMachineService) error

	// GetServiceLabels returns the labels, if any, to place on a Service.
	// This is applicable when VirtualMachineService is translated to a
	// Service and we would like to apply the provider specific labels
	// on the corresponding Service.
	GetServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error)

	// GetToBeRemovedServiceLabels returns the labels, if any, to be
	// removed from a Service.
	// This is applicable when VirtualMachineService is translated to a
	// Service and we would like to remove the provider specific labels
	// from the corresponding Service
	// This is needed because other operators(net operator) might have added
	// labels to the Service so to correctly sync the addition/removal of
	// labels on the Service object without touching the existing ones,
	// we need to have clearly defined ownership
	GetToBeRemovedServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error)

	// GetServiceAnnotations returns the annotations, if any, to place on a Service.
	// This is applicable when VirtualMachineService is translated to a
	// Service and we would like to apply the provider specific annotations
	// on the corresponding Service.
	GetServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error)

	// GetToBeRemovedServiceAnnotations returns the annotations, if any, to be
	// removed from a Service.
	// This is applicable when VirtualMachineService is translated to a
	// Service and we would like to remove the provider specific annotations
	// from the corresponding Service
	// This is needed because other operators(net operator) might have added
	// annotations to the Service so to correctly sync the addition/removal of
	// annotations on the Service object without touching the existing ones,
	// we need to have clearly defined ownership
	GetToBeRemovedServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error)
}

func GetLoadbalancerProviderByType(mgr manager.Manager, providerType string) (LoadbalancerProvider, error) {
	if providerType == NSXTLoadBalancer {
		return NsxtLoadBalancerProvider(), nil
	}
	if providerType == SimpleLoadBalancer {
		return simplelb.New(mgr), nil
	}
	return NoopLoadbalancerProvider{}, nil
}

type NoopLoadbalancerProvider struct{}

func (NoopLoadbalancerProvider) EnsureLoadBalancer(context.Context, *vmopv1.VirtualMachineService) error {
	return nil
}

func (NoopLoadbalancerProvider) GetServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (NoopLoadbalancerProvider) GetToBeRemovedServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (NoopLoadbalancerProvider) GetServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (NoopLoadbalancerProvider) GetToBeRemovedServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

type NsxtLoadbalancerProvider struct {
}

// NsxtLoadBalancerProvider returns a nsxLoadbalancerProvider instance.
func NsxtLoadBalancerProvider() *NsxtLoadbalancerProvider {
	return &NsxtLoadbalancerProvider{}
}

func (nl *NsxtLoadbalancerProvider) EnsureLoadBalancer(ctx context.Context, vmService *vmopv1.VirtualMachineService) error {
	return nil
}

// GetServiceLabels provides the intended NSX-T specific labels on Service. The
// responsibility is left to the caller to actually set them.
func (nl *NsxtLoadbalancerProvider) GetServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	res := make(map[string]string)

	// When externalTrafficPolicy is set to Local, skip kube-proxy for the
	// target Service
	if etp := vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey]; corev1.ServiceExternalTrafficPolicyType(etp) == corev1.ServiceExternalTrafficPolicyTypeLocal {
		res[LabelServiceProxyName] = NSXTServiceProxy
	}

	return res, nil
}

// GetToBeRemovedServiceLabels provides the to be removed NSX-T specific labels on
// Service. The responsibility is left to the caller to actually clear them.
func (nl *NsxtLoadbalancerProvider) GetToBeRemovedServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	res := make(map[string]string)

	// When there is no externalTrafficPolicy configured or it's not Local,
	// remove the service-proxy label
	if etp := vmService.Annotations[utils.AnnotationServiceExternalTrafficPolicyKey]; corev1.ServiceExternalTrafficPolicyType(etp) != corev1.ServiceExternalTrafficPolicyTypeLocal {
		res[LabelServiceProxyName] = NSXTServiceProxy
	}

	return res, nil
}

// GetServiceAnnotations provides the intended NSX-T specific annotations on
// Service. The responsibility is left to the caller to actually set them.
func (nl *NsxtLoadbalancerProvider) GetServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	res := make(map[string]string)

	if healthCheckNodePortString, ok := vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey]; ok {
		res[ServiceLoadBalancerHealthCheckNodePortTagKey] = healthCheckNodePortString
	}

	return res, nil
}

// GetToBeRemovedServiceAnnotations provides the to be removed NSX-T specific annotations on
// Service. The responsibility is left to the caller to actually clear them.
func (nl *NsxtLoadbalancerProvider) GetToBeRemovedServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	res := make(map[string]string)

	// When healthCheckNodePort is NOT present, the corresponding NSX-T
	// annotation should be cleared as well
	if _, ok := vmService.Annotations[utils.AnnotationServiceHealthCheckNodePortKey]; !ok {
		res[ServiceLoadBalancerHealthCheckNodePortTagKey] = ""
	}

	return res, nil
}
