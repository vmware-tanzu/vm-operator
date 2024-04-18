// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"
	"net"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
)

const (
	// Well known device key used for the first disk.
	bootDiskDeviceKey = 2000
)

func Convert_v1alpha1_VirtualMachineVolume_To_v1alpha3_VirtualMachineVolume(
	in *VirtualMachineVolume, out *v1alpha3.VirtualMachineVolume, s apiconversion.Scope) error {

	if claim := in.PersistentVolumeClaim; claim != nil {
		out.PersistentVolumeClaim = &v1alpha3.PersistentVolumeClaimVolumeSource{
			PersistentVolumeClaimVolumeSource: claim.PersistentVolumeClaimVolumeSource,
		}

		if claim.InstanceVolumeClaim != nil {
			out.PersistentVolumeClaim.InstanceVolumeClaim = &v1alpha3.InstanceVolumeClaimVolumeSource{}

			if err := Convert_v1alpha1_InstanceVolumeClaimVolumeSource_To_v1alpha3_InstanceVolumeClaimVolumeSource(
				claim.InstanceVolumeClaim, out.PersistentVolumeClaim.InstanceVolumeClaim, s); err != nil {
				return err
			}
		}
	}

	// NOTE: in.VsphereVolume is dropped in nextver. See filter_out_VirtualMachineVolumes_VsphereVolumes().

	return autoConvert_v1alpha1_VirtualMachineVolume_To_v1alpha3_VirtualMachineVolume(in, out, s)
}

func convert_v1alpha1_VirtualMachinePowerState_To_v1alpha3_VirtualMachinePowerState(
	in VirtualMachinePowerState) v1alpha3.VirtualMachinePowerState {

	switch in {
	case VirtualMachinePoweredOff:
		return v1alpha3.VirtualMachinePowerStateOff
	case VirtualMachinePoweredOn:
		return v1alpha3.VirtualMachinePowerStateOn
	case VirtualMachineSuspended:
		return v1alpha3.VirtualMachinePowerStateSuspended
	}

	return v1alpha3.VirtualMachinePowerState(in)
}

func convert_v1alpha3_VirtualMachinePowerState_To_v1alpha1_VirtualMachinePowerState(
	in v1alpha3.VirtualMachinePowerState) VirtualMachinePowerState {

	switch in {
	case v1alpha3.VirtualMachinePowerStateOff:
		return VirtualMachinePoweredOff
	case v1alpha3.VirtualMachinePowerStateOn:
		return VirtualMachinePoweredOn
	case v1alpha3.VirtualMachinePowerStateSuspended:
		return VirtualMachineSuspended
	}

	return VirtualMachinePowerState(in)
}

func convert_v1alpha1_VirtualMachinePowerOpMode_To_v1alpha3_VirtualMachinePowerOpMode(
	in VirtualMachinePowerOpMode) v1alpha3.VirtualMachinePowerOpMode {

	switch in {
	case VirtualMachinePowerOpModeHard:
		return v1alpha3.VirtualMachinePowerOpModeHard
	case VirtualMachinePowerOpModeSoft:
		return v1alpha3.VirtualMachinePowerOpModeSoft
	case VirtualMachinePowerOpModeTrySoft:
		return v1alpha3.VirtualMachinePowerOpModeTrySoft
	}

	return v1alpha3.VirtualMachinePowerOpMode(in)
}

func convert_v1alpha3_VirtualMachinePowerOpMode_To_v1alpha1_VirtualMachinePowerOpMode(
	in v1alpha3.VirtualMachinePowerOpMode) VirtualMachinePowerOpMode {

	switch in {
	case v1alpha3.VirtualMachinePowerOpModeHard:
		return VirtualMachinePowerOpModeHard
	case v1alpha3.VirtualMachinePowerOpModeSoft:
		return VirtualMachinePowerOpModeSoft
	case v1alpha3.VirtualMachinePowerOpModeTrySoft:
		return VirtualMachinePowerOpModeTrySoft
	}

	return VirtualMachinePowerOpMode(in)
}

func convert_v1alpha3_Conditions_To_v1alpha1_Phase(
	in []metav1.Condition) VMStatusPhase {

	// In practice, "Created" is the only really important value because some consumers
	// like CAPI use that as a part of their VM is-ready check.
	for _, c := range in {
		if c.Type == v1alpha3.VirtualMachineConditionCreated {
			if c.Status == metav1.ConditionTrue {
				return Created
			}
			return Creating
		}
	}

	return Unknown
}

func Convert_v1alpha3_VirtualMachineVolume_To_v1alpha1_VirtualMachineVolume(
	in *v1alpha3.VirtualMachineVolume, out *VirtualMachineVolume, s apiconversion.Scope) error {

	if claim := in.PersistentVolumeClaim; claim != nil {
		out.PersistentVolumeClaim = &PersistentVolumeClaimVolumeSource{
			PersistentVolumeClaimVolumeSource: claim.PersistentVolumeClaimVolumeSource,
		}

		if claim.InstanceVolumeClaim != nil {
			out.PersistentVolumeClaim.InstanceVolumeClaim = &InstanceVolumeClaimVolumeSource{}

			if err := Convert_v1alpha3_InstanceVolumeClaimVolumeSource_To_v1alpha1_InstanceVolumeClaimVolumeSource(
				claim.InstanceVolumeClaim, out.PersistentVolumeClaim.InstanceVolumeClaim, s); err != nil {
				return err
			}
		}
	}

	return autoConvert_v1alpha3_VirtualMachineVolume_To_v1alpha1_VirtualMachineVolume(in, out, s)
}

func convert_v1alpha1_VmMetadata_To_v1alpha3_BootstrapSpec(
	in *VirtualMachineMetadata) *v1alpha3.VirtualMachineBootstrapSpec {

	if in == nil || apiequality.Semantic.DeepEqual(*in, VirtualMachineMetadata{}) {
		return nil
	}

	out := v1alpha3.VirtualMachineBootstrapSpec{}

	objectName := in.SecretName
	if objectName == "" {
		objectName = in.ConfigMapName
	}

	switch in.Transport {
	case VirtualMachineMetadataExtraConfigTransport:
		// This transport is obsolete. It should be combined with LinuxPrep but in nextver we don't
		// allow CloudInit w/ LinuxPrep.
		// out.LinuxPrep = &v1alpha3.VirtualMachineBootstrapLinuxPrepSpec{HardwareClockIsUTC: true}

		out.CloudInit = &v1alpha3.VirtualMachineBootstrapCloudInitSpec{}
		if objectName != "" {
			out.CloudInit.RawCloudConfig = &common.SecretKeySelector{
				Name: objectName,
				Key:  "guestinfo.userdata",
			}
		}
	case VirtualMachineMetadataOvfEnvTransport:
		out.LinuxPrep = &v1alpha3.VirtualMachineBootstrapLinuxPrepSpec{
			HardwareClockIsUTC: true,
		}
		out.VAppConfig = &v1alpha3.VirtualMachineBootstrapVAppConfigSpec{
			RawProperties: objectName,
		}
	case VirtualMachineMetadataVAppConfigTransport:
		out.VAppConfig = &v1alpha3.VirtualMachineBootstrapVAppConfigSpec{
			RawProperties: objectName,
		}
	case VirtualMachineMetadataCloudInitTransport:
		out.CloudInit = &v1alpha3.VirtualMachineBootstrapCloudInitSpec{}
		if objectName != "" {
			out.CloudInit.RawCloudConfig = &common.SecretKeySelector{
				Name: objectName,
				Key:  "user-data",
			}
		}
	case VirtualMachineMetadataSysprepTransport:
		out.Sysprep = &v1alpha3.VirtualMachineBootstrapSysprepSpec{}
		if objectName != "" {
			out.Sysprep.RawSysprep = &common.SecretKeySelector{
				Name: objectName,
				Key:  "unattend",
			}
		}
	}

	return &out
}

func convert_v1alpha3_BootstrapSpec_To_v1alpha1_VmMetadata(
	in *v1alpha3.VirtualMachineBootstrapSpec) *VirtualMachineMetadata {

	if in == nil || apiequality.Semantic.DeepEqual(*in, v1alpha3.VirtualMachineBootstrapSpec{}) {
		return nil
	}

	// TODO: nextver only has a Secret bootstrap field so that's what we set in v1a1. If this was created
	// as v1a1, we need to store the serialized object to know to set either the ConfigMap or Secret field.
	out := &VirtualMachineMetadata{}

	if cloudInit := in.CloudInit; cloudInit != nil {
		if cloudInit.RawCloudConfig != nil {
			out.SecretName = cloudInit.RawCloudConfig.Name

			switch cloudInit.RawCloudConfig.Key {
			case "guestinfo.userdata":
				out.Transport = VirtualMachineMetadataExtraConfigTransport
			case "user-data":
				out.Transport = VirtualMachineMetadataCloudInitTransport
			default:
				// Best approx we can do.
				out.Transport = VirtualMachineMetadataCloudInitTransport
			}
		} else {
			out.Transport = VirtualMachineMetadataCloudInitTransport
		}
	} else if sysprep := in.Sysprep; sysprep != nil {
		out.Transport = VirtualMachineMetadataSysprepTransport
		if in.Sysprep.RawSysprep != nil {
			out.SecretName = sysprep.RawSysprep.Name
		}
	} else if in.VAppConfig != nil {
		out.SecretName = in.VAppConfig.RawProperties

		if in.LinuxPrep != nil {
			out.Transport = VirtualMachineMetadataOvfEnvTransport
		} else {
			out.Transport = VirtualMachineMetadataVAppConfigTransport
		}
	}

	return out
}

func convert_v1alpha1_NetworkInterface_To_v1alpha3_NetworkInterfaceSpec(
	idx int, in VirtualMachineNetworkInterface) v1alpha3.VirtualMachineNetworkInterfaceSpec {

	out := v1alpha3.VirtualMachineNetworkInterfaceSpec{}
	out.Name = fmt.Sprintf("eth%d", idx)

	if in.NetworkName != "" || in.NetworkType != "" {
		out.Network = &common.PartialObjectRef{}
	}

	out.Network.Name = in.NetworkName

	switch in.NetworkType {
	case "vsphere-distributed":
		out.Network.TypeMeta.APIVersion = "netoperator.vmware.com/v1alpha1"
		out.Network.TypeMeta.Kind = "Network"
	case "nsx-t":
		out.Network.TypeMeta.APIVersion = "vmware.com/v1alpha1"
		out.Network.TypeMeta.Kind = "VirtualNetwork"
	case "nsx-t-subnet":
		out.Network.TypeMeta.APIVersion = "nsx.vmware.com/v1alpha1"
		out.Network.TypeMeta.Kind = "Subnet"
	case "nsx-t-subnetset":
		out.Network.TypeMeta.APIVersion = "nsx.vmware.com/v1alpha1"
		out.Network.TypeMeta.Kind = "SubnetSet"
	}

	return out
}

func convert_v1alpha3_NetworkInterfaceSpec_To_v1alpha1_NetworkInterface(
	in v1alpha3.VirtualMachineNetworkInterfaceSpec) VirtualMachineNetworkInterface {

	if in.Network == nil {
		return VirtualMachineNetworkInterface{}
	}

	out := VirtualMachineNetworkInterface{
		NetworkName: in.Network.Name,
	}

	switch in.Network.TypeMeta.Kind {
	case "Network":
		out.NetworkType = "vsphere-distributed"
	case "VirtualNetwork":
		out.NetworkType = "nsx-t"
	case "SubnetSet":
		out.NetworkType = "nsx-t-subnetset"
	case "Subnet":
		out.NetworkType = "nsx-t-subnet"
	}

	return out
}

func Convert_v1alpha1_Probe_To_v1alpha3_VirtualMachineReadinessProbeSpec(in *Probe, out *v1alpha3.VirtualMachineReadinessProbeSpec, s apiconversion.Scope) error {
	probeSpec := convert_v1alpha1_Probe_To_v1alpha3_ReadinessProbeSpec(in)
	if probeSpec != nil {
		*out = *probeSpec
	}

	return nil
}

func convert_v1alpha1_Probe_To_v1alpha3_ReadinessProbeSpec(in *Probe) *v1alpha3.VirtualMachineReadinessProbeSpec {

	if in == nil || apiequality.Semantic.DeepEqual(*in, Probe{}) {
		return nil
	}

	out := v1alpha3.VirtualMachineReadinessProbeSpec{}

	out.TimeoutSeconds = in.TimeoutSeconds
	out.PeriodSeconds = in.PeriodSeconds

	if in.TCPSocket != nil {
		out.TCPSocket = &v1alpha3.TCPSocketAction{
			Port: in.TCPSocket.Port,
			Host: in.TCPSocket.Host,
		}
	}

	if in.GuestHeartbeat != nil {
		out.GuestHeartbeat = &v1alpha3.GuestHeartbeatAction{
			ThresholdStatus: v1alpha3.GuestHeartbeatStatus(in.GuestHeartbeat.ThresholdStatus),
		}
	}

	// out.GuestInfo =

	return &out
}

func Convert_v1alpha3_VirtualMachineReadinessProbeSpec_To_v1alpha1_Probe(in *v1alpha3.VirtualMachineReadinessProbeSpec, out *Probe, s apiconversion.Scope) error {
	probe := convert_v1alpha3_ReadinessProbeSpec_To_v1alpha1_Probe(in)
	if probe != nil {
		*out = *probe
	}

	return nil
}

func convert_v1alpha3_ReadinessProbeSpec_To_v1alpha1_Probe(in *v1alpha3.VirtualMachineReadinessProbeSpec) *Probe {

	if in == nil || apiequality.Semantic.DeepEqual(*in, v1alpha3.VirtualMachineReadinessProbeSpec{}) {
		return nil
	}

	out := &Probe{
		TimeoutSeconds: in.TimeoutSeconds,
		PeriodSeconds:  in.PeriodSeconds,
	}

	if in.TCPSocket != nil {
		out.TCPSocket = &TCPSocketAction{
			Port: in.TCPSocket.Port,
			Host: in.TCPSocket.Host,
		}
	}

	if in.GuestHeartbeat != nil {
		out.GuestHeartbeat = &GuestHeartbeatAction{
			ThresholdStatus: GuestHeartbeatStatus(in.GuestHeartbeat.ThresholdStatus),
		}
	}

	return out
}

func convert_v1alpha1_VirtualMachineAdvancedOptions_To_v1alpha3_VirtualMachineAdvancedSpec(
	in *VirtualMachineAdvancedOptions, inVolumes []VirtualMachineVolume) *v1alpha3.VirtualMachineAdvancedSpec {

	bootDiskCapacity := convert_v1alpha1_VsphereVolumes_To_v1alpha3_BootDiskCapacity(inVolumes)

	if (in == nil || apiequality.Semantic.DeepEqual(*in, VirtualMachineAdvancedOptions{})) && bootDiskCapacity == nil {
		return nil
	}

	out := v1alpha3.VirtualMachineAdvancedSpec{}
	out.BootDiskCapacity = bootDiskCapacity

	if in != nil {
		if opts := in.DefaultVolumeProvisioningOptions; opts != nil {
			if opts.ThinProvisioned != nil {
				if *opts.ThinProvisioned {
					out.DefaultVolumeProvisioningMode = v1alpha3.VirtualMachineVolumeProvisioningModeThin
				} else {
					out.DefaultVolumeProvisioningMode = v1alpha3.VirtualMachineVolumeProvisioningModeThick
				}
			} else if opts.EagerZeroed != nil && *opts.EagerZeroed {
				out.DefaultVolumeProvisioningMode = v1alpha3.VirtualMachineVolumeProvisioningModeThickEagerZero
			}
		}

		if in.ChangeBlockTracking != nil {
			out.ChangeBlockTracking = *in.ChangeBlockTracking
		}
	}

	return &out
}

func convert_v1alpha1_VsphereVolumes_To_v1alpha3_BootDiskCapacity(volumes []VirtualMachineVolume) *resource.Quantity {
	// The v1a1 VsphereVolume was never a great API as you had to know the DeviceKey upfront; at the time our
	// API was private - only used by CAPW - and predates the "VM Service" VMs; In nextver, we only support resizing
	// the boot disk via an explicit field. As good as we can here, map v1a1 volume into the nextver specific field.

	for i := range volumes {
		vsVol := volumes[i].VsphereVolume

		if vsVol != nil && vsVol.DeviceKey != nil && *vsVol.DeviceKey == bootDiskDeviceKey {
			// This VsphereVolume has the well-known boot disk device key. Return that capacity if set.
			if capacity := vsVol.Capacity.StorageEphemeral(); capacity != nil {
				return capacity
			}
			break
		}
	}

	return nil
}

func convert_v1alpha3_VirtualMachineAdvancedSpec_To_v1alpha1_VirtualMachineAdvancedOptions(
	in *v1alpha3.VirtualMachineAdvancedSpec) *VirtualMachineAdvancedOptions {

	if in == nil || apiequality.Semantic.DeepEqual(*in, v1alpha3.VirtualMachineAdvancedSpec{}) {
		return nil
	}

	out := &VirtualMachineAdvancedOptions{}

	if in.ChangeBlockTracking {
		out.ChangeBlockTracking = ptr.To(true)
	}

	switch in.DefaultVolumeProvisioningMode {
	case v1alpha3.VirtualMachineVolumeProvisioningModeThin:
		out.DefaultVolumeProvisioningOptions = &VirtualMachineVolumeProvisioningOptions{
			ThinProvisioned: ptr.To(true),
		}
	case v1alpha3.VirtualMachineVolumeProvisioningModeThick:
		out.DefaultVolumeProvisioningOptions = &VirtualMachineVolumeProvisioningOptions{
			ThinProvisioned: ptr.To(false),
		}
	case v1alpha3.VirtualMachineVolumeProvisioningModeThickEagerZero:
		out.DefaultVolumeProvisioningOptions = &VirtualMachineVolumeProvisioningOptions{
			EagerZeroed: ptr.To(true),
		}
	}

	if reflect.DeepEqual(out, &VirtualMachineAdvancedOptions{}) {
		return nil
	}

	return out
}

func convert_v1alpha3_BootDiskCapacity_To_v1alpha1_VirtualMachineVolume(capacity *resource.Quantity) *VirtualMachineVolume {
	if capacity == nil {
		return nil
	}

	const name = "vmoperator-vm-boot-disk"

	return &VirtualMachineVolume{
		Name: name,
		VsphereVolume: &VsphereVolumeSource{
			Capacity: corev1.ResourceList{
				corev1.ResourceEphemeralStorage: *capacity,
			},
			DeviceKey: ptr.To(bootDiskDeviceKey),
		},
	}
}

func convert_v1alpha1_Network_To_v1alpha3_NetworkStatus(
	vmIP string, in []NetworkInterfaceStatus) *v1alpha3.VirtualMachineNetworkStatus {

	if vmIP == "" && len(in) == 0 {
		return nil
	}

	out := &v1alpha3.VirtualMachineNetworkStatus{}

	if net.ParseIP(vmIP).To4() != nil {
		out.PrimaryIP4 = vmIP
	} else {
		out.PrimaryIP6 = vmIP
	}

	ipAddrsToAddrStatus := func(ipAddr []string) []v1alpha3.VirtualMachineNetworkInterfaceIPAddrStatus {
		statuses := make([]v1alpha3.VirtualMachineNetworkInterfaceIPAddrStatus, 0, len(ipAddr))
		for _, ip := range ipAddr {
			statuses = append(statuses, v1alpha3.VirtualMachineNetworkInterfaceIPAddrStatus{Address: ip})
		}
		return statuses
	}

	for _, inI := range in {
		interfaceStatus := v1alpha3.VirtualMachineNetworkInterfaceStatus{
			IP: &v1alpha3.VirtualMachineNetworkInterfaceIPStatus{
				Addresses: ipAddrsToAddrStatus(inI.IpAddresses),
				MACAddr:   inI.MacAddress,
			},
		}
		out.Interfaces = append(out.Interfaces, interfaceStatus)
	}

	return out
}

func convert_v1alpha3_NetworkStatus_To_v1alpha1_Network(
	in *v1alpha3.VirtualMachineNetworkStatus) (string, []NetworkInterfaceStatus) {

	if in == nil {
		return "", nil
	}

	vmIP := in.PrimaryIP4
	if vmIP == "" {
		vmIP = in.PrimaryIP6
	}

	addrStatusToIPAddrs := func(addrStatus []v1alpha3.VirtualMachineNetworkInterfaceIPAddrStatus) []string {
		ipAddrs := make([]string, 0, len(addrStatus))
		for _, a := range addrStatus {
			ipAddrs = append(ipAddrs, a.Address)
		}
		return ipAddrs
	}

	out := make([]NetworkInterfaceStatus, 0, len(in.Interfaces))
	for _, i := range in.Interfaces {
		interfaceStatus := NetworkInterfaceStatus{
			Connected: true,
		}
		if i.IP != nil {
			interfaceStatus.MacAddress = i.IP.MACAddr
			interfaceStatus.IpAddresses = addrStatusToIPAddrs(i.IP.Addresses)
		}
		out = append(out, interfaceStatus)
	}

	return vmIP, out
}

// In nextver we've dropped the v1a1 VsphereVolumes, and in its place we have a single field for the boot
// disk size. The Convert_v1alpha1_VirtualMachineVolume_To_v1alpha3_VirtualMachineVolume() stub does not
// allow us to not return something so filter those volumes - without a PersistentVolumeClaim set - here.
func filter_out_VirtualMachineVolumes_VsphereVolumes(in []v1alpha3.VirtualMachineVolume) []v1alpha3.VirtualMachineVolume {

	if len(in) == 0 {
		return nil
	}

	out := make([]v1alpha3.VirtualMachineVolume, 0, len(in))

	for _, v := range in {
		if v.PersistentVolumeClaim != nil {
			out = append(out, v)
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func Convert_v1alpha1_VirtualMachineSpec_To_v1alpha3_VirtualMachineSpec(
	in *VirtualMachineSpec, out *v1alpha3.VirtualMachineSpec, s apiconversion.Scope) error {

	// The generated auto convert will convert the power modes as-is strings which breaks things, so keep
	// this first.
	if err := autoConvert_v1alpha1_VirtualMachineSpec_To_v1alpha3_VirtualMachineSpec(in, out, s); err != nil {
		return err
	}

	out.PowerState = convert_v1alpha1_VirtualMachinePowerState_To_v1alpha3_VirtualMachinePowerState(in.PowerState)
	out.PowerOffMode = convert_v1alpha1_VirtualMachinePowerOpMode_To_v1alpha3_VirtualMachinePowerOpMode(in.PowerOffMode)
	out.SuspendMode = convert_v1alpha1_VirtualMachinePowerOpMode_To_v1alpha3_VirtualMachinePowerOpMode(in.SuspendMode)
	out.NextRestartTime = in.NextRestartTime
	out.RestartMode = convert_v1alpha1_VirtualMachinePowerOpMode_To_v1alpha3_VirtualMachinePowerOpMode(in.RestartMode)
	out.Bootstrap = convert_v1alpha1_VmMetadata_To_v1alpha3_BootstrapSpec(in.VmMetadata)
	out.Volumes = filter_out_VirtualMachineVolumes_VsphereVolumes(out.Volumes)

	if len(in.NetworkInterfaces) > 0 {
		out.Network = &v1alpha3.VirtualMachineNetworkSpec{}
		for i, networkInterface := range in.NetworkInterfaces {
			networkInterfaceSpec := convert_v1alpha1_NetworkInterface_To_v1alpha3_NetworkInterfaceSpec(i, networkInterface)
			out.Network.Interfaces = append(out.Network.Interfaces, networkInterfaceSpec)
		}
	}

	out.ReadinessProbe = convert_v1alpha1_Probe_To_v1alpha3_ReadinessProbeSpec(in.ReadinessProbe)
	out.Advanced = convert_v1alpha1_VirtualMachineAdvancedOptions_To_v1alpha3_VirtualMachineAdvancedSpec(in.AdvancedOptions, in.Volumes)

	if in.ResourcePolicyName != "" {
		if out.Reserved == nil {
			out.Reserved = &v1alpha3.VirtualMachineReservedSpec{}
		}
		out.Reserved.ResourcePolicyName = in.ResourcePolicyName
	}

	// Deprecated:
	// in.Ports

	return nil
}

func Convert_v1alpha3_VirtualMachineSpec_To_v1alpha1_VirtualMachineSpec(
	in *v1alpha3.VirtualMachineSpec, out *VirtualMachineSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineSpec_To_v1alpha1_VirtualMachineSpec(in, out, s); err != nil {
		return err
	}

	out.PowerState = convert_v1alpha3_VirtualMachinePowerState_To_v1alpha1_VirtualMachinePowerState(in.PowerState)
	out.PowerOffMode = convert_v1alpha3_VirtualMachinePowerOpMode_To_v1alpha1_VirtualMachinePowerOpMode(in.PowerOffMode)
	out.SuspendMode = convert_v1alpha3_VirtualMachinePowerOpMode_To_v1alpha1_VirtualMachinePowerOpMode(in.SuspendMode)
	out.NextRestartTime = in.NextRestartTime
	out.RestartMode = convert_v1alpha3_VirtualMachinePowerOpMode_To_v1alpha1_VirtualMachinePowerOpMode(in.RestartMode)
	out.VmMetadata = convert_v1alpha3_BootstrapSpec_To_v1alpha1_VmMetadata(in.Bootstrap)

	if in.Network != nil {
		out.NetworkInterfaces = make([]VirtualMachineNetworkInterface, 0, len(in.Network.Interfaces))
		for _, networkInterfaceSpec := range in.Network.Interfaces {
			networkInterface := convert_v1alpha3_NetworkInterfaceSpec_To_v1alpha1_NetworkInterface(networkInterfaceSpec)
			out.NetworkInterfaces = append(out.NetworkInterfaces, networkInterface)
		}
	}

	out.ReadinessProbe = convert_v1alpha3_ReadinessProbeSpec_To_v1alpha1_Probe(in.ReadinessProbe)
	out.AdvancedOptions = convert_v1alpha3_VirtualMachineAdvancedSpec_To_v1alpha1_VirtualMachineAdvancedOptions(in.Advanced)

	if in.Reserved != nil {
		out.ResourcePolicyName = in.Reserved.ResourcePolicyName
	}

	if in.Advanced != nil {
		if bootDiskVol := convert_v1alpha3_BootDiskCapacity_To_v1alpha1_VirtualMachineVolume(in.Advanced.BootDiskCapacity); bootDiskVol != nil {
			out.Volumes = append(out.Volumes, *bootDiskVol)
		}
	}

	// If out.imageName is empty but in.image.name is non-empty, then on down-
	// convert, copy in.image.name to out.imageName.
	if out.ImageName == "" && in.Image != nil {
		out.ImageName = in.Image.Name
	}

	// TODO = in.ReadinessGates

	// Deprecated:
	// out.Ports

	return nil
}

func Convert_v1alpha1_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(
	in *VirtualMachineVolumeStatus, out *v1alpha3.VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	out.DiskUUID = in.DiskUuid

	return autoConvert_v1alpha1_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha3_VirtualMachineVolumeStatus_To_v1alpha1_VirtualMachineVolumeStatus(
	in *v1alpha3.VirtualMachineVolumeStatus, out *VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	out.DiskUuid = in.DiskUUID

	return autoConvert_v1alpha3_VirtualMachineVolumeStatus_To_v1alpha1_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha1_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(
	in *VirtualMachineStatus, out *v1alpha3.VirtualMachineStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha1_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(in, out, s); err != nil {
		return err
	}

	out.PowerState = convert_v1alpha1_VirtualMachinePowerState_To_v1alpha3_VirtualMachinePowerState(in.PowerState)
	out.Network = convert_v1alpha1_Network_To_v1alpha3_NetworkStatus(in.VmIp, in.NetworkInterfaces)

	// WARNING: in.Phase requires manual conversion: does not exist in peer-type

	return nil
}

func translate_v1alpha3_Conditions_To_v1alpha1_Conditions(conditions []Condition) []Condition {
	var preReqCond, vmClassCond, vmImageCond, vmSetResourcePolicyCond, vmBootstrapCond *Condition

	for i := range conditions {
		c := &conditions[i]

		switch c.Type {
		case VirtualMachinePrereqReadyCondition:
			preReqCond = c
		case v1alpha3.VirtualMachineConditionClassReady:
			vmClassCond = c
		case v1alpha3.VirtualMachineConditionImageReady:
			vmImageCond = c
		case v1alpha3.VirtualMachineConditionVMSetResourcePolicyReady:
			vmSetResourcePolicyCond = c
		case v1alpha3.VirtualMachineConditionBootstrapReady:
			vmBootstrapCond = c
		}
	}

	// Try to replicate how the v1a1 provider would set the singular prereqs condition. The class is checked
	// first, then the image. Note that the set resource policy and metadata (bootstrap) are not a part of
	// the v1a1 prereqs, and are optional.
	if vmClassCond != nil && vmClassCond.Status == corev1.ConditionTrue &&
		vmImageCond != nil && vmImageCond.Status == corev1.ConditionTrue &&
		(vmSetResourcePolicyCond == nil || vmSetResourcePolicyCond.Status == corev1.ConditionTrue) &&
		(vmBootstrapCond == nil || vmBootstrapCond.Status == corev1.ConditionTrue) {

		p := Condition{
			Type:   VirtualMachinePrereqReadyCondition,
			Status: corev1.ConditionTrue,
		}

		if preReqCond != nil {
			p.LastTransitionTime = preReqCond.LastTransitionTime
			*preReqCond = p
			return conditions
		}

		p.LastTransitionTime = vmImageCond.LastTransitionTime
		return append(conditions, p)
	}

	p := Condition{
		Type:     VirtualMachinePrereqReadyCondition,
		Status:   corev1.ConditionFalse,
		Severity: ConditionSeverityError,
	}

	if vmClassCond != nil && vmClassCond.Status == corev1.ConditionFalse {
		p.Reason = VirtualMachineClassNotFoundReason
		p.Message = vmClassCond.Message
		p.LastTransitionTime = vmClassCond.LastTransitionTime
	} else if vmImageCond != nil && vmImageCond.Status == corev1.ConditionFalse {
		p.Reason = VirtualMachineImageNotFoundReason
		p.Message = vmImageCond.Message
		p.LastTransitionTime = vmImageCond.LastTransitionTime
	}

	if p.Reason != "" {
		if preReqCond != nil {
			*preReqCond = p
			return conditions
		}

		return append(conditions, p)
	}

	if vmSetResourcePolicyCond != nil && vmSetResourcePolicyCond.Status == corev1.ConditionFalse &&
		vmBootstrapCond != nil && vmBootstrapCond.Status == corev1.ConditionFalse {

		// These are not a part of the v1a1 Prereqs. If either is false, the v1a1 provider would not
		// update the prereqs condition, but we don't set the condition to true either until all these
		// conditions are true. Just leave things as is to see how strict we really need to be here.
		return conditions
	}

	// TBD: For now, leave the nextver conditions if present since those provide more details.

	return conditions
}

func Convert_v1alpha3_VirtualMachineStatus_To_v1alpha1_VirtualMachineStatus(
	in *v1alpha3.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineStatus_To_v1alpha1_VirtualMachineStatus(in, out, s); err != nil {
		return err
	}

	out.PowerState = convert_v1alpha3_VirtualMachinePowerState_To_v1alpha1_VirtualMachinePowerState(in.PowerState)
	out.Phase = convert_v1alpha3_Conditions_To_v1alpha1_Phase(in.Conditions)
	out.VmIp, out.NetworkInterfaces = convert_v1alpha3_NetworkStatus_To_v1alpha1_Network(in.Network)
	out.LastRestartTime = in.LastRestartTime
	out.Conditions = translate_v1alpha3_Conditions_To_v1alpha1_Conditions(out.Conditions)

	// WARNING: in.Image requires manual conversion: does not exist in peer-type
	// WARNING: in.Class requires manual conversion: does not exist in peer-type

	return nil
}

func restore_v1alpha3_VirtualMachineImage(dst, src *v1alpha3.VirtualMachine) {
	dst.Spec.Image = src.Spec.Image
	dst.Spec.ImageName = src.Spec.ImageName
}

func restore_v1alpha3_VirtualMachineBootstrapSpec(
	dst, src *v1alpha3.VirtualMachine) {

	srcBootstrap := src.Spec.Bootstrap
	if srcBootstrap == nil {
		// Nothing to restore from.
		return
	}

	dstBootstrap := dst.Spec.Bootstrap
	if dstBootstrap == nil {
		// v1a1 doesn't have a way to represent standalone LinuxPrep. If we didn't do a
		// conversion in convert_v1alpha1_VmMetadata_To_v1alpha3_BootstrapSpec() but we
		// saved a LinuxPrep in the conversion annotation, restore that here.
		if srcBootstrap.LinuxPrep != nil {
			dst.Spec.Bootstrap = &v1alpha3.VirtualMachineBootstrapSpec{
				LinuxPrep: srcBootstrap.LinuxPrep,
			}
		}
		return
	}

	mergeSecretKeySelector := func(dstSel, srcSel *common.SecretKeySelector) *common.SecretKeySelector {
		if dstSel == nil || srcSel == nil {
			return dstSel
		}

		// Restore with the new object name in case it was changed.
		newSel := *srcSel
		newSel.Name = dstSel.Name
		return &newSel
	}

	if dstCloudInit := dstBootstrap.CloudInit; dstCloudInit != nil {
		if srcCloudInit := srcBootstrap.CloudInit; srcCloudInit != nil {
			dstCloudInit.CloudConfig = srcCloudInit.CloudConfig
			dstCloudInit.RawCloudConfig = mergeSecretKeySelector(dstCloudInit.RawCloudConfig, srcCloudInit.RawCloudConfig)
			dstCloudInit.SSHAuthorizedKeys = srcCloudInit.SSHAuthorizedKeys
		}
	}

	if dstLinuxPrep := dstBootstrap.LinuxPrep; dstLinuxPrep != nil {
		if srcLinuxPrep := srcBootstrap.LinuxPrep; srcLinuxPrep != nil {
			dstLinuxPrep.HardwareClockIsUTC = srcLinuxPrep.HardwareClockIsUTC
			dstLinuxPrep.TimeZone = srcLinuxPrep.TimeZone
		}
	}

	if dstSysPrep := dstBootstrap.Sysprep; dstSysPrep != nil {
		if srcSysPrep := srcBootstrap.Sysprep; srcSysPrep != nil {
			dstSysPrep.Sysprep = srcSysPrep.Sysprep
			dstSysPrep.RawSysprep = mergeSecretKeySelector(dstSysPrep.RawSysprep, srcSysPrep.RawSysprep)

			// In v1a1 we don't have way to denote Sysprep with vAppConfig. LinuxPrep with vAppConfig works
			// because that translates to OvfEnvTransport. If we have a saved vAppConfig initialize the field
			// so we'll restore it next.
			if dstBootstrap.VAppConfig == nil && srcBootstrap.VAppConfig != nil {
				dstBootstrap.VAppConfig = &v1alpha3.VirtualMachineBootstrapVAppConfigSpec{}
			}
		}
	}

	if dstVAppConfig := dstBootstrap.VAppConfig; dstVAppConfig != nil {
		if srcVAppConfig := srcBootstrap.VAppConfig; srcVAppConfig != nil {
			dstVAppConfig.Properties = srcVAppConfig.Properties
			dstVAppConfig.RawProperties = srcVAppConfig.RawProperties
		}
	}
}

func restore_v1alpha3_VirtualMachineNetworkSpec(
	dst, src *v1alpha3.VirtualMachine) {

	srcNetwork := src.Spec.Network
	if srcNetwork == nil || apiequality.Semantic.DeepEqual(*srcNetwork, v1alpha3.VirtualMachineNetworkSpec{}) {
		// Nothing to restore.
		return
	}

	if dst.Spec.Network == nil {
		dst.Spec.Network = &v1alpha3.VirtualMachineNetworkSpec{}
	}
	dstNetwork := dst.Spec.Network

	dstNetwork.HostName = srcNetwork.HostName
	dstNetwork.Disabled = srcNetwork.Disabled
	dstNetwork.Nameservers = srcNetwork.Nameservers
	dstNetwork.SearchDomains = srcNetwork.SearchDomains

	if len(dstNetwork.Interfaces) == 0 {
		// No interfaces so nothing to fixup (the interfaces were removed): we ignore the restored interfaces.
		return
	}

	// In nextver we made the network interfaces immutable. In v1a1 one could, in theory, add, remove,
	// reorder network interfaces, but it hardly would have "worked". The exceedingly common case is
	// just one interface and the list will never change post-create. Here, we overlay the saved
	// source on to the destination, and restore the network Name and Kind. If those were changed in
	// v1a1, then we expect the VM validation webhook to fail.
	// For truly mutable network interfaces, we'll have to get a little creative on how to line the
	// src and dst network interfaces together in the face of changes.
	for i := range dstNetwork.Interfaces {
		if i >= len(srcNetwork.Interfaces) {
			break
		}
		savedDstNetRef := dstNetwork.Interfaces[i].Network
		dstNetwork.Interfaces[i] = srcNetwork.Interfaces[i]
		dstNetwork.Interfaces[i].Network = savedDstNetRef
	}
}

func restore_v1alpha3_VirtualMachineReadinessProbeSpec(
	dst, src *v1alpha3.VirtualMachine) {

	if src.Spec.ReadinessProbe != nil {
		if dst.Spec.ReadinessProbe == nil {
			dst.Spec.ReadinessProbe = &v1alpha3.VirtualMachineReadinessProbeSpec{}
		}
		dst.Spec.ReadinessProbe.GuestInfo = src.Spec.ReadinessProbe.GuestInfo
	}
}

func convert_v1alpha1_PreReqsReadyCondition_to_v1alpha3_Conditions(
	dst *v1alpha3.VirtualMachine) []metav1.Condition {

	var preReqCond, vmClassCond, vmImageCond, vmSetResourcePolicyCond, vmBootstrapCond *metav1.Condition
	var preReqCondIdx int

	for i := range dst.Status.Conditions {
		c := &dst.Status.Conditions[i]

		switch c.Type {
		case string(VirtualMachinePrereqReadyCondition):
			preReqCond = c
			preReqCondIdx = i
		case v1alpha3.VirtualMachineConditionClassReady:
			vmClassCond = c
		case v1alpha3.VirtualMachineConditionImageReady:
			vmImageCond = c
		case v1alpha3.VirtualMachineConditionVMSetResourcePolicyReady:
			vmSetResourcePolicyCond = c
		case v1alpha3.VirtualMachineConditionBootstrapReady:
			vmBootstrapCond = c
		}
	}

	// If we didn't find the v1a1 PrereqReady condition then there is nothing to do.
	if preReqCond == nil {
		return dst.Status.Conditions
	}

	// Remove the v1a1 PrereqReady condition if not nil
	dst.Status.Conditions = append(dst.Status.Conditions[:preReqCondIdx], dst.Status.Conditions[preReqCondIdx+1:]...)

	// If any of the nextver conditions were already evaluated, v1a1 PrereqReady condition has been removed, so move on.
	if vmClassCond != nil || vmImageCond != nil || vmSetResourcePolicyCond != nil || vmBootstrapCond != nil {
		return dst.Status.Conditions
	}

	var conditions []metav1.Condition
	// If we don't have any of the new nextver conditions, use the v1a1 PrereqReady condition to fill
	// that in. This means that this was originally a v1a1 VM.
	if preReqCond.Status == metav1.ConditionTrue {
		// The class and image are always required fields.
		conditions = append(conditions, metav1.Condition{
			Type:               v1alpha3.VirtualMachineConditionClassReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: preReqCond.LastTransitionTime,
			Reason:             string(metav1.ConditionTrue),
		})
		conditions = append(conditions, metav1.Condition{
			Type:               v1alpha3.VirtualMachineConditionImageReady,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: preReqCond.LastTransitionTime,
			Reason:             string(metav1.ConditionTrue),
		})

		if dst.Spec.Reserved != nil && dst.Spec.Reserved.ResourcePolicyName != "" {
			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionVMSetResourcePolicyReady,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionTrue),
			})
		}
		if dst.Spec.Bootstrap != nil {
			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionBootstrapReady,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionTrue),
			})
		}

	} else if preReqCond.Status == metav1.ConditionFalse {
		// Infer what nextver conditions need to be true/false from preReq reason.
		// Order of evaluation of objects for v1a1 PreReq Condition:
		// 1. VM Class (which includes VM Class Bindings)
		// 2. VM Image (which includes ContentSourceBindings and ContentSourceProviders)

		if preReqCond.Reason == VirtualMachineClassNotFoundReason ||
			preReqCond.Reason == VirtualMachineClassBindingNotFoundReason {
			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionClassReady,
				Status:             metav1.ConditionFalse,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             preReqCond.Reason,
			})
			// v1a1 Image preReq hasn't been evaluated yet -- mark unknown
			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionImageReady,
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionUnknown),
			})
		} else if preReqCond.Reason == VirtualMachineImageNotFoundReason ||
			preReqCond.Reason == VirtualMachineImageNotReadyReason ||
			preReqCond.Reason == ContentSourceBindingNotFoundReason ||
			preReqCond.Reason == ContentLibraryProviderNotFoundReason {

			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionClassReady,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionTrue),
			})
			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionImageReady,
				Status:             metav1.ConditionFalse,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             preReqCond.Reason,
			})
		}

		// v1a1 setPolicy has not been evaluated when preReq condition is false. Mark unknown in nextver when
		// resourcePolicy is specified in VM Spec.
		if dst.Spec.Reserved != nil && dst.Spec.Reserved.ResourcePolicyName != "" {
			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionVMSetResourcePolicyReady,
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionUnknown),
			})
		}

		// v1a1 vmMetadata has not been evaluated when preReq condition is false. Mark unknown in nextver when
		// bootstrap is specified in VM Spec.
		if dst.Spec.Bootstrap != nil {
			conditions = append(conditions, metav1.Condition{
				Type:               v1alpha3.VirtualMachineConditionBootstrapReady,
				Status:             metav1.ConditionUnknown,
				LastTransitionTime: preReqCond.LastTransitionTime,
				Reason:             string(metav1.ConditionUnknown),
			})
		}
	}

	return append(dst.Status.Conditions, conditions...)
}

func Convert_v1alpha1_VirtualMachine_To_v1alpha3_VirtualMachine(in *VirtualMachine, out *v1alpha3.VirtualMachine, s apiconversion.Scope) error {
	if err := autoConvert_v1alpha1_VirtualMachine_To_v1alpha3_VirtualMachine(in, out, s); err != nil {
		return err
	}

	// For existing VMs, we want to ensure out.spec.image is only updated if
	// this conversion is not part of a create operation. We can determine that
	// by looking at the object's generation. Any generation value > 0 means the
	// resource has been written to etcd. The only time generation is 0 is the
	// initial application of the resource before it has been written to etcd.
	//
	// For VMs being created, this behavior prevents spec.image from being set,
	// causing the VM's mutation webhook to resolve spec.image from the value of
	// spec.imageName.
	//
	// For existing VMs, out.spec.image can be set to ensure the printer column
	// for spec.image.name is non-empty whenever possible.
	if in.Generation > 0 {
		if in.Spec.ImageName != "" {
			out.Spec.Image = &v1alpha3.VirtualMachineImageRef{
				Name: in.Spec.ImageName,
			}
		}
	}

	return nil
}

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachine)
	if err := Convert_v1alpha1_VirtualMachine_To_v1alpha3_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1alpha3.VirtualMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		// If there is no nextver object to restore, this was originally a v1a1 VM
		// Make sure the v1a1 PreReq conditions are translated over to nextver correctly
		dst.Status.Conditions = convert_v1alpha1_PreReqsReadyCondition_to_v1alpha3_Conditions(dst)
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha3_VirtualMachineImage(dst, restored)
	restore_v1alpha3_VirtualMachineBootstrapSpec(dst, restored)
	restore_v1alpha3_VirtualMachineNetworkSpec(dst, restored)
	restore_v1alpha3_VirtualMachineReadinessProbeSpec(dst, restored)

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachine)
	if err := Convert_v1alpha3_VirtualMachine_To_v1alpha1_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineList to the Hub version.
func (src *VirtualMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineList)
	return Convert_v1alpha1_VirtualMachineList_To_v1alpha3_VirtualMachineList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineList.
func (dst *VirtualMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineList)
	return Convert_v1alpha3_VirtualMachineList_To_v1alpha1_VirtualMachineList(src, dst, nil)
}
