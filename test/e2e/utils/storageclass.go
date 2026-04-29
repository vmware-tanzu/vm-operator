// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"slices"

	"github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
)

// E2E encryption storage identifiers — shared by VMEncryptionSpec and VM hardware
// webhook coverage (VMSVC-3606). Matches vCenter testbed policy names.
const (
	E2EEncryptionStorageProfileName = "VM Service Encryption Policy"
	E2EEncryptionStorageClassName   = "vm-service-encryption-policy"
)

// EnsureE2EEncryptionStorageInNamespace ensures the standard VM Service encryption
// storage policy exists in vCenter, associates it with the given WCP namespace if
// needed, and waits until the encryption StorageClass is visible in that namespace.
func EnsureE2EEncryptionStorageInNamespace(
	ctx context.Context,
	vCenterClient *vim25.Client,
	wcpClient wcp.WorkloadManagementAPI,
	svClientSet kubernetes.Interface,
	svClusterClient ctrlclient.Client,
	cfg config.E2EConfig,
	namespace, baseStorageClassName string,
) error {
	storageClass, err := svClientSet.StorageV1().StorageClasses().
		Get(ctx, baseStorageClassName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get storage class %q: %w", baseStorageClassName, err)
	}

	basePolicyID := storageClass.Parameters["storagePolicyID"]
	if basePolicyID == "" {
		return fmt.Errorf("storage class %q has no storagePolicyID parameter",
			baseStorageClassName)
	}

	encryptionStoragePolicyID, err := vcenter.GetOrCreateEncryptionStoragePolicy(
		ctx, vCenterClient, E2EEncryptionStorageProfileName, basePolicyID)
	if err != nil {
		return fmt.Errorf("get or create encryption storage policy: %w", err)
	}

	if encryptionStoragePolicyID == "" {
		return fmt.Errorf("empty encryption storage policy ID")
	}

	e2eframework.Logf("Encryption storage policy ID: %s",
		encryptionStoragePolicyID)

	details, err := wcpClient.GetNamespace(namespace)
	if err != nil {
		return fmt.Errorf("get namespace %q: %w", namespace, err)
	}

	policyExists := slices.ContainsFunc(details.VMStorageSpec,
		func(spec wcp.StorageSpec) bool {
			return spec.Policy == encryptionStoragePolicyID
		},
	)

	if !policyExists {

		details.VMStorageSpec = append(details.VMStorageSpec,
			wcp.StorageSpec{Policy: encryptionStoragePolicyID})

		if err := wcpClient.SetNamespaceStorageSpecs(namespace,
			details.VMStorageSpec); err != nil {

			return fmt.Errorf("set storage specs for namespace %q: %w",
				namespace, err)
		}

		wcp.WaitForNamespaceReady(wcpClient, namespace)
		e2eframework.Logf("Encryption storage policy added to namespace %s",
			namespace)
	} else {
		e2eframework.Logf(
			"Encryption storage policy already exists in namespace %s", namespace)
	}

	podVMOnStretchedSupervisorEnabled := IsFssEnabled(ctx, svClusterClient,
		cfg.GetVariable("VMOPNamespace"),
		cfg.GetVariable("VMOPDeploymentName"),
		cfg.GetVariable("VMOPManagerCommand"),
		cfg.GetVariable("EnvFSSPodVMOnStretchedSupervisor"))

	EnsureStorageClassInNamespace(ctx, svClusterClient, namespace,
		E2EEncryptionStorageClassName, podVMOnStretchedSupervisorEnabled, cfg)

	return nil
}

func EnsureStorageClassInNamespace(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace,
	name string,
	podVMOnStretchedSupervisorEnabled bool,
	e2eConfig config.E2EConfig) {

	gomega.Eventually(func(g gomega.Gomega) {
		scInNS, err := IsStorageClassInNamespace(ctx, k8sClient, namespace,
			name, podVMOnStretchedSupervisorEnabled)
		g.Expect(err).NotTo(gomega.HaveOccurred(),
			"error checking if storage class %s in namespace %s: %v",
			name, namespace, err)
		g.Expect(scInNS).To(gomega.BeTrue(),
			"storage class %s not found in namespace %s", name, namespace)
	}, e2eConfig.GetIntervals("default", "wait-storage-class-ready")...).
		Should(gomega.Succeed(), "storage class %s not found in namespace %s",
			name, namespace)
}
