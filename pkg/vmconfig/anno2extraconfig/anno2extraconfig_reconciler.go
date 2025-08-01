// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package anno2extraconfig

import (
	"context"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

const (
	ManagementProxyAllowListExtraConfig = "guestinfo.com.vmware.mgmt.proxy.allowlist/"
	ManagementProxyAllowListAnnotation  = "mgmt.proxy.vmware.com/allowlist"

	ManagementProxyWatermarkExtraConfig = "com.vmware.mgmt.proxy.watermark"
	ManagementProxyWatermarkAnnotation  = "mgmt.proxy.vmware.com/watermark"
)

// AnnotationsToExtraConfigKeys is a map whose keys represent
// annotations on VirtualMachine objects for which ExtraConfig key/values should
// be added/removed to/from a vSphere VM.
//
// If a VM has one of these annotations, a key/value should be present on the
// vSphere VM where the key is the value from this map and the value is the
// value of the annotation.
//
// If a vSphere VM's ExtraConfig's keys includes a value from this map but the
// VM does not have the corresponding annotation, then the ExtraConfig key
// should be removed from the vSphere VM.
var AnnotationsToExtraConfigKeys = map[string]string{
	ManagementProxyWatermarkAnnotation: ManagementProxyWatermarkExtraConfig,
	ManagementProxyAllowListAnnotation: ManagementProxyAllowListExtraConfig,
}

// Reconcile configures the VM's annotations that should be ExtraConfig.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

func New() vmconfig.Reconciler {
	return reconciler{}
}

func (r reconciler) Name() string {
	return "anno2extraconfig"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if configSpec == nil {
		panic("configSpec is nil")
	}

	if moVM.Config == nil {
		return nil
	}

	var newEC object.OptionValueList
	outEC := object.OptionValueList(configSpec.ExtraConfig)
	curEC := object.OptionValueList(moVM.Config.ExtraConfig)

	for annotationKey, extraConfigKey := range AnnotationsToExtraConfigKeys {
		// Get the annotation's value.
		curAnnoVal := vm.Annotations[annotationKey]

		if curAnnoVal != "" {
			// The VM *does* have the annotation, so ensure it is added/updated
			// in the ExtraConfig.
			curECVal, _ := curEC.GetString(extraConfigKey)
			if curAnnoVal != curECVal {
				newEC = append(newEC, &vimtypes.OptionValue{
					Key:   extraConfigKey,
					Value: curAnnoVal,
				})
			}
		} else if _, ok := curEC.Get(extraConfigKey); ok {
			// The VM does not have the annotation, but the vSphere VM has the
			// ExtraConfig, so ensure it is removed.
			newEC = append(newEC, &vimtypes.OptionValue{
				Key:   extraConfigKey,
				Value: "",
			})
		} else if _, ok := outEC.Get(extraConfigKey); ok {
			// The VM does not have the annotation, but the existing ConfigSpec
			// has the ExtraConfig, so ensure it is removed.
			newEC = append(newEC, &vimtypes.OptionValue{
				Key:   extraConfigKey,
				Value: "",
			})
		}
	}

	configSpec.ExtraConfig = curEC.Diff(newEC.Join(outEC...)...)

	return nil
}
