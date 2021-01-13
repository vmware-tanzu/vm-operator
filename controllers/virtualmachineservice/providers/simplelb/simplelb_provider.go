// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package simplelb

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice/utils"
)

type simpleLoadBalancerProvider struct {
	client       client.Client
	controlPlane loadbalancerControlPlane
	log          logr.Logger
}

type loadbalancerControlPlane interface {
	UpdateEndpoints(*corev1.Service, *corev1.Endpoints) error
}

func New(mgr manager.Manager) *simpleLoadBalancerProvider {
	log := ctrl.Log.WithName("controllers").WithName("simple-lb")
	return &simpleLoadBalancerProvider{
		client:       mgr.GetClient(),
		controlPlane: NewXdsServer(mgr, log),
		log:          log,
	}
}

func (s *simpleLoadBalancerProvider) EnsureLoadBalancer(ctx context.Context, vmService *vmopv1alpha1.VirtualMachineService) error {
	s.log.Info("ensure load balancer", "VMService", vmService.Name)
	xdsNodes, err := s.getXDSNodes(ctx)
	if err != nil {
		return err
	}
	lbParams := getLBConfigParams(vmService, xdsNodes)
	vm := loadbalancerVM(vmService)
	cm := loadbalancerCM(vmService, lbParams)
	if err := s.ensureLBVM(ctx, vm, cm); err != nil {
		return err
	}
	if err := s.ensureLBIP(ctx, vmService, vm); err != nil {
		return err
	}

	return s.updateLBConfig(ctx, vmService)
}

// No labels is added for simple LoadBalancer
func (s *simpleLoadBalancerProvider) GetServiceLabels(ctx context.Context, vmService *vmopv1alpha1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

// No labels needs to be removed for simple LoadBalancer
func (s *simpleLoadBalancerProvider) GetToBeRemovedServiceLabels(ctx context.Context, vmService *vmopv1alpha1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

// No annotation is added for simple LoadBalancer
func (s *simpleLoadBalancerProvider) GetServiceAnnotations(ctx context.Context, vmService *vmopv1alpha1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

// No annotation needs to be removed for simple LoadBalancer
func (s *simpleLoadBalancerProvider) GetToBeRemovedServiceAnnotations(ctx context.Context, vmService *vmopv1alpha1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (s *simpleLoadBalancerProvider) ensureLBVM(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, cm *corev1.ConfigMap) error {
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: cm.Namespace, Name: cm.Name}, cm); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := s.client.Create(ctx, cm); err != nil {
			return err
		}
	}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, vm); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := s.client.Create(ctx, vm); err != nil {
			return err
		}
	}
	return nil
}

func loadbalancerVM(vmService *vmopv1alpha1.VirtualMachineService) *vmopv1alpha1.VirtualMachine {
	return &vmopv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:            vmService.Name + "-lb",
			Namespace:       vmService.Namespace,
			OwnerReferences: []metav1.OwnerReference{utils.MakeVMServiceOwnerRef(vmService)},
		},
		Spec: vmopv1alpha1.VirtualMachineSpec{
			ImageName:  "loadbalancer-vm",
			ClassName:  "best-effort-xsmall",
			PowerState: "poweredOn",
			VmMetadata: &vmopv1alpha1.VirtualMachineMetadata{
				ConfigMapName: metadataCMName(vmService),
				Transport:     vmopv1alpha1.VirtualMachineMetadataExtraConfigTransport,
			},
		},
	}
}

func loadbalancerCM(vmService *vmopv1alpha1.VirtualMachineService, params lbConfigParams) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            metadataCMName(vmService),
			Namespace:       vmService.Namespace,
			OwnerReferences: []metav1.OwnerReference{utils.MakeVMServiceOwnerRef(vmService)},
		},
		Data: map[string]string{
			"guestinfo.userdata":          renderAndBase64EncodeLBCloudConfig(params),
			"guestinfo.userdata.encoding": "base64",
		},
	}
}

func metadataCMName(vmService *vmopv1alpha1.VirtualMachineService) string {
	return vmService.Name + "-lb" + "-cloud-init"
}

func (s *simpleLoadBalancerProvider) ensureLBIP(ctx context.Context, vmService *vmopv1alpha1.VirtualMachineService, vm *vmopv1alpha1.VirtualMachine) error {
	if len(vmService.Status.LoadBalancer.Ingress) == 0 || vmService.Status.LoadBalancer.Ingress[0].IP == "" {
		if vm.Status.VmIp == "" {
			return errors.New("LB VM IP is not ready yet")
		}
		vmService = vmService.DeepCopy()
		vmService.Status.LoadBalancer.Ingress = []vmopv1alpha1.LoadBalancerIngress{{
			IP: vm.Status.VmIp,
		}}
		err := s.client.Status().Update(ctx, vmService)
		return err
	}
	return nil
}

func (s *simpleLoadBalancerProvider) updateLBConfig(ctx context.Context, vmService *vmopv1alpha1.VirtualMachineService) error {
	service := &corev1.Service{}
	endpoints := &corev1.Endpoints{}
	if err := s.client.Get(ctx, types.NamespacedName{
		Namespace: vmService.Namespace,
		Name:      vmService.Name,
	}, service); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // service is not ready yet
		}
		return err
	}
	if err := s.client.Get(ctx, types.NamespacedName{
		Namespace: vmService.Namespace,
		Name:      vmService.Name,
	}, endpoints); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // endpoints are not ready yet
		}
		return err
	}
	return s.controlPlane.UpdateEndpoints(service, endpoints)
}

func (s *simpleLoadBalancerProvider) getXDSNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := s.client.List(ctx, nodeList); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

func portName(vmSvcPort vmopv1alpha1.VirtualMachineServicePort) string {
	if vmSvcPort.Name == "" {
		protocol := vmSvcPort.Protocol
		if protocol == "" {
			protocol = string(corev1.ProtocolTCP)
		}
		return fmt.Sprintf("%s-%v", protocol, vmSvcPort.Port)
	}
	return vmSvcPort.Name
}

func getLBConfigParams(vmService *vmopv1alpha1.VirtualMachineService, nodes []corev1.Node) lbConfigParams {
	var cpNodes = make([]string, len(nodes))
	for i, node := range nodes {
		cpNodes[i] = node.Status.Addresses[0].Address
	}
	ports := make([]vmopv1alpha1.VirtualMachineServicePort, len(vmService.Spec.Ports))
	for i, port := range vmService.Spec.Ports {
		port.Name = portName(port)
		ports[i] = port
	}
	return lbConfigParams{
		NodeID:      vmService.NamespacedName(),
		Ports:       ports,
		CPNodes:     cpNodes,
		XdsNodePort: XdsNodePort,
	}
}
