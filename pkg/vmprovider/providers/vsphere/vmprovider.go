// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
	"math/rand"
	"strings"

	"github.com/vmware/govmomi/find"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

const (
	VsphereVmProviderName = "vsphere"
)

var log = logf.Log.WithName(VsphereVmProviderName)

type vSphereVmProvider struct {
	sessions      session.Manager
	eventRecorder record.Recorder
}

func NewVSphereVmProviderFromClient(client ctrlruntime.Client, scheme *runtime.Scheme,
	recorder record.Recorder) vmprovider.VirtualMachineProviderInterface {
	vmProvider := &vSphereVmProvider{
		sessions:      session.NewManager(client, scheme),
		eventRecorder: recorder,
	}
	return vmProvider
}

// VSphereVmProviderGetSessionHack is an interface that exposes helpful
// functions to access certain resources without polluting the vm provider
// interface.
//
// Typical usecase might be a VM image test that wants to set a custom content
// library.
//
// ONLY USED IN TESTS.
type VSphereVmProviderGetSessionHack interface {
	GetClient(ctx goctx.Context) (*client.Client, error)
}

func (vs *vSphereVmProvider) Name() string {
	return VsphereVmProviderName
}

func (vs *vSphereVmProvider) Initialize(stop <-chan struct{}) {
}

func (vs *vSphereVmProvider) GetClient(ctx goctx.Context) (*client.Client, error) {
	return vs.sessions.GetClient(ctx)
}

func (vs *vSphereVmProvider) DeleteNamespaceSessionInCache(
	ctx goctx.Context,
	namespace string) error {

	log.V(4).Info("removing namespace from session cache", "namespace", namespace)
	return vs.sessions.DeleteSession(ctx, namespace)
}

// ListVirtualMachineImagesFromContentLibrary lists VM images from a ContentLibrary
func (vs *vSphereVmProvider) ListVirtualMachineImagesFromContentLibrary(
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

func (vs *vSphereVmProvider) DoesVirtualMachineExist(ctx goctx.Context, vm *v1alpha1.VirtualMachine) (bool, error) {
	vmCtx := context.VMContext{
		Context: ctx,
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	ses, err := vs.sessions.GetSession(
		vmCtx,
		vm.Labels[topology.KubernetesTopologyZoneLabelKey],
		vm.Namespace)

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

func (vs *vSphereVmProvider) getOpId(ctx goctx.Context, vm *v1alpha1.VirtualMachine, operation string) string {
	const charset = "0123456789abcdef"

	// TODO: Is this actually useful? Avoid looking up the session multiple times.
	var clusterID string
	if ses, err := vs.sessions.GetSession(
		ctx, vm.Labels[topology.KubernetesTopologyZoneLabelKey], vm.Namespace); err == nil {

		clusterID = ses.Cluster().Reference().Value
	}

	id := make([]byte, 8)
	for i := range id {
		id[i] = charset[rand.Intn(len(charset))]
	}

	return strings.Join([]string{"vmoperator", clusterID, vm.Name, operation, string(id)}, "-")
}

func (vs *vSphereVmProvider) CreateVirtualMachine(ctx goctx.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	vmCtx := context.VMContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "create")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("Creating VirtualMachine")

	ses, err := vs.sessions.GetSession(
		vmCtx,
		vm.Labels[topology.KubernetesTopologyZoneLabelKey],
		vm.Namespace)
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

// UpdateVirtualMachine updates the VM status, power state, phase etc
func (vs *vSphereVmProvider) UpdateVirtualMachine(ctx goctx.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	vmCtx := context.VMContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "update")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.V(4).Info("Updating VirtualMachine")

	ses, err := vs.sessions.GetSession(
		vmCtx,
		vm.Labels[topology.KubernetesTopologyZoneLabelKey],
		vm.Namespace)
	if err != nil {
		return err
	}

	err = ses.UpdateVirtualMachine(vmCtx, vmConfigArgs)
	if err != nil {
		return err
	}

	return nil
}

func (vs *vSphereVmProvider) DeleteVirtualMachine(ctx goctx.Context, vm *v1alpha1.VirtualMachine) error {
	vmCtx := context.VMContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "delete")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	vmCtx.Logger.Info("Deleting VirtualMachine")

	ses, err := vs.sessions.GetSession(
		ctx,
		vm.Labels[topology.KubernetesTopologyZoneLabelKey],
		vmCtx.VM.Namespace)
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

func (vs *vSphereVmProvider) GetVirtualMachineGuestHeartbeat(ctx goctx.Context, vm *v1alpha1.VirtualMachine) (v1alpha1.GuestHeartbeatStatus, error) {
	vmCtx := context.VMContext{
		Context: goctx.WithValue(ctx, vimtypes.ID{}, vs.getOpId(ctx, vm, "heartbeat")),
		Logger:  log.WithValues("vmName", vm.NamespacedName()),
		VM:      vm,
	}

	ses, err := vs.sessions.GetSession(
		ctx,
		vm.Labels[topology.KubernetesTopologyZoneLabelKey],
		vmCtx.VM.Namespace)
	if err != nil {
		return "", err
	}

	status, err := ses.GetVirtualMachineGuestHeartbeat(vmCtx)
	if err != nil {
		return "", err
	}

	return status, nil
}

func (vs *vSphereVmProvider) ComputeClusterCpuMinFrequency(ctx goctx.Context) error {
	if err := vs.sessions.ComputeClusterCpuMinFrequency(ctx); err != nil {
		return err
	}
	return nil
}

func (vs *vSphereVmProvider) UpdateVcPNID(ctx goctx.Context, vcPNID, vcPort string) error {
	return vs.sessions.UpdateVcPNID(ctx, vcPNID, vcPort)
}

func (vs *vSphereVmProvider) ClearSessionsAndClient(ctx goctx.Context) {
	vs.sessions.ClearSessionsAndClient(ctx)
}

func ResVmToVirtualMachineImage(ctx goctx.Context, resVM *res.VirtualMachine) (*v1alpha1.VirtualMachineImage, error) {
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
