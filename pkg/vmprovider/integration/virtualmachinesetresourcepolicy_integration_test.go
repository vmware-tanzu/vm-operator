// +build integration

// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

func getVirtualMachineSetResourcePolicy(name, namespace string) *vmopv1alpha1.VirtualMachineSetResourcePolicy {
	return &vmopv1alpha1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-resourcepolicy", name),
		},
		Spec: vmopv1alpha1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmopv1alpha1.ResourcePoolSpec{
				Name:         fmt.Sprintf("%s-resourcepool", name),
				Reservations: vmopv1alpha1.VirtualMachineResourceSpec{},
				Limits:       vmopv1alpha1.VirtualMachineResourceSpec{},
			},
			Folder: vmopv1alpha1.FolderSpec{
				Name: fmt.Sprintf("%s-folder", name),
			},
			ClusterModules: []vmopv1alpha1.ClusterModuleSpec{
				{GroupName: "ControlPlane"},
				{GroupName: "NodeGroup1"},
			},
		},
	}
}
