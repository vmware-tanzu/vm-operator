// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
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
		return nil, errors.Errorf(
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
			return nil, errors.Errorf(
				"multiple VM images exist for %q in namespace and cluster scope",
				imgName)
		}
		obj = &cvmiList.Items[0]
	default:
		return nil, errors.Errorf(
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
	vmMinVersion := vimtypes.HardwareVersion(vm.Spec.MinHardwareVersion)

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
		imageVersion = vimtypes.HardwareVersion(*imgStatus.HardwareVersion)
	}

	// VMs with PCI pass-through devices require a minimum hardware version, as
	// do VMs with PVCs. Since the version required by PCI pass-through devices
	// is higher than the one required by PVCs, first check if the VM has any
	// PCI pass-through devices, then check if the VM has any PVCs.
	var minVerFromDevs vimtypes.HardwareVersion
	if pkgutil.HasVirtualPCIPassthroughDeviceChange(configSpec.DeviceChange) {
		minVerFromDevs = max(imageVersion, constants.MinSupportedHWVersionForPCIPassthruDevices)
	} else if HasPVC(vm) {
		// This only catches volumes set at VM create time.
		minVerFromDevs = max(imageVersion, constants.MinSupportedHWVersionForPVC)
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

// IsClasslessVM returns true if the provided VM was not deployed from a VM
// class.
func IsClasslessVM(vm vmopv1.VirtualMachine) bool {
	return vm.Spec.ClassName == ""
}
