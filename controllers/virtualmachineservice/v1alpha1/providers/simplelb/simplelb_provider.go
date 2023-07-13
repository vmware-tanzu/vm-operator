// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package simplelb

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

type Provider struct {
	client       client.Client
	controlPlane loadbalancerControlPlane
	log          logr.Logger
}

type loadbalancerControlPlane interface {
	UpdateEndpoints(*corev1.Service, *corev1.Endpoints) error
}

func New(mgr manager.Manager) *Provider {
	log := ctrl.Log.WithName("controllers").WithName("simple-lb")
	return &Provider{
		client:       mgr.GetClient(),
		controlPlane: NewXdsServer(mgr, log),
		log:          log,
	}
}

func (s *Provider) EnsureLoadBalancer(ctx context.Context, vmService *vmopv1.VirtualMachineService) error {
	s.log.Info("ensure load balancer", "VMService", vmService.Name)
	xdsNodes, err := s.getXDSNodes(ctx)
	if err != nil {
		return err
	}
	vmImageName, err := s.GetVirtualMachineImageName(ctx)
	if err != nil {
		return err
	}
	vmClassName, err := s.GetVirtualMachineClassName(ctx, vmService.Namespace)
	if err != nil {
		return err
	}
	lbParams := getLBConfigParams(vmService, xdsNodes)
	vm := loadbalancerVM(vmService, vmClassName, vmImageName)
	cm := loadbalancerCM(vmService, lbParams)
	if err := s.ensureLBVM(ctx, vm, cm); err != nil {
		return err
	}
	if err := s.ensureLBIP(ctx, vmService, vm); err != nil {
		return err
	}

	return s.updateLBConfig(ctx, vmService)
}

// GetVirtualMachineClassName returns the class name for loadbalancer-vm.
// We need to choose the VM class name which the namespace has access to instead of hardcode it.
func (s *Provider) GetVirtualMachineClassName(ctx context.Context, namespace string) (string, error) {
	bindingList := &vmopv1.VirtualMachineClassBindingList{}
	if err := s.client.List(ctx, bindingList, client.InNamespace(namespace)); err != nil {
		s.log.Error(err, "failed to list VirtualMachineClassBindings from control plane")
		return "", err
	}

	if len(bindingList.Items) == 0 {
		return "", fmt.Errorf("no virtual machine class is available in namespace %s", namespace)
	}

	return bindingList.Items[0].Name, nil
}

// GetVirtualMachineImageName returns the image name for loadbalancer-vm image in the cluster.
// Since we use generateName for VirtualMachineImage resources, we cannot directly use 'loadbalancer-vm'.
func (s *Provider) GetVirtualMachineImageName(ctx context.Context) (string, error) {
	imageList := &vmopv1.VirtualMachineImageList{}
	if err := s.client.List(ctx, imageList); err != nil {
		s.log.Error(err, "failed to list VirtualMachineImages from control plane")
		return "", err
	}

	for _, img := range imageList.Items {
		if strings.Contains(img.Name, "loadbalancer-vm") {
			return img.Name, nil
		}
	}
	return "", errors.New("no virtual machine image for loadbalancer-vm")
}

func (s *Provider) GetServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (s *Provider) GetToBeRemovedServiceLabels(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (s *Provider) GetServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (s *Provider) GetToBeRemovedServiceAnnotations(ctx context.Context, vmService *vmopv1.VirtualMachineService) (map[string]string, error) {
	return nil, nil
}

func (s *Provider) ensureLBVM(ctx context.Context, vm *vmopv1.VirtualMachine, cm *corev1.ConfigMap) error {
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

func makeVMServiceOwnerRef(vmService *vmopv1.VirtualMachineService) metav1.OwnerReference {
	virtualMachineServiceKind := reflect.TypeOf(vmopv1.VirtualMachineService{}).Name()
	virtualMachineServiceAPIVersion := vmopv1.SchemeGroupVersion.String()

	return metav1.OwnerReference{
		UID:                vmService.UID,
		Name:               vmService.Name,
		Controller:         pointer.Bool(false),
		BlockOwnerDeletion: pointer.Bool(true),
		Kind:               virtualMachineServiceKind,
		APIVersion:         virtualMachineServiceAPIVersion,
	}
}

func loadbalancerVM(vmService *vmopv1.VirtualMachineService, vmClassName, vmImageName string) *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:            vmService.Name + "-lb",
			Namespace:       vmService.Namespace,
			OwnerReferences: []metav1.OwnerReference{makeVMServiceOwnerRef(vmService)},
		},
		Spec: vmopv1.VirtualMachineSpec{
			ImageName:  vmImageName,
			ClassName:  vmClassName,
			PowerState: "poweredOn",
			VmMetadata: &vmopv1.VirtualMachineMetadata{
				ConfigMapName: metadataCMName(vmService),
				Transport:     vmopv1.VirtualMachineMetadataExtraConfigTransport,
			},
		},
	}
}

func loadbalancerCM(vmService *vmopv1.VirtualMachineService, params lbConfigParams) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            metadataCMName(vmService),
			Namespace:       vmService.Namespace,
			OwnerReferences: []metav1.OwnerReference{makeVMServiceOwnerRef(vmService)},
		},
		Data: map[string]string{
			"guestinfo.userdata":          renderAndBase64EncodeLBCloudConfig(params),
			"guestinfo.userdata.encoding": "base64",
		},
	}
}

func metadataCMName(vmService *vmopv1.VirtualMachineService) string {
	return vmService.Name + "-lb" + "-cloud-init"
}

func (s *Provider) ensureLBIP(ctx context.Context, vmService *vmopv1.VirtualMachineService, vm *vmopv1.VirtualMachine) error {
	if len(vmService.Status.LoadBalancer.Ingress) == 0 || vmService.Status.LoadBalancer.Ingress[0].IP == "" {
		if vm.Status.VmIp == "" {
			return errors.New("LB VM IP is not ready yet")
		}

		service := &corev1.Service{}
		if err := s.client.Get(ctx, types.NamespacedName{
			Namespace: vmService.Namespace,
			Name:      vmService.Name,
		}, service); err == nil {
			service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{
				IP: vm.Status.VmIp,
			}}
			err := s.client.Status().Update(ctx, service)
			if err != nil {
				s.log.Error(err, "Failed to update Service")
				// Keep going.
			}
		}
	}
	return nil
}

func (s *Provider) updateLBConfig(ctx context.Context, vmService *vmopv1.VirtualMachineService) error {
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

func (s *Provider) getXDSNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := s.client.List(ctx, nodeList); err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

func portName(vmSvcPort vmopv1.VirtualMachineServicePort) string {
	if vmSvcPort.Name == "" {
		protocol := vmSvcPort.Protocol
		if protocol == "" {
			protocol = string(corev1.ProtocolTCP)
		}
		return fmt.Sprintf("%s-%v", protocol, vmSvcPort.Port)
	}
	return vmSvcPort.Name
}

func getLBConfigParams(vmService *vmopv1.VirtualMachineService, nodes []corev1.Node) lbConfigParams {
	var cpNodes = make([]string, len(nodes))
	for i, node := range nodes {
		cpNodes[i] = node.Status.Addresses[0].Address
	}
	ports := make([]vmopv1.VirtualMachineServicePort, len(vmService.Spec.Ports))
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
