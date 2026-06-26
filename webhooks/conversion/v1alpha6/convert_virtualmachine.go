// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"
	"reflect"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"


	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
)

// VirtualMachine is the converter for VirtualMachine across all spoke versions.
var VirtualMachine = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachine{},
	whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachine{}, convertVMHubToV1Alpha1, convertVMV1Alpha1ToHub),
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachine{}, convertVMHubToV1Alpha2, convertVMV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachine{}, convertVMHubToV1Alpha3, convertVMV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachine{}, convertVMHubToV1Alpha4, convertVMV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachine{}, convertVMHubToV1Alpha5, convertVMV1Alpha5ToHub),
)

// ============================================================
// Shared restore helpers (body identical across all versions)
// ============================================================

func restoreVirtualMachineResources(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Resources = src.Spec.Resources
}

func restoreVirtualMachineCPUAdvanced(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.CPUAdvanced = src.Spec.CPUAdvanced
}

func restoreVirtualMachineMemoryAdvanced(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.MemoryAdvanced = src.Spec.MemoryAdvanced
}

func restoreVirtualMachineBootOptions(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.BootOptions = src.Spec.BootOptions
}

func restoreVirtualMachinePromoteDisksMode(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.PromoteDisksMode = src.Spec.PromoteDisksMode
}

func restoreVirtualMachineInstanceUUID(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.InstanceUUID = src.Spec.InstanceUUID
}

func restoreVirtualMachineBiosUUID(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.BiosUUID = src.Spec.BiosUUID
}

func restoreVirtualMachineGuestID(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.GuestID = src.Spec.GuestID
}

func restoreVirtualMachineCryptoVTPM(dst, src *vmopv1.VirtualMachine) {
	if dst.Spec.Crypto != nil && src.Spec.Crypto != nil {
		dst.Spec.Crypto.VTPMMode = src.Spec.Crypto.VTPMMode
	}
}

func restoreVirtualMachineCurrentSnapshotName(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.CurrentSnapshotName = src.Spec.CurrentSnapshotName
}

func restoreVirtualMachineVolumeAttributesClassName(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.VolumeAttributesClassName != "" {
		dst.Spec.VolumeAttributesClassName = src.Spec.VolumeAttributesClassName
	}
}

func restoreVirtualMachineNetworkVLANs(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Network == nil || len(src.Spec.Network.VLANs) == 0 {
		return
	}
	if dst.Spec.Network == nil {
		dst.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
	}
	dst.Spec.Network.VLANs = src.Spec.Network.VLANs
}

func restoreVirtualMachinePolicies(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Policies = slices.Clone(src.Spec.Policies)
}

func restoreVirtualMachineBootstrapDisabled(dst, src *vmopv1.VirtualMachine) {
	if bs := src.Spec.Bootstrap; bs != nil {
		if bs.Disabled {
			if dst.Spec.Bootstrap == nil {
				dst.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
			}
			dst.Spec.Bootstrap.Disabled = true
		}
	}
}

func restoreVirtualMachineBootstrapCloudInitInstanceID(dst, src *vmopv1.VirtualMachine) {
	var iid string
	if bs := src.Spec.Bootstrap; bs != nil {
		if ci := bs.CloudInit; ci != nil {
			iid = ci.InstanceID
		}
	}
	if iid == "" {
		return
	}
	if dst.Spec.Bootstrap == nil {
		dst.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
	}
	if dst.Spec.Bootstrap.CloudInit == nil {
		dst.Spec.Bootstrap.CloudInit = &vmopv1.VirtualMachineBootstrapCloudInitSpec{}
	}
	dst.Spec.Bootstrap.CloudInit.InstanceID = iid
}

func restoreVirtualMachineBootstrapCloudInitWaitOnNetwork(dst, src *vmopv1.VirtualMachine) {
	if bs := src.Spec.Bootstrap; bs != nil {
		if ci := bs.CloudInit; ci != nil {
			if ci.WaitOnNetwork4 != nil || ci.WaitOnNetwork6 != nil {
				if dst.Spec.Bootstrap != nil && dst.Spec.Bootstrap.CloudInit != nil {
					dst.Spec.Bootstrap.CloudInit.WaitOnNetwork4 = ci.WaitOnNetwork4
					dst.Spec.Bootstrap.CloudInit.WaitOnNetwork6 = ci.WaitOnNetwork6
				}
			}
		}
	}
}

func restoreVirtualMachineBootstrapLinuxPrep(dst, src *vmopv1.VirtualMachine) {
	if bs := src.Spec.Bootstrap; bs != nil {
		if lp := bs.LinuxPrep; lp != nil {
			if dst.Spec.Bootstrap != nil && dst.Spec.Bootstrap.LinuxPrep != nil {
				dst.Spec.Bootstrap.LinuxPrep.ExpirePasswordAfterNextLogin = lp.ExpirePasswordAfterNextLogin
				dst.Spec.Bootstrap.LinuxPrep.Password = lp.Password
				dst.Spec.Bootstrap.LinuxPrep.ScriptText = lp.ScriptText
				dst.Spec.Bootstrap.LinuxPrep.CustomizeAtNextPowerOn = lp.CustomizeAtNextPowerOn
			}
		}
	}
}

func restoreVirtualMachineBootstrapSysprep(dst, src *vmopv1.VirtualMachine) {
	if bs := src.Spec.Bootstrap; bs != nil {
		if sp := bs.Sysprep; sp != nil {
			if dst.Spec.Bootstrap != nil && dst.Spec.Bootstrap.Sysprep != nil {
				if sp.Sysprep != nil && dst.Spec.Bootstrap.Sysprep.Sysprep != nil {
					dst.Spec.Bootstrap.Sysprep.Sysprep.ExpirePasswordAfterNextLogin = sp.Sysprep.ExpirePasswordAfterNextLogin
					dst.Spec.Bootstrap.Sysprep.Sysprep.ScriptText = sp.Sysprep.ScriptText
				}
				dst.Spec.Bootstrap.Sysprep.CustomizeAtNextPowerOn = sp.CustomizeAtNextPowerOn
			}
		}
	}
}

// restoreVirtualMachineImage is shared across all versions.
func restoreVirtualMachineImage(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Image = src.Spec.Image
	dst.Spec.ImageName = src.Spec.ImageName
}

// restoreVirtualMachineHardwareShared restores hardware fields for v2-v5.
// v1alpha1 has a simpler variant (see restoreV1Alpha1VirtualMachineHardware).
func restoreVirtualMachineHardwareShared(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Hardware == nil {
		return
	}
	var cdromChanges []vmopv1.VirtualMachineCdromSpec
	if dst.Spec.Hardware != nil && len(dst.Spec.Hardware.Cdrom) > 0 {
		cdromChanges = dst.Spec.Hardware.Cdrom
	}
	dst.Spec.Hardware = src.Spec.Hardware.DeepCopy()
	dst.Spec.Hardware.Cdrom = cdromChanges
	for i := range dst.Spec.Hardware.Cdrom {
		dstCdrom := &dst.Spec.Hardware.Cdrom[i]
		for _, srcCdrom := range src.Spec.Hardware.Cdrom {
			if srcCdrom.Name == dstCdrom.Name {
				dstCdrom.ControllerBusNumber = srcCdrom.ControllerBusNumber
				dstCdrom.ControllerType = srcCdrom.ControllerType
				dstCdrom.UnitNumber = srcCdrom.UnitNumber
				break
			}
		}
	}
}

// restoreVirtualMachineAffinity is shared across v2-v5.
func restoreVirtualMachineAffinity(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Affinity == nil {
		dst.Spec.Affinity = nil
	} else {
		dst.Spec.Affinity = src.Spec.Affinity.DeepCopy()
	}
}

// ============================================================
// Version-specific restore helpers for VirtualMachineAdvanced
// ============================================================

// restoreV1Alpha1VirtualMachineAdvanced restores Advanced fields for v1alpha1.
// v1alpha1 additionally restores DefaultVolumeProvisioningMode.
func restoreV1Alpha1VirtualMachineAdvanced(dst, src *vmopv1.VirtualMachine) {
	adv := src.Spec.Advanced
	if adv == nil {
		return
	}
	if dst.Spec.Advanced == nil {
		dst.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
	}
	dst.Spec.Advanced.DefaultVolumeProvisioningMode = adv.DefaultVolumeProvisioningMode
	dst.Spec.Advanced.PreferHTEnabled = adv.PreferHTEnabled
	dst.Spec.Advanced.HugePages1GEnabled = adv.HugePages1GEnabled
	dst.Spec.Advanced.TimeTrackerLowLatencyEnabled = adv.TimeTrackerLowLatencyEnabled
	dst.Spec.Advanced.CPUAffinityExclusiveNoStatsEnabled = adv.CPUAffinityExclusiveNoStatsEnabled
	dst.Spec.Advanced.VMXSwapEnabled = adv.VMXSwapEnabled
	dst.Spec.Advanced.PNUMANodeAffinity = adv.PNUMANodeAffinity
	dst.Spec.Advanced.ExtraConfig = adv.ExtraConfig
}

// restoreV1Alpha2VirtualMachineAdvanced restores Advanced fields for v1alpha2-v5
// (no DefaultVolumeProvisioningMode).
func restoreV1Alpha2VirtualMachineAdvanced(dst, src *vmopv1.VirtualMachine) {
	adv := src.Spec.Advanced
	if adv == nil {
		return
	}
	if dst.Spec.Advanced == nil {
		dst.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
	}
	dst.Spec.Advanced.PreferHTEnabled = adv.PreferHTEnabled
	dst.Spec.Advanced.HugePages1GEnabled = adv.HugePages1GEnabled
	dst.Spec.Advanced.TimeTrackerLowLatencyEnabled = adv.TimeTrackerLowLatencyEnabled
	dst.Spec.Advanced.CPUAffinityExclusiveNoStatsEnabled = adv.CPUAffinityExclusiveNoStatsEnabled
	dst.Spec.Advanced.VMXSwapEnabled = adv.VMXSwapEnabled
	dst.Spec.Advanced.PNUMANodeAffinity = adv.PNUMANodeAffinity
	dst.Spec.Advanced.ExtraConfig = adv.ExtraConfig
}

// ============================================================
// Version-specific restore helpers for VirtualMachineVolumes
// ============================================================

// restoreV1Alpha1VirtualMachineVolumes restores volume fields for v1alpha1.
// v1alpha1 additionally filters out boot-disk-size volumes.
func restoreV1Alpha1VirtualMachineVolumes(dst, src *vmopv1.VirtualMachine) {
	srcVolMap := map[string]*vmopv1.VirtualMachineVolume{}
	for i := range src.Spec.Volumes {
		vol := &src.Spec.Volumes[i]
		srcVolMap[vol.Name] = vol
	}
	var vols []vmopv1.VirtualMachineVolume
	for _, v := range dst.Spec.Volumes {
		if srcVol, ok := srcVolMap[v.Name]; ok {
			v.ApplicationType = srcVol.ApplicationType
			v.ControllerBusNumber = srcVol.ControllerBusNumber
			v.ControllerType = srcVol.ControllerType
			v.DiskMode = srcVol.DiskMode
			v.SharingMode = srcVol.SharingMode
			v.UnitNumber = srcVol.UnitNumber
			v.Removable = srcVol.Removable
		}
		if v.PersistentVolumeClaim != nil ||
			(v.ControllerType != "" &&
				v.ControllerBusNumber != nil &&
				v.UnitNumber != nil) {
			vols = append(vols, v)
		}
	}
	if len(vols) == 0 {
		vols = nil
	}
	dst.Spec.Volumes = vols
}

// restoreVirtualMachineVolumes restores volume fields for v2-v5.
func restoreVirtualMachineVolumes(dst, src *vmopv1.VirtualMachine) {
	srcVolMap := map[string]*vmopv1.VirtualMachineVolume{}
	for i := range src.Spec.Volumes {
		vol := &src.Spec.Volumes[i]
		srcVolMap[vol.Name] = vol
	}
	for i := range dst.Spec.Volumes {
		dstVol := &dst.Spec.Volumes[i]
		if srcVol, ok := srcVolMap[dstVol.Name]; ok {
			dstVol.ApplicationType = srcVol.ApplicationType
			dstVol.ControllerBusNumber = srcVol.ControllerBusNumber
			dstVol.ControllerType = srcVol.ControllerType
			dstVol.DiskMode = srcVol.DiskMode
			dstVol.SharingMode = srcVol.SharingMode
			dstVol.UnitNumber = srcVol.UnitNumber
			dstVol.Removable = srcVol.Removable
		}
	}
}

// ============================================================
// Version-specific restore for network interfaces (v2-v5 shared)
// ============================================================

func restoreVirtualMachineNetworkInterfaces(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Network == nil || len(src.Spec.Network.Interfaces) == 0 {
		return
	}
	if dst.Spec.Network == nil || len(dst.Spec.Network.Interfaces) == 0 {
		return
	}
	srcByName := make(map[string]*vmopv1.VirtualMachineNetworkInterfaceSpec, len(src.Spec.Network.Interfaces))
	for i := range src.Spec.Network.Interfaces {
		srcByName[src.Spec.Network.Interfaces[i].Name] = &src.Spec.Network.Interfaces[i]
	}
	for i := range dst.Spec.Network.Interfaces {
		srcIface, ok := srcByName[dst.Spec.Network.Interfaces[i].Name]
		if !ok {
			continue
		}
		dstIface := &dst.Spec.Network.Interfaces[i]
		dstIface.IPAMModes = append([]corev1.IPFamily(nil), srcIface.IPAMModes...)
		dstIface.Type = srcIface.Type
		dstIface.VNUMANodeID = srcIface.VNUMANodeID
		dstIface.VMXNet3 = srcIface.VMXNet3
		dstIface.AdvancedProperties = srcIface.AdvancedProperties
		if dstIface.DHCP4 == nil && srcIface.DHCP4 != nil && !*srcIface.DHCP4 {
			dstIface.DHCP4 = srcIface.DHCP4
		}
		if dstIface.DHCP6 == nil && srcIface.DHCP6 != nil && !*srcIface.DHCP6 {
			dstIface.DHCP6 = srcIface.DHCP6
		}
	}
}

// ============================================================
// v1alpha1-specific restore helpers
// ============================================================

func restoreV1Alpha1VirtualMachineHardware(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Hardware != nil {
		dst.Spec.Hardware = src.Spec.Hardware.DeepCopy()
	} else {
		dst.Spec.Hardware = nil
	}
}

func restoreV1Alpha1VirtualMachineBootstrapSpec(dst, src *vmopv1.VirtualMachine) {
	srcBootstrap := src.Spec.Bootstrap
	if srcBootstrap == nil {
		return
	}

	dstBootstrap := dst.Spec.Bootstrap
	if dstBootstrap == nil {
		if reflect.DeepEqual(&srcBootstrap, &vmopv1.VirtualMachineBootstrapSpec{}) {
			dst.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
			return
		}
		if srcBootstrap.LinuxPrep != nil || srcBootstrap.Disabled {
			dst.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
				LinuxPrep: srcBootstrap.LinuxPrep,
				Disabled:  srcBootstrap.Disabled,
			}
		}
		return
	}

	mergeSecretKeySelector := func(dstSel, srcSel *vmopv1common.SecretKeySelector) *vmopv1common.SecretKeySelector {
		if dstSel == nil || srcSel == nil {
			return dstSel
		}
		newSel := *srcSel
		newSel.Name = dstSel.Name
		return &newSel
	}

	if dstCloudInit := dstBootstrap.CloudInit; dstCloudInit != nil {
		if srcCloudInit := srcBootstrap.CloudInit; srcCloudInit != nil {
			dstCloudInit.CloudConfig = srcCloudInit.CloudConfig
			dstCloudInit.RawCloudConfig = mergeSecretKeySelector(dstCloudInit.RawCloudConfig, srcCloudInit.RawCloudConfig)
			dstCloudInit.SSHAuthorizedKeys = srcCloudInit.SSHAuthorizedKeys
			dstCloudInit.UseGlobalNameserversAsDefault = srcCloudInit.UseGlobalNameserversAsDefault
			dstCloudInit.UseGlobalSearchDomainsAsDefault = srcCloudInit.UseGlobalSearchDomainsAsDefault
			dstCloudInit.WaitOnNetwork4 = srcCloudInit.WaitOnNetwork4
			dstCloudInit.WaitOnNetwork6 = srcCloudInit.WaitOnNetwork6
		}
	}

	if dstLinuxPrep := dstBootstrap.LinuxPrep; dstLinuxPrep != nil {
		if srcLinuxPrep := srcBootstrap.LinuxPrep; srcLinuxPrep != nil {
			dstLinuxPrep.HardwareClockIsUTC = srcLinuxPrep.HardwareClockIsUTC
			dstLinuxPrep.TimeZone = srcLinuxPrep.TimeZone
			dstLinuxPrep.ExpirePasswordAfterNextLogin = srcLinuxPrep.ExpirePasswordAfterNextLogin
			dstLinuxPrep.Password = srcLinuxPrep.Password
			dstLinuxPrep.ScriptText = srcLinuxPrep.ScriptText
			dstLinuxPrep.CustomizeAtNextPowerOn = srcLinuxPrep.CustomizeAtNextPowerOn
		}
	}

	if dstSysPrep := dstBootstrap.Sysprep; dstSysPrep != nil {
		if srcSysPrep := srcBootstrap.Sysprep; srcSysPrep != nil {
			dstSysPrep.Sysprep = srcSysPrep.Sysprep
			dstSysPrep.RawSysprep = mergeSecretKeySelector(dstSysPrep.RawSysprep, srcSysPrep.RawSysprep)
			dstSysPrep.CustomizeAtNextPowerOn = srcSysPrep.CustomizeAtNextPowerOn
			if dstBootstrap.VAppConfig == nil && srcBootstrap.VAppConfig != nil {
				dstBootstrap.VAppConfig = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{}
			}
		}
	}

	if dstVAppConfig := dstBootstrap.VAppConfig; dstVAppConfig != nil {
		if srcVAppConfig := srcBootstrap.VAppConfig; srcVAppConfig != nil {
			dstVAppConfig.Properties = srcVAppConfig.Properties
			dstVAppConfig.RawProperties = srcVAppConfig.RawProperties
		}
	}

	dstBootstrap.Disabled = srcBootstrap.Disabled
}

func restoreV1Alpha1VirtualMachineNetworkSpec(dst, src *vmopv1.VirtualMachine) {
	srcNetwork := src.Spec.Network
	if srcNetwork == nil {
		return
	}
	if dst.Spec.Network == nil {
		dst.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
	}
	dstNetwork := dst.Spec.Network
	dstNetwork.DomainName = srcNetwork.DomainName
	dstNetwork.HostName = srcNetwork.HostName
	dstNetwork.Disabled = srcNetwork.Disabled
	dstNetwork.Nameservers = srcNetwork.Nameservers
	dstNetwork.SearchDomains = srcNetwork.SearchDomains
	dstNetwork.VLANs = srcNetwork.VLANs
	if len(dstNetwork.Interfaces) == 0 {
		return
	}
	dstNetwork.Interfaces = srcNetwork.Interfaces
}

func restoreV1Alpha1VirtualMachineReadinessProbeSpec(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.ReadinessProbe != nil {
		if dst.Spec.ReadinessProbe == nil {
			dst.Spec.ReadinessProbe = &vmopv1.VirtualMachineReadinessProbeSpec{}
		}
		dst.Spec.ReadinessProbe.GuestInfo = src.Spec.ReadinessProbe.GuestInfo
	}
}

func restoreV1Alpha1VirtualMachineCryptoSpec(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Crypto = src.Spec.Crypto
}

func restoreV1Alpha1VirtualMachineGroupName(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.GroupName = src.Spec.GroupName
}

func restoreV1Alpha1VirtualMachineAffinitySpec(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Affinity = src.Spec.Affinity
}

func convertV1Alpha1PreReqsReadyConditionToV1Alpha6Conditions(dst *vmopv1.VirtualMachine) []metav1.Condition {
	var preReqCond, vmClassCond, vmImageCond, vmSetResourcePolicyCond, vmBootstrapCond *metav1.Condition
	var preReqCondIdx int

	for i := range dst.Status.Conditions {
		c := &dst.Status.Conditions[i]
		switch c.Type {
		case string(vmopv1a1.VirtualMachinePrereqReadyCondition):
			preReqCond = c
			preReqCondIdx = i
		case vmopv1.VirtualMachineConditionClassReady:
			vmClassCond = c
		case vmopv1.VirtualMachineConditionImageReady:
			vmImageCond = c
		case vmopv1.VirtualMachineConditionVMSetResourcePolicyReady:
			vmSetResourcePolicyCond = c
		case vmopv1.VirtualMachineConditionBootstrapReady:
			vmBootstrapCond = c
		}
	}

	if preReqCond == nil {
		return dst.Status.Conditions
	}

	dst.Status.Conditions = append(dst.Status.Conditions[:preReqCondIdx], dst.Status.Conditions[preReqCondIdx+1:]...)

	if vmClassCond != nil || vmImageCond != nil || vmSetResourcePolicyCond != nil || vmBootstrapCond != nil {
		return dst.Status.Conditions
	}

	var conditions []metav1.Condition
	if preReqCond.Status == metav1.ConditionTrue {
		conditions = append(conditions, metav1.Condition{
			Type:               vmopv1.VirtualMachineConditionClassReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: preReqCond.LastTransitionTime,
			Reason:             string(metav1.ConditionTrue),
		})
		conditions = append(conditions, metav1.Condition{
			Type:               vmopv1.VirtualMachineConditionImageReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: preReqCond.LastTransitionTime,
			Reason:             string(metav1.ConditionTrue),
		})
		if dst.Spec.Reserved != nil && dst.Spec.Reserved.ResourcePolicyName != "" {
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionVMSetResourcePolicyReady,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionTrue),
			})
		}
		if dst.Spec.Bootstrap != nil {
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionBootstrapReady,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionTrue),
			})
		}
	} else if preReqCond.Status == metav1.ConditionFalse {
		if preReqCond.Reason == string(vmopv1a1.VirtualMachineClassNotFoundReason) ||
			preReqCond.Reason == string(vmopv1a1.VirtualMachineClassBindingNotFoundReason) {
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionClassReady,
				Status:             metav1.ConditionFalse,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             preReqCond.Reason,
			})
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionImageReady,
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionUnknown),
			})
		} else if preReqCond.Reason == string(vmopv1a1.VirtualMachineImageNotFoundReason) ||
			preReqCond.Reason == string(vmopv1a1.VirtualMachineImageNotReadyReason) ||
			preReqCond.Reason == string(vmopv1a1.ContentSourceBindingNotFoundReason) ||
			preReqCond.Reason == string(vmopv1a1.ContentLibraryProviderNotFoundReason) {
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionClassReady,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionTrue),
			})
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionImageReady,
				Status:             metav1.ConditionFalse,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             preReqCond.Reason,
			})
		}
		if dst.Spec.Reserved != nil && dst.Spec.Reserved.ResourcePolicyName != "" {
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionVMSetResourcePolicyReady,
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionUnknown),
			})
		}
		if dst.Spec.Bootstrap != nil {
			conditions = append(conditions, metav1.Condition{
				Type:               vmopv1.VirtualMachineConditionBootstrapReady,
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionUnknown),
			})
		}
	}

	return append(dst.Status.Conditions, conditions...)
}

// restoreV1Alpha2VirtualMachineSpecNetworkDomainName restores the DomainName
// from the hub if the down-conversion lost it (v1alpha2 stores it in sysprep).
func restoreV1Alpha2VirtualMachineSpecNetworkDomainName(dst, src *vmopv1.VirtualMachine) {
	var dstDN, srcDN string
	if net := dst.Spec.Network; net != nil {
		dstDN = net.DomainName
	}
	if net := src.Spec.Network; net != nil {
		srcDN = net.DomainName
	}
	if dstDN == "" && srcDN != dstDN {
		if dst.Spec.Network == nil {
			dst.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
		}
		dst.Spec.Network.DomainName = srcDN
	}
}

// ============================================================
// v1alpha1 converter functions
// ============================================================

func convertVMHubToV1Alpha1(_ context.Context, hub *vmopv1.VirtualMachine, spoke *vmopv1a1.VirtualMachine) error {
	if err := vmopv1a1.Convert_v1alpha6_VirtualMachine_To_v1alpha1_VirtualMachine(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMV1Alpha1ToHub(_ context.Context, spoke *vmopv1a1.VirtualMachine, hub *vmopv1.VirtualMachine) error {
	if err := vmopv1a1.Convert_v1alpha1_VirtualMachine_To_v1alpha6_VirtualMachine(spoke, hub, nil); err != nil {
		return err
	}

	restored := &vmopv1.VirtualMachine{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil {
		return err
	}
	if !ok {
		hub.Status.Conditions = convertV1Alpha1PreReqsReadyConditionToV1Alpha6Conditions(hub)
		return nil
	}

	restoreVirtualMachineImage(hub, restored)
	restoreV1Alpha1VirtualMachineBootstrapSpec(hub, restored)
	restoreV1Alpha1VirtualMachineNetworkSpec(hub, restored)
	restoreV1Alpha1VirtualMachineReadinessProbeSpec(hub, restored)
	restoreVirtualMachineBiosUUID(hub, restored)
	restoreVirtualMachineBootstrapCloudInitInstanceID(hub, restored)
	restoreVirtualMachineInstanceUUID(hub, restored)
	restoreVirtualMachineGuestID(hub, restored)
	restoreV1Alpha1VirtualMachineCryptoSpec(hub, restored)
	restoreVirtualMachinePromoteDisksMode(hub, restored)
	restoreVirtualMachineBootOptions(hub, restored)
	restoreV1Alpha1VirtualMachineAffinitySpec(hub, restored)
	restoreV1Alpha1VirtualMachineGroupName(hub, restored)
	restoreV1Alpha1VirtualMachineVolumes(hub, restored)
	restoreV1Alpha1VirtualMachineHardware(hub, restored)
	restoreVirtualMachinePolicies(hub, restored)
	restoreVirtualMachineVolumeAttributesClassName(hub, restored)
	restoreV1Alpha1VirtualMachineAdvanced(hub, restored)
	restoreVirtualMachineCurrentSnapshotName(hub, restored)
	restoreVirtualMachineResources(hub, restored)
	restoreVirtualMachineCPUAdvanced(hub, restored)
	restoreVirtualMachineMemoryAdvanced(hub, restored)

	hub.Status = restored.Status
	return nil
}

// ============================================================
// v1alpha2 converter functions
// ============================================================

func convertVMHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachine, spoke *vmopv1a2.VirtualMachine) error {
	if err := vmopv1a2.Convert_v1alpha6_VirtualMachine_To_v1alpha2_VirtualMachine(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachine, hub *vmopv1.VirtualMachine) error {
	if err := vmopv1a2.Convert_v1alpha2_VirtualMachine_To_v1alpha6_VirtualMachine(spoke, hub, nil); err != nil {
		return err
	}

	restored := &vmopv1.VirtualMachine{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}

	restoreVirtualMachineImage(hub, restored)
	restoreVirtualMachineInstanceUUID(hub, restored)
	restoreVirtualMachineBiosUUID(hub, restored)
	restoreVirtualMachineBootstrapCloudInitInstanceID(hub, restored)
	restoreVirtualMachineBootstrapCloudInitWaitOnNetwork(hub, restored)
	restoreVirtualMachineBootstrapLinuxPrep(hub, restored)
	restoreVirtualMachineBootstrapSysprep(hub, restored)
	restoreVirtualMachineBootstrapDisabled(hub, restored)
	restoreV1Alpha2VirtualMachineSpecNetworkDomainName(hub, restored)
	restoreVirtualMachineGuestID(hub, restored)
	restoreVirtualMachinePromoteDisksMode(hub, restored)
	restoreVirtualMachineBootOptions(hub, restored)
	restoreVirtualMachineVolumes(hub, restored)
	restoreVirtualMachineHardwareShared(hub, restored)
	restoreVirtualMachinePolicies(hub, restored)
	restoreVirtualMachineCryptoVTPM(hub, restored)
	restoreVirtualMachineAffinity(hub, restored)
	restoreVirtualMachineVolumeAttributesClassName(hub, restored)
	restoreVirtualMachineNetworkVLANs(hub, restored)
	restoreV1Alpha2VirtualMachineAdvanced(hub, restored)
	restoreVirtualMachineNetworkInterfaces(hub, restored)
	restoreVirtualMachineCurrentSnapshotName(hub, restored)
	restoreVirtualMachineResources(hub, restored)
	restoreVirtualMachineCPUAdvanced(hub, restored)
	restoreVirtualMachineMemoryAdvanced(hub, restored)

	hub.Status = restored.Status
	return nil
}

// ============================================================
// v1alpha3 converter functions
// ============================================================

func convertVMHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachine, spoke *vmopv1a3.VirtualMachine) error {
	if err := vmopv1a3.Convert_v1alpha6_VirtualMachine_To_v1alpha3_VirtualMachine(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachine, hub *vmopv1.VirtualMachine) error {
	if err := vmopv1a3.Convert_v1alpha3_VirtualMachine_To_v1alpha6_VirtualMachine(spoke, hub, nil); err != nil {
		return err
	}

	restored := &vmopv1.VirtualMachine{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}

	restoreVirtualMachineBootstrapCloudInitWaitOnNetwork(hub, restored)
	restoreVirtualMachineBootstrapLinuxPrep(hub, restored)
	restoreVirtualMachineBootstrapSysprep(hub, restored)
	restoreVirtualMachineBootstrapDisabled(hub, restored)
	restoreVirtualMachinePromoteDisksMode(hub, restored)
	restoreVirtualMachineBootOptions(hub, restored)
	restoreVirtualMachineVolumes(hub, restored)
	restoreVirtualMachineHardwareShared(hub, restored)
	restoreVirtualMachinePolicies(hub, restored)
	restoreVirtualMachineCryptoVTPM(hub, restored)
	restoreVirtualMachineAffinity(hub, restored)
	restoreVirtualMachineVolumeAttributesClassName(hub, restored)
	restoreVirtualMachineNetworkVLANs(hub, restored)
	restoreV1Alpha2VirtualMachineAdvanced(hub, restored)
	restoreVirtualMachineNetworkInterfaces(hub, restored)
	restoreVirtualMachineCurrentSnapshotName(hub, restored)
	restoreVirtualMachineResources(hub, restored)
	restoreVirtualMachineCPUAdvanced(hub, restored)
	restoreVirtualMachineMemoryAdvanced(hub, restored)

	hub.Status = restored.Status
	return nil
}

// ============================================================
// v1alpha4 converter functions
// ============================================================

func convertVMHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachine, spoke *vmopv1a4.VirtualMachine) error {
	if err := vmopv1a4.Convert_v1alpha6_VirtualMachine_To_v1alpha4_VirtualMachine(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachine, hub *vmopv1.VirtualMachine) error {
	if err := vmopv1a4.Convert_v1alpha4_VirtualMachine_To_v1alpha6_VirtualMachine(spoke, hub, nil); err != nil {
		return err
	}

	restored := &vmopv1.VirtualMachine{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}

	restoreVirtualMachineHardwareShared(hub, restored)
	restoreVirtualMachinePolicies(hub, restored)
	restoreVirtualMachineBootOptions(hub, restored)
	restoreVirtualMachineCryptoVTPM(hub, restored)
	restoreVirtualMachineBootstrapCloudInitWaitOnNetwork(hub, restored)
	restoreVirtualMachineBootstrapLinuxPrep(hub, restored)
	restoreVirtualMachineBootstrapSysprep(hub, restored)
	restoreVirtualMachineBootstrapDisabled(hub, restored)
	restoreVirtualMachineAffinity(hub, restored)
	restoreVirtualMachineVolumes(hub, restored)
	restoreVirtualMachineVolumeAttributesClassName(hub, restored)
	restoreVirtualMachineNetworkVLANs(hub, restored)
	restoreV1Alpha2VirtualMachineAdvanced(hub, restored)
	restoreVirtualMachineNetworkInterfaces(hub, restored)
	restoreVirtualMachineCurrentSnapshotName(hub, restored)
	restoreVirtualMachineResources(hub, restored)
	restoreVirtualMachineCPUAdvanced(hub, restored)
	restoreVirtualMachineMemoryAdvanced(hub, restored)

	hub.Status = restored.Status
	return nil
}

// ============================================================
// v1alpha5 converter functions
// ============================================================

func convertVMHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachine, spoke *vmopv1a5.VirtualMachine) error {
	if err := vmopv1a5.Convert_v1alpha6_VirtualMachine_To_v1alpha5_VirtualMachine(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachine, hub *vmopv1.VirtualMachine) error {
	if err := vmopv1a5.Convert_v1alpha5_VirtualMachine_To_v1alpha6_VirtualMachine(spoke, hub, nil); err != nil {
		return err
	}

	restored := &vmopv1.VirtualMachine{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}

	restoreVirtualMachineBootstrapDisabled(hub, restored)
	restoreVirtualMachineVolumeAttributesClassName(hub, restored)
	restoreVirtualMachineNetworkVLANs(hub, restored)
	restoreV1Alpha2VirtualMachineAdvanced(hub, restored)
	restoreVirtualMachineNetworkInterfaces(hub, restored)
	restoreVirtualMachineResources(hub, restored)
	restoreVirtualMachineCPUAdvanced(hub, restored)
	restoreVirtualMachineMemoryAdvanced(hub, restored)

	hub.Status = restored.Status
	return nil
}
