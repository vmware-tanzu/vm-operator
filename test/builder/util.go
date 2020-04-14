// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

const (
	DummyImageName   = "dummyImageName"
	DummyClassName   = "dummyClassName"
	DummyNetworkName = "dummyNetworkName"
	DummyVolumeName  = "dummyVolumeName"
	DummyPVCName     = "dummyPVCName"
)

var (
	converter runtime.UnstructuredConverter = runtime.DefaultUnstructuredConverter
)

/*
import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// DummyDistributionVersion must be in semver format to pass validation
	DummyDistributionVersion             = "v1.15.5+vmware.1.66-guest.1.821"
	DummyIncompatibleDistributionVersion = "v1.19.0+vmware.1-guest.1"
)

func DummyVirtualMachineImage() *vmopv1.VirtualMachineImage {
	image := FakeVirtualMachineImage(DummyDistributionVersion)
	// Using image.GenerateName causes problems with unit tests
	// nolint:gosec This doesn't need to be a secure prng
	image.Name = fmt.Sprintf("test-%d", rand.Int())
	return image
}
*/

func DummyVirtualMachineClass() *vmopv1.VirtualMachineClass {
	return &vmopv1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
		},
		Spec: vmopv1.VirtualMachineClassSpec{
			Hardware: vmopv1.VirtualMachineClassHardware{
				Cpus:   int64(2),
				Memory: resource.MustParse("4Gi"),
			},
			Policies: vmopv1.VirtualMachineClassPolicies{
				Resources: vmopv1.VirtualMachineClassResources{
					Requests: vmopv1.VirtualMachineResourceSpec{
						Cpu:    resource.MustParse("1Gi"),
						Memory: resource.MustParse("2Gi"),
					},
					Limits: vmopv1.VirtualMachineResourceSpec{
						Cpu:    resource.MustParse("2Gi"),
						Memory: resource.MustParse("4Gi"),
					},
				},
			},
		},
	}
}

func DummyVirtualMachine() *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
		},
		Spec: vmopv1.VirtualMachineSpec{
			ImageName: DummyImageName,
			ClassName: DummyClassName,
			NetworkInterfaces: []vmopv1.VirtualMachineNetworkInterface{
				{
					NetworkName: DummyNetworkName,
					NetworkType: "",
				},
			},
			Volumes: []vmopv1.VirtualMachineVolume{
				{
					Name: DummyVolumeName,
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: DummyPVCName,
					},
				},
			},
		},
	}
}

func DummyVirtualMachineService() *vmopv1.VirtualMachineService {
	return &vmopv1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
		},
		Spec: vmopv1.VirtualMachineServiceSpec{
			Type: vmopv1.VirtualMachineServiceTypeLoadBalancer,
			Ports: []vmopv1.VirtualMachineServicePort{
				{
					Name:       "dummy-port",
					Protocol:   "TCP",
					Port:       42,
					TargetPort: 4242,
				},
			},
			Selector: map[string]string{
				"foo": "bar",
			},
		},
	}
}

func DummyVirtualMachineSetResourcePolicy() *vmopv1.VirtualMachineSetResourcePolicy {
	return &vmopv1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
		},
		Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmopv1.ResourcePoolSpec{
				Name: "dummy-resource-pool",
				Reservations: vmopv1.VirtualMachineResourceSpec{
					Cpu:    resource.MustParse("1Gi"),
					Memory: resource.MustParse("2Gi"),
				},
				Limits: vmopv1.VirtualMachineResourceSpec{
					Cpu:    resource.MustParse("2Gi"),
					Memory: resource.MustParse("4Gi"),
				},
			},
			Folder: vmopv1.FolderSpec{
				Name: "dummy-folder",
			},
			ClusterModules: []vmopv1.ClusterModuleSpec{
				{
					GroupName: "dummy-cluster-modules",
				},
			},
		},
	}
}

func ToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	content, err := converter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{}
	u.SetUnstructuredContent(content)
	return u, nil
}
