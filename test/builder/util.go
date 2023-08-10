// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
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

func applyFeatureStateFnsToCRD(
	crd apiextensionsv1.CustomResourceDefinition,
	fssMap map[string]bool,
	fns ...func(apiextensionsv1.CustomResourceDefinition, map[string]bool) apiextensionsv1.CustomResourceDefinition) apiextensionsv1.CustomResourceDefinition {

	for i := range fns {
		crd = fns[i](crd, fssMap)
	}
	return crd
}

func applyV1Alpha2FSSToCRD(
	crd apiextensionsv1.CustomResourceDefinition,
	fssMap map[string]bool) apiextensionsv1.CustomResourceDefinition {

	idxV1a1 := indexOfVersion(crd, "v1alpha1")
	idxV1a2 := indexOfVersion(crd, "v1alpha2")

	if enabled, ok := fssMap[lib.VMServiceV1Alpha2FSS]; ok && enabled {
		// Whether the v1a2 version of this CRD is the storage version
		// depends on existence of the v1a2 version of this CRD.
		if idxV1a1 >= 0 {
			crd.Spec.Versions[idxV1a1].Storage = !(idxV1a2 >= 0)
			crd.Spec.Versions[idxV1a1].Served = true
		}

		// If there is a v1a2 version of this CRD, it is the storage version.
		if idxV1a2 >= 0 {
			crd.Spec.Versions[idxV1a2].Storage = true
			crd.Spec.Versions[idxV1a2].Served = true
		}
	} else if idxV1a1 >= 0 {
		crd.Spec.Versions[idxV1a1].Storage = true
		crd.Spec.Versions[idxV1a1].Served = true

		// If there is a v1a2 version of this CRD, remove it.
		if idxV1a2 >= 0 {
			copy(crd.Spec.Versions[idxV1a2:], crd.Spec.Versions[idxV1a2+1:])
			var zeroVal apiextensionsv1.CustomResourceDefinitionVersion
			crd.Spec.Versions[len(crd.Spec.Versions)-1] = zeroVal
			crd.Spec.Versions = crd.Spec.Versions[:len(crd.Spec.Versions)-1]
		}
	}

	return crd
}

func indexOfVersion(
	crd apiextensionsv1.CustomResourceDefinition,
	version string) int {

	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Name == version {
			return i
		}
	}
	return -1
}

func LoadCRDs(rootFilePath string) ([]*apiextensionsv1.CustomResourceDefinition, error) {
	// Read the CRD files.
	files, err := ioutil.ReadDir(rootFilePath)
	if err != nil {
		return nil, err
	}

	// Valid file extensions for CRDs.
	crdExts := sets.NewString(".json", ".yaml", ".yml")

	var out []*apiextensionsv1.CustomResourceDefinition
	for i := range files {
		if !crdExts.Has(filepath.Ext(files[i].Name())) {
			continue
		}

		docs, err := readDocuments(filepath.Join(rootFilePath, files[i].Name()))
		if err != nil {
			return nil, err
		}

		for _, d := range docs {
			var crd apiextensionsv1.CustomResourceDefinition
			if err = yaml.Unmarshal(d, &crd); err != nil {
				return nil, err
			}
			if crd.Spec.Names.Kind == "" || crd.Spec.Group == "" {
				continue
			}
			out = append(out, &crd)
		}
	}
	return out, nil
}

// readDocuments reads documents from file
// copied from https://github.com/kubernetes-sigs/controller-runtime/blob/5bf44d2ffd6201703508e11fbae74fcedc5ce148/pkg/envtest/crd.go#L434-L458
func readDocuments(fp string) ([][]byte, error) {
	//nolint:gosec
	b, err := ioutil.ReadFile(fp)
	if err != nil {
		return nil, err
	}
	docs := [][]byte{}
	reader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(b)))
	for {
		// Read document
		doc, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
