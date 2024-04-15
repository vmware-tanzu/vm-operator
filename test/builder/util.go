// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"

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

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	cnsstoragev1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/storagepolicy/v1alpha1"
)

const (
	DummyVMIID                = "vmi-0123456789"
	DummyImageName            = "dummy-image-name"
	DummyClassName            = "dummyClassName"
	DummyVolumeName           = "dummy-volume-name"
	DummyPVCName              = "dummyPVCName"
	DummyDistroVersion        = "dummyDistroVersion"
	DummyOSType               = "centosGuest"
	DummyStorageClassName     = "dummy-storage-class"
	DummyResourceQuotaName    = "dummy-resource-quota"
	DummyAvailabilityZoneName = "dummy-availability-zone"
)

var (
	converter runtime.UnstructuredConverter = runtime.DefaultUnstructuredConverter
)

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
	return DummyNamedAvailabilityZone(DummyAvailabilityZoneName)
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

func DummyStoragePolicyQuota(quotaName, quotaNs, className string) *cnsstoragev1.StoragePolicyQuota {
	return &cnsstoragev1.StoragePolicyQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: quotaNs,
		},
		Spec: cnsstoragev1.StoragePolicyQuotaSpec{StoragePolicyId: "uuid-abcd-1234"},
		Status: cnsstoragev1.StoragePolicyQuotaStatus{
			SCLevelQuotaStatuses: []cnsstoragev1.SCLevelQuotaStatus{
				{
					StorageClassName: className,
				},
			},
			ResourceTypeLevelQuotaStatuses: []cnsstoragev1.ResourceTypeLevelQuotaStatus{
				{
					ResourceExtensionName: "volume.cns.vsphere.vmware.com",
					ResourceTypeSCLevelQuotaStatuses: []cnsstoragev1.SCLevelQuotaStatus{{
						StorageClassName: className,
					}},
				},
				{
					ResourceExtensionName: "volume.cns.vsphere.vmware.com",
					ResourceTypeSCLevelQuotaStatuses: []cnsstoragev1.SCLevelQuotaStatus{{
						StorageClassName: className + "-abcde",
					}},
				},
			},
		},
	}
}

func applyFeatureStateFnsToCRD(
	ctx context.Context,
	crd apiextensionsv1.CustomResourceDefinition,
	fns ...func(context.Context, apiextensionsv1.CustomResourceDefinition) apiextensionsv1.CustomResourceDefinition) apiextensionsv1.CustomResourceDefinition {

	for i := range fns {
		crd = fns[i](ctx, crd)
	}
	return crd
}

// indexOfVersion returns the index of the specified schema version for a given
// CRD. This function is useful for writing the functions that are passed into
// the applyFeatureStateFnsToCRD function.
//
//nolint:unused
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
	files, err := os.ReadDir(rootFilePath)
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
	b, err := os.ReadFile(fp)
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
