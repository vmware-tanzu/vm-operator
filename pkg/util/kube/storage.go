// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/internal"
)

const (
	csiRequestedTopologyAnnotation  = "csi.vsphere.volume-requested-topology"
	csiAccessibleTopologyAnnotation = "csi.vsphere.volume-accessible-topology"
)

func getPVCZones(pvc corev1.PersistentVolumeClaim, key string) sets.Set[string] {
	v, ok := pvc.Annotations[key]
	if !ok {
		return nil
	}

	zones := sets.Set[string]{}

	var topology []map[string]string
	if json.Unmarshal([]byte(v), &topology) == nil {
		for i := range topology {
			if z := topology[i]["topology.kubernetes.io/zone"]; z != "" {
				zones.Insert(z)
			}
		}
	}

	return zones
}

func getPVCRequestedZones(pvc corev1.PersistentVolumeClaim) sets.Set[string] {
	return getPVCZones(pvc, csiRequestedTopologyAnnotation)
}

func getPVCAccessibleZones(pvc corev1.PersistentVolumeClaim) sets.Set[string] {
	return getPVCZones(pvc, csiAccessibleTopologyAnnotation)
}

func GetPVCZoneConstraints(
	storageClasses map[string]storagev1.StorageClass,
	pvcs []corev1.PersistentVolumeClaim) (sets.Set[string], error) {

	var zones sets.Set[string]

	for _, pvc := range pvcs {
		var z sets.Set[string]

		// We don't expect anything else but check since we check CSI specific annotations below.
		if v, ok := pvc.Annotations["volume.kubernetes.io/storage-provisioner"]; ok && v != "csi.vsphere.vmware.com" {
			continue
		}

		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			z = getPVCAccessibleZones(pvc)
			if z.Len() > 1 {
				if reqZones := getPVCRequestedZones(pvc); reqZones.Len() > 0 {
					// A PVC's accessible zones are all the zones that the PV datastore is accessible on,
					// which may include zones that were not requested. Constrain our candidates to the
					// accessible zones which were also requested.
					z = z.Intersection(reqZones)
				}
			}

		case corev1.ClaimPending:
			var isImmediate bool

			// We need to determine if the PVC's StorageClass is Immediate and we just
			// caught this PVC before it was bound, or otherwise it is WFFC and we need
			// to limit our placement candidates to its requested zones.

			// While we have the DefaultStorageClass feature gate enabled, we don't mark an
			// StorageClass with storageclass.kubernetes.io/is-default-class, so don't bother
			// trying to resolve that here now.
			if scName := pvc.Spec.StorageClassName; scName != nil && *scName != "" {
				if sc, ok := storageClasses[*scName]; ok {
					isImmediate = sc.VolumeBindingMode == nil || *sc.VolumeBindingMode == storagev1.VolumeBindingImmediate
				}
			}

			if isImmediate {
				// Must wait for this PVC to get bound so we know its accessible zones.
				return nil, fmt.Errorf("PVC %s is not bound", pvc.Name)
			}

			z = getPVCRequestedZones(pvc)

		default: // corev1.ClaimLost
			// TBD: For now preserve our prior behavior: don't explicitly fail here, and create the
			// VM and wait for the volume to be attached prior to powering on the VM.
			continue
		}

		if z.Len() > 0 {
			if zones == nil {
				zones = z
			} else {
				t := zones.Intersection(z)
				if t.Len() == 0 {
					return nil, fmt.Errorf("no allowed zones remaining after applying PVC zone constraints")
				}

				zones = t
			}
		}
	}

	return zones, nil
}

// ErrMissingParameter is returned from GetStoragePolicyID if the StorageClass
// does not have the storage policy ID parameter.
type ErrMissingParameter struct {
	StorageClassName string
	ParameterName    string
}

func (e ErrMissingParameter) String() string {
	return fmt.Sprintf(
		"StorageClass %q does not have %q parameter",
		e.StorageClassName, e.ParameterName)
}

func (e ErrMissingParameter) Error() string {
	return e.String()
}

// GetStoragePolicyID returns the storage policy ID for a given StorageClass.
// If no ID is found, an error is returned.
func GetStoragePolicyID(obj storagev1.StorageClass) (string, error) {
	policyID, ok := obj.Parameters[internal.StoragePolicyIDParameter]
	if !ok {
		return "", ErrMissingParameter{
			StorageClassName: obj.Name,
			ParameterName:    internal.StoragePolicyIDParameter,
		}
	}
	return policyID, nil
}

// SetStoragePolicyID sets the storage policy ID on the given StorageClass.
// An empty id removes the parameter from the StorageClass.
func SetStoragePolicyID(obj *storagev1.StorageClass, id string) {
	if obj == nil {
		panic("storageClass is nil")
	}
	if id == "" {
		delete(obj.Parameters, internal.StoragePolicyIDParameter)
		return
	}
	if obj.Parameters == nil {
		obj.Parameters = map[string]string{}
	}
	obj.Parameters[internal.StoragePolicyIDParameter] = id
}

// MarkEncryptedStorageClass records the provided StorageClass as encrypted.
func MarkEncryptedStorageClass(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	storageClass storagev1.StorageClass,
	encrypted bool) error {

	var (
		obj = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: pkgcfg.FromContext(ctx).PodNamespace,
				Name:      internal.EncryptedStorageClassNamesConfigMapName,
			},
		}
		ownerRef = internal.GetOwnerRefForStorageClass(storageClass)
	)

	// Get the ConfigMap.
	err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(&obj), &obj)

	if err != nil {
		// Return any error other than 404.
		if !apierrors.IsNotFound(err) {
			return err
		}

		//
		// The ConfigMap was not found.
		//

		if !encrypted {
			// If the goal is to mark the StorageClass as not encrypted, then we
			// do not need to actually create the underlying ConfigMap if it
			// does not exist.
			return nil
		}

		// The ConfigMap does not already exist and the goal is to mark the
		// StorageClass as encrypted, so go ahead and create the ConfigMap and
		// return early.
		obj.OwnerReferences = []metav1.OwnerReference{ownerRef}
		return k8sClient.Create(ctx, &obj)
	}

	//
	// The ConfigMap already exists, so check if it needs to be updated.
	//

	storageClassIsOwner := slices.Contains(obj.OwnerReferences, ownerRef)

	switch {
	case encrypted && storageClassIsOwner:
		// The StorageClass should be marked encrypted, which means it should
		// be in the ConfigMap. Since the StorageClass is currently set in the
		// ConfigMap, we can return early.
		return nil
	case !encrypted && !storageClassIsOwner:
		// The StorageClass should be marked as not encrypted, which means it
		// should not be in the ConfigMap. Since the StorageClass is not
		// currently in the ConfigMap, we can return return early.
		return nil
	}

	// Create the patch used to update the ConfigMap.
	objPatch := ctrlclient.StrategicMergeFrom(obj.DeepCopyObject().(ctrlclient.Object))
	if encrypted {
		// Add the StorageClass as an owner of the ConfigMap.
		obj.OwnerReferences = append(obj.OwnerReferences, ownerRef)
	} else {
		// Remove the StorageClass as an owner of the ConfigMap.
		obj.OwnerReferences = slices.DeleteFunc(
			obj.OwnerReferences,
			func(o metav1.OwnerReference) bool { return o == ownerRef })
	}

	// Patch the ConfigMap with the change.
	return k8sClient.Patch(ctx, &obj, objPatch)
}

// IsEncryptedStorageClass returns true if the provided StorageClass name was
// marked as encrypted. If encryption is supported, the StorageClass's profile
// ID is also returned.
func IsEncryptedStorageClass(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	name string) (bool, string, error) {

	var obj storagev1.StorageClass
	if err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKey{Name: name},
		&obj); err != nil {

		return false, "", ctrlclient.IgnoreNotFound(err)
	}

	return isEncryptedStorageClass(ctx, k8sClient, obj)
}

// IsEncryptedStorageProfile returns true if the provided storage profile ID was
// marked as encrypted.
func IsEncryptedStorageProfile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	profileID string) (bool, error) {

	var obj storagev1.StorageClassList
	if err := k8sClient.List(ctx, &obj); err != nil {
		return false, err
	}

	for i := range obj.Items {
		if pid, _ := GetStoragePolicyID(obj.Items[i]); pid == profileID {
			ok, _, err := isEncryptedStorageClass(
				ctx,
				k8sClient,
				obj.Items[i])
			return ok, err
		}
	}

	return false, nil
}

func isEncryptedStorageClass(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	storageClass storagev1.StorageClass) (bool, string, error) {

	var (
		obj    corev1.ConfigMap
		objKey = ctrlclient.ObjectKey{
			Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			Name:      internal.EncryptedStorageClassNamesConfigMapName,
		}
		ownerRef = internal.GetOwnerRefForStorageClass(storageClass)
	)

	if err := k8sClient.Get(ctx, objKey, &obj); err != nil {
		return false, "", ctrlclient.IgnoreNotFound(err)
	}

	if slices.Contains(obj.OwnerReferences, ownerRef) {
		profileID, err := GetStoragePolicyID(storageClass)
		return true, profileID, err
	}

	return false, "", nil
}
