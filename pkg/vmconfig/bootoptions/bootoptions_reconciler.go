// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package bootoptions

import (
	"context"
	"errors"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

var (
	// ErrVMConfigUnavailable is returned by the reconcile function if the VirtualMachineConfigInfo
	// of the virtual machine managed object is not set.
	ErrVMConfigUnavailable = errors.New("VM config is not available")
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

func New() vmconfig.Reconciler {
	return reconciler{}
}

func (r reconciler) Name() string {
	return "bootoptions"
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
	k8sClient client.Client,
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
		return ErrVMConfigUnavailable
	}
	ci := *moVM.Config

	vmBootOptions := vmopv1.VirtualMachineBootOptions{}
	if vm.Spec.BootOptions != nil {
		vmBootOptions = *vm.Spec.BootOptions
	}

	csBootOptions := vimtypes.VirtualMachineBootOptions{}

	if vmBootOptions.BootDelay != nil {
		csBootOptions.BootDelay = vmBootOptions.BootDelay.Duration.Milliseconds()
	}

	if vmBootOptions.BootRetry != "" {
		csBootOptions.BootRetryEnabled = ptr.To(vmBootOptions.BootRetry == vmopv1.VirtualMachineBootOptionsBootRetryEnabled)
	}

	if vmBootOptions.BootRetryDelay != nil {
		csBootOptions.BootRetryDelay = vmBootOptions.BootRetryDelay.Duration.Milliseconds()
	}

	if vmBootOptions.EnterBootSetup != "" {
		csBootOptions.EnterBIOSSetup = ptr.To(vmBootOptions.EnterBootSetup == vmopv1.VirtualMachineBootOptionsForceBootEntryEnabled)
	}

	if vmBootOptions.EFISecureBoot != "" {
		csBootOptions.EfiSecureBootEnabled = ptr.To(vmBootOptions.EFISecureBoot == vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled)
	}

	var networkBootProtocol string
	switch vmBootOptions.NetworkBootProtocol {
	case vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP6:
		networkBootProtocol = string(vimtypes.VirtualMachineBootOptionsNetworkBootProtocolTypeIpv6)
	case vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP4:
		networkBootProtocol = string(vimtypes.VirtualMachineBootOptionsNetworkBootProtocolTypeIpv4)
	default:
	}
	csBootOptions.NetworkBootProtocol = networkBootProtocol

	cs := vimtypes.VirtualMachineConfigSpec{
		BootOptions: &csBootOptions,
	}

	resize.CompareBootOptions(ci, cs, configSpec)

	return nil
}
