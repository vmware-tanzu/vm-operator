/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package utils

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

var (
	virtualMachineServiceKind       = reflect.TypeOf(vmoperatorv1alpha1.VirtualMachineService{}).Name()
	virtualMachineServiceAPIVersion = vmoperatorv1alpha1.SchemeGroupVersion.String()
)

func MakeVMServiceOwnerRef(vmService *vmoperatorv1alpha1.VirtualMachineService) metav1.OwnerReference {
	return metav1.OwnerReference{
		UID:                vmService.UID,
		Name:               vmService.Name,
		Controller:         pointer.BoolPtr(false),
		BlockOwnerDeletion: pointer.BoolPtr(true),
		Kind:               virtualMachineServiceKind,
		APIVersion:         virtualMachineServiceAPIVersion,
	}
}
