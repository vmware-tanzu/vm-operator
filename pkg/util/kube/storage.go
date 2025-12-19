// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"

	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
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
		if pvc.Spec.DataSourceRef != nil {
			// Do not worry about PVCs with data source refs.
			continue
		}

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
	policyID := obj.Parameters[internal.StoragePolicyIDParameter]
	if policyID == "" {
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

	profileID, _ := GetStoragePolicyID(obj)
	if profileID == "" {
		return false, "", nil
	}

	ok, err := IsEncryptedStorageProfile(ctx, k8sClient, profileID)
	if err != nil {
		return false, "", err
	}

	return ok, profileID, nil
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
			var (
				obj infrav1.StoragePolicy
				key = ctrlclient.ObjectKey{
					Namespace: pkgcfg.FromContext(ctx).PodNamespace,
					Name:      GetStoragePolicyObjectName(pid),
				}
			)
			if key.Name != "" {
				if err := k8sClient.Get(ctx, key, &obj); err != nil {
					return false, ctrlclient.IgnoreNotFound(err)
				}
				return obj.Status.Encrypted, nil
			}
		}
	}

	return false, nil
}

// GetStoragePolicyObjectName returns the expected name of a StoragePolicy
// object based on the policy's profile ID.
func GetStoragePolicyObjectName(profileID string) string {
	if profileID == "" {
		return ""
	}

	if _, err := uuid.Parse(profileID); err != nil {

		if !testing.Testing() {
			// If not used for testing then an invalid profile ID should
			// return an empty string to indicate this is invalid.
			return ""
		}

		logger := pkglog.FromContextOrDefault(context.Background()).
			WithName("GetStoragePolicyObjectName").
			V(0)
		logger.Error(
			nil,
			"!!! GetStoragePolicyObjectName called with invalid profileID !!!",
			"profileID", profileID)

		// If the provided profileID is not a valid UUID, then just hash it to
		// construct the StoragePolicy name. This is for testing.
		h := xxhash.New()
		if _, err := h.Write([]byte(profileID)); err != nil {
			panic(err)
		}
		return fmt.Sprintf("pol-%x", h.Sum(nil))
	}

	s := strings.ReplaceAll(strings.ToLower(profileID), "-", "")
	return fmt.Sprintf("pol-%s-%s", s[0:8], s[16:32])
}

// MarkEncryptedStorageClass records the provided StorageClass as encrypted.
func MarkEncryptedStorageClass(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	storageClass storagev1.StorageClass,
	encrypted bool) error {

	profileID, err := GetStoragePolicyID(storageClass)
	if err != nil {
		return err
	}

	var (
		obj = infrav1.StoragePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: pkgcfg.FromContext(ctx).PodNamespace,
				Name:      GetStoragePolicyObjectName(profileID),
			},
		}
		ownerRef = internal.GetOwnerRefForStorageClass(storageClass)
	)

	// Get the StoragePolicy.
	if err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKeyFromObject(&obj),
		&obj); err != nil {

		// Return any error other than 404.
		if !apierrors.IsNotFound(err) {
			return err
		}

		//
		// The StoragePolicy was not found.
		//

		// The StoragePolicy does not already exist, so go ahead and create the
		// StoragePolicy and return early.
		obj.OwnerReferences = []metav1.OwnerReference{ownerRef}
		obj.Spec.ID = profileID
		if err := k8sClient.Create(ctx, &obj); err != nil {
			return fmt.Errorf("failed to create StoragePolicy \"%s/%s\" "+
				"for StorageClass %q: %w",
				obj.Namespace,
				obj.Name,
				storageClass.Name,
				err)
		}

		obj.Status.Encrypted = encrypted
		if err := k8sClient.Status().Update(ctx, &obj); err != nil {
			return fmt.Errorf("failed to update StoragePolicy \"%s/%s\" "+
				"for StorageClass %q: %w",
				obj.Namespace,
				obj.Name,
				storageClass.Name,
				err)
		}

		return nil
	}

	//
	// The StoragePolicy already exists, so check if it needs to be updated.
	//

	var (
		alreadyEncrypted    = obj.Status.Encrypted
		storageClassIsOwner = slices.Contains(obj.OwnerReferences, ownerRef)
	)

	if encrypted && alreadyEncrypted && storageClassIsOwner {
		// The StorageClass should be marked encrypted, and the
		// StoragePolicy already reflects that. There is nothing to do, so
		// return early.
		return nil

	} else if !encrypted && !alreadyEncrypted && storageClassIsOwner {
		// The StorageClass should NOT be marked encrypted, and the
		// StoragePolicy already reflects that. There is nothing to do, so
		// return early.
		return nil
	}

	// Create the patch used to update the StoragePolicy.
	objPatch := ctrlclient.StrategicMergeFrom(
		obj.DeepCopyObject().(ctrlclient.Object))

	if !storageClassIsOwner {
		obj.OwnerReferences = append(obj.OwnerReferences, ownerRef)
	}
	obj.Status.Encrypted = encrypted

	// Patch the StoragePolicy with the change.
	if err := k8sClient.Patch(ctx, &obj, objPatch); err != nil {
		return fmt.Errorf("failed to patch StoragePolicy \"%s/%s\" "+
			"for StorageClass %q: %w",
			obj.Namespace,
			obj.Name,
			storageClass.Name,
			err)
	}

	return nil
}
