// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"


	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
)

// VirtualMachineImage is the converter for VirtualMachineImage across all spoke versions.
var VirtualMachineImage = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineImage{},
	whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachineImage{}, convertVMImageHubToV1Alpha1, convertVMImageV1Alpha1ToHub),
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachineImage{}, convertVMImageHubToV1Alpha2, convertVMImageV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineImage{}, convertVMImageHubToV1Alpha3, convertVMImageV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineImage{}, convertVMImageHubToV1Alpha4, convertVMImageV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineImage{}, convertVMImageHubToV1Alpha5, convertVMImageV1Alpha5ToHub),
)

// ClusterVirtualMachineImage is the converter for ClusterVirtualMachineImage across all spoke versions.
var ClusterVirtualMachineImage = whconversion.NewHubSpokeConverter(
	&vmopv1.ClusterVirtualMachineImage{},
	whconversion.NewSpokeConverter(&vmopv1a1.ClusterVirtualMachineImage{}, convertClusterVMImageHubToV1Alpha1, convertClusterVMImageV1Alpha1ToHub),
	whconversion.NewSpokeConverter(&vmopv1a2.ClusterVirtualMachineImage{}, convertClusterVMImageHubToV1Alpha2, convertClusterVMImageV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.ClusterVirtualMachineImage{}, convertClusterVMImageHubToV1Alpha3, convertClusterVMImageV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.ClusterVirtualMachineImage{}, convertClusterVMImageHubToV1Alpha4, convertClusterVMImageV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.ClusterVirtualMachineImage{}, convertClusterVMImageHubToV1Alpha5, convertClusterVMImageV1Alpha5ToHub),
)

// readVMIContentLibRefAnnotation reads the VMIContentLibRefAnnotation from a
// metav1.Object and unmarshals it into a TypedLocalObjectReference.
func readVMIContentLibRefAnnotation(from metav1.Object) (objRef *corev1.TypedLocalObjectReference) {
	if data, ok := from.GetAnnotations()[vmopv1.VMIContentLibRefAnnotation]; ok {
		objRef = &corev1.TypedLocalObjectReference{}
		_ = json.Unmarshal([]byte(data), objRef)
	}
	return
}

// convertV1Alpha1OVFEnvToV1Alpha6OVFProperties converts v1alpha1 OvfProperty map
// to v1alpha6 OVFProperty slice. This replicates the unexported helper in api/v1alpha1.
func convertV1Alpha1OVFEnvToV1Alpha6OVFProperties(in map[string]vmopv1a1.OvfProperty, out *[]vmopv1.OVFProperty) {
	if in != nil {
		*out = make([]vmopv1.OVFProperty, 0, len(in))
		for _, v := range in {
			*out = append(*out, vmopv1.OVFProperty{
				Key:     v.Key,
				Type:    v.Type,
				Default: v.Default,
			})
		}
	}
}

// convertV1Alpha6OVFPropertiesToV1Alpha1OVFEnv converts v1alpha6 OVFProperty slice
// to v1alpha1 OvfProperty map. This replicates the unexported helper in api/v1alpha1.
func convertV1Alpha6OVFPropertiesToV1Alpha1OVFEnv(in []vmopv1.OVFProperty, out *map[string]vmopv1a1.OvfProperty) {
	if in != nil {
		*out = make(map[string]vmopv1a1.OvfProperty)
		for _, p := range in {
			(*out)[p.Key] = vmopv1a1.OvfProperty{
				Key:         p.Key,
				Type:        p.Type,
				Default:     p.Default,
				Description: "",
				Label:       "",
			}
		}
	}
}

// convertV1Alpha1VMISpecToV1Alpha6Status moves fields from the v1alpha1 ImageSpec
// into the v1alpha6 ImageStatus (they were reorganized between versions).
func convertV1Alpha1VMISpecToV1Alpha6Status(in *vmopv1a1.VirtualMachineImageSpec, out *vmopv1.VirtualMachineImageStatus) error {
	out.ProviderItemID = in.ImageID
	if in.HardwareVersion != 0 {
		out.HardwareVersion = &in.HardwareVersion
	}

	if err := vmopv1a1.Convert_v1alpha1_VirtualMachineImageOSInfo_To_v1alpha6_VirtualMachineImageOSInfo(&in.OSInfo, &out.OSInfo, nil); err != nil {
		return err
	}

	convertV1Alpha1OVFEnvToV1Alpha6OVFProperties(in.OVFEnv, &out.OVFProperties)

	if err := vmopv1a1.Convert_v1alpha1_VirtualMachineImageProductInfo_To_v1alpha6_VirtualMachineImageProductInfo(&in.ProductInfo, &out.ProductInfo, nil); err != nil {
		return err
	}

	return nil
}

// convertV1Alpha6StatusToV1Alpha1VMISpec moves fields from v1alpha6 ImageStatus
// back into v1alpha1 ImageSpec.
func convertV1Alpha6StatusToV1Alpha1VMISpec(in *vmopv1.VirtualMachineImageStatus, out *vmopv1a1.VirtualMachineImageSpec) error {
	out.ImageID = in.ProviderItemID
	if in.HardwareVersion != nil {
		out.HardwareVersion = *in.HardwareVersion
	}

	if err := vmopv1a1.Convert_v1alpha6_VirtualMachineImageOSInfo_To_v1alpha1_VirtualMachineImageOSInfo(&in.OSInfo, &out.OSInfo, nil); err != nil {
		return err
	}

	convertV1Alpha6OVFPropertiesToV1Alpha1OVFEnv(in.OVFProperties, &out.OVFEnv)

	if err := vmopv1a1.Convert_v1alpha6_VirtualMachineImageProductInfo_To_v1alpha1_VirtualMachineImageProductInfo(&in.ProductInfo, &out.ProductInfo, nil); err != nil {
		return err
	}

	return nil
}

// convertV1Alpha6VMwareSystemPropertiesToV1Alpha1Annotations copies vmware-system
// KeyValuePairs from the hub status into the spoke annotation map.
func convertV1Alpha6VMwareSystemPropertiesToV1Alpha1Annotations(
	in *[]vmopv1common.KeyValuePair, out *map[string]string) error {

	if in != nil && len(*in) > 0 {
		if *out == nil {
			*out = make(map[string]string)
		}
		for _, pair := range *in {
			(*out)[pair.Key] = pair.Value
		}
	}
	return nil
}

// convertV1Alpha1AnnotationsToV1Alpha6VMwareSystemProperties copies vmware-system
// annotations from the spoke into the hub's VMwareSystemProperties status field,
// then removes them from the hub's annotation map.
func convertV1Alpha1AnnotationsToV1Alpha6VMwareSystemProperties(
	in *map[string]string, dstAnnotations *map[string]string, dstSystemProperties *[]vmopv1common.KeyValuePair) error {

	if *in == nil {
		return nil
	}
	for k, v := range *in {
		if strings.HasPrefix(k, "vmware-system") {
			*dstSystemProperties = append(*dstSystemProperties, vmopv1common.KeyValuePair{
				Key:   k,
				Value: v,
			})
		}
	}
	if *dstAnnotations != nil {
		for k := range *dstAnnotations {
			if strings.HasPrefix(k, "vmware-system") {
				delete(*dstAnnotations, k)
			}
		}
	}
	return nil
}

// ============================================================
// v1alpha1 — complex conversion (spec/status rearrangement)
// ============================================================

func convertVMImageHubToV1Alpha1(_ context.Context, hub *vmopv1.VirtualMachineImage, spoke *vmopv1a1.VirtualMachineImage) error {
	if err := vmopv1a1.Convert_v1alpha6_VirtualMachineImage_To_v1alpha1_VirtualMachineImage(hub, spoke, nil); err != nil {
		return err
	}
	if err := convertV1Alpha6StatusToV1Alpha1VMISpec(&hub.Status, &spoke.Spec); err != nil {
		return err
	}
	if err := convertV1Alpha6VMwareSystemPropertiesToV1Alpha1Annotations(
		&hub.Status.VMwareSystemProperties, &spoke.Annotations); err != nil {
		return err
	}
	if spoke.Spec.ProviderRef.Name != "" {
		// The Namespace isn't a field in the hub LocalObjectRef so backfill it here.
		spoke.Spec.ProviderRef.Namespace = hub.Namespace
	}
	spoke.Status.ContentLibraryRef = readVMIContentLibRefAnnotation(hub)
	return nil
}

func convertVMImageV1Alpha1ToHub(_ context.Context, spoke *vmopv1a1.VirtualMachineImage, hub *vmopv1.VirtualMachineImage) error {
	if err := vmopv1a1.Convert_v1alpha1_VirtualMachineImage_To_v1alpha6_VirtualMachineImage(spoke, hub, nil); err != nil {
		return err
	}
	if err := convertV1Alpha1VMISpecToV1Alpha6Status(&spoke.Spec, &hub.Status); err != nil {
		return err
	}
	return convertV1Alpha1AnnotationsToV1Alpha6VMwareSystemProperties(
		&spoke.Annotations, &hub.Annotations, &hub.Status.VMwareSystemProperties)
}

func convertClusterVMImageHubToV1Alpha1(_ context.Context, hub *vmopv1.ClusterVirtualMachineImage, spoke *vmopv1a1.ClusterVirtualMachineImage) error {
	if err := vmopv1a1.Convert_v1alpha6_ClusterVirtualMachineImage_To_v1alpha1_ClusterVirtualMachineImage(hub, spoke, nil); err != nil {
		return err
	}
	if err := convertV1Alpha6StatusToV1Alpha1VMISpec(&hub.Status, &spoke.Spec); err != nil {
		return err
	}
	if err := convertV1Alpha6VMwareSystemPropertiesToV1Alpha1Annotations(
		&hub.Status.VMwareSystemProperties, &spoke.Annotations); err != nil {
		return err
	}
	spoke.Status.ContentLibraryRef = readVMIContentLibRefAnnotation(hub)
	return nil
}

func convertClusterVMImageV1Alpha1ToHub(_ context.Context, spoke *vmopv1a1.ClusterVirtualMachineImage, hub *vmopv1.ClusterVirtualMachineImage) error {
	if err := vmopv1a1.Convert_v1alpha1_ClusterVirtualMachineImage_To_v1alpha6_ClusterVirtualMachineImage(spoke, hub, nil); err != nil {
		return err
	}
	if err := convertV1Alpha1VMISpecToV1Alpha6Status(&spoke.Spec, &hub.Status); err != nil {
		return err
	}
	return convertV1Alpha1AnnotationsToV1Alpha6VMwareSystemProperties(
		&spoke.Annotations, &hub.Annotations, &hub.Status.VMwareSystemProperties)
}

// ============================================================
// v1alpha2 — simple conversion
// ============================================================

func convertVMImageHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachineImage, spoke *vmopv1a2.VirtualMachineImage) error {
	return vmopv1a2.Convert_v1alpha6_VirtualMachineImage_To_v1alpha2_VirtualMachineImage(hub, spoke, nil)
}

func convertVMImageV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachineImage, hub *vmopv1.VirtualMachineImage) error {
	return vmopv1a2.Convert_v1alpha2_VirtualMachineImage_To_v1alpha6_VirtualMachineImage(spoke, hub, nil)
}

func convertClusterVMImageHubToV1Alpha2(_ context.Context, hub *vmopv1.ClusterVirtualMachineImage, spoke *vmopv1a2.ClusterVirtualMachineImage) error {
	return vmopv1a2.Convert_v1alpha6_ClusterVirtualMachineImage_To_v1alpha2_ClusterVirtualMachineImage(hub, spoke, nil)
}

func convertClusterVMImageV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.ClusterVirtualMachineImage, hub *vmopv1.ClusterVirtualMachineImage) error {
	return vmopv1a2.Convert_v1alpha2_ClusterVirtualMachineImage_To_v1alpha6_ClusterVirtualMachineImage(spoke, hub, nil)
}

// ============================================================
// v1alpha3 — simple conversion
// ============================================================

func convertVMImageHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineImage, spoke *vmopv1a3.VirtualMachineImage) error {
	return vmopv1a3.Convert_v1alpha6_VirtualMachineImage_To_v1alpha3_VirtualMachineImage(hub, spoke, nil)
}

func convertVMImageV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineImage, hub *vmopv1.VirtualMachineImage) error {
	return vmopv1a3.Convert_v1alpha3_VirtualMachineImage_To_v1alpha6_VirtualMachineImage(spoke, hub, nil)
}

func convertClusterVMImageHubToV1Alpha3(_ context.Context, hub *vmopv1.ClusterVirtualMachineImage, spoke *vmopv1a3.ClusterVirtualMachineImage) error {
	return vmopv1a3.Convert_v1alpha6_ClusterVirtualMachineImage_To_v1alpha3_ClusterVirtualMachineImage(hub, spoke, nil)
}

func convertClusterVMImageV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.ClusterVirtualMachineImage, hub *vmopv1.ClusterVirtualMachineImage) error {
	return vmopv1a3.Convert_v1alpha3_ClusterVirtualMachineImage_To_v1alpha6_ClusterVirtualMachineImage(spoke, hub, nil)
}

// ============================================================
// v1alpha4 — simple conversion
// ============================================================

func convertVMImageHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineImage, spoke *vmopv1a4.VirtualMachineImage) error {
	return vmopv1a4.Convert_v1alpha6_VirtualMachineImage_To_v1alpha4_VirtualMachineImage(hub, spoke, nil)
}

func convertVMImageV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineImage, hub *vmopv1.VirtualMachineImage) error {
	return vmopv1a4.Convert_v1alpha4_VirtualMachineImage_To_v1alpha6_VirtualMachineImage(spoke, hub, nil)
}

func convertClusterVMImageHubToV1Alpha4(_ context.Context, hub *vmopv1.ClusterVirtualMachineImage, spoke *vmopv1a4.ClusterVirtualMachineImage) error {
	return vmopv1a4.Convert_v1alpha6_ClusterVirtualMachineImage_To_v1alpha4_ClusterVirtualMachineImage(hub, spoke, nil)
}

func convertClusterVMImageV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.ClusterVirtualMachineImage, hub *vmopv1.ClusterVirtualMachineImage) error {
	return vmopv1a4.Convert_v1alpha4_ClusterVirtualMachineImage_To_v1alpha6_ClusterVirtualMachineImage(spoke, hub, nil)
}

// ============================================================
// v1alpha5 — simple conversion
// ============================================================

func convertVMImageHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineImage, spoke *vmopv1a5.VirtualMachineImage) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineImage_To_v1alpha5_VirtualMachineImage(hub, spoke, nil)
}

func convertVMImageV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineImage, hub *vmopv1.VirtualMachineImage) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineImage_To_v1alpha6_VirtualMachineImage(spoke, hub, nil)
}

func convertClusterVMImageHubToV1Alpha5(_ context.Context, hub *vmopv1.ClusterVirtualMachineImage, spoke *vmopv1a5.ClusterVirtualMachineImage) error {
	return vmopv1a5.Convert_v1alpha6_ClusterVirtualMachineImage_To_v1alpha5_ClusterVirtualMachineImage(hub, spoke, nil)
}

func convertClusterVMImageV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.ClusterVirtualMachineImage, hub *vmopv1.ClusterVirtualMachineImage) error {
	return vmopv1a5.Convert_v1alpha5_ClusterVirtualMachineImage_To_v1alpha6_ClusterVirtualMachineImage(spoke, hub, nil)
}
