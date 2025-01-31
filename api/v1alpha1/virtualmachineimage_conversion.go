// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
)

func Convert_v1alpha4_VirtualMachineImageOSInfo_To_v1alpha1_VirtualMachineImageOSInfo(
	in *vmopv1.VirtualMachineImageOSInfo, out *VirtualMachineImageOSInfo, s apiconversion.Scope) error {

	// = in.ID
	out.Type = in.Type
	out.Version = in.Version

	return nil
}

func convert_v1alpha1_VirtualMachineImageOSInfo_To_v1alpha4_VirtualMachineImageOSInfo(
	in *VirtualMachineImageOSInfo, out *vmopv1.VirtualMachineImageOSInfo, s apiconversion.Scope) error {

	out.Type = in.Type
	out.Version = in.Version

	return nil
}

func convert_v1alpha4_VirtualMachineImage_OVFProperties_To_v1alpha1_VirtualMachineImage_OVFEnv(
	in []vmopv1.OVFProperty, out *map[string]OvfProperty, s apiconversion.Scope) error {

	if in != nil {
		*out = map[string]OvfProperty{}
		for _, p := range in {
			(*out)[p.Key] = OvfProperty{
				Key:         p.Key,
				Type:        p.Type,
				Default:     p.Default,
				Description: "",
				Label:       "",
			}
		}
	}

	return nil
}

func convert_v1alpha1_VirtualMachineImage_OVFEnv_To_v1alpha4_VirtualMachineImage_OVFProperties(
	in map[string]OvfProperty, out *[]vmopv1.OVFProperty, s apiconversion.Scope) error {

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

	return nil
}

func convert_v1alpha4_VirtualMachineImageStatusConditions_To_v1alpha1_VirtualMachineImageStatusConditions(
	conditions []metav1.Condition) []Condition {

	if len(conditions) == 0 {
		return nil
	}

	var (
		imageSyncedCondition, imageProviderReadyCondition, securityCompliantCondition *Condition
		readyCondition                                                                metav1.Condition
	)

	for i := range conditions {
		c := conditions[i]
		if c.Type == vmopv1.ReadyConditionType {
			readyCondition = c
			break
		}
	}

	trueCondition := func(conditionType ConditionType) *Condition {
		return &Condition{
			Type:               conditionType,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: readyCondition.LastTransitionTime,
		}
	}
	falseConditionWithReason := func(conditionType ConditionType) *Condition {
		return &Condition{
			Type:               conditionType,
			Status:             corev1.ConditionFalse,
			Severity:           ConditionSeverityError,
			LastTransitionTime: readyCondition.LastTransitionTime,
			Reason:             readyCondition.Reason,
			Message:            readyCondition.Message,
		}
	}

	switch readyCondition.Reason {
	case vmopv1.VirtualMachineImageProviderSecurityNotCompliantReason:
		securityCompliantCondition = falseConditionWithReason(VirtualMachineImageProviderSecurityComplianceCondition)
	case vmopv1.VirtualMachineImageProviderNotReadyReason:
		securityCompliantCondition = trueCondition(VirtualMachineImageProviderSecurityComplianceCondition)
		imageProviderReadyCondition = falseConditionWithReason(VirtualMachineImageProviderReadyCondition)
	case vmopv1.VirtualMachineImageNotSyncedReason:
		securityCompliantCondition = trueCondition(VirtualMachineImageProviderSecurityComplianceCondition)
		imageProviderReadyCondition = trueCondition(VirtualMachineImageProviderReadyCondition)
		imageSyncedCondition = falseConditionWithReason(VirtualMachineImageSyncedCondition)
	default:
		securityCompliantCondition = trueCondition(VirtualMachineImageProviderSecurityComplianceCondition)
		imageProviderReadyCondition = trueCondition(VirtualMachineImageProviderReadyCondition)
		imageSyncedCondition = trueCondition(VirtualMachineImageSyncedCondition)
	}

	var v1a1Conditions []Condition
	if securityCompliantCondition != nil {
		v1a1Conditions = append(v1a1Conditions, *securityCompliantCondition)
	}
	if imageProviderReadyCondition != nil {
		v1a1Conditions = append(v1a1Conditions, *imageProviderReadyCondition)
	}
	if imageSyncedCondition != nil {
		v1a1Conditions = append(v1a1Conditions, *imageSyncedCondition)
	}

	return v1a1Conditions
}

func convert_v1alpha1_VirtualMachineImageStatusConditions_To_v1alpha4_VirtualMachineImageStatusConditions(
	conditions []Condition) []metav1.Condition {

	if len(conditions) == 0 {
		return nil
	}

	var (
		readyCondition *metav1.Condition
		// We calculate the latest transition time to best case set the
		// latest transition time when the ready condition would be set.
		latestTransitionTime metav1.Time
	)

	// Condition types which are folded into the Ready condition in v1alpha4
	oldConditionTypes := map[ConditionType]struct{}{
		VirtualMachineImageSyncedCondition:                     {},
		VirtualMachineImageProviderReadyCondition:              {},
		VirtualMachineImageProviderSecurityComplianceCondition: {},
	}

	for _, condition := range conditions {
		if _, ok := oldConditionTypes[condition.Type]; ok && condition.Status == corev1.ConditionFalse {
			readyCondition = &metav1.Condition{
				Type:               vmopv1.ReadyConditionType,
				Status:             metav1.ConditionFalse,
				LastTransitionTime: condition.LastTransitionTime,
				Reason:             condition.Reason,
				Message:            condition.Message,
			}
			break
		}
		if latestTransitionTime.Before(&condition.LastTransitionTime) {
			latestTransitionTime = condition.LastTransitionTime
		}
	}

	if readyCondition == nil {
		readyCondition = &metav1.Condition{
			Type:               vmopv1.ReadyConditionType,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: latestTransitionTime,
		}
	}

	if readyCondition.Reason == "" {
		// Reason is a required field in metav1.Condition. This is what
		// Convert_v1alpha1_Condition_To_v1_Condition() does, but we're
		// creating our own metav1.Condition here.
		readyCondition.Reason = string(readyCondition.Status)
	}

	return []metav1.Condition{*readyCondition}
}

func Convert_v1alpha1_VirtualMachineImageSpec_To_v1alpha4_VirtualMachineImageSpec(
	in *VirtualMachineImageSpec, out *vmopv1.VirtualMachineImageSpec, s apiconversion.Scope) error {

	// in.Type
	// in.ImageSourceType
	// in.ImageID
	// in.ProductInfo
	// in.OSInfo
	// in.OVFEnv
	// in.HardwareVersion

	if in.ProviderRef.APIVersion != "" || in.ProviderRef.Kind != "" || in.ProviderRef.Name != "" {
		out.ProviderRef = &vmopv1common.LocalObjectRef{
			APIVersion: in.ProviderRef.APIVersion,
			Kind:       in.ProviderRef.Kind,
			Name:       in.ProviderRef.Name,
		}
	}

	return autoConvert_v1alpha1_VirtualMachineImageSpec_To_v1alpha4_VirtualMachineImageSpec(in, out, s)
}

func Convert_v1alpha1_VirtualMachineImageStatus_To_v1alpha4_VirtualMachineImageStatus(
	in *VirtualMachineImageStatus, out *vmopv1.VirtualMachineImageStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha1_VirtualMachineImageStatus_To_v1alpha4_VirtualMachineImageStatus(in, out, s); err != nil {
		return err
	}

	out.Name = in.ImageName
	out.ProviderContentVersion = in.ContentVersion
	out.Conditions = convert_v1alpha1_VirtualMachineImageStatusConditions_To_v1alpha4_VirtualMachineImageStatusConditions(in.Conditions)
	// in.ImageSupported
	// in.ContentLibraryRef

	// Deprecated:
	// in.Uuid
	// in.PowerState
	// in.InternalId

	return nil
}

func Convert_v1alpha4_VirtualMachineImageSpec_To_v1alpha1_VirtualMachineImageSpec(
	in *vmopv1.VirtualMachineImageSpec, out *VirtualMachineImageSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineImageSpec_To_v1alpha1_VirtualMachineImageSpec(in, out, s); err != nil {
		return err
	}

	if in.ProviderRef != nil {
		out.ProviderRef = ContentProviderReference{
			APIVersion: in.ProviderRef.APIVersion,
			Kind:       in.ProviderRef.Kind,
			Name:       in.ProviderRef.Name,
		}
	}

	return nil
}

func Convert_v1alpha4_VirtualMachineImageStatus_To_v1alpha1_VirtualMachineImageStatus(
	in *vmopv1.VirtualMachineImageStatus, out *VirtualMachineImageStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineImageStatus_To_v1alpha1_VirtualMachineImageStatus(in, out, s); err != nil {
		return err
	}

	out.ImageName = in.Name
	out.ContentVersion = in.ProviderContentVersion
	// out.ContentLibraryRef =

	// in.Capabilities

	out.Conditions = convert_v1alpha4_VirtualMachineImageStatusConditions_To_v1alpha1_VirtualMachineImageStatusConditions(in.Conditions)

	// Deprecated:
	out.Uuid = ""
	out.InternalId = ""
	out.PowerState = ""

	return nil
}

func convert_v1alpha1_VirtualMachineImageSpec_To_v1alpha4_VirtualMachineImageStatus(
	in *VirtualMachineImageSpec, out *vmopv1.VirtualMachineImageStatus, s apiconversion.Scope) error {

	// Some fields of the v1a1 ImageSpec moved into the nextver ImageStatus.
	// conversion-gen doesn't handle that so do those here.

	out.ProviderItemID = in.ImageID
	if in.HardwareVersion != 0 {
		out.HardwareVersion = &in.HardwareVersion
	}

	if err := convert_v1alpha1_VirtualMachineImageOSInfo_To_v1alpha4_VirtualMachineImageOSInfo(&in.OSInfo, &out.OSInfo, s); err != nil {
		return err
	}

	if err := convert_v1alpha1_VirtualMachineImage_OVFEnv_To_v1alpha4_VirtualMachineImage_OVFProperties(in.OVFEnv, &out.OVFProperties, s); err != nil {
		return err
	}

	if err := Convert_v1alpha1_VirtualMachineImageProductInfo_To_v1alpha4_VirtualMachineImageProductInfo(&in.ProductInfo, &out.ProductInfo, s); err != nil {
		return err
	}

	return nil
}

func convert_v1alpha4_VirtualMachineImageStatus_To_v1alpha1_VirtualMachineImageSpec(
	in *vmopv1.VirtualMachineImageStatus, out *VirtualMachineImageSpec, s apiconversion.Scope) error {

	// Some fields of the v1a1 ImageSpec moved into the nextver ImageStatus.
	// conversion-gen doesn't handle that so do those here.

	out.ImageID = in.ProviderItemID
	if in.HardwareVersion != nil {
		out.HardwareVersion = *in.HardwareVersion
	}

	if err := Convert_v1alpha4_VirtualMachineImageOSInfo_To_v1alpha1_VirtualMachineImageOSInfo(&in.OSInfo, &out.OSInfo, s); err != nil {
		return err
	}

	if err := convert_v1alpha4_VirtualMachineImage_OVFProperties_To_v1alpha1_VirtualMachineImage_OVFEnv(in.OVFProperties, &out.OVFEnv, s); err != nil {
		return err
	}

	if err := Convert_v1alpha4_VirtualMachineImageProductInfo_To_v1alpha1_VirtualMachineImageProductInfo(&in.ProductInfo, &out.ProductInfo, s); err != nil {
		return err
	}

	return nil
}

func convert_v1alpha4_VirtualMachineImage_VMwareSystemProperties_To_v1alpha1_VirtualMachineImageAnnotations(
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

func convert_v1alpha1_VirtualMachineImageAnnotations_To_v1alpha4_VirtualMachineImage_VMwareSystemProperties(
	in *map[string]string, dstAnnotations *map[string]string, dstSystemProperties *[]vmopv1common.KeyValuePair) error {
	if *in == nil {
		return nil
	}

	// copy v1a1 system annotations to nextver system properties status field
	for k, v := range *in {
		if strings.HasPrefix(k, "vmware-system") {
			*dstSystemProperties = append(*dstSystemProperties, vmopv1common.KeyValuePair{
				Key:   k,
				Value: v,
			})
		}
	}

	// remove any system annotations in nextver object
	if *dstAnnotations != nil {
		for k, _ := range *dstAnnotations {
			if strings.HasPrefix(k, "vmware-system") {
				delete(*dstAnnotations, k)
			}
		}
	}

	return nil
}

// ConvertTo converts this VirtualMachineImage to the Hub version.
func (src *VirtualMachineImage) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImage)
	if err := Convert_v1alpha1_VirtualMachineImage_To_v1alpha4_VirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	if err := convert_v1alpha1_VirtualMachineImageSpec_To_v1alpha4_VirtualMachineImageStatus(&src.Spec, &dst.Status, nil); err != nil {
		return err
	}

	if err := convert_v1alpha1_VirtualMachineImageAnnotations_To_v1alpha4_VirtualMachineImage_VMwareSystemProperties(
		&src.Annotations, &dst.Annotations, &dst.Status.VMwareSystemProperties); err != nil {
		return err
	}

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineImage.
func (dst *VirtualMachineImage) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImage)

	if err := Convert_v1alpha4_VirtualMachineImage_To_v1alpha1_VirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	if err := convert_v1alpha4_VirtualMachineImageStatus_To_v1alpha1_VirtualMachineImageSpec(&src.Status, &dst.Spec, nil); err != nil {
		return err
	}

	if err := convert_v1alpha4_VirtualMachineImage_VMwareSystemProperties_To_v1alpha1_VirtualMachineImageAnnotations(
		&src.Status.VMwareSystemProperties, &dst.Annotations); err != nil {
		return err
	}

	if dst.Spec.ProviderRef.Name != "" {
		// The Namespace isn't a field in the nextver LocalObjectRef so backfill the namespace here.
		// The provider is always in the same namespace.
		dst.Spec.ProviderRef.Namespace = src.Namespace
	}
	dst.Status.ContentLibraryRef = readContentLibRefConversionAnnotation(src)

	return nil
}

// ConvertTo converts this VirtualMachineImageList to the Hub version.
func (src *VirtualMachineImageList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineImageList)
	return Convert_v1alpha1_VirtualMachineImageList_To_v1alpha4_VirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineImageList.
func (dst *VirtualMachineImageList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineImageList)
	return Convert_v1alpha4_VirtualMachineImageList_To_v1alpha1_VirtualMachineImageList(src, dst, nil)
}

// ConvertTo converts this ClusterVirtualMachineImage to the Hub version.
func (src *ClusterVirtualMachineImage) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.ClusterVirtualMachineImage)
	if err := Convert_v1alpha1_ClusterVirtualMachineImage_To_v1alpha4_ClusterVirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	if err := convert_v1alpha1_VirtualMachineImageSpec_To_v1alpha4_VirtualMachineImageStatus(&src.Spec, &dst.Status, nil); err != nil {
		return err
	}

	if err := convert_v1alpha1_VirtualMachineImageAnnotations_To_v1alpha4_VirtualMachineImage_VMwareSystemProperties(
		&src.Annotations, &dst.Annotations, &dst.Status.VMwareSystemProperties); err != nil {
		return err
	}

	return nil
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImage.
func (dst *ClusterVirtualMachineImage) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.ClusterVirtualMachineImage)
	if err := Convert_v1alpha4_ClusterVirtualMachineImage_To_v1alpha1_ClusterVirtualMachineImage(src, dst, nil); err != nil {
		return err
	}

	if err := convert_v1alpha4_VirtualMachineImageStatus_To_v1alpha1_VirtualMachineImageSpec(&src.Status, &dst.Spec, nil); err != nil {
		return err
	}

	if err := convert_v1alpha4_VirtualMachineImage_VMwareSystemProperties_To_v1alpha1_VirtualMachineImageAnnotations(
		&src.Status.VMwareSystemProperties, &dst.Annotations); err != nil {
		return err
	}

	dst.Status.ContentLibraryRef = readContentLibRefConversionAnnotation(src)

	return nil
}

func readContentLibRefConversionAnnotation(from metav1.Object) (objRef *corev1.TypedLocalObjectReference) {
	if data, ok := from.GetAnnotations()[vmopv1.VMIContentLibRefAnnotation]; ok {
		objRef = &corev1.TypedLocalObjectReference{}
		_ = json.Unmarshal([]byte(data), objRef)
	}
	return
}

// ConvertTo converts this ClusterVirtualMachineImageList to the Hub version.
func (src *ClusterVirtualMachineImageList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.ClusterVirtualMachineImageList)
	return Convert_v1alpha1_ClusterVirtualMachineImageList_To_v1alpha4_ClusterVirtualMachineImageList(src, dst, nil)
}

// ConvertFrom converts the hub version to this ClusterVirtualMachineImageList.
func (dst *ClusterVirtualMachineImageList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.ClusterVirtualMachineImageList)
	return Convert_v1alpha4_ClusterVirtualMachineImageList_To_v1alpha1_ClusterVirtualMachineImageList(src, dst, nil)
}
