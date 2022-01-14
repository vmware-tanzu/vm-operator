// Copyright (c) 2018-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
	"crypto/rand"
	"math/big"
	"strings"

	"github.com/vmware/govmomi/find"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

const (
	VsphereVMProviderName = "vsphere"
)

var log = logf.Log.WithName(VsphereVMProviderName)

type vSphereVMProvider struct {
	sessions      session.Manager
	eventRecorder record.Recorder
}

func NewVSphereVMProviderFromClient(
	client ctrlruntime.Client,
	recorder record.Recorder) vmprovider.VirtualMachineProviderInterface {

	return &vSphereVMProvider{
		sessions:      session.NewManager(client),
		eventRecorder: recorder,
	}
}

// VSphereVMProviderGetSessionHack is an interface that exposes helpful
// functions to access certain resources without polluting the vm provider
// interface.
//
// Typical usecase might be a VM image test that wants to set a custom content
// library.
//
// ONLY USED IN TESTS.
//nolint: revive // Ignore the warning about stuttering since this is only used in tests.
type VSphereVMProviderGetSessionHack interface {
	GetClient(ctx goctx.Context) (*vcclient.Client, error)
}

func (vs *vSphereVMProvider) Name() string {
	return VsphereVMProviderName
}

func (vs *vSphereVMProvider) Initialize(stop <-chan struct{}) {
}

func (vs *vSphereVMProvider) GetClient(ctx goctx.Context) (*vcclient.Client, error) {
	return vs.sessions.GetClient(ctx)
}

func (vs *vSphereVMProvider) DeleteNamespaceSessionInCache(
	ctx goctx.Context,
	namespace string) error {

	log.V(4).Info("removing namespace from session cache", "namespace", namespace)
	return vs.sessions.DeleteSession(ctx, namespace)
}

// ListVirtualMachineImagesFromContentLibrary lists VM images from a ContentLibrary.
func (vs *vSphereVMProvider) ListVirtualMachineImagesFromContentLibrary(
	ctx goctx.Context,
	contentLibrary v1alpha1.ContentLibraryProvider,
	currentCLImages map[string]v1alpha1.VirtualMachineImage) ([]*v1alpha1.VirtualMachineImage, error) {

	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary",
		"name", contentLibrary.Name,
		"UUID", contentLibrary.Spec.UUID)

	client, err := vs.sessions.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	return client.ContentLibClient().VirtualMachineImageResourcesForLibrary(
		ctx,
		contentLibrary.Spec.UUID,
		currentCLImages)
}

func (vs *vSphereVMProvider) DoesVirtualMachineExist(ctx goctx.Context, vm *v1alpha1.VirtualMachine) (bool, error) {
	vmCtx := context.VirtualMachineContext{
		Context: ctx,
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	ses, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return false, err
	}

	if _, err = ses.GetVirtualMachine(vmCtx); err != nil {
		switch err.(type) {
		case *find.NotFoundError, *find.DefaultNotFoundError:
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

func (vs *vSphereVMProvider) getOpID(ctx goctx.Context, vm *v1alpha1.VirtualMachine, operation string) string {
	const charset = "0123456789abcdef"

	// TODO: Is this actually useful? Avoid looking up the session multiple times.
	var clusterID string
	if ses, err := vs.sessions.GetSession(ctx, vm.Labels[topology.KubernetesTopologyZoneLabelKey], vm.Namespace); err == nil {
		clusterID = ses.Cluster().Reference().Value
	}

	id := make([]byte, 8)
	for i := range id {
		randOpID, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))

		id[i] = charset[int(randOpID.Int64())]
	}

	return strings.Join([]string{"vmoperator", clusterID, vm.Name, operation, string(id)}, "-")
}

func (vs *vSphereVMProvider) CreateVirtualMachine(ctx goctx.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) error {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "create")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("Creating VirtualMachine")

	ses, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return err
	}

	resVM, err := ses.CloneVirtualMachine(vmCtx, vmConfigArgs)
	if err != nil {
		vmCtx.Logger.Error(err, "Clone VirtualMachine failed")
		return err
	}

	// Set a few Status fields that we easily have on hand here. The controller will immediately call
	// UpdateVirtualMachine() which will set it all.
	vm.Status.Phase = v1alpha1.Created
	vm.Status.UniqueID = resVM.MoRef().Value

	return nil
}

// UpdateVirtualMachine updates the VM status, power state, phase etc.
func (vs *vSphereVMProvider) UpdateVirtualMachine(ctx goctx.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) error {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "update")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.V(4).Info("Updating VirtualMachine")

	ses, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return err
	}

	err = ses.UpdateVirtualMachine(vmCtx, vmConfigArgs)
	if err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVMProvider) DeleteVirtualMachine(ctx goctx.Context, vm *v1alpha1.VirtualMachine) error {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "delete")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("Deleting VirtualMachine")

	ses, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return err
	}

	err = ses.DeleteVirtualMachine(vmCtx)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to delete VM")
		return err
	}

	return nil
}

func (vs *vSphereVMProvider) GetVirtualMachineGuestHeartbeat(ctx goctx.Context, vm *v1alpha1.VirtualMachine) (v1alpha1.GuestHeartbeatStatus, error) {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "heartbeat")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	ses, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return "", err
	}

	status, err := ses.GetVirtualMachineGuestHeartbeat(vmCtx)
	if err != nil {
		return "", err
	}

	return status, nil
}

func (vs *vSphereVMProvider) GetCompatibleHosts(ctx goctx.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VMConfigArgs) ([]string, error) {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "create")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmSession, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return nil, err
	}

	selectedNodes, err := vmSession.GetCompatibleHosts(vmCtx, vmConfigArgs)
	if err != nil {
		return nil, err
	}

	return selectedNodes, nil
}

// GetHostNetworkInfoFn is added for integration tests.
// VC Simulator does not configure Network Info of Hosts.
// Also, we can not update DNS config of host as govmomi does not implement UpdateDnsConfig method for host NetworkSystem.
var GetHostNetworkInfoFn = func(vmSession *session.Session, vmCtx context.VirtualMachineContext, hostMoID string) (string, error) {
	return vmSession.GetHostNetworkInfo(vmCtx, hostMoID)
}

func (vs *vSphereVMProvider) GetHostNetworkInfo(ctx goctx.Context, vm *v1alpha1.VirtualMachine, hostMoID string) (string, error) {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "create")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmSession, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return "", err
	}

	return GetHostNetworkInfoFn(vmSession, vmCtx, hostMoID)
}

func (vs *vSphereVMProvider) GetVirtualMachineWebMKSTicket(ctx goctx.Context, vm *v1alpha1.VirtualMachine, pubKey string) (string, error) {
	vmCtx := context.VirtualMachineContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "webconsole")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	ses, err := vs.sessions.GetSessionForVM(vmCtx)
	if err != nil {
		return "", err
	}

	return ses.GetVirtualMachineWebMKSTicket(vmCtx, pubKey)
}

func (vs *vSphereVMProvider) ComputeClusterCPUMinFrequency(ctx goctx.Context) error {
	return vs.sessions.ComputeClusterCPUMinFrequency(ctx)
}

func (vs *vSphereVMProvider) UpdateVcPNID(ctx goctx.Context, vcPNID, vcPort string) error {
	return vs.sessions.UpdateVcPNID(ctx, vcPNID, vcPort)
}

func (vs *vSphereVMProvider) ClearSessionsAndClient(ctx goctx.Context) {
	vs.sessions.ClearSessionsAndClient(ctx)
}

func ResVMToVirtualMachineImage(ctx goctx.Context, resVM *res.VirtualMachine) (*v1alpha1.VirtualMachineImage, error) {
	ovfProperties, err := resVM.GetOvfProperties(ctx)
	if err != nil {
		return nil, err
	}

	// Prior code just used default values if the Properties called failed.
	moVM, _ := resVM.GetProperties(ctx, []string{"config.createDate", "summary"})

	var createTimestamp metav1.Time
	if moVM.Config != nil && moVM.Config.CreateDate != nil {
		createTimestamp = metav1.NewTime(*moVM.Config.CreateDate)
	}

	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              resVM.Name,
			Annotations:       ovfProperties,
			CreationTimestamp: createTimestamp,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            "VM",
			ImageSourceType: "Inventory",
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       moVM.Summary.Config.Uuid,
			InternalId: resVM.ReferenceValue(),
			PowerState: string(moVM.Summary.Runtime.PowerState),
		},
	}, nil
}
