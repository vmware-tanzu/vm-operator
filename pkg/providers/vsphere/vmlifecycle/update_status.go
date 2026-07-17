// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vmdk"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/networkextraconfig"
	vmoprecord "github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	pkgvol "github.com/vmware-tanzu/vm-operator/pkg/util/volumes"
)

type ReconcileStatusData struct {
	// NetworkDeviceKeysToSpecIdx maps the network device's DeviceKey to its
	// corresponding index in the VM's Spec.Network.Interfaces[].
	NetworkDeviceKeysToSpecIdx map[int32]int
}

func ReconcileStatus(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vcVM *object.VirtualMachine,
	data ReconcileStatusData) error {

	vm := vmCtx.VM

	// This is implicitly true: ensure the condition is set since it is how we
	// determine the old v1a1 Phase.
	conditions.MarkTrue(vm, vmopv1.VirtualMachineConditionCreated)

	var errs []error

	errs = append(errs, reconcileStatusAnno2Conditions(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusClass(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusPowerState(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusPlatform(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusHardware(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusHardwareVersion(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusGuest(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusStorage(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusZone(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusNodeName(vmCtx, k8sClient, vcVM, data)...)
	errs = append(errs, reconcileStatusController(vmCtx, k8sClient, vcVM, data)...)

	if pkgcfg.FromContext(vmCtx).Features.TelcoVMServiceAPI {
		errs = append(errs, reconcileStatusExtraConfig(vmCtx, k8sClient, vcVM, data)...)
		errs = append(errs, reconcileStatusNetworkExtraConfig(vmCtx, k8sClient, vcVM, data)...)
	}

	if pkgcfg.FromContext(vmCtx).Features.VMSharedDisks {
		errs = append(errs, reconcileHardwareCondition(vmCtx, k8sClient, vcVM, data)...)
	}

	if pkgcfg.FromContext(vmCtx).AsyncSignalEnabled {
		errs = append(errs, reconcileStatusProbe(vmCtx, k8sClient, vcVM, data)...)
	}

	if pkgcfg.FromContext(vmCtx).Features.VMGroups {
		errs = append(errs, reconcileStatusGroup(vmCtx, k8sClient, vcVM, data)...)
	}

	if pkgcfg.FromContext(vmCtx).Features.VMSnapshots {
		errs = append(errs, reconcileStatusSnapshot(vmCtx, k8sClient, vcVM, data)...)
	}

	MarkReconciliationCondition(vmCtx.VM)

	return apierrorsutil.NewAggregate(errs)
}

var anno2ConditionRegex = regexp.MustCompile(`^condition.vmoperator.vmware.com.protected/(.+)?$`)

// reconcileStatusAnno2Conditions sets conditions on the VM based on
// annotation values.
func reconcileStatusAnno2Conditions(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	for k, v := range vmCtx.VM.Annotations {
		if anno2ConditionRegex.MatchString(k) {
			var (
				t string
				s metav1.ConditionStatus
				r string
				m string
			)
			p := strings.Split(v, ";")
			if len(p) > 0 {
				t = p[0]
			}
			if len(p) > 1 {
				s = metav1.ConditionStatus(p[1])
			}
			if len(p) > 2 {
				r = p[2]
			}
			if len(p) > 3 {
				m = p[3]
			}
			if t != "" {
				switch s {
				case metav1.ConditionFalse:
					conditions.MarkFalse(vmCtx.VM, t, r, m+"%s", "")
				case metav1.ConditionTrue:
					conditions.MarkTrue(vmCtx.VM, t)
				default:
					conditions.MarkUnknown(vmCtx.VM, t, r, m+"%s", "")
				}
			}
		}
	}

	return nil
}

func reconcileStatusClass(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	if vmopv1util.IsClasslessVM(*vmCtx.VM) {
		vmCtx.VM.Status.Class = nil
	} else if vmCtx.VM.Status.Class == nil {
		// When resize is enabled, don't backfill the class from the spec
		// since we don't know if the class has been applied to the VM.
		// When resize is enabled, this field is updated after a successful
		// resize.
		if f := pkgcfg.FromContext(vmCtx).Features; !f.VMResize && !f.VMResizeCPUMemory {
			vmCtx.VM.Status.Class = &common.LocalObjectRef{
				APIVersion: vmopv1.GroupVersion.String(),
				Kind:       "VirtualMachineClass",
				Name:       vmCtx.VM.Spec.ClassName,
			}
		}
	}

	if f := pkgcfg.FromContext(vmCtx).Features; f.VMResize || f.VMResizeCPUMemory {
		MarkVMClassConfigurationSynced(vmCtx, vmCtx.VM, k8sClient)
	}

	return nil
}

func reconcileStatusGroup(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error {

	var errs []error

	if err := vmopv1util.UpdateGroupLinkedCondition(
		vmCtx,
		vmCtx.VM,
		k8sClient,
	); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func reconcileStatusHardware(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	config := vmCtx.MoVM.Config
	if config == nil {
		return nil
	}

	var (
		cpuTotal       = config.Hardware.NumCPU
		cpuReservation int64
	)
	if a := config.CpuAllocation; a != nil {
		if a.Reservation != nil && *a.Reservation > 0 {
			cpuReservation = *a.Reservation
		}
	}
	if cpuTotal > 0 || cpuReservation > 0 {
		if vmCtx.VM.Status.Hardware == nil {
			vmCtx.VM.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{}
		}

		vmCtx.VM.Status.Hardware.CPU = &vmopv1.VirtualMachineCPUAllocationStatus{
			Total:       cpuTotal,
			Reservation: cpuReservation,
		}
	}

	var (
		memTotal       = int64(config.Hardware.MemoryMB)
		memReservation int64
	)
	if a := config.MemoryAllocation; a != nil {
		if a.Reservation != nil && *a.Reservation > 0 {
			memReservation = *a.Reservation
		}
	}
	if memTotal > 0 || memReservation > 0 {
		if vmCtx.VM.Status.Hardware == nil {
			vmCtx.VM.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{}
		}

		vmCtx.VM.Status.Hardware.Memory = &vmopv1.VirtualMachineMemoryAllocationStatus{}

		if r := memTotal; r > 0 {
			b := r * 1000 * 1000
			q := kubeutil.BytesToResource(b)
			vmCtx.VM.Status.Hardware.Memory.Total = q
		}
		if r := memReservation; r > 0 {
			b := r * 1000 * 1000
			q := kubeutil.BytesToResource(b)
			vmCtx.VM.Status.Hardware.Memory.Reservation = q
		}
	}

	if vmCtx.VM.Status.Hardware != nil {
		vmCtx.VM.Status.Hardware.VGPUs = nil
	}

	for _, d := range config.Hardware.Device {
		switch td := d.(type) {

		//
		// PCI Passthrough
		//
		case *vimtypes.VirtualPCIPassthrough:

			//
			// nVidia vGPU
			//
			if b, ok := td.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo); ok {
				migrationType := vmopv1.VirtualMachineVGPUMigrationTypeNone
				if m := b.EnhancedMigrateCapability; m != nil && *m {
					migrationType = vmopv1.VirtualMachineVGPUMigrationTypeEnhanced
				} else if m := b.MigrateSupported; m != nil && *m {
					migrationType = vmopv1.VirtualMachineVGPUMigrationTypeNormal
				}

				if vmCtx.VM.Status.Hardware == nil {
					vmCtx.VM.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{}
				}

				vmCtx.VM.Status.Hardware.VGPUs = append(
					vmCtx.VM.Status.Hardware.VGPUs,
					vmopv1.VirtualMachineHardwareVGPUStatus{
						Type:          vmopv1.VirtualMachineVGPUTypeNVIDIA,
						Profile:       b.Vgpu,
						MigrationType: migrationType,
					})
			}

		//
		// vTPM
		//
		case *vimtypes.VirtualTPM:
			if vmCtx.VM.Status.Crypto == nil {
				vmCtx.VM.Status.Crypto = &vmopv1.VirtualMachineCryptoStatus{}
			}
			vmCtx.VM.Status.Crypto.HasVTPM = true
		}

	}

	return nil
}

func reconcileStatusPowerState(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	vmCtx.VM.Status.PowerState = vmopv1util.ConvertPowerState(vmCtx.Logger,
		vmCtx.MoVM.Runtime.PowerState)

	if vmCtx.VM.Status.PowerState == vmCtx.VM.Spec.PowerState {
		c := conditions.TrueCondition(vmopv1.VirtualMachinePowerStateSynced)
		c.Reason = "Synced"
		c.Message = string(vmCtx.VM.Spec.PowerState)
		conditions.Set(vmCtx.VM, c)
	} else {
		conditions.MarkFalse(
			vmCtx.VM,
			vmopv1.VirtualMachinePowerStateSynced,
			"NotSynced",
			"spec.powerState=%s != status.powerState=%s",
			vmCtx.VM.Spec.PowerState, vmCtx.VM.Status.PowerState)
	}

	return nil
}

// reconcileStatusPlatform updates the VM's status with immutable metadata
// from vCenter, including identifiers (UniqueID, BiosUUID, InstanceUUID)
// and provider-specific information like the creation timestamp.
func reconcileStatusPlatform(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	// TODO(v1alpha6): Deprecate and migrate these fields to the Provider status.
	vmCtx.VM.Status.UniqueID = vmCtx.MoVM.Self.Value
	vmCtx.VM.Status.BiosUUID = vmCtx.MoVM.Summary.Config.Uuid
	vmCtx.VM.Status.InstanceUUID = vmCtx.MoVM.Summary.Config.InstanceUuid

	if vmCtx.VM.Status.Provider == nil {
		vmCtx.VM.Status.Provider = &vmopv1.VirtualMachineProviderStatus{}
	}
	// The creation timestamp is immutable once set.
	if vmCtx.VM.Status.Provider.CreationTimestamp == nil {
		if config := vmCtx.MoVM.Config; config != nil && config.CreateDate != nil {
			vmCtx.VM.Status.Provider.CreationTimestamp = &metav1.Time{Time: *config.CreateDate}
		}
	}

	return nil
}

func reconcileStatusHardwareVersion(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	hardwareVersion, _ := vimtypes.ParseHardwareVersion(
		vmCtx.MoVM.Summary.Config.HwVersion)
	vmCtx.VM.Status.HardwareVersion = int32(hardwareVersion)

	return nil
}

func reconcileStatusZone(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vcVM *object.VirtualMachine,
	_ ReconcileStatusData) []error {

	var errs []error

	zoneLabel := vmCtx.VM.Labels[corev1.LabelTopologyZone]
	zoneStatus := vmCtx.VM.Status.Zone

	// The label value is protected by the VM validation webhook for non-privileged users,
	// but in case the value was accidentally changed by like kube-admin relook the zone.
	if zoneLabel == "" || zoneLabel != zoneStatus {
		clusterMoRef, err := vcenter.GetResourcePoolOwnerMoRef(
			vmCtx, vcVM.Client(), vmCtx.MoVM.ResourcePool.Value)
		if err != nil {
			errs = append(errs, err)
		} else {
			zoneName, err := topology.LookupZoneForClusterMoID(
				vmCtx, k8sClient, clusterMoRef.Value)
			if err != nil {
				errs = append(errs, err)
			} else {
				if vmCtx.VM.Labels == nil {
					vmCtx.VM.Labels = map[string]string{}
				}
				vmCtx.VM.Labels[corev1.LabelTopologyZone] = zoneName
				vmCtx.VM.Status.Zone = zoneName
			}
		}
	}

	return errs
}

func reconcileStatusSnapshot(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error {

	var errs []error
	if err := SyncVMSnapshotTreeStatus(vmCtx, k8sClient); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func reconcileStatusGuest(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	data ReconcileStatusData) []error { //nolint:unparam

	var extraConfig map[string]string
	if config := vmCtx.MoVM.Config; config != nil {
		extraConfig = object.OptionValueList(config.ExtraConfig).StringMap()
	}

	updateGuestNetworkStatus(
		vmCtx.VM,
		vmCtx.MoVM.Guest,
		extraConfig,
		data.NetworkDeviceKeysToSpecIdx)

	if vmCtx.MoVM.Summary.Guest != nil && vmCtx.MoVM.Summary.Guest.HostName != "" {
		if vmCtx.VM.Status.Network == nil {
			vmCtx.VM.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
		}
		vmCtx.VM.Status.Network.HostName = vmCtx.MoVM.Summary.Guest.HostName
	}
	MarkVMToolsRunningStatusCondition(vmCtx.VM, vmCtx.MoVM.Guest)
	MarkCustomizationInfoCondition(vmCtx.VM, vmCtx.MoVM.Guest)
	MarkBootstrapCondition(vmCtx.VM, extraConfig)

	if config := vmCtx.MoVM.Config; config != nil {
		guestID := vmCtx.MoVM.Config.GuestId
		guestName := vmCtx.MoVM.Config.GuestFullName
		if guestID != "" || guestName != "" {
			if vmCtx.VM.Status.Guest == nil {
				vmCtx.VM.Status.Guest = &vmopv1.VirtualMachineGuestStatus{}
			}
			vmCtx.VM.Status.Guest.GuestID = guestID
			vmCtx.VM.Status.Guest.GuestFullName = guestName
		}
	}

	return nil
}

// reconcileStatusStorage updates the status for all storage-related fields.
func reconcileStatusStorage(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	var errs []error

	updateChangeBlockTracking(vmCtx.VM, vmCtx.MoVM)
	updateVolumeStatus(vmCtx)
	errs = append(errs, updateStorageUsage(vmCtx)...)

	return errs
}

func reconcileStatusNodeName(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	vcVM *object.VirtualMachine,
	_ ReconcileStatusData) []error {

	var errs []error

	nodeName, err := getRuntimeHostHostname(
		vmCtx, vcVM, vmCtx.MoVM.Summary.Runtime.Host)
	if err != nil {
		errs = append(errs, err)
	} else {
		vmCtx.VM.Status.NodeName = nodeName
	}

	return errs
}

// updateProbeStatus updates a VM's status with the results of the configured
// readiness probes.
// Please note, this function returns early if the configured probe is TCP.
func reconcileStatusProbe(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	p := vmCtx.VM.Spec.ReadinessProbe
	if p == nil || p.TCPSocket != nil {
		return nil
	}

	var (
		result      probeResult
		resultMsg   string
		extraConfig map[string]string
	)

	if config := vmCtx.MoVM.Config; config != nil {
		extraConfig = object.OptionValueList(config.ExtraConfig).StringMap()
	}

	switch {
	case p.GuestHeartbeat != nil:
		result, resultMsg = updateProbeStatusHeartbeat(vmCtx.VM, vmCtx.MoVM)
	case p.GuestInfo != nil:
		result, resultMsg = updateProbeStatusGuestInfo(vmCtx.VM, extraConfig)
	}

	var cond *metav1.Condition
	switch result {
	case probeResultSuccess:
		cond = conditions.TrueCondition(vmopv1.ReadyConditionType)
	case probeResultFailure:
		cond = conditions.FalseCondition(
			vmopv1.ReadyConditionType, probeReasonNotReady, "%s", resultMsg)
	default:
		cond = conditions.UnknownCondition(
			vmopv1.ReadyConditionType, probeReasonUnknown, "%s", resultMsg)
	}

	// Emit event when the condition is added or its status changes.
	if c := conditions.Get(vmCtx.VM, cond.Type); c == nil || c.Status != cond.Status {
		recorder := vmoprecord.FromContext(vmCtx)
		if cond.Status == metav1.ConditionTrue {
			recorder.Eventf(vmCtx.VM, probeReasonReady, "")
		} else {
			recorder.Eventf(vmCtx.VM, cond.Reason, cond.Message)
		}

		// Log the time when the VM changes its readiness condition.
		pkglog.FromContextOrDefault(vmCtx).Info(
			"VM resource readiness probe condition updated",
			"condition.status", cond.Status,
			"time", cond.LastTransitionTime,
			"reason", cond.Reason)
	}

	conditions.Set(vmCtx.VM, cond)

	return nil
}

func getRuntimeHostHostname(
	ctx context.Context,
	vcVM *object.VirtualMachine,
	host *vimtypes.ManagedObjectReference) (string, error) {

	if host != nil {
		return object.NewHostSystem(vcVM.Client(), *host).ObjectName(ctx)
	}
	return "", nil
}

// extractIPFromAddress extracts the IP address from a string that may contain CIDR notation.
func extractIPFromAddress(address string) string {
	ip, _, err := pkgutil.ParseIP(address)
	if err != nil || ip == nil {
		return ""
	}
	return ip.String()
}

// findInterfaceContainingIP finds the index of the interface that contains the given IP.
func findInterfaceContainingIP(ip string, ifaces []vmopv1.VirtualMachineNetworkInterfaceStatus) int {
	// Normalize the input IP for comparison
	normalizedIP := extractIPFromAddress(ip)
	if normalizedIP == "" {
		return -1
	}
	for i, iface := range ifaces {
		if iface.IP == nil {
			continue
		}
		for _, addr := range iface.IP.Addresses {
			if extractIPFromAddress(addr.Address) == normalizedIP {
				return i
			}
		}
	}
	return -1
}

// primaryIPsInNICData returns true when the per-NIC data from gi.Net[] is
// either absent (nothing to cross-check) or already contains every non-empty
// primary IP. When gi.IpAddress (fast, summary scalar set by vCenter) and
// gi.Net[] (slow, per-NIC data published by VMware Tools) diverge across a
// reconcile loop this returns false, preventing a premature Synced condition.
func primaryIPsInNICData(
	ip4, ip6 string,
	ifaces []vmopv1.VirtualMachineNetworkInterfaceStatus) bool {

	if len(ifaces) == 0 {
		return true
	}
	if ip4 != "" && findInterfaceContainingIP(ip4, ifaces) < 0 {
		return false
	}
	if ip6 != "" && findInterfaceContainingIP(ip6, ifaces) < 0 {
		return false
	}
	return true
}

// extractIPsFromInterface extracts IP addresses of the specified family from an interface.
func extractIPsFromInterface(iface vmopv1.VirtualMachineNetworkInterfaceStatus, isIPv4Required bool, validatePrimaryIP func(string) net.IP) []string {
	var result []string
	if iface.IP == nil {
		return result
	}
	for _, addr := range iface.IP.Addresses {
		ipStr := extractIPFromAddress(addr.Address)
		if ipStr == "" {
			continue
		}
		if a := validatePrimaryIP(ipStr); a != nil {
			if (isIPv4Required && a.To4() != nil) || (!isIPv4Required && a.To4() == nil) {
				result = append(result, ipStr)
			}
		}
	}
	return result
}

func guestNicInfoToInterfaceStatus(
	name string,
	deviceKey int32,
	guestNicInfo *vimtypes.GuestNicInfo) vmopv1.VirtualMachineNetworkInterfaceStatus {

	status := vmopv1.VirtualMachineNetworkInterfaceStatus{
		Name:      name,
		DeviceKey: deviceKey,
	}

	if guestNicInfo.MacAddress != "" {
		status.IP = &vmopv1.VirtualMachineNetworkInterfaceIPStatus{
			MACAddr: guestNicInfo.MacAddress,
		}
	}

	if guestIPConfig := guestNicInfo.IpConfig; guestIPConfig != nil {
		if status.IP == nil {
			status.IP = &vmopv1.VirtualMachineNetworkInterfaceIPStatus{}
		}

		status.IP.AutoConfigurationEnabled = guestIPConfig.AutoConfigurationEnabled
		status.IP.Addresses = convertNetIPConfigInfoIPAddresses(guestIPConfig.IpAddress)

		if guestIPConfig.Dhcp != nil {
			status.IP.DHCP = convertNetDhcpConfigInfo(guestIPConfig.Dhcp)
		}
	}

	if dnsConfig := guestNicInfo.DnsConfig; dnsConfig != nil {
		status.DNS = convertNetDNSConfigInfo(dnsConfig)
	}

	return status
}

func guestIPStackInfoToIPStackStatus(guestIPStack *vimtypes.GuestStackInfo) vmopv1.VirtualMachineNetworkIPStackStatus {
	status := vmopv1.VirtualMachineNetworkIPStackStatus{}

	if dhcpConfig := guestIPStack.DhcpConfig; dhcpConfig != nil {
		status.DHCP = convertNetDhcpConfigInfo(dhcpConfig)
	}

	if dnsConfig := guestIPStack.DnsConfig; dnsConfig != nil {
		status.DNS = convertNetDNSConfigInfo(dnsConfig)
	}

	if ipRouteConfig := guestIPStack.IpRouteConfig; ipRouteConfig != nil {
		status.IPRoutes = convertNetIPRouteConfigInfo(ipRouteConfig)
	}

	status.KernelConfig = convertKeyValueSlice(guestIPStack.IpStackConfig)

	return status
}

func convertNetIPConfigInfoIPAddresses(ipAddresses []vimtypes.NetIpConfigInfoIpAddress) []vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus {
	if len(ipAddresses) == 0 {
		return nil
	}

	out := make([]vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus, 0, len(ipAddresses))
	for _, guestIPAddr := range ipAddresses {
		ipAddrStatus := vmopv1.VirtualMachineNetworkInterfaceIPAddrStatus{
			Address: guestIPAddr.IpAddress,
			Origin:  guestIPAddr.Origin,
			State:   guestIPAddr.State,
		}
		if guestIPAddr.Lifetime != nil {
			ipAddrStatus.Lifetime = metav1.NewTime(*guestIPAddr.Lifetime)
		}

		out = append(out, ipAddrStatus)
	}
	return out
}

func convertNetDNSConfigInfo(dnsConfig *vimtypes.NetDnsConfigInfo) *vmopv1.VirtualMachineNetworkDNSStatus {
	return &vmopv1.VirtualMachineNetworkDNSStatus{
		DHCP:          dnsConfig.Dhcp,
		DomainName:    dnsConfig.DomainName,
		HostName:      dnsConfig.HostName,
		Nameservers:   pkgutil.Dedupe(dnsConfig.IpAddress),
		SearchDomains: pkgutil.Dedupe(dnsConfig.SearchDomain),
	}
}

func convertNetDhcpConfigInfo(dhcpConfig *vimtypes.NetDhcpConfigInfo) *vmopv1.VirtualMachineNetworkDHCPStatus {
	if ipv4, ipv6 := dhcpConfig.Ipv4, dhcpConfig.Ipv6; ipv4 != nil || ipv6 != nil {
		status := &vmopv1.VirtualMachineNetworkDHCPStatus{}

		if ipv4 != nil {
			status.IP4.Enabled = ipv4.Enable
			status.IP4.Config = convertKeyValueSlice(ipv4.Config)
		}
		if ipv6 != nil {
			status.IP6.Enabled = ipv6.Enable
			status.IP6.Config = convertKeyValueSlice(ipv6.Config)
		}

		return status
	}

	return nil
}

func convertNetIPRouteConfigInfo(routeConfig *vimtypes.NetIpRouteConfigInfo) []vmopv1.VirtualMachineNetworkIPRouteStatus {
	if len(routeConfig.IpRoute) == 0 {
		return nil
	}

	// Try to skip routes that are likely not interesting or useful to external users - especially on
	// TKG nodes - that would otherwise just clutter the Status output.
	skipRoute := func(ipRoute vimtypes.NetIpRouteConfigInfoIpRoute) bool {
		network, prefix := ipRoute.Network, ipRoute.PrefixLength

		ip := net.ParseIP(network)
		if ip == nil {
			return true
		}

		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}

		if ip.To4() != nil {
			return prefix == 32
		}

		return ip.To16() == nil || ip.IsInterfaceLocalMulticast() || ip.IsMulticast()
	}

	out := make([]vmopv1.VirtualMachineNetworkIPRouteStatus, 0, 1)
	for _, ipRoute := range routeConfig.IpRoute {
		if skipRoute(ipRoute) {
			continue
		}

		out = append(out, vmopv1.VirtualMachineNetworkIPRouteStatus{
			Gateway: vmopv1.VirtualMachineNetworkIPRouteGatewayStatus{
				Device:  ipRoute.Gateway.Device,
				Address: ipRoute.Gateway.IpAddress,
			},
			NetworkAddress: fmt.Sprintf("%s/%d", ipRoute.Network, ipRoute.PrefixLength),
		})
	}
	return out
}

func convertKeyValueSlice(s []vimtypes.KeyValue) []common.KeyValuePair {
	if len(s) == 0 {
		return nil
	}

	out := make([]common.KeyValuePair, 0, len(s))
	for i := range s {
		out = append(out, common.KeyValuePair{Key: s[i].Key, Value: s[i].Value})
	}
	return out
}

func MarkVMToolsRunningStatusCondition(
	vm *vmopv1.VirtualMachine,
	guestInfo *vimtypes.GuestInfo) {

	if guestInfo == nil || guestInfo.ToolsRunningStatus == "" {
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "NoGuestInfo", "")
		return
	}

	switch guestInfo.ToolsRunningStatus {
	case string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning):
		conditions.MarkFalse(vm, vmopv1.VirtualMachineToolsCondition, vmopv1.VirtualMachineToolsNotRunningReason, "VMware Tools is not running")
	case string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsRunning), string(vimtypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts):
		conditions.MarkTrue(vm, vmopv1.VirtualMachineToolsCondition)
	default:
		conditions.MarkUnknown(vm, vmopv1.VirtualMachineToolsCondition, "Unknown", "Unexpected VMware Tools running status")
	}
}

func MarkCustomizationInfoCondition(vm *vmopv1.VirtualMachine, guestInfo *vimtypes.GuestInfo) {
	if guestInfo == nil || guestInfo.CustomizationInfo == nil {
		conditions.MarkUnknown(vm, vmopv1.GuestCustomizationCondition, "NoGuestInfo", "")
		return
	}

	switch guestInfo.CustomizationInfo.CustomizationStatus {
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE), "":
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationPendingReason, "")
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING):
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationRunningReason, "")
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED):
		conditions.MarkTrue(vm, vmopv1.GuestCustomizationCondition)
	case string(vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED):
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "vSphere VM Customization failed due to an unknown error."
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, vmopv1.GuestCustomizationFailedReason, "%s", errorMsg)
	default:
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "Unexpected VM Customization status"
		}
		conditions.MarkFalse(vm, vmopv1.GuestCustomizationCondition, "Unknown", "%s", errorMsg)
	}
}
func MarkReconciliationCondition(vm *vmopv1.VirtualMachine) {
	switch vm.Labels[vmopv1.PausedVMLabelKey] {
	case "devops":
		conditions.MarkFalse(vm, vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason,
			"Virtual Machine reconciliation paused by DevOps")
	case "admin":
		conditions.MarkFalse(vm, vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason,
			"Virtual Machine reconciliation paused by Admin")
	case "both":
		conditions.MarkFalse(vm, vmopv1.VirtualMachineReconcileReady, vmopv1.VirtualMachineReconcilePausedReason,
			"Virtual Machine reconciliation paused by Admin, DevOps")
	default:
		conditions.MarkTrue(vm, vmopv1.VirtualMachineReconcileReady)
	}
}

func MarkBootstrapCondition(
	vm *vmopv1.VirtualMachine,
	extraConfig map[string]string) {

	status, reason, msg, ok := pkgutil.GetBootstrapConditionValues(extraConfig)
	if !ok {
		conditions.Delete(vm, vmopv1.GuestBootstrapCondition)
		return
	}

	if status {
		c := conditions.TrueCondition(vmopv1.GuestBootstrapCondition)
		if reason != "" {
			c.Reason = reason
		}
		c.Message = msg
		conditions.Set(vm, c)
	} else {
		conditions.MarkFalse(vm, vmopv1.GuestBootstrapCondition, reason, "%s", msg)
	}
}

func MarkVMClassConfigurationSynced(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	k8sClient ctrlclient.Client) {

	className := vm.Spec.ClassName
	if className == "" {
		conditions.Delete(vm, vmopv1.VirtualMachineClassConfigurationSynced)
		return
	}

	// NOTE: This performs the same checks as vmopv1util.ResizeNeeded() but we can't use
	// just the return value of that function because we need more details, and we want
	// to avoid having to fetch the class if we can.

	lraName, lraUID, lraGeneration, exists := vmopv1util.GetLastResizedAnnotation(*vm)
	if !exists || lraName == "" {
		_, sameClassResize := vm.Annotations[vmopv1.VirtualMachineSameVMClassResizeAnnotation]
		if sameClassResize {
			conditions.MarkFalse(vm, vmopv1.VirtualMachineClassConfigurationSynced, "SameClassResize", "")
		} else {
			// Brownfield VM so just marked as synced.
			conditions.MarkTrue(vm, vmopv1.VirtualMachineClassConfigurationSynced)
		}
		return
	}

	if vm.Spec.ClassName != lraName {
		// Most common need resize case.
		conditions.MarkFalse(vm, vmopv1.VirtualMachineClassConfigurationSynced, "ClassNameChanged", "")
		return
	}

	// Depending on what we did in the prior session update code for this VM, we might already
	// have fetched the class but the way the code is structured today, it isn't very easy for
	// us to pass that to here. So just refetch it again.
	//
	// Note that for this situation we'll only do a resize if the SameVMClassResizeAnnotation
	// is present but use this condition to inform if the class has changed and a user could
	// opt-in to a resize.

	vmClass := vmopv1.VirtualMachineClass{}
	if err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: className, Namespace: vm.Namespace}, &vmClass); err != nil {
		if apierrors.IsNotFound(err) {
			conditions.MarkUnknown(vm, vmopv1.VirtualMachineClassConfigurationSynced, "ClassNotFound", "")
		} else {
			conditions.MarkUnknown(vm, vmopv1.VirtualMachineClassConfigurationSynced, err.Error(), "")
		}
		return
	}

	if string(vmClass.UID) == lraUID && vmClass.Generation == lraGeneration {
		conditions.MarkTrue(vm, vmopv1.VirtualMachineClassConfigurationSynced)
	} else {
		conditions.MarkFalse(vm, vmopv1.VirtualMachineClassConfigurationSynced, "ClassUpdated", "")
	}
}

var (
	emptyNetConfig   vmopv1.VirtualMachineNetworkConfigStatus
	emptyIfaceConfig vmopv1.VirtualMachineNetworkConfigInterfaceStatus
)

// UpdateNetworkStatusConfig updates the provided VM's status.network.config
// field with information from the provided bootstrap arguments. This is useful
// for folks booting VMs without bootstrap engines who may wish to manually
// configure the VM's networking with the valid IP configuration for this VM.
//
//nolint:gocyclo
func UpdateNetworkStatusConfig(vm *vmopv1.VirtualMachine, args BootstrapArgs) {

	if vm == nil {
		panic("vm is nil")
	}

	// Define the network configuration to update. However, it is not assigned
	// to the VM's status.network.config field unless nc is non-empty before
	// this function ends.
	var nc vmopv1.VirtualMachineNetworkConfigStatus

	// Update the global DNS information.
	{
		hn, dn, ns, sd := args.HostName, args.DomainName, args.DNSServers, args.SearchSuffixes
		lhn, ldn, lns, lsd := len(hn) > 0, len(dn) > 0, len(ns) > 0, len(sd) > 0
		if lhn || ldn || lns || lsd {
			nc.DNS = &vmopv1.VirtualMachineNetworkConfigDNSStatus{}
			if lhn {
				nc.DNS.HostName = hn
			}
			if ldn {
				nc.DNS.DomainName = dn
			}
			if lns {
				nc.DNS.Nameservers = ns
			}
			if lsd {
				nc.DNS.SearchDomains = sd
			}
		}
	}

	// Iterate over each network result.
	for i := range args.NetworkResults.Results {

		// Declare the interface's config status.
		var ifc vmopv1.VirtualMachineNetworkConfigInterfaceStatus

		// Define a short alias for the indexed result.
		r := args.NetworkResults.Results[i]

		// Grab a temp copy of the result's IP configuration list to make it
		// more obvious that the purpose of the next three lines of code is not
		// to update the r.IPConfigs directly.
		ipConfigs := args.NetworkResults.Results[i].IPConfigs

		// The intended DHCP configuration is presented per interface, so if
		// there are no resulting IP configs, but DHCP4 or DHCP6 is configured,
		// go ahead and create a single, fake IP config so the DHCP info can be
		// collected the same way below.
		if len(ipConfigs) == 0 && (r.DHCP4 || r.DHCP6) {
			ipConfigs = []network.NetworkInterfaceIPConfig{{}}
		}

		// If there *are* resulting IP configs, then ensure the field ifc.IP
		// is not nil so avoid an NPE later. We do not initialize this field
		// unless there *are* resulting IP configs to avoid an empty object
		// when printing the VM's status.
		if len(ipConfigs) > 0 {
			ifc.IP = &vmopv1.VirtualMachineNetworkConfigInterfaceIPStatus{}
		}

		// Iterate over each of the result's IP configurations.
		for j := range ipConfigs {
			ipc := ipConfigs[j]

			// Assign the gateways.
			if gw := ipc.Gateway; gw != "" {
				if ipc.IsIPv4 && ifc.IP.Gateway4 == "" {
					ifc.IP.Gateway4 = gw
				} else if !ipc.IsIPv4 && ifc.IP.Gateway6 == "" {
					ifc.IP.Gateway6 = gw
				}
			}

			// Append the IP address.
			if ip := ipc.IPCIDR; ip != "" {
				ifc.IP.Addresses = append(ifc.IP.Addresses, ip)
			}

			// Update DHCP information.
			if v4, v6 := r.DHCP4, r.DHCP6; v4 || v6 {
				ifc.IP.DHCP = &vmopv1.VirtualMachineNetworkConfigDHCPStatus{}
				if v4 {
					ifc.IP.DHCP.IP4 = &vmopv1.VirtualMachineNetworkConfigDHCPOptionsStatus{
						Enabled: v4,
					}
				}
				if v6 {
					ifc.IP.DHCP.IP6 = &vmopv1.VirtualMachineNetworkConfigDHCPOptionsStatus{
						Enabled: v6,
					}
				}
			}

			// Update DNS information.
			{
				ns, sd := r.Nameservers, r.SearchDomains
				if ln, ls := len(ns), len(sd); ln > 0 || ls > 0 {
					ifc.DNS = &vmopv1.VirtualMachineNetworkConfigDNSStatus{}
					if ln > 0 {
						ifc.DNS.Nameservers = ns
					}
					if ls > 0 {
						ifc.DNS.SearchDomains = sd
					}
				}
			}
		}

		if ip := ifc.IP; ip != nil && len(ip.Addresses) > 0 {
			slices.Sort(ifc.IP.Addresses)
		}

		// Only append the interface config if it is not empty.
		if !reflect.DeepEqual(ifc, emptyIfaceConfig) {
			// Do not assign the name until the very end so as to not disrupt
			// the comparison with the empty struct.
			ifc.Name = r.Name

			nc.Interfaces = append(nc.Interfaces, ifc)
		}
	}

	// If the network config ended up empty, then ensure the VM's field
	// status.network.config is nil IFF status.network is non-nil.
	// Otherwise, assign the network config to the VM's status.network.config
	// field.
	if reflect.DeepEqual(nc, emptyNetConfig) {
		if vm.Status.Network != nil {
			vm.Status.Network.Config = nil
		}
	} else {
		if vm.Status.Network == nil {
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
		}
		vm.Status.Network.Config = &nc
	}
}

// updateGuestNetworkStatus updates the provided VM's status.network
// field with information from the guestInfo.
//
//nolint:gocyclo
func updateGuestNetworkStatus(
	vm *vmopv1.VirtualMachine,
	gi *vimtypes.GuestInfo,
	extraConfig map[string]string,
	deviceKeyToSpecIdx map[int32]int) {

	var (
		primaryIP4      string
		primaryIP6      string
		ifaceStatuses   []vmopv1.VirtualMachineNetworkInterfaceStatus
		ipStackStatuses []vmopv1.VirtualMachineNetworkIPStackStatus
	)

	validatePrimaryIP := func(ip string) net.IP {
		if a := net.ParseIP(ip); len(a) > 0 {
			// Ignore local IP addresses, i.e. addresses that are only valid on
			// the guest OS. Please note this does not include private, or RFC
			// 1918 (IPv4) and RFC 4193 (IPv6) addresses, ex. 192.168.0.2.
			if !a.IsUnspecified() &&
				!a.IsLinkLocalMulticast() &&
				!a.IsLinkLocalUnicast() &&
				!a.IsLoopback() {
				return a
			}
		}
		return nil
	}

	if gi != nil {
		if ip := gi.IpAddress; ip != "" {
			if a := validatePrimaryIP(ip); a != nil {
				if a.To4() != nil {
					primaryIP4 = ip
				} else {
					primaryIP6 = ip
				}
			}
		}

		if len(gi.Net) > 0 {
			var ifaceSpecs []vmopv1.VirtualMachineNetworkInterfaceSpec
			if vm.Spec.Network != nil {
				ifaceSpecs = vm.Spec.Network.Interfaces
			}

			for i := range gi.Net {
				deviceKey := gi.Net[i].DeviceConfigId

				// Skip pseudo devices.
				if deviceKey < 0 {
					continue
				}

				var ifaceName string
				if idx, ok := deviceKeyToSpecIdx[deviceKey]; ok && idx < len(ifaceSpecs) {
					ifaceName = ifaceSpecs[idx].Name
				}

				ifaceStatuses = append(
					ifaceStatuses,
					guestNicInfoToInterfaceStatus(
						ifaceName,
						deviceKey,
						&gi.Net[i]))
			}
		}

		if lip := len(gi.IpStack); lip > 0 {
			ipStackStatuses = make([]vmopv1.VirtualMachineNetworkIPStackStatus, lip)
			for i := range gi.IpStack {
				ipStackStatuses[i] = guestIPStackInfoToIPStackStatus(&gi.IpStack[i])
			}
		}
	}

	if bs := vm.Spec.Bootstrap; bs != nil && bs.CloudInit != nil {
		// When CloudInit is used, prefer the local IPs that the VMware datasource publishes.
		if ip4 := extraConfig[constants.CloudInitGuestInfoLocalIPv4Key]; ip4 != "" {
			if a := validatePrimaryIP(ip4); a != nil && a.To4() != nil {
				primaryIP4 = ip4
			}
		}

		if ip6 := extraConfig[constants.CloudInitGuestInfoLocalIPv6Key]; ip6 != "" {
			if a := validatePrimaryIP(ip6); a != nil && a.To4() == nil {
				primaryIP6 = ip6
			}
		}
	}

	// See cloud-init issue: https://github.com/canonical/cloud-init/issues/6851
	// LinuxPrep does not report dual stack IPs. It reports only one primary IP.
	// Fallback when guest-reported primaries are incomplete (common in some dual-stack / cloud-init cases).
	// Same-interface cross-family fill only applies if that interface reports exactly one usable address
	// of the missing family—otherwise we do not guess. The all-empty sole-NIC path uses the same rule
	// per family. Usable addresses exclude loopback, unspecified, and link-local (validatePrimaryIP).
	if len(ifaceStatuses) > 0 {
		switch {
		case primaryIP4 != "" && primaryIP6 == "":
			// Try to find IPv6 on the same interface as IPv4
			if idx := findInterfaceContainingIP(primaryIP4, ifaceStatuses); idx >= 0 {
				ipv6Addrs := extractIPsFromInterface(ifaceStatuses[idx], false, validatePrimaryIP)
				if len(ipv6Addrs) == 1 {
					primaryIP6 = ipv6Addrs[0]
				}
			}
		case primaryIP6 != "" && primaryIP4 == "":
			// Try to find IPv4 on the same interface as IPv6
			if idx := findInterfaceContainingIP(primaryIP6, ifaceStatuses); idx >= 0 {
				ipv4Addrs := extractIPsFromInterface(ifaceStatuses[idx], true, validatePrimaryIP)
				if len(ipv4Addrs) == 1 {
					primaryIP4 = ipv4Addrs[0]
				}
			}
		case primaryIP4 == "" && primaryIP6 == "":
			// Both empty, try first interface, if its the only interface present.
			if len(ifaceStatuses) == 1 {
				ipv4Addrs := extractIPsFromInterface(ifaceStatuses[0], true, validatePrimaryIP)
				ipv6Addrs := extractIPsFromInterface(ifaceStatuses[0], false, validatePrimaryIP)
				if len(ipv4Addrs) == 1 {
					primaryIP4 = ipv4Addrs[0]
				}
				if len(ipv6Addrs) == 1 {
					primaryIP6 = ipv6Addrs[0]
				}
			}
		}
	}

	var (
		lip4 = len(primaryIP4) > 0
		lip6 = len(primaryIP6) > 0
		lip  = len(ipStackStatuses) > 0
		lis  = len(ifaceStatuses) > 0
	)

	if lip4 || lip6 || lip || lis {
		if vm.Status.Network == nil {
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{}
		}
		vm.Status.Network.PrimaryIP4 = primaryIP4
		vm.Status.Network.PrimaryIP6 = primaryIP6
		vm.Status.Network.IPStacks = ipStackStatuses
		vm.Status.Network.Interfaces = ifaceStatuses

		// TODO(akutz) Handle additional situations:
		//             - The VM has IPv4 but not IPv6 and vice versa
		//             - The network may be disabled, not have interfaces, etc.
		if primaryIP4 != "" || primaryIP6 != "" {
			if primaryIPsInNICData(primaryIP4, primaryIP6, ifaceStatuses) {
				c := conditions.TrueCondition(
					vmopv1.VirtualMachineGuestNetworkConfigSynced)
				c.Reason = "Synced"
				c.Message = fmt.Sprintf(
					"IPv4=%q, IPv6=%q", primaryIP4, primaryIP6)
				conditions.Set(vm, c)
			} else {
				conditions.MarkFalse(vm,
					vmopv1.VirtualMachineGuestNetworkConfigSynced,
					"NotSynced",
					"guest primary IPs (IPv4=%q, IPv6=%q)"+
						" not yet reflected in per-NIC data",
					primaryIP4, primaryIP6)
			}
		} else {
			conditions.MarkFalse(
				vm,
				vmopv1.VirtualMachineGuestNetworkConfigSynced,
				"NotSynced",
				"%s",
				"Neither IPv4 nor IPv6 address reported by guest")
		}
	} else if vm.Status.Network != nil {
		if cfg := vm.Status.Network.Config; cfg != nil {
			// Config is the only field we need to preserve.
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{
				Config: cfg,
			}
		} else {
			vm.Status.Network = nil
		}
	}

}

func updateChangeBlockTracking(vm *vmopv1.VirtualMachine, moVM mo.VirtualMachine) {
	if moVM.Config != nil {
		vm.Status.ChangeBlockTracking = moVM.Config.ChangeTrackingEnabled
	} else {
		vm.Status.ChangeBlockTracking = nil
	}
}

func updateStorageUsage(vmCtx pkgctx.VirtualMachineContext) []error {

	var (
		other           int64
		disksUsed       int64
		disksReqd       int64
		vmSnapshotUsed  int64
		volSnapshotUsed int64

		moVM        = vmCtx.MoVM
		vm          = vmCtx.VM
		snapEnabled = pkgcfg.FromContext(vmCtx).Features.VMSnapshots

		errs []error
	)
	// Get the storage consumed by non-disks.
	if moVM.LayoutEx != nil {
		for i := range moVM.LayoutEx.File {
			f := moVM.LayoutEx.File[i]
			switch vimtypes.VirtualMachineFileLayoutExFileType(f.Type) {
			case vimtypes.VirtualMachineFileLayoutExFileTypeDiskDescriptor,
				vimtypes.VirtualMachineFileLayoutExFileTypeDiskExtent,
				vimtypes.VirtualMachineFileLayoutExFileTypeDigestDescriptor,
				vimtypes.VirtualMachineFileLayoutExFileTypeDigestExtent:

				// Skip disks
			case vimtypes.VirtualMachineFileLayoutExFileTypeSnapshotList,
				vimtypes.VirtualMachineFileLayoutExFileTypeSnapshotMemory,
				vimtypes.VirtualMachineFileLayoutExFileTypeSnapshotData:

				// Storage consumed by snapshots is tracked separately from
				// the VM's files if VM Service vmsnapshot feature is enabled.
				// So, only include snapshot files in the VM's total usage
				// if vmsnapshot feature is enabled.
				if !snapEnabled {
					other += f.UniqueSize
				}
			default:
				other += f.UniqueSize
			}
		}
	}

	// Get the storage consumed by disks.
	for i := range vm.Status.Volumes {
		v := vm.Status.Volumes[i]
		if v.Type == vmopv1.VolumeTypeClassic {
			if v.Used != nil {
				i, _ := v.Used.AsInt64()
				disksUsed += i
			}
			if v.Requested != nil {
				i, _ := v.Requested.AsInt64()
				disksReqd += i
			}
		}
	}

	// Get the storage consumed by snapshots.
	if snapEnabled {
		var err error
		vmSnapshotUsed, volSnapshotUsed, err = virtualmachine.GetAllSnapshotSize(vmCtx, moVM)
		if err != nil {
			errs = append(errs,
				fmt.Errorf("failed to compute snapshot size of VM: %w", err),
			)
		}
	}

	if disksReqd == 0 &&
		disksUsed == 0 &&
		other == 0 &&
		vmSnapshotUsed == 0 &&
		volSnapshotUsed == 0 {
		return errs
	}

	if vm.Status.Storage == nil {
		vm.Status.Storage = &vmopv1.VirtualMachineStorageStatus{}
	}

	if disksReqd > 0 {
		if vm.Status.Storage.Requested == nil {
			vm.Status.Storage.Requested = &vmopv1.VirtualMachineStorageStatusRequested{}
		}
		vm.Status.Storage.Requested.Disks = nil
	}
	if disksUsed > 0 || other > 0 {
		if vm.Status.Storage.Used == nil {
			vm.Status.Storage.Used = &vmopv1.VirtualMachineStorageStatusUsed{}
		}
		vm.Status.Storage.Used.Disks = nil
		vm.Status.Storage.Used.Other = nil
	}

	if disksReqd > 0 {
		vm.Status.Storage.Requested.Disks = kubeutil.BytesToResource(disksReqd)
	}
	if disksUsed > 0 {
		vm.Status.Storage.Used.Disks = kubeutil.BytesToResource(disksUsed)
	}

	if other > 0 {
		vm.Status.Storage.Used.Other = kubeutil.BytesToResource(other)
	}

	if vmSnapshotUsed > 0 || volSnapshotUsed > 0 {
		if vm.Status.Storage.Used.Snapshots == nil {
			vm.Status.Storage.Used.Snapshots =
				&vmopv1.VirtualMachineStorageStatusUsedSnapshotDetails{}
		}
		if vmSnapshotUsed > 0 {
			vm.Status.Storage.Used.Snapshots.VM = kubeutil.BytesToResource(vmSnapshotUsed)
		}
		if volSnapshotUsed > 0 {
			vm.Status.Storage.Used.Snapshots.Volume = kubeutil.BytesToResource(volSnapshotUsed)
		}
	}

	vm.Status.Storage.Total = kubeutil.BytesToResource(disksReqd + other)

	return errs
}

func updateVolumeStatus(vmCtx pkgctx.VirtualMachineContext) {
	var (
		moVM        = vmCtx.MoVM
		vm          = vmCtx.VM
		snapEnabled = pkgcfg.FromContext(vmCtx).Features.VMSnapshots
	)
	if moVM.Config == nil ||
		moVM.LayoutEx == nil ||
		len(moVM.LayoutEx.Disk) == 0 ||
		len(moVM.LayoutEx.File) == 0 ||
		len(moVM.Config.Hardware.Device) == 0 {

		return
	}

	info, ok := pkgvol.FromContext(vmCtx)
	if !ok {
		info = pkgvol.GetVolumeInfoFromVM(vm, moVM)
	}

	// Filter out disks with invalid paths.
	info.Disks = slices.DeleteFunc(info.Disks, func(
		e pkgvol.VirtualDiskInfo) bool {
		var diskPath object.DatastorePath
		return !diskPath.FromString(e.FileName)
	})

	// Remove stale entries from the status.
	existingDisksInStatus := map[string]int{}
	for i := range vm.Status.Volumes {
		if vol := vm.Status.Volumes[i]; vol.DiskUUID != "" {
			existingDisksInStatus[vol.DiskUUID] = i
		}
	}
	// Collect indices to delete first.
	indicesToDelete := []int{}
	for _, di := range info.Disks {
		if volSpec, ok := info.Volumes[di.Target.String()]; ok {
			if diskIndex, ok := existingDisksInStatus[di.UUID]; ok {
				if volSpec.Name != vm.Status.Volumes[diskIndex].Name {
					indicesToDelete = append(indicesToDelete, diskIndex)
					delete(existingDisksInStatus, di.UUID)
				}
			}
		}
	}
	// Delete in reverse order so that we don't shift indices before deleting
	// next one.
	slices.Sort(indicesToDelete)
	for i := len(indicesToDelete) - 1; i >= 0; i-- {
		vm.Status.Volumes = slices.Delete(
			vm.Status.Volumes,
			indicesToDelete[i],
			indicesToDelete[i]+1,
		)
	}
	// Update existingDisksInStatus with new indexes.
	for i := range vm.Status.Volumes {
		if vol := vm.Status.Volumes[i]; vol.DiskUUID != "" {
			existingDisksInStatus[vol.DiskUUID] = i
		}
	}

	// Update the status.
	existingDisksInConfig := map[string]struct{}{}
	for _, di := range info.Disks {
		existingDisksInConfig[di.UUID] = struct{}{}

		if diskIndex, ok := existingDisksInStatus[di.UUID]; ok {
			// The disk is already in the list of volume statuses, so update the
			// existing status with the usage information.
			ddi, _ := vmdk.GetVirtualDiskInfoByUUID(
				vmCtx,
				nil,         /* no client since props aren't re-fetched */
				moVM,        /* use props from this object */
				false,       /* do not refetch props */
				snapEnabled, /* exclude disks related to snapshots */
				di.UUID)
			vm.Status.Volumes[diskIndex].Used = kubeutil.BytesToResource(ddi.UniqueSize)
			if ddi.CryptoKey.ProviderID != "" || ddi.CryptoKey.KeyID != "" {
				vm.Status.Volumes[diskIndex].Crypto = &vmopv1.VirtualMachineVolumeCryptoStatus{
					ProviderID: ddi.CryptoKey.ProviderID,
					KeyID:      ddi.CryptoKey.KeyID,
				}
			}
			// This is for a rare case when VM is upgraded from v1alpha3 to
			// v1alpha4+. Since vm.status.volume.requested was introduced in
			// v1alpha4. So we need to patch it if it's missing from status for
			// Classic disk. Managed disk is taken care of in volume controller.
			if !di.FCD && vm.Status.Volumes[diskIndex].Requested == nil {
				vm.Status.Volumes[diskIndex].Requested = kubeutil.BytesToResource(di.CapacityInBytes)
			}

			if pkgcfg.FromContext(vmCtx).Features.AllDisksArePVCs ||
				pkgcfg.FromContext(vmCtx).Features.VMSharedDisks {

				// Update target ID info.
				vm.Status.Volumes[diskIndex].UnitNumber = di.UnitNumber
				if c, ok := info.Controllers[di.ControllerKey]; ok {
					vm.Status.Volumes[diskIndex].ControllerBusNumber = &c.Bus
					vm.Status.Volumes[diskIndex].ControllerType = c.Type
				}
				if diskMode, err := pkgutil.GetVolumeDiskModeFromDiskMode(di.DiskMode); err == nil {
					vm.Status.Volumes[diskIndex].DiskMode = diskMode
				}
				if sharingMode, err := pkgutil.GetVolumeSharingModeFromDiskSharing(di.Sharing); err == nil {
					vm.Status.Volumes[diskIndex].SharingMode = sharingMode
				}
			}

			// Classic disk should be converted to PVC in the end.
			if pkgcfg.FromContext(vmCtx).Features.AllDisksArePVCs {
				if di.FCD && vm.Status.Volumes[diskIndex].Type == vmopv1.VolumeTypeClassic {
					vm.Status.Volumes[diskIndex].Type = vmopv1.VolumeTypeManaged
				}
			}

		} else if !di.FCD {

			var volName string
			if volSpec, ok := info.Volumes[di.Target.String()]; ok {
				volName = volSpec.Name
			} else {
				volName = pkgutil.GeneratePVCName("disk", di.UUID)
			}

			// The disk is a classic, non-FCD that must be added to the list
			// of volume statuses.
			ddi, _ := vmdk.GetVirtualDiskInfoByUUID(
				vmCtx,
				nil,         /* no client since props aren't re-fetched */
				moVM,        /* use props from this object */
				false,       /* do not refetch props */
				snapEnabled, /* exclude disks related to snapshots */
				di.UUID)

			volStatus := vmopv1.VirtualMachineVolumeStatus{
				Name:      volName,
				Type:      vmopv1.VolumeTypeClassic,
				Attached:  true,
				DiskUUID:  di.UUID,
				Limit:     kubeutil.BytesToResource(di.CapacityInBytes),
				Requested: kubeutil.BytesToResource(di.CapacityInBytes),
				Used:      kubeutil.BytesToResource(ddi.UniqueSize),
			}

			if pkgcfg.FromContext(vmCtx).Features.AllDisksArePVCs ||
				pkgcfg.FromContext(vmCtx).Features.VMSharedDisks {

				volStatus.UnitNumber = di.UnitNumber
				if c, ok := info.Controllers[di.ControllerKey]; ok {
					volStatus.ControllerBusNumber = &c.Bus
					volStatus.ControllerType = c.Type
				}
				if diskMode, err := pkgutil.GetVolumeDiskModeFromDiskMode(di.DiskMode); err == nil {
					volStatus.DiskMode = diskMode
				}
				if sharingMode, err := pkgutil.GetVolumeSharingModeFromDiskSharing(di.Sharing); err == nil {
					volStatus.SharingMode = sharingMode
				}
			}

			if ddi.CryptoKey.ProviderID != "" || ddi.CryptoKey.KeyID != "" {
				volStatus.Crypto = &vmopv1.VirtualMachineVolumeCryptoStatus{
					ProviderID: ddi.CryptoKey.ProviderID,
					KeyID:      ddi.CryptoKey.KeyID,
				}
			}
			vm.Status.Volumes = append(vm.Status.Volumes, volStatus)
		}
	}

	// Remove any status entries for classic disks that no longer exist in
	// config.hardware.device.
	vm.Status.Volumes = slices.DeleteFunc(vm.Status.Volumes,
		func(e vmopv1.VirtualMachineVolumeStatus) bool {
			switch e.Type {
			case vmopv1.VolumeTypeClassic:
				_, keep := existingDisksInConfig[e.DiskUUID]
				return !keep
			default:
				return false
			}
		})

	// This sort order is consistent with the logic from the volumes controller.
	vmopv1.SortVirtualMachineVolumeStatuses(vm.Status.Volumes)
}

type probeResult uint8

const (
	probeResultUnknown probeResult = iota
	probeResultSuccess
	probeResultFailure
)

const (
	probeReasonReady    string = "Ready"
	probeReasonNotReady string = "NotReady"
	probeReasonUnknown  string = "Unknown"
)

func heartbeatValue(value string) int {
	switch value {
	case string(vmopv1.GreenHeartbeatStatus):
		return 3
	case string(vmopv1.YellowHeartbeatStatus):
		return 2
	case string(vmopv1.RedHeartbeatStatus):
		return 1
	case string(vmopv1.GrayHeartbeatStatus):
		return 0
	default:
		return -1
	}
}

func updateProbeStatusHeartbeat(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) (probeResult, string) {

	chb := string(moVM.GuestHeartbeatStatus)
	mhb := string(vm.Spec.ReadinessProbe.GuestHeartbeat.ThresholdStatus)
	chv := heartbeatValue(chb)
	if chv < 0 {
		return probeResultUnknown, ""
	}
	if mhv := heartbeatValue(mhb); chv < mhv {
		return probeResultFailure, fmt.Sprintf(
			"heartbeat status %q is below threshold", chb)
	}
	return probeResultSuccess, ""
}

func updateProbeStatusGuestInfo(
	vm *vmopv1.VirtualMachine,
	extraConfig map[string]string) (probeResult, string) { //nolint:unparam

	if len(vm.Spec.ReadinessProbe.GuestInfo) == 0 {
		return probeResultUnknown, ""
	}

	for _, gi := range vm.Spec.ReadinessProbe.GuestInfo {
		key := fmt.Sprintf("guestinfo.%s", gi.Key)

		actualVal, ok := extraConfig[key]
		if !ok {
			return probeResultFailure, ""
		}

		if gi.Value == "" {
			// Matches everything.
			continue
		}

		expectedValRx, err := regexp.Compile(gi.Value)
		if err != nil {
			// Treat an invalid expressions as a wildcard too.
			continue
		}

		if !expectedValRx.MatchString(actualVal) {
			return probeResultFailure, ""
		}
	}

	return probeResultSuccess, ""
}

// SyncVMSnapshotTreeStatus syncs whole snapshot tree by
// recursively updating snapshots' childrenList status
// and updates the VM's current and root snapshots status.
func SyncVMSnapshotTreeStatus(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client) error {
	vmCtx.Logger.V(4).Info("Syncing snapshot tree status")

	if err := updateSnapshotTreeChildrenStatus(vmCtx, k8sClient); err != nil {
		return err
	}

	if err := updateCurrentSnapshotStatus(vmCtx, k8sClient); err != nil {
		return err
	}

	return updateRootSnapshots(vmCtx, k8sClient)
}

// updateSnapshotTreeChildrenStatus updates Status.Children for
// every snapshot in the VM's snapshot tree.
func updateSnapshotTreeChildrenStatus(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client) error {
	mo := vmCtx.MoVM
	if mo.Snapshot == nil || len(mo.Snapshot.RootSnapshotList) == 0 {
		return nil
	}

	for _, root := range mo.Snapshot.RootSnapshotList {
		if _, err := updateSnapshotChildrenStatus(vmCtx, k8sClient, root); err != nil {
			return err
		}
	}
	return nil
}

// updateSnapshotChildrenStatus returns the LocalObjectRef
// for the current snapshot (if CR exists),
// and ensures its Status.Children is up-to-date.
func updateSnapshotChildrenStatus(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	node vimtypes.VirtualMachineSnapshotTree,
) (*vmopv1.VirtualMachineSnapshotReference, error) {
	curSnapshot, err := getSnapshotCR(vmCtx, k8sClient, node.Name)
	if err != nil {
		return nil, err
	}

	if curSnapshot == nil {
		// If the current snapshot custom resource is not found,
		// it means it's an Unmanaged snapshot.
		// Nothing to update here.
		vmCtx.Logger.V(4).Info("VirtualMachineSnapshot not found, skipping",
			"snapshotName", node.Name)
		return nil, nil
	}

	// Recurse into children and collect their refs (only if their CRs exist).
	children := make([]vmopv1.VirtualMachineSnapshotReference, 0, len(node.ChildSnapshotList))
	for _, child := range node.ChildSnapshotList {
		ref, err := updateSnapshotChildrenStatus(vmCtx, k8sClient, child)
		if err != nil {
			return nil, err
		}

		if ref != nil {
			children = append(children, *ref)
		} else {
			vmCtx.Logger.V(4).Info(
				"VirtualMachineSnapshot not found, adding an Unmanaged reference",
				"snapshotName", child.Name)
			children = append(children, vmopv1.VirtualMachineSnapshotReference{
				Type: vmopv1.VirtualMachineSnapshotReferenceTypeUnmanaged,
			})
		}
	}

	if !sameChildrenList(curSnapshot.Status.Children, children) {
		patch := ctrlclient.MergeFrom(curSnapshot.DeepCopy())
		curSnapshot.Status.Children = children
		if err := k8sClient.Status().Patch(vmCtx, curSnapshot, patch); err != nil {
			return nil, fmt.Errorf("failed to update snapshot children status %q: %w",
				curSnapshot.Name, err)
		}
	}

	// Return this node's reference to the parent.
	curSnapshotRef := &vmopv1.VirtualMachineSnapshotReference{
		Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
		Name: curSnapshot.Name,
	}

	return curSnapshotRef, nil
}

// getSnapshotCR gets the snapshot custom resource by name.
// If the snapshot custom resource is not found, it returns nil.
// Return other errors otherwise.
func getSnapshotCR(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	snapshotName string,
) (*vmopv1.VirtualMachineSnapshot, error) {

	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	objKey := ctrlclient.ObjectKey{
		Name:      snapshotName,
		Namespace: vmCtx.VM.Namespace,
	}

	if err := k8sClient.Get(vmCtx, objKey, vmSnapshot); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get VirtualMachineSnapshot %q: %w",
				snapshotName, err)
		}

		vmCtx.Logger.V(4).Info("VirtualMachineSnapshot not found, the snapshot might be Unmanaged",
			"snapshotName", snapshotName)
		return nil, nil
	}

	return vmSnapshot, nil
}

// updateCurrentSnapshotStatus updates the VM status to reflect the
// current snapshot on the VM.
func updateCurrentSnapshotStatus(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client) error {

	vmCtx.Logger.V(5).Info("Updating current snapshot in VM status")

	vm := vmCtx.VM

	// If VM has no snapshots, clear the status.
	if vmCtx.MoVM.Snapshot == nil {
		if vm.Status.CurrentSnapshot != nil {
			vmCtx.Logger.V(4).Info("VM has no snapshots, clearing status.currentSnapshot")
			vm.Status.CurrentSnapshot = nil
		}
		return nil
	}

	// If VM has no current snapshot, clear the status.
	if vmCtx.MoVM.Snapshot.CurrentSnapshot == nil {
		if vm.Status.CurrentSnapshot != nil {
			vmCtx.Logger.V(4).Info("VM has no current snapshot, clearing status.currentSnapshot")
			vm.Status.CurrentSnapshot = nil
		}
		return nil
	}

	// Get the current snapshot reference from vCenter.
	currentSnapMoref := vmCtx.MoVM.Snapshot.CurrentSnapshot

	// Find the snapshot name by traversing the snapshot tree.
	snapshot, err := virtualmachine.FindSnapshot(vmCtx.MoVM, currentSnapMoref.Value)
	if err != nil || snapshot == nil {
		vmCtx.Logger.V(4).Info("Could not find snapshot name in tree",
			"snapshotRef", currentSnapMoref.Value)
		// Clear the status if we can't find the snapshot name.
		if vm.Status.CurrentSnapshot != nil {
			vm.Status.CurrentSnapshot = nil
		}
		return nil
	}

	vmCtx.Logger.V(5).Info("Found snapshot name in tree",
		"snapshotName", snapshot.Name,
		"snapshotRef", currentSnapMoref.Value)

	// Check if there's a VirtualMachineSnapshot custom resource with this name.
	vmSnapshot, err := getSnapshotCR(vmCtx, k8sClient, snapshot.Name)
	if err != nil {
		return err
	}

	if vmSnapshot == nil {
		vmCtx.Logger.V(4).Info("VirtualMachineSnapshot not found, adding an Unmanaged reference",
			"snapshotName", snapshot.Name)
		vm.Status.CurrentSnapshot = &vmopv1.VirtualMachineSnapshotReference{
			Type: vmopv1.VirtualMachineSnapshotReferenceTypeUnmanaged,
		}
		return nil
	}

	// If the snapshot is being deleted, don't update the status.
	if !vmSnapshot.DeletionTimestamp.IsZero() {
		vmCtx.Logger.Error(nil, "VM points to a snapshot that is marked for deletion",
			"snapshotName", snapshot.Name)
		return nil
	}

	// Update the status to reflect the current snapshot name.
	vm.Status.CurrentSnapshot = &vmopv1.VirtualMachineSnapshotReference{
		Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
		Name: vmSnapshot.Name,
	}

	return nil
}

// updateRootSnapshots updates the VM status to reflect the
// root snapshots on the VM.
func updateRootSnapshots(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client) error {
	vmCtx.Logger.V(4).Info("Updating root snapshots in VM status")

	vm := vmCtx.VM
	// If VM has no snapshots, clear the root snapshots from the status.
	if vmCtx.MoVM.Snapshot == nil {
		if len(vm.Status.RootSnapshots) > 0 {
			vmCtx.Logger.V(4).Info("VM has no snapshots, clearing status.rootSnapshots")
			vm.Status.RootSnapshots = nil
		}
		return nil
	}

	// If VM has no root snapshots, clear the root snapshots from the status.
	if len(vmCtx.MoVM.Snapshot.RootSnapshotList) == 0 {
		if len(vm.Status.RootSnapshots) > 0 {
			vmCtx.Logger.V(4).Info("VM has no root snapshots, clearing status.rootSnapshots")
			vm.Status.RootSnapshots = nil
		}
		return nil
	}

	// Refresh the root snapshots from the VM mo
	var newRootSnapshots []vmopv1.VirtualMachineSnapshotReference //nolint:prealloc
	for _, rootSnapshot := range vmCtx.MoVM.Snapshot.RootSnapshotList {
		rootSnapshotCR, err := getSnapshotCR(vmCtx, k8sClient, rootSnapshot.Name)
		if err != nil {
			return err
		}

		if rootSnapshotCR == nil {
			vmCtx.Logger.V(4).Info("VirtualMachineSnapshot not found, adding an Unmanaged reference",
				"snapshotName", rootSnapshot.Name)
			newRootSnapshots = append(newRootSnapshots, vmopv1.VirtualMachineSnapshotReference{
				Type: vmopv1.VirtualMachineSnapshotReferenceTypeUnmanaged,
			})
			continue
		}

		newRootSnapshots = append(newRootSnapshots, vmopv1.VirtualMachineSnapshotReference{
			Type: vmopv1.VirtualMachineSnapshotReferenceTypeManaged,
			Name: rootSnapshotCR.Name,
		})
	}

	vm.Status.RootSnapshots = newRootSnapshots

	return nil
}

// sameChildrenList checks if two lists of VirtualMachineSnapshotReference
// are the same regardless of the order.
func sameChildrenList(a, b []vmopv1.VirtualMachineSnapshotReference) bool {
	if len(a) != len(b) {
		return false
	}

	m := sets.New[vmopv1.VirtualMachineSnapshotReference]()
	for i := range a {
		m.Insert(a[i])
	}
	for i := range b {
		if !m.Has(b[i]) {
			return false
		}
	}
	return true
}

// reconcileStatusController updates the VM's status.hardware.controllers field
// with information about all the controller types.
func reconcileStatusController(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	var (
		vm   = vmCtx.VM
		moVM = vmCtx.MoVM
	)

	if moVM.Config == nil || len(moVM.Config.Hardware.Device) == 0 {
		return nil
	}

	if vm.Status.Hardware == nil {
		vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{}
	}

	// Map controller's device key to virtual controller status.
	controllerKeyMap := make(map[int32]*vmopv1.VirtualControllerStatus)

	// Collect all the controllers.
	for _, device := range moVM.Config.Hardware.Device {
		var cs *vmopv1.VirtualControllerStatus

		switch ctrl := device.(type) {
		case *vimtypes.VirtualIDEController:
			cs = &vmopv1.VirtualControllerStatus{
				BusNumber: ctrl.BusNumber,
				Type:      vmopv1.VirtualControllerTypeIDE,
			}

		case vimtypes.BaseVirtualSCSIController:
			scsiCtrl := ctrl.GetVirtualSCSIController()
			cs = &vmopv1.VirtualControllerStatus{
				BusNumber: scsiCtrl.BusNumber,
				Type:      vmopv1.VirtualControllerTypeSCSI,
			}

		case vimtypes.BaseVirtualSATAController:
			sataCtrl := ctrl.GetVirtualSATAController()
			cs = &vmopv1.VirtualControllerStatus{
				BusNumber: sataCtrl.BusNumber,
				Type:      vmopv1.VirtualControllerTypeSATA,
			}

		case *vimtypes.VirtualNVMEController:
			cs = &vmopv1.VirtualControllerStatus{
				BusNumber: ctrl.BusNumber,
				Type:      vmopv1.VirtualControllerTypeNVME,
			}
		}

		if cs != nil {
			deviceKey := device.GetVirtualDevice().Key
			cs.DeviceKey = deviceKey
			controllerKeyMap[deviceKey] = cs
		}
	}

	// Collect all devices attached to controllers.
	for _, device := range moVM.Config.Hardware.Device {

		var (
			unitNumber    *int32
			controllerKey int32
			deviceType    vmopv1.VirtualDeviceType
		)

		switch dev := device.(type) {
		case *vimtypes.VirtualDisk:
			vdi := pkgutil.GetVirtualDiskInfo(dev)
			unitNumber = vdi.UnitNumber
			controllerKey = vdi.ControllerKey
			deviceType = vmopv1.VirtualDeviceTypeDisk

		case *vimtypes.VirtualCdrom:
			cdi := pkgutil.GetVirtualCdromInfo(dev)
			unitNumber = cdi.UnitNumber
			controllerKey = cdi.ControllerKey
			deviceType = vmopv1.VirtualDeviceTypeCDROM
		}

		// skip devices that are not attached nor owned by the controllers
		if _, ok := controllerKeyMap[controllerKey]; !ok ||
			unitNumber == nil || deviceType == "" {
			continue
		}

		deviceStatus := vmopv1.VirtualDeviceStatus{
			Type:       deviceType,
			UnitNumber: *unitNumber,
		}
		controllerKeyMap[controllerKey].Devices = append(
			controllerKeyMap[controllerKey].Devices, deviceStatus)

	}

	// Convert map to slice and sort for consistent output.
	var (
		ctlStatusesIndex int
		ctlStatuses      = make([]vmopv1.VirtualControllerStatus, len(controllerKeyMap))
	)
	for _, cs := range controllerKeyMap {

		// Sort the device statuses by type and unit number.
		slices.SortFunc(cs.Devices, func(a, b vmopv1.VirtualDeviceStatus) int {
			if a.Type != b.Type {
				return strings.Compare(string(a.Type), string(b.Type))
			}
			return int(a.UnitNumber - b.UnitNumber)
		})

		ctlStatuses[ctlStatusesIndex] = *cs
		ctlStatusesIndex++
	}

	// Sort controllers by type and then by bus number
	slices.SortFunc(ctlStatuses, func(a, b vmopv1.VirtualControllerStatus) int {
		if a.Type != b.Type {
			return strings.Compare(string(a.Type), string(b.Type))
		}
		return int(a.BusNumber - b.BusNumber)
	})

	vm.Status.Hardware.Controllers = ctlStatuses

	return nil
}

// reconcileStatusExtraConfig populates status.ExtraConfig with the
// first-class advanced VMX keys and spec.advanced.extraConfig bag keys
// currently observed on the VM, and sets ExtraConfigSynced from that same
// observed state: True once it fully matches spec.advanced and no power
// cycle is pending, or False with a specific reason (PowerOffRequired,
// PowerCyclePending, or ExtraConfigMismatch for a pending change that is
// neither) otherwise. It always reflects moVM as fetched at the start of
// this reconcile, so the condition is only ever set from vSphere's confirmed
// state — never optimistically from a configSpec this reconcile is still in
// the process of applying.
//
// This function is the sole source of the condition for every case except a
// real Reconfigure task failure, which only extraconfig.OnResult can see (it
// runs after the Reconfigure attempt, with the resulting error). So there is
// no correctness dependency between the two on ordering or on OnResult
// running in the same pass: whichever last touches the condition this cycle
// wins, and both independently agree on non-error cases.
func reconcileStatusExtraConfig(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	vm := vmCtx.VM
	moVM := vmCtx.MoVM
	if moVM.Config == nil {
		return nil
	}

	observed := pkgutil.OptionValues(moVM.Config.ExtraConfig)
	managedKeys := extraconfig.LoadVMManagedKeys(observed)

	var statusEC []common.KeyValuePair
	for _, k := range vmopv1util.SortedAdvancedVMXKeys() {
		if v, ok := observed.GetString(k); ok && v != "" {
			statusEC = append(statusEC, common.KeyValuePair{Key: k, Value: v})
		}
	}

	// Bag keys have no fixed candidate list like first-class keys do (their
	// names are arbitrary), so the previously-tracked managedKeys marker is
	// normally what tells us which keys to look up in observed. That marker
	// is empty until this reconciler has applied a given bag key at least
	// once, so also check any bag key currently requested in spec — covering
	// a key that already exists on the VM (e.g. pre-set by the class) before
	// this reconciler has ever applied it, matching how a first-class key's
	// current value is reported regardless of whether it matches desired.
	bagKeyCandidates := extraconfig.SpecBagKeys(vm.Spec.Advanced)
	for _, k := range managedKeys {
		bagKeyCandidates[k] = true
	}
	for _, k := range sets.List(sets.KeySet(bagKeyCandidates)) {
		if v, ok := observed.GetString(k); ok && v != "" {
			statusEC = append(statusEC, common.KeyValuePair{Key: k, Value: v})
		}
	}
	vm.Status.ExtraConfig = statusEC

	// Comparing spec.advanced against observed below is only meaningful once
	// this VM has been through the one-time TelcoVMServiceAPI schema-upgrade
	// backfill (pkg/providers/vsphere/upgrade/virtualmachine/backfill). Before
	// that, spec.advanced may still be nil/zero for a field a VM Class already
	// applied directly to vSphere, and comparing against it here would
	// misreport a pending clear for a value nothing actually wants cleared.
	fv := vmopv1util.ParseFeatureVersion(vm.Annotations[pkgconst.UpgradedToFeatureVersionAnnotationKey])
	if !fv.Has(vmopv1util.FeatureVersionTelcoVMServiceAPI) {
		return nil
	}

	poweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn

	// A prior cycle may have left vmx.reboot.powerCycle=TRUE on the VM: the
	// value already matches (so it won't appear in semanticResult below), but
	// the change only takes effect once the guest actually reboots. While the
	// VM is on, that flag — not value-matching alone — is authoritative for
	// "synced". Once the VM is off, all config is applied and ESXi clears the
	// flag automatically on next boot, so it's no longer treated as pending.
	var powerCycleOnVM bool
	if poweredOn {
		_, powerCycleOnVM = observed.GetString(constants.ExtraConfigReservedKeyVMXRebootPowerCycle)
	}

	// Diff spec.advanced against observed (scoped to only its own keys, via a
	// nil existingEC, so an unrelated reconciler's pending change can't be
	// mistaken for an ExtraConfig mismatch) and route it by vmxmode, mirroring
	// extraconfig.Reconcile's use of the same diff.
	applied, deferred, powerCyclePending := extraconfig.VMExtraConfigDiff(
		vmCtx, vm, observed, managedKeys, nil)

	switch {
	case len(deferred) > 0:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineExtraConfigSynced,
			vmopv1.VirtualMachinePowerOffRequiredReason,
			"VM power off required to apply: %s", strings.Join(deferred, ", "))
	case powerCyclePending || powerCycleOnVM:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineExtraConfigSynced,
			vmopv1.VirtualMachinePowerCyclePendingReason,
			"Applied changes take effect on next power cycle")
	case len(applied) == 0:
		conditions.MarkTrue(vm, vmopv1.VirtualMachineExtraConfigSynced)
	default:
		// A genuine pending change that is neither deferred nor
		// power-cycle-mode (e.g. a bag key just added). Mark it explicitly
		// rather than leaving a stale True or a stale reason from a prior
		// cycle in place — it resolves to True on the reconcile after this
		// one's Reconfigure completes and a fresh Properties() read
		// confirms it.
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineExtraConfigSynced,
			vmopv1.VirtualMachineExtraConfigMismatchReason,
			"Spec extraconfig differs from the live vSphere configuration")
	}

	return nil
}

// reconcileStatusNetworkExtraConfig appends NIC-level ExtraConfig entries
// (first-class VMXNet3 fields and spec.network.interfaces[].advancedProperties
// bag keys) onto status.ExtraConfig, populates the device-spec status fields
// (VNUMANodeID, VMXNet3), and sets NetworkConfigSynced from that same
// observed state. It always reflects moVM as fetched at the start of this
// reconcile, so the condition is only ever set from vSphere's confirmed
// state — never optimistically from a configSpec this reconcile is still in
// the process of applying, and never by mutating vmCtx.MoVM's device objects
// (see ReconcileNICFields's dryRun).
//
// It must run after reconcileStatusExtraConfig, since that function resets
// status.ExtraConfig via assignment — this one only appends.
//
// This function is the sole source of the condition for every case except a
// real Reconfigure task failure, which only networkextraconfig.OnResult can
// see (it runs after the Reconfigure attempt, with the resulting error). So
// there is no correctness dependency between the two on ordering or on
// OnResult running in the same pass: whichever last touches the condition
// this cycle wins, and both independently agree on non-error cases.
func reconcileStatusNetworkExtraConfig(
	vmCtx pkgctx.VirtualMachineContext,
	_ ctrlclient.Client,
	_ *object.VirtualMachine,
	_ ReconcileStatusData) []error { //nolint:unparam

	vm := vmCtx.VM
	moVM := vmCtx.MoVM
	if moVM.Config == nil || vm.Spec.Network == nil {
		return nil
	}

	ci := *moVM.Config
	observed := pkgutil.OptionValues(ci.ExtraConfig)
	poweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn
	matcher := networkextraconfig.DefaultNICMatcher(ci.Hardware.Device)
	nicKeyMap := vmopv1util.VMXNet3NICKeyMap()

	var overlay pkgutil.OptionValues
	var blocked, blockedPowerOff []string

	log := pkglog.FromContextOrDefault(vmCtx)
	for i, iface := range vm.Spec.Network.Interfaces {
		matchedDev := matcher(iface, i)
		if matchedDev == nil {
			log.V(4).Info("no hardware device for spec NIC; skipping status update", "interfaceName", iface.Name, "specIdx", i)
			continue
		}

		namespaceIdx, ok := networkextraconfig.EthernetDeviceIndex(matchedDev)
		if !ok {
			continue
		}
		devKey := matchedDev.GetVirtualDevice().Key
		mkKey := fmt.Sprintf(constants.NICExtraConfigManagedKeysKeyFmt, namespaceIdx)
		managed := extraconfig.LoadDeviceManagedKeys(observed, mkKey)
		prefix := vmopv1util.EthernetExtraConfigPrefix(devKey)

		overlay = append(overlay, networkextraconfig.DesiredNICExtraConfig(vmCtx, iface, devKey, managed)...)

		// First-class ExtraConfig fields: expand template keys with the
		// device's namespace index to get the live VMX key.
		for tmplKey := range nicKeyMap {
			fullKey := fmt.Sprintf(tmplKey, namespaceIdx)
			if v, ok2 := observed.GetString(fullKey); ok2 && v != "" {
				vm.Status.ExtraConfig = append(vm.Status.ExtraConfig,
					common.KeyValuePair{Key: fullKey, Value: v})
			}
		}

		// Managed bag keys. Same "also check spec-requested keys" reasoning
		// as reconcileStatusExtraConfig's bagKeyCandidates.
		bagKeyCandidates := networkextraconfig.NICSpecBagKeys(vmCtx, iface)
		for _, mk := range managed {
			bagKeyCandidates[mk] = true
		}
		for _, k := range sets.List(sets.KeySet(bagKeyCandidates)) {
			fullKey := prefix + k
			if v, ok2 := observed.GetString(fullKey); ok2 && v != "" {
				vm.Status.ExtraConfig = append(vm.Status.ExtraConfig,
					common.KeyValuePair{Key: fullKey, Value: v})
			}
		}

		// Device-spec fields in status.network.interfaces[i].
		updateInterfaceStatus(vm, iface, matchedDev, moVM)

		// Device-spec Blocked/PowerOffRequired reasons, dry-run: never
		// mutates matchedDev (and therefore never mutates vmCtx.MoVM).
		b, bpo := networkextraconfig.ReconcileNICFields(*vm, iface, matchedDev, ci, &vimtypes.VirtualMachineConfigSpec{}, true)
		blocked = append(blocked, b...)
		blockedPowerOff = append(blockedPowerOff, bpo...)
	}

	// Diff the assembled overlay against observed (scoped to only this NIC
	// set's own keys, via a nil existingEC, so an unrelated reconciler's
	// pending change can't be mistaken for a NIC ExtraConfig mismatch) and
	// route it by vmxmode, mirroring networkextraconfig.Reconcile's use of
	// the same diff.
	applied, deferred, powerCyclePending := networkextraconfig.NICExtraConfigDiff(
		vmCtx, observed, overlay, nil, poweredOn)
	blockedPowerOff = append(blockedPowerOff, deferred...)

	// A prior cycle may have left vmx.reboot.powerCycle=TRUE on the VM: see
	// reconcileStatusExtraConfig's powerCycleOnVM for the same reasoning.
	var powerCycleOnVM bool
	if poweredOn {
		_, powerCycleOnVM = observed.GetString(constants.ExtraConfigReservedKeyVMXRebootPowerCycle)
	}

	switch {
	case len(blocked) > 0:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachinePrerequisiteNotMetReason,
			"Prerequisites not met: %s", strings.Join(blocked, "; "))
	case len(blockedPowerOff) > 0:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachinePowerOffRequiredReason,
			"VM power off required to apply: %s", strings.Join(blockedPowerOff, ", "))
	case powerCyclePending || powerCycleOnVM:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachinePowerCyclePendingReason,
			"Applied changes take effect on next power cycle")
	case len(applied) == 0:
		conditions.MarkTrue(vm, vmopv1.VirtualMachineNetworkConfigSynced)
	default:
		// A genuine pending change that is neither deferred nor
		// power-cycle-mode (e.g. a bag key just added). Mark it explicitly
		// rather than leaving a stale True or a stale reason from a prior
		// cycle in place — it resolves to True on the reconcile after this
		// one's Reconfigure completes and a fresh Properties() read
		// confirms it.
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachineNetworkConfigMismatchReason,
			"Spec network interfaces ExtraConfig differs from the live vSphere configuration")
	}

	return nil
}

// updateInterfaceStatus populates the device-spec status fields
// (VNUMANodeID, VMXNet3) for interface i in vm.Status.Network.Interfaces.
func updateInterfaceStatus(
	vm *vmopv1.VirtualMachine,
	iface vmopv1.VirtualMachineNetworkInterfaceSpec,
	matchedDev vimtypes.BaseVirtualDevice,
	moVM mo.VirtualMachine,
) {
	if vm.Status.Network == nil {
		return
	}

	// Find the matching status entry by interface name or index.
	var statusIface *vmopv1.VirtualMachineNetworkInterfaceStatus
	for j := range vm.Status.Network.Interfaces {
		if vm.Status.Network.Interfaces[j].Name == iface.Name {
			statusIface = &vm.Status.Network.Interfaces[j]
			break
		}
	}
	if statusIface == nil {
		return
	}

	vdev := matchedDev.GetVirtualDevice()

	// VNUMANodeID: reflect observed device NumaNode; nil or negative means no affinity.
	numaNode := vdev.NumaNode
	if numaNode != nil && *numaNode >= 0 {
		statusIface.VNUMANodeID = numaNode
	} else {
		statusIface.VNUMANodeID = nil
	}

	// VMXNet3-specific status.
	vmxnet3Dev, isVMXNet3 := matchedDev.(*vimtypes.VirtualVmxnet3)
	if !isVMXNet3 {
		statusIface.VMXNet3 = nil
		return
	}

	vmxnet3Status := &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Status{
		UPTv2Enabled: vmxnet3Dev.Uptv2Enabled,
	}

	// UPTv2 runtime state from moVM.Runtime.Device.
	for _, rdi := range moVM.Runtime.Device {
		if rdi.Key != vdev.Key {
			continue
		}
		ethState, ok := rdi.RuntimeState.(*vimtypes.VirtualMachineDeviceRuntimeInfoVirtualEthernetCardRuntimeState)
		if !ok {
			break
		}
		vmxnet3Status.UPTv2Active = ethState.Uptv2Active
		vmxnet3Status.UPTv2InactiveReasonVM = ethState.Uptv2InactiveReasonVm
		vmxnet3Status.UPTv2InactiveReasonOther = ethState.Uptv2InactiveReasonOther
		break
	}

	statusIface.VMXNet3 = vmxnet3Status
}
