// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	spqutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/spq"
)

const (
	vmiKind           = "VirtualMachineImage"
	cvmiKind          = "Cluster" + vmiKind
	imgNotFoundFormat = "no VM image exists for %q in namespace or cluster scope"
)

// ErrImageNotFound is returned from ResolveImageName if the image cannot be
// found at the namespace or cluster scopes. This type will return true when
// provided to the apierrors.IsNotFound function.
type ErrImageNotFound struct {
	msg string
}

func (e ErrImageNotFound) Error() string {
	return e.msg
}

func (e ErrImageNotFound) Status() metav1.Status {
	return metav1.Status{
		Reason: metav1.StatusReasonNotFound,
		Code:   http.StatusNotFound,
	}
}

// ResolveImageName resolves the provided name of a VM image either to a
// VirtualMachineImage resource or ClusterVirtualMachineImage resource.
func ResolveImageName(
	ctx context.Context,
	k8sClient client.Client,
	namespace, imgName string) (client.Object, error) {

	// Return early if the VM image name is empty.
	if imgName == "" {
		return nil, fmt.Errorf("imgName is empty")
	}

	// Query the image from the object name in order to set the result in
	// spec.image.
	if strings.HasPrefix(imgName, "vmi-") {

		var obj client.Object

		obj = &vmopv1.VirtualMachineImage{}
		if err := k8sClient.Get(
			ctx,
			client.ObjectKey{Namespace: namespace, Name: imgName},
			obj); err != nil {

			if !apierrors.IsNotFound(err) {
				return nil, err
			}

			obj = &vmopv1.ClusterVirtualMachineImage{}
			if err := k8sClient.Get(
				ctx,
				client.ObjectKey{Name: imgName},
				obj); err != nil {

				if !apierrors.IsNotFound(err) {
					return nil, err
				}

				return nil, ErrImageNotFound{
					msg: fmt.Sprintf(imgNotFoundFormat, imgName)}
			}
		}

		return obj, nil
	}

	var obj client.Object

	// Check if a single namespace scope image exists by the status name.
	var vmiList vmopv1.VirtualMachineImageList
	if err := k8sClient.List(ctx, &vmiList, client.InNamespace(namespace),
		client.MatchingFields{
			"status.name": imgName,
		},
	); err != nil {
		return nil, err
	}
	switch len(vmiList.Items) {
	case 0:
		break
	case 1:
		obj = &vmiList.Items[0]
	default:
		return nil, fmt.Errorf(
			"multiple VM images exist for %q in namespace scope", imgName)
	}

	// Check if a single cluster scope image exists by the status name.
	var cvmiList vmopv1.ClusterVirtualMachineImageList
	if err := k8sClient.List(ctx, &cvmiList, client.MatchingFields{
		"status.name": imgName,
	}); err != nil {
		return nil, err
	}
	switch len(cvmiList.Items) {
	case 0:
		break
	case 1:
		if obj != nil {
			return nil, fmt.Errorf(
				"multiple VM images exist for %q in namespace and cluster scope",
				imgName)
		}
		obj = &cvmiList.Items[0]
	default:
		return nil, fmt.Errorf(
			"multiple VM images exist for %q in cluster scope", imgName)
	}

	if obj == nil {
		return nil,
			ErrImageNotFound{msg: fmt.Sprintf(imgNotFoundFormat, imgName)}
	}

	return obj, nil
}

// DetermineHardwareVersion returns the hardware version recommended for the
// provided VirtualMachine based on its own spec.minHardwareVersion, as well as
// the hardware in the provided ConfigSpec and requirements of the given
// VirtualMachineImage.
func DetermineHardwareVersion(
	vm vmopv1.VirtualMachine,
	configSpec vimtypes.VirtualMachineConfigSpec,
	imgStatus vmopv1.VirtualMachineImageStatus) vimtypes.HardwareVersion {

	// Get the minimum hardware version required by the VM from the VM spec.
	vmMinVersion := vimtypes.HardwareVersion(vm.Spec.MinHardwareVersion) //nolint:gosec // disable G115

	// Check to see if the ConfigSpec specifies a hardware version.
	var configSpecVersion vimtypes.HardwareVersion
	if configSpec.Version != "" {
		configSpecVersion, _ = vimtypes.ParseHardwareVersion(configSpec.Version)
	}

	// If the ConfigSpec contained a valid hardware version, then the version
	// the VM will use is determined by comparing the ConfigSpec version and the
	// one from the VM's spec.minHardwareVersion field and returning the largest
	// of the two versions.
	if configSpecVersion.IsValid() {
		return max(vmMinVersion, configSpecVersion)
	}

	// A VM Class with an embedded ConfigSpec should have the version set, so
	// this is a ConfigSpec we created from the HW devices in the class. If the
	// image's version is too old to support passthrough devices or PVCs if
	// configured, bump the version so those devices will work.
	var imageVersion vimtypes.HardwareVersion
	if imgStatus.HardwareVersion != nil {
		imageVersion = vimtypes.HardwareVersion(*imgStatus.HardwareVersion) //nolint:gosec // disable G115
	}

	// VMs with PCI pass-through devices require a minimum hardware version, as
	// do VMs with PVCs. Since the version required by PCI pass-through devices
	// is higher than the one required by PVCs, first check if the VM has any
	// PCI pass-through devices, then check if the VM has any PVCs.
	var minVerFromDevs vimtypes.HardwareVersion
	switch {
	case pkgutil.HasVirtualPCIPassthroughDeviceChange(configSpec.DeviceChange):
		minVerFromDevs = max(imageVersion, constants.MinSupportedHWVersionForPCIPassthruDevices)
	case HasPVC(vm):
		// This only catches volumes set at VM create time.
		minVerFromDevs = max(imageVersion, constants.MinSupportedHWVersionForPVC)
	case hasvTPM(configSpec.DeviceChange):
		minVerFromDevs = max(imageVersion, constants.MinSupportedHWVersionForVTPM)
	}

	// Return the larger of the two versions. If both versions are zero, then
	// the underlying platform determines the default hardware version.
	return max(vmMinVersion, minVerFromDevs)
}

// HasPVC returns true if any of spec.volumes contains a PVC.
func HasPVC(vm vmopv1.VirtualMachine) bool {
	for i := range vm.Spec.Volumes {
		if vm.Spec.Volumes[i].PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

func hasvTPM(devChanges []vimtypes.BaseVirtualDeviceConfigSpec) bool {
	for i := range devChanges {
		if _, ok := devChanges[i].GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualTPM); ok {
			return true
		}
	}
	return false
}

// IsClasslessVM returns true if the provided VM was not deployed from a VM
// class.
func IsClasslessVM(vm vmopv1.VirtualMachine) bool {
	return vm.Spec.ClassName == ""
}

// IsImagelessVM returns true if the provided VM was not deployed from a VM
// image.
func IsImagelessVM(vm vmopv1.VirtualMachine) bool {
	return vm.Spec.Image == nil && vm.Spec.ImageName == ""
}

// ImageRefsEqual returns true if the two image refs match.
func ImageRefsEqual(ref1, ref2 *vmopv1.VirtualMachineImageRef) bool {
	if ref1 == nil && ref2 == nil {
		return true
	}

	if ref1 != nil && ref2 != nil {
		return *ref1 == *ref2
	}

	return false
}

// SyncStorageUsageForNamespace updates the StoragePolicyUsage resource for
// the given namespace and storage class with the reported usage information
// for VMs in that namespace that use the specified storage class.
func SyncStorageUsageForNamespace(
	ctx context.Context,
	namespace, storageClass string) {

	go func() {
		if namespace == "" || storageClass == "" {
			return
		}
		spqutil.FromContext(ctx) <- event.GenericEvent{
			Object: &unstructured.Unstructured{
				Object: map[string]any{
					"metadata": map[string]any{
						"namespace": namespace,
						"name":      storageClass,
					},
				},
			},
		}
	}()
}

// EncryptionClassToVirtualMachineMapper returns a mapper function used to
// enqueue reconcile requests for VMs in response to an event on the
// EncryptionClass resource.
func EncryptionClassToVirtualMachineMapper(
	ctx context.Context,
	k8sClient client.Client) handler.MapFunc {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}

	// For a given EncryptionClass, return reconcile requests for VMs that
	// specify the same EncryptionClass.
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		if ctx == nil {
			panic("context is nil")
		}
		if o == nil {
			panic("object is nil")
		}
		obj, ok := o.(*byokv1.EncryptionClass)
		if !ok {
			panic(fmt.Sprintf("object is %T", o))
		}

		logger := logr.FromContextOrDiscard(ctx).
			WithValues("name", o.GetName(), "namespace", o.GetNamespace())
		logger.V(4).Info("Reconciling all VMs referencing an EncryptionClass")

		// Find all VM resources that reference this EncryptionClass.
		vmList := &vmopv1.VirtualMachineList{}
		if err := k8sClient.List(
			ctx,
			vmList,
			client.InNamespace(obj.Namespace)); err != nil {

			if !apierrors.IsNotFound(err) {
				logger.Error(
					err,
					"Failed to list VirtualMachines for "+
						"reconciliation due to EncryptionClass watch")
			}
			return nil
		}

		// Populate reconcile requests for VMs that reference this
		// EncryptionClass.
		var requests []reconcile.Request
		for i := range vmList.Items {
			vm := vmList.Items[i]
			if vm.Spec.Crypto == nil {
				continue
			}
			if vm.Spec.Crypto.EncryptionClassName == obj.Name {
				requests = append(
					requests,
					reconcile.Request{
						NamespacedName: client.ObjectKey{
							Namespace: vm.Namespace,
							Name:      vm.Name,
						},
					})
			}
		}

		if len(requests) > 0 {
			logger.V(4).Info(
				"Reconciling VMs due to EncryptionClass watch",
				"requests", requests)
		}

		return requests
	}
}

// KubernetesNodeLabelKey is the name of the label key used to identify a
// Kubernetes cluster node.
const KubernetesNodeLabelKey = "cluster.x-k8s.io/cluster-name"

// IsKubernetesNode returns true if the provided VM has the label
// cluster.x-k8s.io/cluster-name.
func IsKubernetesNode(vm vmopv1.VirtualMachine) bool {
	_, ok := vm.Labels[KubernetesNodeLabelKey]
	return ok
}

// GetContextWithWorkloadDomainIsolation gets a new context with the
// WorkloadDomainIsolation capability set to a value based on the provided VM.
func GetContextWithWorkloadDomainIsolation(
	ctx context.Context,
	vm vmopv1.VirtualMachine) context.Context {

	// Create a copy of the config so the WorkloadDomainIsolation capability
	// may be altered for just this VM.
	cfg := pkgcfg.FromContext(ctx)

	// By default the WorkloadDomainIsolation capability is enabled for all
	// VMs.
	cfg.Features.WorkloadDomainIsolation = true

	// If the VM is a Kubernetes node, the WorkloadDomainIsolation
	// capability is determined by inspecting the value of the cluster's
	// capability, which may be disabled based on the version of the vSphere
	// Kubernetes Service (VKS).
	if IsKubernetesNode(vm) {
		cfg.Features.WorkloadDomainIsolation = pkgcfg.FromContext(ctx).
			Features.WorkloadDomainIsolation
	}

	// Layer the updated Config into the context so all logic beneath this
	// line in the call stack will use the updated value.
	return pkgcfg.WithContext(ctx, cfg)
}
