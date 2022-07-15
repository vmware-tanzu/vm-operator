// Copyright (c) 2018-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	goctx "context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	vcclient "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
)

const (
	VsphereVMProviderName = "vsphere"
)

var log = logf.Log.WithName(VsphereVMProviderName)

type vSphereVMProvider struct {
	sessions      session.Manager
	k8sClient     ctrlruntime.Client
	eventRecorder record.Recorder
}

func NewVSphereVMProviderFromClient(
	client ctrlruntime.Client,
	recorder record.Recorder) vmprovider.VirtualMachineProviderInterface {

	return &vSphereVMProvider{
		sessions:      session.NewManager(client),
		k8sClient:     client,
		eventRecorder: recorder,
	}
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

// ListItemsFromContentLibrary list items from a content library.
func (vs *vSphereVMProvider) ListItemsFromContentLibrary(ctx goctx.Context,
	contentLibrary *v1alpha1.ContentLibraryProvider) ([]string, error) {
	log.V(4).Info("Listing VirtualMachineImages from ContentLibrary",
		"name", contentLibrary.Name,
		"UUID", contentLibrary.Spec.UUID)

	client, err := vs.sessions.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	return client.ContentLibClient().ListLibraryItems(ctx, contentLibrary.Spec.UUID)
}

// GetVirtualMachineImageFromContentLibrary gets a VM image from a ContentLibrary.
func (vs *vSphereVMProvider) GetVirtualMachineImageFromContentLibrary(
	ctx goctx.Context,
	contentLibrary *v1alpha1.ContentLibraryProvider,
	itemID string,
	currentCLImages map[string]v1alpha1.VirtualMachineImage) (*v1alpha1.VirtualMachineImage, error) {

	log.V(4).Info("Getting VirtualMachineImage from ContentLibrary",
		"name", contentLibrary.Name,
		"UUID", contentLibrary.Spec.UUID)

	client, err := vs.sessions.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	return client.ContentLibClient().VirtualMachineImageResourceForLibrary(
		ctx,
		itemID,
		contentLibrary.Spec.UUID,
		currentCLImages)
}

func (vs *vSphereVMProvider) getOpID(vm *v1alpha1.VirtualMachine, operation string) string {
	const charset = "0123456789abcdef"

	// TODO: Is this actually useful?
	id := make([]byte, 8)
	for i := range id {
		idx := rand.Intn(len(charset)) //nolint:gosec
		id[i] = charset[idx]
	}

	return strings.Join([]string{"vmoperator", vm.Name, operation, string(id)}, "-")
}

func (vs *vSphereVMProvider) ComputeClusterCPUMinFrequency(ctx goctx.Context) error {
	_, err := vs.sessions.ComputeAndGetCPUMinFrequency(ctx)
	return err
}

func (vs *vSphereVMProvider) ComputeAndGetCPUMinFrequency(ctx goctx.Context) (uint64, error) {
	return vs.sessions.ComputeAndGetCPUMinFrequency(ctx)
}

func (vs *vSphereVMProvider) UpdateVcPNID(ctx goctx.Context, vcPNID, vcPort string) error {
	return vs.sessions.UpdateVcPNID(ctx, vcPNID, vcPort)
}

func (vs *vSphereVMProvider) ClearSessionsAndClient(ctx goctx.Context) {
	vs.sessions.ClearSessionsAndClient(ctx)
}

func (vs *vSphereVMProvider) getVM(vmCtx context.VirtualMachineContext) (*object.VirtualMachine, error) {
	client, err := vs.GetClient(vmCtx)
	if err != nil {
		return nil, err
	}

	return vcenter.GetVirtualMachine(vmCtx, vs.k8sClient, client.Finder(), nil)
}

// ResVMToVirtualMachineImage isn't currently used.
func ResVMToVirtualMachineImage(ctx goctx.Context, vm *object.VirtualMachine) (*v1alpha1.VirtualMachineImage, error) {
	var o mo.VirtualMachine
	err := vm.Properties(ctx, vm.Reference(), []string{"summary", "config.createDate", "config.vAppConfig"}, &o)
	if err != nil {
		return nil, err
	}

	var createTimestamp metav1.Time
	ovfProps := make(map[string]string)

	if o.Config != nil {
		if o.Config.CreateDate != nil {
			createTimestamp = metav1.NewTime(*o.Config.CreateDate)
		}

		if o.Config.VAppConfig != nil {
			if vAppConfig := o.Config.VAppConfig.GetVmConfigInfo(); vAppConfig != nil {
				for _, prop := range vAppConfig.Property {
					if strings.HasPrefix(prop.Id, "vmware-system") {
						if prop.Value != "" {
							ovfProps[prop.Id] = prop.Value
						} else {
							ovfProps[prop.Id] = prop.DefaultValue
						}
					}
				}
			}
		}
	}

	return &v1alpha1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              o.Summary.Config.Name,
			Annotations:       ovfProps,
			CreationTimestamp: createTimestamp,
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            "VM",
			ImageSourceType: "Inventory",
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       o.Summary.Config.Uuid,
			InternalId: vm.Reference().Value,
			PowerState: string(o.Summary.Runtime.PowerState),
		},
	}, nil
}

func (vs *vSphereVMProvider) reverseVMZoneLookup(vmCtx context.VirtualMachineContext) error {
	if vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] != "" {
		return nil
	}

	availabilityZones, err := topology.GetAvailabilityZones(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	if len(availabilityZones) == 0 {
		return fmt.Errorf("no AvailabilityZones")
	}

	zone, err := vs.vmToZoneLookup(vmCtx, availabilityZones)
	if err != nil {
		return err
	}

	if vmCtx.VM.Labels == nil {
		vmCtx.VM.Labels = map[string]string{}
	}
	vmCtx.VM.Labels[topology.KubernetesTopologyZoneLabelKey] = zone
	return nil
}

func (vs *vSphereVMProvider) vmToZoneLookup(
	vmCtx context.VirtualMachineContext,
	availabilityZones []topologyv1.AvailabilityZone) (string, error) {

	client, err := vs.GetClient(vmCtx)
	if err != nil {
		return "", err
	}

	vcVM, err := vcenter.GetVirtualMachine(vmCtx, vs.k8sClient, client.Finder(), nil)
	if err != nil {
		return "", err
	}

	rp, err := vcVM.ResourcePool(vmCtx)
	if err != nil {
		return "", err
	}

	cluster, err := rp.Owner(vmCtx)
	if err != nil {
		return "", err
	}

	clusterMoID := cluster.Reference().Value

	for _, az := range availabilityZones {
		if az.Spec.ClusterComputeResourceMoId == clusterMoID {
			return az.Name, nil
		}

		for _, moID := range az.Spec.ClusterComputeResourceMoIDs {
			if moID == clusterMoID {
				return az.Name, nil
			}
		}
	}

	return "", fmt.Errorf("failed to find zone for cluster MoID %s", clusterMoID)
}
