// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

const (
	storageResourceQuotaStrPattern = ".storageclass.storage.k8s.io/"
)

func GetVirtualMachine(ctx context.Context, client ctrlclient.Client, ns, name string) (*vmopv1.VirtualMachine, error) {
	virtualMachine := &vmopv1.VirtualMachine{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, virtualMachine)
	if err != nil {
		return nil, err
	}

	return virtualMachine, nil
}

// TODO: the above should return vmopv1a5, but requires some refactoring of existing tests.
func GetVirtualMachineA5(ctx context.Context, client ctrlclient.Client, ns, name string) (*vmopv1a5.VirtualMachine, error) {
	virtualMachine := &vmopv1a5.VirtualMachine{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, virtualMachine)
	if err != nil {
		return nil, err
	}

	return virtualMachine, nil
}

func DeleteVirtualMachine(ctx context.Context, client ctrlclient.Client, ns, name string) error {
	virtualMachine := &vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}

	return client.Delete(ctx, virtualMachine)
}

func DeleteVirtualMachineSnapshot(ctx context.Context, client ctrlclient.Client, ns, name string) error {
	virtualMachineSnapshot := &vmopv1a5.VirtualMachineSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}

	return client.Delete(ctx, virtualMachineSnapshot)
}

func GetVirtualMachineClass(ctx context.Context, client ctrlclient.Client, ns, name string) (*vmopv1.VirtualMachineClass, error) {
	virtualMachineClass := &vmopv1.VirtualMachineClass{}

	key := types.NamespacedName{
		Name:      name,
		Namespace: ns,
	}

	err := client.Get(ctx, key, virtualMachineClass)
	if err != nil {
		return nil, err
	}

	return virtualMachineClass, nil
}

func ListVirtualMachineImagesWithOptions(ctx context.Context, client ctrlclient.Client, options []ctrlclient.ListOption) (*vmopv1.VirtualMachineImageList, error) {
	virtualMachineImageList := &vmopv1.VirtualMachineImageList{}

	err := client.List(ctx, virtualMachineImageList, options...)
	if err != nil {
		return nil, err
	}

	return virtualMachineImageList, nil
}

// ListClusterVirtualMachineImages returns the CVMI List in the client's cluster.
func ListClusterVirtualMachineImages(ctx context.Context, client ctrlclient.Client) (*vmopv1.ClusterVirtualMachineImageList, error) {
	clusterVirtualMachineImageList := &vmopv1.ClusterVirtualMachineImageList{}

	err := client.List(ctx, clusterVirtualMachineImageList)
	if err != nil {
		return nil, err
	}

	return clusterVirtualMachineImageList, nil
}

func GetVirtualMachineSnapshot(ctx context.Context, client ctrlclient.Client, ns, name string) (*vmopv1a5.VirtualMachineSnapshot, error) {
	virtualMachineSnapshot := &vmopv1a5.VirtualMachineSnapshot{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, virtualMachineSnapshot)
	if err != nil {
		return nil, err
	}

	return virtualMachineSnapshot, nil
}

func GetVirtualMachinePublishRequest(ctx context.Context, client ctrlclient.Client, ns, name string) (*vmopv1.VirtualMachinePublishRequest, error) {
	virtualMachinePublishRequest := &vmopv1.VirtualMachinePublishRequest{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, virtualMachinePublishRequest)
	if err != nil {
		return nil, err
	}

	return virtualMachinePublishRequest, nil
}

func GetVirtualMachineGroupPublishRequest(ctx context.Context, client ctrlclient.Client, ns, name string) (
	*vmopv1a5.VirtualMachineGroupPublishRequest, error) {
	virtualMachineGroupPublishRequest := &vmopv1a5.VirtualMachineGroupPublishRequest{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, virtualMachineGroupPublishRequest)
	if err != nil {
		return nil, err
	}

	return virtualMachineGroupPublishRequest, nil
}

func GetVirtualMachineGroup(ctx context.Context, client ctrlclient.Client, ns, name string) (
	*vmopv1a5.VirtualMachineGroup, error) {
	virtualMachineGroup := &vmopv1a5.VirtualMachineGroup{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, virtualMachineGroup)
	if err != nil {
		return nil, err
	}

	return virtualMachineGroup, nil
}

func GetVirtualMachineGroupMemberStatus(ctx context.Context, client ctrlclient.Client, ns, groupName, memberName, memberKind string) (vmopv1a5.VirtualMachineGroupMemberStatus, error) {
	vmg, err := GetVirtualMachineGroup(ctx, client, ns, groupName)
	if err != nil {
		return vmopv1a5.VirtualMachineGroupMemberStatus{}, fmt.Errorf("failed to get VirtualMachineGroup: %w", err)
	}

	for _, memberStatus := range vmg.Status.Members {
		if memberStatus.Name == memberName && memberStatus.Kind == memberKind {
			return memberStatus, nil
		}
	}

	return vmopv1a5.VirtualMachineGroupMemberStatus{}, fmt.Errorf("member %s/%s not found in VirtualMachineGroup %q status", memberKind, memberName, groupName)
}

func GetVWebConsoleRequest(ctx context.Context, client ctrlclient.Client, ns, name string) (*vmopv1a1.WebConsoleRequest, error) {
	webConsoleRequest := &vmopv1a1.WebConsoleRequest{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, webConsoleRequest)
	if err != nil {
		return nil, err
	}

	return webConsoleRequest, nil
}

func GetVirtualMachineWebConsoleRequest(ctx context.Context, client ctrlclient.Client, ns, name string) (*vmopv1a2.VirtualMachineWebConsoleRequest, error) {
	virtualMachineWebConsoleRequest := &vmopv1a2.VirtualMachineWebConsoleRequest{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	err := client.Get(ctx, key, virtualMachineWebConsoleRequest)
	if err != nil {
		return nil, err
	}

	return virtualMachineWebConsoleRequest, nil
}

func IsStorageClassInNamespace(ctx context.Context, k8sClient ctrlclient.Client, namespace, name string, podVMOnStretchedSupervisorEnabled bool) (bool, error) {
	if podVMOnStretchedSupervisorEnabled {
		var obj spqv1.StoragePolicyQuotaList

		err := k8sClient.List(ctx, &obj, ctrlclient.InNamespace(namespace))
		if err != nil {
			return false, err
		}

		for i := range obj.Items {
			for j := range obj.Items[i].Status.SCLevelQuotaStatuses {
				// TODO: This should really check the Spec.StoragePolicyId instead.
				qs := obj.Items[i].Status.SCLevelQuotaStatuses[j]
				if qs.StorageClassName == name {
					return true, nil
				}
			}
		}
	} else {
		var obj corev1.ResourceQuotaList

		err := k8sClient.List(ctx, &obj, ctrlclient.InNamespace(namespace))
		if err != nil {
			return false, err
		}

		prefix := name + storageResourceQuotaStrPattern
		for i := range obj.Items {
			for name := range obj.Items[i].Spec.Hard {
				if strings.HasPrefix(name.String(), prefix) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func GetStoragePolicyUsage(ctx context.Context, client ctrlclient.Client, ns, vmclassName string) (*spqv1.StoragePolicyUsage, error) {
	storagePolicyUsage := &spqv1.StoragePolicyUsage{}

	key := types.NamespacedName{
		Namespace: ns,
		Name:      vmclassName,
	}

	err := client.Get(ctx, key, storagePolicyUsage)
	if err != nil {
		return nil, err
	}

	return storagePolicyUsage, nil
}

func IsFssEnabled(ctx context.Context, client ctrlclient.Client, deploymentNS, deploymentName, command, fss string) bool {
	envs, err := GetCommandEnvVars(ctx, client, deploymentNS, deploymentName, command)
	Expect(err).ToNot(HaveOccurred(), "%q FSS cannot not be fetched from deployment command name %q", fss, command)
	mmEnabled, _ := strconv.ParseBool(envs[fss])

	return mmEnabled
}

func GetVcPNID(ctx context.Context, svClusterClient ctrlclient.Client, vmopNamespace string) string {
	cm, err := GetConfigMap(ctx, svClusterClient, vmopNamespace, "vmware-system-vmop-config")
	Expect(err).ToNot(HaveOccurred())

	return cm.Data["VcPNID"]
}
