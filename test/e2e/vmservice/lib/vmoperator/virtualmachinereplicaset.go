// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
)

// WaitForVirtualMachineReplicaSetReplicas waits for both the status.replicas
// field and the actual number of owned VirtualMachine objects to reach
// expectedReplicas, so a test never observes a stale/aspirational count.
func WaitForVirtualMachineReplicaSetReplicas(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string,
	expectedReplicas int32) {
	Eventually(func(g Gomega) {
		rs, err := utils.GetVirtualMachineReplicaSet(ctx, client, ns, name)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rs.Status.Replicas).To(Equal(expectedReplicas))

		owned, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, client, ns, name)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(owned).To(HaveLen(int(expectedReplicas)))
	}, config.GetIntervals("default", "wait-virtual-machine-replicaset-status")...).Should(Succeed(),
		"Timed out waiting for VirtualMachineReplicaSet %s/%s to reach %d replicas", ns, name, expectedReplicas)
}

// WaitForVirtualMachineReplicaSetToBeDeleted waits for the
// VirtualMachineReplicaSet itself, and every VirtualMachine it owned, to be
// gone -- i.e. that owner-reference-driven cascading deletion completed.
func WaitForVirtualMachineReplicaSetToBeDeleted(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string) {
	Eventually(func(g Gomega) {
		_, err := utils.GetVirtualMachineReplicaSet(ctx, client, ns, name)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected VirtualMachineReplicaSet %s/%s to be deleted", ns, name)

		owned, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, client, ns, name)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(owned).To(BeEmpty(), "expected all VirtualMachines owned by %s/%s to be garbage collected", ns, name)
	}, config.GetIntervals("default", "wait-virtual-machine-replicaset-deletion")...).Should(Succeed(),
		"Timed out waiting for VirtualMachineReplicaSet %s/%s and its owned VirtualMachines to be deleted", ns, name)
}

// DeleteVirtualMachineReplicaSetAndWait deletes the VirtualMachineReplicaSet
// and waits for it, and its owned VirtualMachines, to be gone. It is a no-op
// if the object does not exist, so it can be registered unconditionally in
// cleanup (e.g. via DeferCleanup).
func DeleteVirtualMachineReplicaSetAndWait(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, name string) {
	rs, err := utils.GetVirtualMachineReplicaSet(ctx, client, ns, name)
	if apierrors.IsNotFound(err) {
		return
	}
	Expect(err).ToNot(HaveOccurred(), "failed to get VirtualMachineReplicaSet %s/%s", ns, name)
	Expect(client.Delete(ctx, rs)).To(Succeed(), fmt.Sprintf("failed to delete VirtualMachineReplicaSet %s/%s", ns, name))
	WaitForVirtualMachineReplicaSetToBeDeleted(ctx, config, client, ns, name)
}

// WaitForOwnedVirtualMachinesPoweredOn waits until every VirtualMachine
// currently owned by the named VirtualMachineReplicaSet reports
// status.powerState == PoweredOn.
func WaitForOwnedVirtualMachinesPoweredOn(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string) {
	Eventually(func(g Gomega) {
		owned, err := utils.ListVirtualMachinesOwnedByReplicaSet(ctx, client, ns, name)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(owned).ToNot(BeEmpty())

		for _, vm := range owned {
			g.Expect(vm.Status.PowerState).To(Equal(vmopv1.VirtualMachinePowerStateOn),
				"expected VirtualMachine %s/%s to be PoweredOn", vm.Namespace, vm.Name)
		}
	}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(Succeed(),
		"Timed out waiting for VirtualMachines owned by VirtualMachineReplicaSet %s/%s to power on", ns, name)
}
