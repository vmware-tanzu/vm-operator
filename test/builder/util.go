// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"

	"github.com/google/uuid"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

const (
	DummyImageName            = "dummy-image-name"
	DummyClassName            = "dummyClassName"
	DummyNetworkName          = "dummyNetworkName"
	DummyVolumeName           = "dummy-volume-name"
	DummyPVCName              = "dummyPVCName"
	DummyMetadataCMName       = "dummyMetadataCMName"
	DummyDistroVersion        = "dummyDistroVersion"
	DummyOSType               = "centosGuest"
	DummyStorageClassName     = "dummy-storage-class"
	DummyResourceQuotaName    = "dummy-resource-quota"
	DummyAvailabilityZoneName = "dummy-availability-zone"
)

var (
	converter runtime.UnstructuredConverter = runtime.DefaultUnstructuredConverter
)

func DummyContentSourceProviderAndBinding(uuid, namespace string) (
	*vmopv1.ContentSource,
	*vmopv1.ContentLibraryProvider,
	*vmopv1.ContentSourceBinding) {

	contentSourceName := "dummy-content-source"
	contentLibraryProvider := &vmopv1.ContentLibraryProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dummy-content-library-provider",
			OwnerReferences: []metav1.OwnerReference{{
				Name: contentSourceName,
				Kind: "ContentSource",
			}},
		},
		Spec: vmopv1.ContentLibraryProviderSpec{
			UUID: uuid,
		},
	}

	contentSource := &vmopv1.ContentSource{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentSourceName,
		},
		Spec: vmopv1.ContentSourceSpec{
			ProviderRef: vmopv1.ContentProviderReference{
				Name: contentLibraryProvider.Name,
				Kind: "ContentLibraryProvider",
			},
		},
	}

	csBinding := &vmopv1.ContentSourceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      contentSourceName,
			Namespace: namespace,
		},
		ContentSourceRef: vmopv1.ContentSourceReference{
			Kind: "ContentSource",
			Name: contentSourceName,
		},
	}

	return contentSource, contentLibraryProvider, csBinding
}

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

func DummyVirtualMachineClassBinding(className, namespace string) *vmopv1.VirtualMachineClassBinding {
	return &vmopv1.VirtualMachineClassBinding{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Namespace:    namespace,
		},
		ClassRef: vmopv1.ClassReference{
			Name: className,
			Kind: "VirtualMachineClass",
		},
	}
}

func DummyVirtualMachineClassAndBinding(className, namespace string) (
	*vmopv1.VirtualMachineClass,
	*vmopv1.VirtualMachineClassBinding) {

	class := &vmopv1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: className,
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

	binding := &vmopv1.VirtualMachineClassBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      className,
			Namespace: namespace,
		},
		ClassRef: vmopv1.ClassReference{
			Name: className,
			Kind: "VirtualMachineClass",
		},
	}

	return class, binding
}

func DummyInstanceStorage() vmopv1.InstanceStorage {
	return vmopv1.InstanceStorage{
		StorageClass: DummyStorageClassName,
		Volumes: []vmopv1.InstanceStorageVolume{
			{
				Size: resource.MustParse("256Gi"),
			},
			{
				Size: resource.MustParse("512Gi"),
			},
		},
	}
}

func DummyInstanceStorageVirtualMachineVolumes() []vmopv1.VirtualMachineVolume {
	return []vmopv1.VirtualMachineVolume{
		{
			Name: "instance-pvc-1",
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
		{
			Name: "instance-pvc-2",
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
	}
}

func DummyBasicVirtualMachine(name, namespace string) *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: vmopv1.VirtualMachineSpec{
			ImageName:    DummyImageName,
			ClassName:    DummyClassName,
			PowerState:   vmopv1.VirtualMachinePoweredOn,
			PowerOffMode: vmopv1.VirtualMachinePowerOpModeHard,
			SuspendMode:  vmopv1.VirtualMachinePowerOpModeHard,
		},
	}
}

func DummyVirtualMachine() *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Labels:       map[string]string{},
			Annotations:  map[string]string{},
		},
		Spec: vmopv1.VirtualMachineSpec{
			ImageName:    DummyImageName,
			ClassName:    DummyClassName,
			PowerState:   vmopv1.VirtualMachinePoweredOn,
			PowerOffMode: vmopv1.VirtualMachinePowerOpModeHard,
			SuspendMode:  vmopv1.VirtualMachinePowerOpModeHard,
			NetworkInterfaces: []vmopv1.VirtualMachineNetworkInterface{
				{
					NetworkName: DummyNetworkName,
					NetworkType: "",
				},
				{
					NetworkName: DummyNetworkName + "-2",
					NetworkType: "",
				},
			},
			Volumes: []vmopv1.VirtualMachineVolume{
				{
					Name: DummyVolumeName,
					PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: DummyPVCName,
						},
					},
				},
			},
			VmMetadata: &vmopv1.VirtualMachineMetadata{
				ConfigMapName: DummyMetadataCMName,
				Transport:     "ExtraConfig",
			},
		},
	}
}

func AddDummyInstanceStorageVolume(vm *vmopv1.VirtualMachine) {
	vm.Spec.Volumes = append(vm.Spec.Volumes, DummyInstanceStorageVirtualMachineVolumes()...)
}

func DummyVirtualMachineService() *vmopv1.VirtualMachineService {
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

func DummyVirtualMachineSetResourcePolicy2(name, namespace string) *vmopv1.VirtualMachineSetResourcePolicy {
	return &vmopv1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vmopv1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmopv1.ResourcePoolSpec{
				Name: name,
			},
			Folder: vmopv1.FolderSpec{
				Name: name,
			},
		},
	}
}

func DummyVirtualMachineImage(imageName string) *vmopv1.VirtualMachineImage {
	return &vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: imageName,
		},
		Spec: vmopv1.VirtualMachineImageSpec{
			ProductInfo: vmopv1.VirtualMachineImageProductInfo{
				FullVersion: DummyDistroVersion,
			},
			OSInfo: vmopv1.VirtualMachineImageOSInfo{
				Type: DummyOSType,
			},
		},
		Status: vmopv1.VirtualMachineImageStatus{
			ImageName: imageName,
		},
	}
}

func DummyClusterVirtualMachineImage(imageName string) *vmopv1.ClusterVirtualMachineImage {
	return &vmopv1.ClusterVirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name: imageName,
		},
		Spec: vmopv1.VirtualMachineImageSpec{
			ProductInfo: vmopv1.VirtualMachineImageProductInfo{
				FullVersion: DummyDistroVersion,
			},
			OSInfo: vmopv1.VirtualMachineImageOSInfo{
				Type: DummyOSType,
			},
		},
		Status: vmopv1.VirtualMachineImageStatus{
			ImageName: imageName,
		},
	}
}

func DummyStorageClass() *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: DummyStorageClassName,
		},
		Provisioner: "foo",
		Parameters: map[string]string{
			"storagePolicyID": "id42",
		},
	}
}

func DummyResourceQuota(namespace, rlName string) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DummyResourceQuotaName,
			Namespace: namespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceName(rlName): resource.MustParse("1"),
			},
		},
	}
}

func DummyAvailabilityZone() *topologyv1.AvailabilityZone {
	return &topologyv1.AvailabilityZone{
		ObjectMeta: metav1.ObjectMeta{
			Name: DummyAvailabilityZoneName,
		},
		Spec: topologyv1.AvailabilityZoneSpec{
			ClusterComputeResourceMoIDs: []string{"cluster"},
			Namespaces:                  map[string]topologyv1.NamespaceInfo{},
		},
	}
}

func DummyWebConsoleRequest(namespace, wcrName, vmName, pubKey string) *vmopv1.WebConsoleRequest {
	return &vmopv1.WebConsoleRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wcrName,
			Namespace: namespace,
		},
		Spec: vmopv1.WebConsoleRequestSpec{
			VirtualMachineName: vmName,
			PublicKey:          pubKey,
		},
	}
}

func WebConsoleRequestKeyPair() (privateKey *rsa.PrivateKey, publicKeyPem string) {
	privateKey, _ = rsa.GenerateKey(rand.Reader, 2048)
	publicKey := privateKey.PublicKey
	publicKeyPem = string(pem.EncodeToMemory(
		&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(&publicKey),
		},
	))
	return privateKey, publicKeyPem
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

func DummyPersistentVolumeClaim() *corev1.PersistentVolumeClaim {
	var storageClass = "dummy-storage-class"
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "validate-webhook-pvc",
			Labels: make(map[string]string),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("5Gi"),
				},
			},
			StorageClassName: &storageClass,
		},
	}
}

func DummyVirtualMachinePublishRequest(name, namespace, sourceName, itemName, clName string) *vmopv1.VirtualMachinePublishRequest {
	return &vmopv1.VirtualMachinePublishRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{"virtualmachinepublishrequest.vmoperator.vmware.com"},
		},
		Spec: vmopv1.VirtualMachinePublishRequestSpec{
			Source: vmopv1.VirtualMachinePublishRequestSource{
				Name:       sourceName,
				APIVersion: "vmoperator.vmware.com/v1alpha1",
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

func DummyContentLibrary(name, namespace, uuid string) *imgregv1a1.ContentLibrary {
	return &imgregv1a1.ContentLibrary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: imgregv1a1.ContentLibrarySpec{
			UUID:     types.UID(uuid),
			Writable: true,
		},
		Status: imgregv1a1.ContentLibraryStatus{
			Conditions: []imgregv1a1.Condition{
				{
					Type:   imgregv1a1.ReadyCondition,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

func DummyClusterContentLibrary(name, uuid string) *imgregv1a1.ClusterContentLibrary {
	return &imgregv1a1.ClusterContentLibrary{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: imgregv1a1.ClusterContentLibrarySpec{
			UUID: types.UID(uuid),
		},
	}
}
