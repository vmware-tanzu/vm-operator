/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package utils

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// ServiceEqual compares the new Service's spec with old Service's last applied config
func ServiceEqual(newService, oldService *corev1.Service) (bool, error) {
	if lastAppliedConfig, ok := oldService.GetAnnotations()[corev1.LastAppliedConfigAnnotation]; ok {
		lastAppliedService := corev1.Service{}
		err := json.Unmarshal([]byte(lastAppliedConfig), &lastAppliedService)
		if err != nil {
			return false, err
		}
		return apiequality.Semantic.DeepEqual(lastAppliedService.Spec, newService.Spec), nil
	}
	return false, fmt.Errorf("old service doesn't have last applied config ")
}

// VMServiceEqual compares the new vm service's old vm service's last applied config
func VMServiceEqual(newVMService, oldVMService *vmoperatorv1alpha1.VirtualMachineService) (bool, error) {
	if lastAppliedConfig, ok := oldVMService.GetAnnotations()[corev1.LastAppliedConfigAnnotation]; ok {
		return VMServiceCompareToLastApplied(newVMService, lastAppliedConfig)
	}
	return false, fmt.Errorf("old vm service doesn't have last applied config ")
}

// VMServiceCompareToLastApplied compares the new vm service's spec with last applied vm service config
func VMServiceCompareToLastApplied(newVMService *vmoperatorv1alpha1.VirtualMachineService, lastAppliedAnnotation string) (bool, error) {
	oldVMService := vmoperatorv1alpha1.VirtualMachineService{}
	err := json.Unmarshal([]byte(lastAppliedAnnotation), &oldVMService)
	if err != nil {
		return false, err
	}
	return apiequality.Semantic.DeepEqual(oldVMService.Spec, newVMService.Spec), nil
}
