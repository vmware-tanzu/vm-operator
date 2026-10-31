// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/validation"
)

// benchmarkStorageProfileID is the storage policy ID of the StorageClass used
// by the VM and its PVCs. It is a valid UUID so the StoragePolicy object name
// is derived as it is in production.
const benchmarkStorageProfileID = "aa6d5a82-1c88-45da-85d3-3d74b91a5bad"

// benchmarkDiskLayout describes the PVC-backed data disks attached to a
// benchmark VM.
type benchmarkDiskLayout string

const (
	// benchmarkLayoutWSFC1Disk is a single Microsoft WSFC disk on a
	// Physical-sharing SCSI controller.
	benchmarkLayoutWSFC1Disk benchmarkDiskLayout = "WSFC1Disk"

	// benchmarkLayoutMixed60Disks is 60 disks spread across four
	// controllers: 14 ordinary disks on SCSI:0 (leaving one free unit), 15
	// WSFC disks on the Physical-sharing SCSI:1, 15 ordinary disks on SCSI:2,
	// and 16 ordinary disks on NVME:0.
	benchmarkLayoutMixed60Disks benchmarkDiskLayout = "Mixed60Disks"
)

// benchmarkUpdate describes how the new VM differs from the old VM.
type benchmarkUpdate string

const (
	// benchmarkUpdateMetadataOnly adds a label, so no volume is revalidated.
	benchmarkUpdateMetadataOnly benchmarkUpdate = "MetadataOnly"

	// benchmarkUpdateAddDisk adds one disk to a free unit of an existing
	// controller, so only the new disk is revalidated.
	benchmarkUpdateAddDisk benchmarkUpdate = "AddDisk"

	// benchmarkUpdateAddController adds a SCSI controller and a disk on it,
	// which changes spec.hardware and revalidates every disk.
	benchmarkUpdateAddController benchmarkUpdate = "AddController"
)

// BenchmarkValidateVirtualMachine reports the cost of VirtualMachine admission
// validation for VMs shaped like ones the mutation webhook and schema upgrade
// have already processed: image, class, network, CD-ROMs, IDE controllers, and
// PVC-backed data disks with assigned controllers and unit numbers. The VM is
// powered off so that controllers may be added. Every PVC, its StorageClass,
// and the unencrypted StoragePolicy exist, and shared disks use ReadWriteMany
// PVCs, so all cases are admitted.
//
// Setup and one warm-up call happen before measurement. The warm-up asserts
// that the request is allowed and that the validator reads the expected
// number of PVCs, so each case exercises the intended volume validation path.
//
// Run it with:
//
//	go test ./webhooks/virtualmachine/validation -run '^$' \
//	  -bench '^BenchmarkValidateVirtualMachine$' -benchmem
func BenchmarkValidateVirtualMachine(b *testing.B) {
	for _, tc := range []struct {
		layout        benchmarkDiskLayout
		update        benchmarkUpdate
		expectedReads int
	}{
		{layout: benchmarkLayoutWSFC1Disk, expectedReads: 1},
		{layout: benchmarkLayoutMixed60Disks, expectedReads: 60},
		{layout: benchmarkLayoutWSFC1Disk, update: benchmarkUpdateMetadataOnly},
		{layout: benchmarkLayoutMixed60Disks, update: benchmarkUpdateMetadataOnly},
		{layout: benchmarkLayoutMixed60Disks, update: benchmarkUpdateAddDisk, expectedReads: 1},
		{layout: benchmarkLayoutWSFC1Disk, update: benchmarkUpdateAddController, expectedReads: 2},
		{layout: benchmarkLayoutMixed60Disks, update: benchmarkUpdateAddController, expectedReads: 61},
	} {
		name := "Create/" + string(tc.layout)
		if tc.update != "" {
			name = "Update/" + string(tc.update) + "/" + string(tc.layout)
		}
		b.Run(name, func(b *testing.B) {
			benchmarkValidateVM(b, tc.layout, tc.update, tc.expectedReads)
		})
	}
}

func benchmarkValidateVM(
	b *testing.B,
	layout benchmarkDiskLayout,
	update benchmarkUpdate,
	expectedReads int) {

	b.Helper()
	gomega.RegisterFailHandler(func(message string, _ ...int) {
		b.Helper()
		b.Fatal(message)
	})
	g := gomega.NewWithT(b)

	vm, objects := newBenchmarkVM(layout)

	var oldVM *vmopv1.VirtualMachine
	if update != "" {
		oldVM = vm.DeepCopy()
		objects = append(objects, applyBenchmarkUpdate(vm, update)...)
	}

	ctx := builder.NewUnitTestContextForValidatingWebhook(
		validation.NewValidator, &unstructured.Unstructured{}, nil)

	pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
		config.Features.VMSharedDisks = true
	})
	bypassUpgradeCheck(&ctx.Context, vm)
	if oldVM != nil {
		bypassUpgradeCheck(&ctx.Context, oldVM)
	}

	objects = append(objects, &infrav1.StoragePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			Name:      kubeutil.GetStoragePolicyObjectName(benchmarkStorageProfileID),
		},
		Spec: infrav1.StoragePolicySpec{
			ID: benchmarkStorageProfileID,
		},
	})

	var (
		countReads bool
		pvcReads   int
	)
	ctx.Client = builder.NewFakeClientWithInterceptors(interceptor.Funcs{
		Get: func(
			c context.Context,
			cli ctrlclient.WithWatch,
			key ctrlclient.ObjectKey,
			obj ctrlclient.Object,
			opts ...ctrlclient.GetOption) error {

			if _, ok := obj.(*corev1.PersistentVolumeClaim); ok && countReads {
				pvcReads++
			}
			return cli.Get(c, key, obj, opts...)
		},
	}, objects...)
	ctx.Validator = validation.NewValidator(ctx.Client)

	var err error
	ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(vm)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	if oldVM != nil {
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(oldVM)
		g.Expect(err).ToNot(gomega.HaveOccurred())
	}

	validate := ctx.ValidateCreate
	if oldVM != nil {
		validate = ctx.ValidateUpdate
	}

	countReads = true
	response := validate(&ctx.WebhookRequestContext)
	countReads = false
	g.Expect(response.Allowed).To(gomega.BeTrue(), string(response.Result.Reason))
	g.Expect(pvcReads).To(gomega.Equal(expectedReads),
		"unexpected number of PVC reads")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if response := validate(&ctx.WebhookRequestContext); !response.Allowed {
			b.Fatalf("request was denied: %s", response.Result.Reason)
		}
	}
}

// newBenchmarkVM returns a VM with the specified disk layout and the objects
// the validator reads for it.
func newBenchmarkVM(
	layout benchmarkDiskLayout) (*vmopv1.VirtualMachine, []ctrlclient.Object) {

	storageClass := builder.DummyStorageClassWithID(benchmarkStorageProfileID)
	storageClass.Name = dummyDefaultStorageClassName

	quota := builder.DummyResourceQuota(
		dummyNamespaceName,
		storageClass.Name+".storageclass.storage.k8s.io/persistentvolumeclaims")
	quota.Name = storageClass.Name + "-quota"

	vm := builder.DummyVirtualMachine()
	vm.Name = "benchmark-vm"
	vm.Namespace = dummyNamespaceName
	vm.Spec.StorageClass = storageClass.Name
	vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
	vm.Spec.Volumes = nil

	objects := []ctrlclient.Object{
		builder.DummyAvailabilityZone(),
		builder.DummyZone(dummyNamespaceName),
		storageClass,
		quota,
	}

	switch layout {
	case benchmarkLayoutWSFC1Disk:
		vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
			benchmarkSCSIController(0, vmopv1.VirtualControllerSharingModePhysical),
		}
		objects = append(objects, addBenchmarkDisks(
			vm, vmopv1.VirtualControllerTypeSCSI, 0, 1, true)...)

	case benchmarkLayoutMixed60Disks:
		vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
			benchmarkSCSIController(0, vmopv1.VirtualControllerSharingModeNone),
			benchmarkSCSIController(1, vmopv1.VirtualControllerSharingModePhysical),
			benchmarkSCSIController(2, vmopv1.VirtualControllerSharingModeNone),
		}
		vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
			{
				BusNumber:   0,
				SharingMode: vmopv1.VirtualControllerSharingModeNone,
			},
		}
		objects = append(objects, addBenchmarkDisks(
			vm, vmopv1.VirtualControllerTypeSCSI, 0, 14, false)...)
		objects = append(objects, addBenchmarkDisks(
			vm, vmopv1.VirtualControllerTypeSCSI, 1, 15, true)...)
		objects = append(objects, addBenchmarkDisks(
			vm, vmopv1.VirtualControllerTypeSCSI, 2, 15, false)...)
		objects = append(objects, addBenchmarkDisks(
			vm, vmopv1.VirtualControllerTypeNVME, 0, 16, false)...)

	default:
		panic(fmt.Sprintf("unknown benchmark disk layout %q", layout))
	}

	return vm, objects
}

// applyBenchmarkUpdate changes vm as described by update and returns any new
// objects the changed VM references.
func applyBenchmarkUpdate(
	vm *vmopv1.VirtualMachine,
	update benchmarkUpdate) []ctrlclient.Object {

	switch update {
	case benchmarkUpdateMetadataOnly:
		vm.Labels["benchmark.vmoperator.vmware.com/updated"] = "true"
		return nil

	case benchmarkUpdateAddDisk:
		return addBenchmarkDisks(vm, vmopv1.VirtualControllerTypeSCSI, 0, 1, false)

	case benchmarkUpdateAddController:
		busNumber := int32(len(vm.Spec.Hardware.SCSIControllers))
		vm.Spec.Hardware.SCSIControllers = append(
			vm.Spec.Hardware.SCSIControllers,
			benchmarkSCSIController(busNumber, vmopv1.VirtualControllerSharingModeNone))
		return addBenchmarkDisks(
			vm, vmopv1.VirtualControllerTypeSCSI, busNumber, 1, false)

	default:
		panic(fmt.Sprintf("unknown benchmark update %q", update))
	}
}

func benchmarkSCSIController(
	busNumber int32,
	sharingMode vmopv1.VirtualControllerSharingMode) vmopv1.SCSIControllerSpec {

	return vmopv1.SCSIControllerSpec{
		BusNumber:   busNumber,
		Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
		SharingMode: sharingMode,
	}
}

// addBenchmarkDisks appends count PVC-backed disks to the next free unit
// numbers of the specified controller and returns their PVCs. Shared disks
// are Microsoft WSFC disks backed by ReadWriteMany PVCs.
func addBenchmarkDisks(
	vm *vmopv1.VirtualMachine,
	controllerType vmopv1.VirtualControllerType,
	busNumber int32,
	count int,
	shared bool) []ctrlclient.Object {

	usedUnits := map[int32]bool{}
	for _, vol := range vm.Spec.Volumes {
		if vol.ControllerType == controllerType &&
			vol.ControllerBusNumber != nil &&
			*vol.ControllerBusNumber == busNumber &&
			vol.UnitNumber != nil {

			usedUnits[*vol.UnitNumber] = true
		}
	}

	var (
		objects []ctrlclient.Object
		unit    int32
	)
	for range count {
		// SCSI unit 7 is reserved for the controller itself.
		for usedUnits[unit] ||
			(controllerType == vmopv1.VirtualControllerTypeSCSI && unit == 7) {
			unit++
		}
		usedUnits[unit] = true

		name := fmt.Sprintf("disk-%d", len(vm.Spec.Volumes))
		accessMode := corev1.ReadWriteOnce
		vol := vmopv1.VirtualMachineVolume{
			Name:                name,
			ControllerType:      controllerType,
			ControllerBusNumber: ptr.To(busNumber),
			UnitNumber:          ptr.To(unit),
			VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
				PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: name,
					},
				},
			},
		}
		if shared {
			accessMode = corev1.ReadWriteMany
			vol.ApplicationType = vmopv1.VolumeApplicationTypeMicrosoftWSFC
			vol.DiskMode = vmopv1.VolumeDiskModeIndependentPersistent
		}
		vm.Spec.Volumes = append(vm.Spec.Volumes, vol)

		objects = append(objects, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: vm.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{accessMode},
				StorageClassName: ptr.To(vm.Spec.StorageClass),
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		})
	}

	return objects
}
