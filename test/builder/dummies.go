// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/google/uuid"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	DummyVMIName                   = "vmi-0123456789"
	DummyCVMIName                  = "vmi-9876543210"
	DummyImageName                 = "dummy-image-name"
	DummyClassName                 = "dummyClassName"
	DummyVolumeName                = "dummy-volume-name"
	DummyPVCName                   = "dummyPVCName"
	DummyDistroVersion             = "dummyDistroVersion"
	DummyOSType                    = "centosGuest"
	DummyStorageClassName          = "dummy-storage-class"
	DummyResourceQuotaName         = "dummy-resource-quota"
	DummyZoneName                  = "dummy-zone"
	DummyDeletedZoneName           = "dummy-zone-deleted"
	DummyNamespaceName             = "dummy-ns"
	DummyVMGroupName               = "dummy-vm-group"
	DummyVMGroupPublishRequestName = "dummy-vm-group-publish-request"
	DummyContentLibraryName        = "dummy-cl"
	DummyImageVM0Name              = "dummy-image-vm0-name"
	DummyImageVM1Name              = "dummy-image-vm1-name"
	DummyVirtualMachine0Name       = "dummy-vm0"
	DummyVirtualMachine1Name       = "dummy-vm1"
)

const (
	vmiKind  = "VirtualMachineImage"
	cvmiKind = "Cluster" + vmiKind
)

func DummyStorageClass() *storagev1.StorageClass {
	return DummyStorageClassWithID("id42")
}

func DummyStorageClassWithID(policyID string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: DummyStorageClassName,
		},
		Provisioner: "foo",
		Parameters: map[string]string{
			"storagePolicyID": policyID,
		},
	}
}

func DummyResourceQuota(namespace string, rlNames ...string) *corev1.ResourceQuota {
	obj := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DummyResourceQuotaName,
			Namespace: namespace,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{},
		},
	}
	for i := range rlNames {
		obj.Spec.Hard[corev1.ResourceName(rlNames[i])] = resource.MustParse("1")
	}
	return obj
}

func DummyAvailabilityZone() *topologyv1.AvailabilityZone {
	return DummyNamedAvailabilityZone(DummyZoneName)
}

func DummyNamedAvailabilityZone(name string) *topologyv1.AvailabilityZone {
	return &topologyv1.AvailabilityZone{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: topologyv1.AvailabilityZoneSpec{
			ClusterComputeResourceMoIDs: []string{"cluster"},
			Namespaces:                  map[string]topologyv1.NamespaceInfo{},
		},
	}
}

// DummyZone uses the same name with AZ.
func DummyZone(namespace string) *topologyv1.Zone {
	return &topologyv1.Zone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DummyZoneName,
			Namespace: namespace,
		},
		Spec: topologyv1.ZoneSpec{},
	}
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
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("5Gi"),
				},
			},
			StorageClassName: &storageClass,
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
			BaseContentLibrarySpec: imgregv1a1.BaseContentLibrarySpec{
				UUID: types.UID(uuid),
			},
			Writable: true,
		},
		Status: imgregv1a1.ContentLibraryStatus{
			Conditions: []imgregv1a1.Condition{
				{
					Type:               imgregv1a1.ReadyCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
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
			BaseContentLibrarySpec: imgregv1a1.BaseContentLibrarySpec{
				UUID: types.UID(uuid),
			},
		},
	}
}

func DummyStoragePolicyQuota(name, namespace, policyID string) *spqv1.StoragePolicyQuota {
	return &spqv1.StoragePolicyQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spqv1.StoragePolicyQuotaSpec{
			StoragePolicyId: policyID,
		},
	}
}

func DummyWebConsoleRequest(namespace, wcrName, vmName, pubKey string) *vmopv1a1.WebConsoleRequest {
	return &vmopv1a1.WebConsoleRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wcrName,
			Namespace: namespace,
		},
		Spec: vmopv1a1.WebConsoleRequestSpec{
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

func DummyVirtualMachineClass(name string) *vmopv1.VirtualMachineClass {
	return &vmopv1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
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

func DummyVirtualMachineClassGenName() *vmopv1.VirtualMachineClass {
	class := DummyVirtualMachineClass("")
	class.GenerateName = "test-"
	return class
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

func DummyBasicVirtualMachine(name, namespace string) *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			Kind: "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: vmopv1.VirtualMachineSpec{
			InstanceUUID: uuid.NewString(),
			Image: &vmopv1.VirtualMachineImageRef{
				Kind: vmiKind,
				Name: DummyVMIName,
			},
			ImageName:    DummyImageName,
			ClassName:    DummyClassName,
			PowerState:   vmopv1.VirtualMachinePowerStateOn,
			PowerOffMode: vmopv1.VirtualMachinePowerOpModeHard,
			SuspendMode:  vmopv1.VirtualMachinePowerOpModeHard,
		},
	}
}

func DummyVirtualMachine() *vmopv1.VirtualMachine {
	return &vmopv1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			Kind: "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Labels:       map[string]string{},
			Annotations:  map[string]string{},
		},
		Spec: vmopv1.VirtualMachineSpec{
			Image: &vmopv1.VirtualMachineImageRef{
				Kind: vmiKind,
				Name: DummyVMIName,
			},
			ImageName:          DummyImageName,
			ClassName:          DummyClassName,
			PowerState:         vmopv1.VirtualMachinePowerStateOn,
			PowerOffMode:       vmopv1.VirtualMachinePowerOpModeHard,
			SuspendMode:        vmopv1.VirtualMachinePowerOpModeHard,
			MinHardwareVersion: 13,
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
			Network: &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
					{
						Name: "eth0",
					},
				},
			},
			Cdrom: []vmopv1.VirtualMachineCdromSpec{
				{
					Name: "cdrom1",
					Image: vmopv1.VirtualMachineImageRef{
						Kind: vmiKind,
						Name: DummyVMIName,
					},
					Connected:         ptr.To(true),
					AllowGuestControl: ptr.To(true),
				},
				{
					Name: "cdrom2",
					Image: vmopv1.VirtualMachineImageRef{
						Kind: cvmiKind,
						Name: DummyCVMIName,
					},
					Connected:         ptr.To(true),
					AllowGuestControl: ptr.To(true),
				},
			},
			GuestID: string(vimtypes.VirtualMachineGuestOsIdentifierCentosGuest),
		},
	}
}

func DummyVirtualMachineReplicaSet() *vmopv1.VirtualMachineReplicaSet {
	return &vmopv1.VirtualMachineReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind: "VirtualMachineReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-",
			Labels:       map[string]string{},
			Annotations:  map[string]string{},
		},
		Spec: vmopv1.VirtualMachineReplicaSetSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: make(map[string]string),
			},
			Template: vmopv1.VirtualMachineTemplateSpec{
				ObjectMeta: vmopv1common.ObjectMeta{
					Labels:      make(map[string]string),
					Annotations: make(map[string]string),
				},
				Spec: vmopv1.VirtualMachineSpec{
					Image: &vmopv1.VirtualMachineImageRef{
						Kind: vmiKind,
						Name: DummyVMIName,
					},
					ImageName:  DummyImageName,
					ClassName:  DummyClassName,
					PowerState: vmopv1.VirtualMachinePowerStateOn,
					Network: &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name: "eth0",
							},
						},
					},
				},
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
			Folder:              "dummy-folder",
			ClusterModuleGroups: []string{"dummy-cluster-modules"},
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
			Folder: name,
		},
	}
}

func DummyVirtualMachinePublishRequest(name, namespace, sourceName, itemName, clName string) *vmopv1.VirtualMachinePublishRequest {
	return &vmopv1.VirtualMachinePublishRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{"vmoperator.vmware.com/virtualmachinepublishrequest"},
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

func VirtualMachineGroupPublishRequest(name, namespace string, vms []string) *vmopv1.VirtualMachineGroupPublishRequest {
	return &vmopv1.VirtualMachineGroupPublishRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vmopv1.VirtualMachineGroupPublishRequestSpec{
			Source:          DummyVMGroupName,
			Target:          DummyContentLibraryName,
			VirtualMachines: vms,
		},
	}
}

func DummyVirtualMachineImage(imageName string) *vmopv1.VirtualMachineImage {
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

func DummyClusterVirtualMachineImage(imageName string) *vmopv1.ClusterVirtualMachineImage {
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

func DummyVirtualMachineWebConsoleRequest(namespace, wcrName, vmName, pubKey string) *vmopv1.VirtualMachineWebConsoleRequest {
	return &vmopv1.VirtualMachineWebConsoleRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wcrName,
			Namespace: namespace,
		},
		Spec: vmopv1.VirtualMachineWebConsoleRequestSpec{
			Name:      vmName,
			PublicKey: pubKey,
		},
	}
}

func DummyVirtualMachineSnapshot(namespace, name, vmName string) *vmopv1.VirtualMachineSnapshot {
	return &vmopv1.VirtualMachineSnapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualMachineSnapshot",
			APIVersion: "vmoperator.vmware.com/v1alpha4",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Finalizers: []string{
				"vmoperator.vmware.com/virtualmachinesnapshot",
			},
			Annotations: map[string]string{},
		},
		Spec: vmopv1.VirtualMachineSnapshotSpec{
			VMRef: &vmopv1common.LocalObjectRef{
				APIVersion: "vmoperator.vmware.com/v1alpha4",
				Kind:       "VirtualMachine",
				Name:       vmName,
			},
			Quiesce: &vmopv1.QuiesceSpec{
				Timeout: &metav1.Duration{
					Duration: 10 * time.Minute,
				},
			},
		},
	}
}

func DummyImageAndItemObjectsForCdromBacking(
	name, ns, kind, storageURI, libItemUUID string,
	imgReady, imgCached bool, imgSize resource.Quantity, imgHasProviderRef, itemObjExists bool,
	itemType imgregv1a1.ContentLibraryItemType) []ctrlclient.Object {
	var imageObj, itemObj ctrlclient.Object

	// Populate minimal fields in image and content library item objects to
	// be able to sync and retrieve CD-ROM backing file name.
	imgSpec := vmopv1.VirtualMachineImageSpec{}
	if imgHasProviderRef {
		imgSpec.ProviderRef = &vmopv1common.LocalObjectRef{
			Name: name,
		}
	}

	imgStatus := vmopv1.VirtualMachineImageStatus{
		Type:           string(itemType),
		ProviderItemID: libItemUUID,
	}
	if imgReady {
		imgStatus.Conditions = []metav1.Condition{
			{
				Type:   vmopv1.ReadyConditionType,
				Status: metav1.ConditionTrue,
			},
		}
	}

	itemSpec := imgregv1a1.ContentLibraryItemSpec{
		UUID: types.UID(libItemUUID),
	}

	itemStatus := imgregv1a1.ContentLibraryItemStatus{
		Type:        itemType,
		Cached:      imgCached,
		SizeInBytes: imgSize,
		FileInfo: []imgregv1a1.FileInfo{
			{
				StorageURI:  storageURI,
				Cached:      imgCached,
				SizeInBytes: imgSize,
			},
		},
	}

	if kind == vmiKind {
		if imgSpec.ProviderRef != nil {
			imgSpec.ProviderRef.Kind = "ContentLibraryItem"
		}

		imageObj = &vmopv1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Spec:   imgSpec,
			Status: imgStatus,
		}

		itemObj = &imgregv1a1.ContentLibraryItem{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
			},
			Spec:   itemSpec,
			Status: itemStatus,
		}
	} else {

		if imgSpec.ProviderRef != nil {
			imgSpec.ProviderRef.Kind = "ClusterContentLibraryItem"
		}

		imageObj = &vmopv1.ClusterVirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec:   imgSpec,
			Status: imgStatus,
		}

		itemObj = &imgregv1a1.ClusterContentLibraryItem{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec:   itemSpec,
			Status: itemStatus,
		}
	}

	if itemObjExists {
		return []ctrlclient.Object{imageObj, itemObj}
	}

	return []ctrlclient.Object{imageObj}
}
