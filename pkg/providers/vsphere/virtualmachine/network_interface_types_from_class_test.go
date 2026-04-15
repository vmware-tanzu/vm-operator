// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"testing"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopapi "github.com/vmware-tanzu/vm-operator/api"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	networkInterfaceTestNs = "default"
)

func fakeClientScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = vmopapi.AddToScheme(s)
	return s
}

func TestFillEmptyNetworkInterfaceTypesFromClass_NoClass_DefaultsVMXNet3(t *testing.T) {
	t.Parallel()
	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns"},
		Spec: vmopv1.VirtualMachineSpec{
			Network: &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
			},
		},
	}
	ok, err := FillEmptyNetworkInterfaceTypesFromClass(t.Context(), fake.NewClientBuilder().Build(), vm)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected mutation")
	}
	if vm.Spec.Network.Interfaces[0].Type != vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3 {
		t.Fatalf("got %q", vm.Spec.Network.Interfaces[0].Type)
	}
}

func TestFillEmptyNetworkInterfaceTypesFromClass_UsesClassConfigSpec(t *testing.T) {
	t.Parallel()
	className := "test-class"

	cs := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device: &vimtypes.VirtualSriovEthernetCard{
					VirtualEthernetCard: vimtypes.VirtualEthernetCard{
						VirtualDevice: vimtypes.VirtualDevice{},
					},
				},
			},
		},
	}
	raw, err := pkgutil.MarshalConfigSpecToJSON(cs)
	if err != nil {
		t.Fatal(err)
	}

	vmClass := &vmopv1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{Name: className, Namespace: networkInterfaceTestNs},
		Spec:       vmopv1.VirtualMachineClassSpec{ConfigSpec: raw},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(fakeClientScheme()).
		WithObjects(vmClass).
		Build()

	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: networkInterfaceTestNs},
		Spec: vmopv1.VirtualMachineSpec{
			ClassName: className,
			Network: &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
			},
		},
	}

	ok, err := FillEmptyNetworkInterfaceTypesFromClass(t.Context(), k8sClient, vm)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected mutation")
	}
	if vm.Spec.Network.Interfaces[0].Type != vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV {
		t.Fatalf("got %q", vm.Spec.Network.Interfaces[0].Type)
	}
}

func TestFillEmptyNetworkInterfaceTypesFromClass_ClassInstanceWithoutClassName(t *testing.T) {
	t.Parallel()
	instName := "active-inst"

	cs := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device: &vimtypes.VirtualSriovEthernetCard{
					VirtualEthernetCard: vimtypes.VirtualEthernetCard{
						VirtualDevice: vimtypes.VirtualDevice{},
					},
				},
			},
		},
	}
	raw, err := pkgutil.MarshalConfigSpecToJSON(cs)
	if err != nil {
		t.Fatal(err)
	}

	inst := &vmopv1.VirtualMachineClassInstance{
		ObjectMeta: metav1.ObjectMeta{Name: instName, Namespace: networkInterfaceTestNs},
		Spec: vmopv1.VirtualMachineClassInstanceSpec{
			VirtualMachineClassSpec: vmopv1.VirtualMachineClassSpec{ConfigSpec: raw},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(fakeClientScheme()).
		WithObjects(inst).
		Build()

	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: networkInterfaceTestNs},
		Spec: vmopv1.VirtualMachineSpec{
			Class: &common.LocalObjectRef{Name: instName},
			Network: &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
			},
		},
	}

	ok, err := FillEmptyNetworkInterfaceTypesFromClass(t.Context(), k8sClient, vm)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected mutation")
	}
	if vm.Spec.Network.Interfaces[0].Type != vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV {
		t.Fatalf("got %q", vm.Spec.Network.Interfaces[0].Type)
	}
}

func TestFillEmptyNetworkInterfaceTypesFromClass_PrefersClassInstanceOverClassName(t *testing.T) {
	t.Parallel()
	className := "template-class"
	instName := "pinned-inst"

	csVMX := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device: &vimtypes.VirtualVmxnet3{
					VirtualVmxnet: vimtypes.VirtualVmxnet{
						VirtualEthernetCard: vimtypes.VirtualEthernetCard{
							VirtualDevice: vimtypes.VirtualDevice{},
						},
					},
				},
			},
		},
	}
	rawVMX, err := pkgutil.MarshalConfigSpecToJSON(csVMX)
	if err != nil {
		t.Fatal(err)
	}

	csSRIOV := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device: &vimtypes.VirtualSriovEthernetCard{
					VirtualEthernetCard: vimtypes.VirtualEthernetCard{
						VirtualDevice: vimtypes.VirtualDevice{},
					},
				},
			},
		},
	}
	rawSRIOV, err := pkgutil.MarshalConfigSpecToJSON(csSRIOV)
	if err != nil {
		t.Fatal(err)
	}

	vmClass := &vmopv1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{Name: className, Namespace: networkInterfaceTestNs},
		Spec:       vmopv1.VirtualMachineClassSpec{ConfigSpec: rawVMX},
	}
	inst := &vmopv1.VirtualMachineClassInstance{
		ObjectMeta: metav1.ObjectMeta{Name: instName, Namespace: networkInterfaceTestNs},
		Spec: vmopv1.VirtualMachineClassInstanceSpec{
			VirtualMachineClassSpec: vmopv1.VirtualMachineClassSpec{ConfigSpec: rawSRIOV},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(fakeClientScheme()).
		WithObjects(vmClass, inst).
		Build()

	vm := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Namespace: networkInterfaceTestNs},
		Spec: vmopv1.VirtualMachineSpec{
			ClassName: className,
			Class:     &common.LocalObjectRef{Name: instName},
			Network: &vmopv1.VirtualMachineNetworkSpec{
				Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
			},
		},
	}

	ok, err := FillEmptyNetworkInterfaceTypesFromClass(t.Context(), k8sClient, vm)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected mutation")
	}
	if vm.Spec.Network.Interfaces[0].Type != vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV {
		t.Fatalf("got %q, want SRIOV from instance snapshot", vm.Spec.Network.Interfaces[0].Type)
	}
}

func TestNetworkInterfaceTypesFromClassConfigSpec_Order(t *testing.T) {
	t.Parallel()
	cs := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device: &vimtypes.VirtualPCIPassthrough{
					VirtualDevice: vimtypes.VirtualDevice{},
				},
			},
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device: &vimtypes.VirtualVmxnet3{
					VirtualVmxnet: vimtypes.VirtualVmxnet{
						VirtualEthernetCard: vimtypes.VirtualEthernetCard{
							VirtualDevice: vimtypes.VirtualDevice{},
						},
					},
				},
			},
		},
	}
	got := NetworkInterfaceTypesFromClassConfigSpec(cs)
	if len(got) != 1 || got[0] != vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3 {
		t.Fatalf("got %#v", got)
	}
}
