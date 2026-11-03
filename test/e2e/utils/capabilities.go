// Copyright (c) 2024-2025 Broadcom. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
)

const (
	UserPodsOnVDS = "User_Pods_On_VDS_Supported"

	SupervisorAsyncUpgradeFSS = "WCP_Supervisor_Async_Upgrade"
	SupervisorVMSnapshotFSS   = "WCP_VMService_VM_Snapshots"

	APIServerToWebhookAuth = "supports_apiserver_to_webhook_authentication"

	capabilityConfigMapName = "wcp-cluster-capabilities"
	capabilityConfigMapNs   = "kube-system"

	capabilitiesCRName  = "supervisor-capabilities"
	capabilitiesCRDName = "capabilities.iaas.vmware.com"
)

var (
	clientAuthToWebhookEnabled     bool
	clientAuthToWebhookEnabledOnce sync.Once
)

func DoesSupervisorCapabilityExist(ctx context.Context, client ctrlclient.Client, capabilityKey string, asyncSupervisorFSSEnabled bool) bool {
	if !asyncSupervisorFSSEnabled {
		_, exists := getWcpClusterCapabilityFromCM(ctx, client, capabilityKey)
		return exists
	}

	_, exists := getSupervisorCapabilityFromCR(ctx, client, capabilityKey)

	return exists
}

// IsSupervisorCapabilityEnabled returns whether the given capability is enabled on the Supervisor.
// A false will be returned if the given capability doesn't exist.
func IsSupervisorCapabilityEnabled(ctx context.Context, client ctrlclient.Client, capabilityKey string, asyncSupervisorFSSEnabled bool) bool {
	if !asyncSupervisorFSSEnabled {
		enabled, _ := getWcpClusterCapabilityFromCM(ctx, client, capabilityKey)
		return enabled
	}

	enabled, _ := getSupervisorCapabilityFromCR(ctx, client, capabilityKey)

	return enabled
}

func IsWcpClusterCapabilityEnabled(ctx context.Context, client ctrlclient.Client, capability string) bool {
	enabled, _ := getWcpClusterCapabilityFromCM(ctx, client, capability)
	return enabled
}

func IsClientAuthToWebhookEnabled(ctx context.Context, client ctrlclient.Client) bool {
	clientAuthToWebhookEnabledOnce.Do(func() {
		clientAuthToWebhookEnabled = IsWcpClusterCapabilityEnabled(ctx, client, APIServerToWebhookAuth)
	})

	return clientAuthToWebhookEnabled
}

// CheckSupervisorCapabilitiesCRDSupport is used to check if the supervisor capability CRD exists.
func CheckSupervisorCapabilitiesCRDSupport(ctx context.Context, client ctrlclient.Client) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	crd.SetName(capabilitiesCRDName)

	err := client.Get(ctx, ctrlclient.ObjectKey{Name: capabilitiesCRDName}, crd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// return whether the capability enabled, whether the capability exists.
func getWcpClusterCapabilityFromCM(ctx context.Context, client ctrlclient.Client, capabilityKey string) (bool, bool) {
	cm := getWcpClusterCapabilitiesCM(ctx, client)

	val, ok := cm.Data[capabilityKey]
	if !ok {
		return false, false
	}

	return val == "true", true
}

// return whether the capability enabled, whether the capability exists.
func getSupervisorCapabilityFromCR(ctx context.Context, client ctrlclient.Client, capabilityKey string) (bool, bool) {
	cr := getSupervisorCapabilitiesCR(ctx, client)

	status, found := cr.Status.Supervisor[capv1.CapabilityName(capabilityKey)]
	if !found {
		return false, false
	}

	return status.Activated, true
}

func getWcpClusterCapabilitiesCM(ctx context.Context, client ctrlclient.Client) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, ctrlclient.ObjectKey{Namespace: capabilityConfigMapNs, Name: capabilityConfigMapName}, cm)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error getting ConfigMap(%s/%s): %v", capabilityConfigMapNs, capabilitiesCRName, err))

	return cm
}

func getSupervisorCapabilitiesCR(ctx context.Context, client ctrlclient.Client) *capv1.Capabilities {
	cr := &capv1.Capabilities{}
	err := client.Get(ctx, ctrlclient.ObjectKey{Name: capabilitiesCRName}, cr)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error getting Capabilities %s: %v", capabilitiesCRName, err))

	return cr
}
