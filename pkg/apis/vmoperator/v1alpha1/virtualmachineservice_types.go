/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package v1alpha1

import (
	"context"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/runtime"

	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	VirtualMachineServiceFinalizer string = "virtualmachineservice.vmoperator.vmware.com"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualMachineService
// +k8s:openapi-gen=true
// +resource:path=virtualmachineservices,strategy=VirtualMachineServiceStrategy
type VirtualMachineService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineServiceSpec   `json:"spec,omitempty"`
	Status VirtualMachineServiceStatus `json:"status,omitempty"`
}

type VirtualMachineServicePort struct {
	Name string `json:"name"`

	// The IP protocol for this port.  Supports "TCP", "UDP", and "SCTP".
	//Protocol corev1.Protocol
	Protocol string `json:"protocol"`

	// The port that will be exposed on the service.
	Port int32 `json:"port"`

	TargetPort int32 `json:"targetPort"`
}

// VirtualMachineService Type string describes ingress methods for a service
type VirtualMachineServiceType string

// These types correspond to a subset of the core Service Types
const (
	// VirtualMachineServiceTypeClusterIP means a service will only be accessible inside the
	// cluster, via the cluster IP.
	VirtualMachineServiceTypeClusterIP VirtualMachineServiceType = "ClusterIP"

	// VirtualMachineServiceTypeLoadBalancer means a service will be exposed via an
	// external load balancer (if the cloud provider supports it), in addition
	// to 'NodePort' type.
	VirtualMachineServiceTypeLoadBalancer VirtualMachineServiceType = "LoadBalancer"

	// VirtualMachineServiceTypeExternalName means a service consists of only a reference to
	// an external name that kubedns or equivalent will return as a CNAME
	// record, with no exposing or proxying of any pods involved.
	VirtualMachineServiceTypeExternalName VirtualMachineServiceType = "ExternalName"
)

// VirtualMachineServiceSpec defines the desired state of VirtualMachineService
type VirtualMachineServiceSpec struct {
	Type     string                      `json:"type"`
	Ports    []VirtualMachineServicePort `json:"ports"`
	Selector map[string]string           `json:"selector"`

	// Just support cluster IP for now
	ClusterIP    string `json:"clusterIp,omitempty"`
	ExternalName string `json:"externalName,omitempty"`
}

// VirtualMachineServiceStatus defines the observed state of VirtualMachineService
type VirtualMachineServiceStatus struct {
}

func (v VirtualMachineServiceStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	// Invoke the parent implementation to strip the Status
	v.DefaultStorageStrategy.PrepareForCreate(ctx, obj)

	o := obj.(*vmoperator.VirtualMachineService)

	// Add a finalizer so that our controllers can process deletion
	finalizers := append(o.GetFinalizers(), VirtualMachineServiceFinalizer)
	o.SetFinalizers(finalizers)
}

// Validate checks that an instance of VirtualMachineService is well formed
func (VirtualMachineServiceStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	service := obj.(*vmoperator.VirtualMachineService)
	glog.V(4).Infof("Validating fields for VirtualMachineService %s\n", service.Name)
	errors := field.ErrorList{}

	// Confirm that the required fields are present and within valid ranges, if applicable
	if service.Spec.Type == "" {
		glog.Errorf("Type empty for service %s", service.Name)
		errors = append(errors, field.Required(field.NewPath("spec", "type"), ""))
	}

	if len(service.Spec.Ports) == 0 {
		glog.Errorf("Ports empty for service %s", service.Name)
		errors = append(errors, field.Required(field.NewPath("spec", "ports"), ""))
	}

	if len(service.Spec.Selector) == 0 {
		glog.Errorf("Label selector empty for service %s", service.Name)
		errors = append(errors, field.Required(field.NewPath("spec", "selector"), ""))
	}
	return errors
}

// DefaultingFunction sets default VirtualMachineService field values
func (VirtualMachineServiceSchemeFns) DefaultingFunction(o interface{}) {
	obj := o.(*VirtualMachineService)
	// set default field values here
	glog.V(4).Infof("Defaulting fields for VirtualMachineService %s\n", obj.Name)
}
