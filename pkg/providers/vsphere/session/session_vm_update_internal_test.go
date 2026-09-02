package session

import (
	"testing"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReconcileSnapshotDisks_NotFound(t *testing.T) {
	vm := &vmopv1.VirtualMachine{
		Spec: vmopv1.VirtualMachineSpec{
			Volumes: []vmopv1.VirtualMachineVolume{
				{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:   "non-existent-snap",
							DiskID: "disk-123",
						},
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = vmopv1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := t.Context()

	err := reconcileSnapshotDisks(ctx, k8sClient, vm, nil, mo.VirtualMachine{Config: &types.VirtualMachineConfigInfo{}}, &types.VirtualMachineConfigSpec{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(vm.Status.Volumes) != 1 {
		t.Fatalf("expected 1 volume status, got %d", len(vm.Status.Volumes))
	}
	if vm.Status.Volumes[0].Error != "VirtualMachineSnapshot non-existent-snap not found" {
		t.Fatalf("unexpected error message: %s", vm.Status.Volumes[0].Error)
	}
}

func TestReconcileSnapshotDisks_NotReady(t *testing.T) {
	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "vm-namespace",
		},
		Spec: vmopv1.VirtualMachineSpec{
			Volumes: []vmopv1.VirtualMachineVolume{
				{
					Name: "snap-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						VirtualMachineSnapshot: &vmopv1.VirtualMachineSnapshotDiskSpec{
							Name:      "not-ready-snap",
							Namespace: "snap-namespace",
							DiskID:    "disk-123",
						},
					},
				},
			},
		},
	}

	snapshot := &vmopv1.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "not-ready-snap",
			Namespace: "snap-namespace",
		},
	}

	scheme := runtime.NewScheme()
	_ = vmopv1.AddToScheme(scheme)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapshot).Build()
	ctx := t.Context()

	err := reconcileSnapshotDisks(ctx, k8sClient, vm, nil, mo.VirtualMachine{Config: &types.VirtualMachineConfigInfo{}}, &types.VirtualMachineConfigSpec{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(vm.Status.Volumes) != 1 {
		t.Fatalf("expected 1 volume status, got %d", len(vm.Status.Volumes))
	}
	if vm.Status.Volumes[0].Error != "VirtualMachineSnapshot not-ready-snap is not ready" {
		t.Fatalf("unexpected error message: %s", vm.Status.Volumes[0].Error)
	}
}

func TestRemoveDetachedSnapshotDisks(t *testing.T) {
	// Setup a VM with a snapshot disk in its config, but NOT in its spec.volumes
	moVM := mo.VirtualMachine{
		Config: &types.VirtualMachineConfigInfo{
			Hardware: types.VirtualHardware{
				Device: []types.BaseVirtualDevice{
					&types.VirtualDisk{
						VirtualDevice: types.VirtualDevice{
							Key: 2000,
							Backing: &types.VirtualDiskFlatVer2BackingInfo{
								Uuid:     "disk-uuid-123",
								Parent:   &types.VirtualDiskFlatVer2BackingInfo{}, // Indicates it's a snapshot disk backing
								DiskMode: string(types.VirtualDiskModeIndependent_nonpersistent),
							},
						},
					},
					&types.VirtualDisk{
						VirtualDevice: types.VirtualDevice{
							Key: 2001,
							Backing: &types.VirtualDiskFlatVer2BackingInfo{
								Uuid: "disk-uuid-456",
								// No parent, normal disk
							},
						},
					},
				},
			},
		},
	}

	snapshotVolumes := []vmopv1.VirtualMachineVolume{} // Empty, meaning no snapshot disks should be attached
	configSpec := &types.VirtualMachineConfigSpec{}

	vm := &vmopv1.VirtualMachine{}
	removeDetachedSnapshotDisks(moVM, vm, snapshotVolumes, configSpec)

	if len(configSpec.DeviceChange) != 1 {
		t.Fatalf("expected 1 device change for removal, got %d", len(configSpec.DeviceChange))
	}

	change := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
	if change.Operation != types.VirtualDeviceConfigSpecOperationRemove {
		t.Fatalf("expected remove operation, got %v", change.Operation)
	}
	if change.Device.GetVirtualDevice().Key != 2000 {
		t.Fatalf("expected to remove device key 2000, got %d", change.Device.GetVirtualDevice().Key)
	}
}
