// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"fmt"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

func DummyVirtualMachineSetResourcePolicyA2() *vmopv1.VirtualMachineSetResourcePolicy {
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
			Folder:              "dummy-folder",
			ClusterModuleGroups: []string{"dummy-cluster-modules"},
		},
	}
}

func DummyVirtualMachineServiceA2() *vmopv1.VirtualMachineService {
	return &vmopv1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			// Using image.GenerateName causes problems with unit tests
			Name: fmt.Sprintf("test-%s", uuid.New()),
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

func DummyVirtualMachineA2() *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Labels:       map[string]string{},
			Annotations:  map[string]string{},
		},
		Spec: vmopv1.VirtualMachineSpec{
			ImageName:  DummyImageName,
			ClassName:  DummyClassName,
			PowerState: vmopv1.VirtualMachinePowerStateOn,
			Volumes: []vmopv1.VirtualMachineVolume{
				{
					Name: DummyVolumeName,
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: DummyPVCName,
							},
						},
					},
				},
			},
		},
	}
}

func DummyVirtualMachinePublishRequestA2(name, namespace, sourceName, itemName, clName string) *vmopv1.VirtualMachinePublishRequest {
	return &vmopv1.VirtualMachinePublishRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{"virtualmachinepublishrequest.vmoperator.vmware.com"},
		},
		Spec: vmopv1.VirtualMachinePublishRequestSpec{
			Source: vmopv1.VirtualMachinePublishRequestSource{
				Name:       sourceName,
				APIVersion: "vmoperator.vmware.com/v1alpha2",
				Kind:       "VirtualMachine",
			},
			Target: vmopv1.VirtualMachinePublishRequestTarget{
				Item: vmopv1.VirtualMachinePublishRequestTargetItem{
					Name: itemName,
				},
				Location: vmopv1.VirtualMachinePublishRequestTargetLocation{
					Name:       clName,
					APIVersion: "imageregistry.vmware.com/v1alpha1",
					Kind:       "ContentLibrary",
				},
			},
		},
	}
}

func DummyVirtualMachineClassA2() *vmopv1.VirtualMachineClass {
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

func DummyInstanceStorageVirtualMachineVolumesA2() []vmopv1.VirtualMachineVolume {
	return []vmopv1.VirtualMachineVolume{
		{
			Name: "instance-pvc-1",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "instance-pvc-1",
					},
					InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
						StorageClass: DummyStorageClassName,
						Size:         resource.MustParse("256Gi"),
					},
				},
			},
		},
		{
			Name: "instance-pvc-2",
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "instance-pvc-2",
					},
					InstanceVolumeClaim: &vmopv1.InstanceVolumeClaimVolumeSource{
						StorageClass: DummyStorageClassName,
						Size:         resource.MustParse("512Gi"),
					},
				},
			},
		},
	}
}

func DummyVirtualMachineImageA2(imageName string) *vmopv1.VirtualMachineImage {
	return &vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: imageName,
		},
		Status: vmopv1.VirtualMachineImageStatus{
			Name: imageName,
			ProductInfo: vmopv1.VirtualMachineImageProductInfo{
				FullVersion: DummyDistroVersion,
			},
			OSInfo: vmopv1.VirtualMachineImageOSInfo{
				Type: DummyOSType,
			},
		},
	}
}

func DummyClusterVirtualMachineImageA2(imageName string) *vmopv1.ClusterVirtualMachineImage {
	return &vmopv1.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: imageName,
		},
		Status: vmopv1.VirtualMachineImageStatus{
			Name: imageName,
			ProductInfo: vmopv1.VirtualMachineImageProductInfo{
				FullVersion: DummyDistroVersion,
			},
			OSInfo: vmopv1.VirtualMachineImageOSInfo{
				Type: DummyOSType,
			},
		},
	}
}
