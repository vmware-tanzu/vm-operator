// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// BenchmarkCreateOrUpdateVirtualMachineUpdatePath reports allocations for the
// steady-state VM update reconciliation path with networking disabled (Baseline)
// and with two NICs reporting guest addresses, DNS, and routes plus ten
// PVC-backed data disks (GuestNetwork10PVCs).
// Run it with:
//
//	go test ./pkg/providers/vsphere -run '^$' -bench '^BenchmarkCreateOrUpdateVirtualMachineUpdatePath$' -benchmem
func BenchmarkCreateOrUpdateVirtualMachineUpdatePath(b *testing.B) {
	for _, tc := range []struct {
		name         string
		guestNetwork bool
		pvcCount     int
	}{
		{name: "Baseline"},
		{name: "GuestNetwork10PVCs", guestNetwork: true, pvcCount: 10},
	} {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkVMUpdate(b, tc.guestNetwork, tc.pvcCount)
		})
	}
}

func benchmarkVMUpdate(b *testing.B, withGuestNetwork bool, pvcCount int) {
	b.Helper()
	gomega.RegisterFailHandler(func(message string, callerSkip ...int) {
		b.Helper()
		b.Fatal(message)
	})

	parentCtx := newVMTestParentContext()
	vmClass, vm := newVMTestObjects("allocation-benchmark-vm")
	testConfig := newVMTestConfig()
	if withGuestNetwork {
		testConfig.NumNetworks = 0
		testConfig.WithNetworkEnv = builder.NetworkEnvNamed
		vm.Spec.Network.Disabled = false
		vm.Spec.Network.Interfaces = []vmopv1.VirtualMachineNetworkInterfaceSpec{
			{Name: "eth0", Network: &vmopv1common.PartialObjectRef{Name: "VM Network"}},
			{Name: "eth1", Network: &vmopv1common.PartialObjectRef{Name: dvpgName}},
		}
	}
	ctx, vmProvider, _ := setupVMTest(parentCtx, testConfig, vmClass, vm)
	b.Cleanup(ctx.AfterEach)

	// Settle create and initial status work before measuring steady-state
	// updates. This follows the retry behavior used by the vmprovider_vm tests.
	benchmarkReconcileUntilSettled(b, ctx, vmProvider, vm)
	vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
	if pvcCount > 0 {
		benchmarkAttachPVCs(b, ctx, vcVM, vm, pvcCount)
		benchmarkReconcileUntilSettled(b, ctx, vmProvider, vm)
		benchmarkAssertPVCs(b, ctx, vcVM, vm, pvcCount)
	}
	if withGuestNetwork {
		benchmarkPopulateGuestNetwork(b, ctx, vcVM)
		benchmarkReconcileUntilSettled(b, ctx, vmProvider, vm)
		benchmarkAssertGuestNetwork(b, vm)
	}

	var before mo.VirtualMachine
	if err := vcVM.Properties(ctx, vcVM.Reference(), []string{"recentTask"}, &before); err != nil {
		b.Fatalf("failed to fetch tasks before measurement: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		opCtx := ctxop.WithContext(ctx)
		if err := vmProvider.CreateOrUpdateVirtualMachine(opCtx, vm); err != nil {
			b.Fatalf("failed to reconcile VM update: %v", err)
		}
		if !ctxop.IsUpdate(opCtx) {
			b.Fatal("benchmark reconciliation did not take the VM update path")
		}
		ctxop.MarkUpdate(ctx)
	}
	b.StopTimer()

	var after mo.VirtualMachine
	if err := vcVM.Properties(ctx, vcVM.Reference(), []string{"recentTask"}, &after); err != nil {
		b.Fatalf("failed to fetch tasks after measurement: %v", err)
	}
	if !reflect.DeepEqual(before.RecentTask, after.RecentTask) {
		b.Fatal("measured reconciliation changed the VM's recent tasks")
	}
	if withGuestNetwork {
		benchmarkAssertGuestNetwork(b, vm)
	}
	if pvcCount > 0 {
		benchmarkAssertPVCs(b, ctx, vcVM, vm, pvcCount)
	}
}

func benchmarkAttachPVCs(
	b *testing.B,
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	vm *vmopv1.VirtualMachine,
	count int) {
	b.Helper()
	g := gomega.NewWithT(b)
	var state mo.VirtualMachine
	g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.hardware.device"}, &state)).To(gomega.Succeed())
	devices := object.VirtualDeviceList(state.Config.Hardware.Device)
	controllerDevice, err := devices.CreateSCSIController("pvscsi")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(vcVM.AddDevice(ctx, controllerDevice)).To(gomega.Succeed())
	g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.hardware.device"}, &state)).To(gomega.Succeed())
	devices = object.VirtualDeviceList(state.Config.Hardware.Device)
	controllers := devices.SelectByType((*vimtypes.VirtualSCSIController)(nil))
	g.Expect(controllers).To(gomega.HaveLen(1))
	controller := controllers[0].(vimtypes.BaseVirtualSCSIController)
	bootDisks := devices.SelectByType((*vimtypes.VirtualDisk)(nil))
	g.Expect(bootDisks).To(gomega.HaveLen(1))
	bootBacking := bootDisks[0].GetVirtualDevice().Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
	g.Expect(bootBacking.Datastore).ToNot(gomega.BeNil())
	var vmPath object.DatastorePath
	g.Expect(vmPath.FromString(bootBacking.FileName)).To(gomega.BeTrue())

	// Model the completed work of the external CSI and volume controllers.
	// Create real simulator devices and bound PVC/PV pairs before measuring.
	var added []vimtypes.BaseVirtualDevice
	claimByDiskID := make(map[string]string, count)
	for i := range count {
		claimName := fmt.Sprintf("benchmark-data-%02d", i)
		diskID := uuid.NewString()
		diskPath := vmPath
		diskPath.Path = path.Join(path.Dir(vmPath.Path), claimName+".vmdk")
		disk := devices.CreateDisk(controller.GetVirtualSCSIController(), *bootBacking.Datastore, diskPath.String())
		disk.CapacityInKB = 1024 * 1024
		disk.CapacityInBytes = 1024 * 1024 * 1024
		disk.VDiskId = &vimtypes.ID{Id: diskID}
		devices = append(devices, disk)
		added = append(added, disk)
		claimByDiskID[diskID] = claimName

		capacity := corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: claimName, Namespace: vm.Namespace, UID: types.UID(uuid.NewString())},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: ptr.To(ctx.StorageClassName),
				VolumeName:       claimName,
				Resources:        corev1.VolumeResourceRequirements{Requests: capacity},
			},
		}
		g.Expect(ctx.Client.Create(ctx, pvc)).To(gomega.Succeed())
		pvc.Status = corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound, AccessModes: pvc.Spec.AccessModes, Capacity: capacity}
		g.Expect(ctx.Client.Status().Update(ctx, pvc)).To(gomega.Succeed())
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: claimName},
			Spec: corev1.PersistentVolumeSpec{
				Capacity:                      capacity,
				AccessModes:                   pvc.Spec.AccessModes,
				StorageClassName:              ctx.StorageClassName,
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
				ClaimRef:                      &corev1.ObjectReference{APIVersion: "v1", Kind: "PersistentVolumeClaim", Name: pvc.Name, Namespace: pvc.Namespace, UID: pvc.UID},
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{Driver: "csi.vsphere.vmware.com", VolumeHandle: diskID},
				},
			},
		}
		g.Expect(ctx.Client.Create(ctx, pv)).To(gomega.Succeed())
		pv.Status.Phase = corev1.VolumeBound
		g.Expect(ctx.Client.Status().Update(ctx, pv)).To(gomega.Succeed())
	}
	g.Expect(vcVM.AddDevice(ctx, added...)).To(gomega.Succeed())
	g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.hardware.device"}, &state)).To(gomega.Succeed())

	var statuses []vmopv1.VirtualMachineVolumeStatus
	for _, device := range state.Config.Hardware.Device {
		disk, ok := device.(*vimtypes.VirtualDisk)
		if !ok || disk.VDiskId == nil {
			continue
		}
		claimName, ok := claimByDiskID[disk.VDiskId.Id]
		if !ok {
			continue
		}
		backing := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
		g.Expect(backing.Uuid).ToNot(gomega.BeEmpty())
		vm.Spec.Volumes = append(vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
			Name:                claimName,
			ControllerType:      vmopv1.VirtualControllerTypeSCSI,
			ControllerBusNumber: ptr.To(controller.GetVirtualSCSIController().BusNumber),
			UnitNumber:          disk.UnitNumber,
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
				},
			},
		})
		statuses = append(statuses, vmopv1.VirtualMachineVolumeStatus{
			Name: claimName, Type: vmopv1.VolumeTypeManaged, Attached: true,
			DiskUUID: backing.Uuid, Requested: resource.NewQuantity(disk.CapacityInBytes, resource.BinarySI),
		})
	}
	g.Expect(statuses).To(gomega.HaveLen(count))
	// The fake client does not increment generation for spec changes.
	vm.Generation++
	g.Expect(ctx.Client.Update(ctx, vm)).To(gomega.Succeed())
	vm.Status.Volumes = append(vm.Status.Volumes, statuses...)
	g.Expect(ctx.Client.Status().Update(ctx, vm)).To(gomega.Succeed())
}

func benchmarkAssertPVCs(
	b *testing.B,
	ctx *builder.TestContextForVCSim,
	vcVM *object.VirtualMachine,
	vm *vmopv1.VirtualMachine,
	count int) {
	b.Helper()
	g := gomega.NewWithT(b)
	var state mo.VirtualMachine
	g.Expect(vcVM.Properties(ctx, vcVM.Reference(), []string{"config.hardware.device"}, &state)).To(gomega.Succeed())
	disks := object.VirtualDeviceList(state.Config.Hardware.Device).SelectByType((*vimtypes.VirtualDisk)(nil))
	g.Expect(disks).To(gomega.HaveLen(count + 1))
	diskIDByUUID := make(map[string]string, count)
	for _, device := range disks {
		disk := device.(*vimtypes.VirtualDisk)
		if disk.VDiskId != nil {
			backing := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
			diskIDByUUID[backing.Uuid] = disk.VDiskId.Id
		}
	}
	g.Expect(diskIDByUUID).To(gomega.HaveLen(count))
	mapping, err := vsphere.GetAttachedDiskUUIDToPVC(pkgctx.VirtualMachineContext{Context: ctx, VM: vm, MoVM: state}, ctx.Client)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(mapping).To(gomega.HaveLen(count))
	for i := range count {
		name := fmt.Sprintf("benchmark-data-%02d", i)
		var pvc corev1.PersistentVolumeClaim
		g.Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: name}, &pvc)).To(gomega.Succeed())
		g.Expect(pvc.Status.Phase).To(gomega.Equal(corev1.ClaimBound))
		var pv corev1.PersistentVolume
		g.Expect(ctx.Client.Get(ctx, ctrlclient.ObjectKey{Name: pvc.Spec.VolumeName}, &pv)).To(gomega.Succeed())
		g.Expect(pv.Status.Phase).To(gomega.Equal(corev1.VolumeBound))
		g.Expect(pv.Spec.ClaimRef.UID).To(gomega.Equal(pvc.UID))
		var matched bool
		for _, vol := range vm.Status.Volumes {
			if vol.Name == name {
				matched = true
				g.Expect(vol.Type).To(gomega.Equal(vmopv1.VolumeTypeManaged))
				g.Expect(vol.Attached).To(gomega.BeTrue())
				g.Expect(vol.Used).ToNot(gomega.BeNil())
				g.Expect(mapping[vol.DiskUUID].Name).To(gomega.Equal(name))
				g.Expect(diskIDByUUID[vol.DiskUUID]).To(gomega.Equal(pv.Spec.CSI.VolumeHandle))
			}
		}
		g.Expect(matched).To(gomega.BeTrue(), "missing volume status for %s", name)
	}
}

func benchmarkPopulateGuestNetwork(b *testing.B, ctx *builder.TestContextForVCSim, vcVM *object.VirtualMachine) {
	b.Helper()

	var state mo.VirtualMachine
	if err := vcVM.Properties(ctx, vcVM.Reference(), []string{"config.hardware.device", "guest"}, &state); err != nil {
		b.Fatalf("failed to fetch VM devices: %v", err)
	}
	nics := object.VirtualDeviceList(state.Config.Hardware.Device).SelectByType((*vimtypes.VirtualEthernetCard)(nil))
	if len(nics) != 2 {
		b.Fatalf("expected two NICs, got %d", len(nics))
	}
	guest := state.Guest
	if guest == nil {
		guest = &vimtypes.GuestInfo{}
	}
	guest.HostName = "allocation-benchmark-vm.example.test"
	guest.IpAddress = "192.0.2.10"
	guest.Net = nil
	for i, device := range nics {
		card := device.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
		guest.Net = append(guest.Net, vimtypes.GuestNicInfo{
			DeviceConfigId: card.Key,
			MacAddress:     card.MacAddress,
			Connected:      true,
			IpConfig: &vimtypes.NetIpConfigInfo{
				IpAddress: []vimtypes.NetIpConfigInfoIpAddress{
					{IpAddress: fmt.Sprintf("192.0.2.%d", 10+i), PrefixLength: 24, Origin: "manual", State: "preferred"},
					{IpAddress: fmt.Sprintf("2001:db8::%d", 10+i), PrefixLength: 64, Origin: "manual", State: "preferred"},
				},
			},
			DnsConfig: &vimtypes.NetDnsConfigInfo{
				HostName:     "allocation-benchmark-vm",
				DomainName:   "example.test",
				IpAddress:    []string{"192.0.2.53", "2001:db8::53"},
				SearchDomain: []string{"example.test", "svc.example.test"},
			},
		})
	}
	guest.IpStack = []vimtypes.GuestStackInfo{{
		DnsConfig: guest.Net[0].DnsConfig,
		IpRouteConfig: &vimtypes.NetIpRouteConfigInfo{
			IpRoute: []vimtypes.NetIpRouteConfigInfoIpRoute{
				{Network: "0.0.0.0", PrefixLength: 0, Gateway: vimtypes.NetIpRouteConfigInfoGateway{IpAddress: "192.0.2.1", Device: "0"}},
				{Network: "::", PrefixLength: 0, Gateway: vimtypes.NetIpRouteConfigInfoGateway{IpAddress: "2001:db8::1", Device: "0"}},
			},
		},
	}}

	// Populate the simulator itself so every measured property retrieval sees
	// the same guest data, rather than only changing the Kubernetes status.
	sctx := ctx.SimulatorContext()
	sctx.WithLock(vcVM.Reference(), func() {
		sctx.Map.Get(vcVM.Reference()).(*simulator.VirtualMachine).Guest = guest
	})
}

func benchmarkAssertGuestNetwork(b *testing.B, vm *vmopv1.VirtualMachine) {
	b.Helper()
	g := gomega.NewWithT(b)
	g.Expect(vm.Status.Network).ToNot(gomega.BeNil())
	network := vm.Status.Network
	g.Expect(network.PrimaryIP4).To(gomega.Equal("192.0.2.10"))
	g.Expect(network.Interfaces).To(gomega.HaveLen(2))
	for i, iface := range network.Interfaces {
		g.Expect(iface.Name).To(gomega.Equal(fmt.Sprintf("eth%d", i)))
		g.Expect(iface.IP).ToNot(gomega.BeNil())
		g.Expect(iface.IP.MACAddr).ToNot(gomega.BeEmpty())
		g.Expect(iface.IP.Addresses).To(gomega.HaveLen(2))
		g.Expect(iface.IP.Addresses[0].Address).To(gomega.Equal(fmt.Sprintf("192.0.2.%d", 10+i)))
		g.Expect(iface.IP.Addresses[1].Address).To(gomega.Equal(fmt.Sprintf("2001:db8::%d", 10+i)))
		g.Expect(iface.DNS).ToNot(gomega.BeNil())
		g.Expect(iface.DNS.Nameservers).To(gomega.HaveLen(2))
		g.Expect(iface.DNS.SearchDomains).To(gomega.HaveLen(2))
	}
	g.Expect(network.IPStacks).To(gomega.HaveLen(1))
	g.Expect(network.IPStacks[0].IPRoutes).To(gomega.HaveLen(2))
	g.Expect(network.IPStacks[0].DNS.Nameservers).To(gomega.HaveLen(2))
}

func benchmarkReconcileUntilSettled(
	b *testing.B,
	ctx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) {
	b.Helper()

	var consecutiveNonErrorCallCount int
	for range createOrUpdateVMMaxAllowedCallCount {
		opCtx := ctxop.WithContext(ctx)
		err := provider.CreateOrUpdateVirtualMachine(opCtx, vm)
		if ctxop.IsUpdate(opCtx) {
			ctxop.MarkUpdate(ctx)
		}

		if err == nil {
			consecutiveNonErrorCallCount++
			if consecutiveNonErrorCallCount == 2 {
				return
			}
			continue
		}

		consecutiveNonErrorCallCount = 0
		if !benchmarkReconcileErrorCanRetry(err) {
			b.Fatalf("failed to prepare VM update benchmark: %v", err)
		}
	}

	b.Fatalf("VM did not settle after %d calls", createOrUpdateVMMaxAllowedCallCount)
}

func benchmarkReconcileErrorCanRetry(err error) bool {
	return errors.Is(err, vsphere.ErrCreate) ||
		errors.Is(err, vsphere.ErrBackup) ||
		errors.Is(err, vsphere.ErrBootstrapCustomize) ||
		errors.Is(err, vsphere.ErrBootstrapReconfigure) ||
		errors.Is(err, vsphere.ErrReconfigure) ||
		errors.Is(err, vsphere.ErrRestart) ||
		errors.Is(err, vsphere.ErrSetPowerState) ||
		errors.Is(err, vsphere.ErrUpgradeHardwareVersion) ||
		errors.Is(err, vsphere.ErrPromoteDisks) ||
		errors.Is(err, vsphere.ErrSnapshotRevert) ||
		errors.Is(err, vsphere.ErrPolicyNotReady) ||
		errors.Is(err, vsphere.ErrUpgradeSchema) ||
		errors.Is(err, vsphere.ErrUpgradeObject)
}
