// Copyright (c) 2024-2025 Broadcom. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	clientset "k8s.io/client-go/kubernetes"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
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
	CapabilityGRV = schema.GroupVersionResource{
		Group:    "iaas.vmware.com",
		Version:  "v1alpha1",
		Resource: "capabilities",
	}

	clientAuthToWebhookEnabled     bool
	clientAuthToWebhookEnabledOnce sync.Once
)

func DoesSupervisorCapabilityExist(ctx context.Context, client clientset.Interface, dynamicClient dynamic.Interface, capabilityKey string, asyncSupervisorFSSEnabled bool) bool {
	if !asyncSupervisorFSSEnabled {
		_, exists := getWcpClusterCapabilityFromCM(ctx, client, capabilityKey)
		return exists
	}

	_, exists := getSupervisorCapabilityFromCR(ctx, dynamicClient, capabilityKey)

	return exists
}

// IsSupervisorCapabilityEnabled returns whether the given capability is enabled on the Supervisor.
// A false will be returned if the given capability doesn't exist.
func IsSupervisorCapabilityEnabled(ctx context.Context, client clientset.Interface, dynamicClient dynamic.Interface, capabilityKey string, asyncSupervisorFSSEnabled bool) bool {
	if !asyncSupervisorFSSEnabled {
		enabled, _ := getWcpClusterCapabilityFromCM(ctx, client, capabilityKey)
		return enabled
	}

	enabled, _ := getSupervisorCapabilityFromCR(ctx, dynamicClient, capabilityKey)

	return enabled
}

// EnableSupervisorCapability enables the capability on the given Supervisor *if the capability exists*.
func EnableSupervisorCapability(ctx context.Context, client clientset.Interface, dynamicClient dynamic.Interface, svSSHCommandRunner e2essh.SSHCommandRunner, capability string, asyncSupervisorFSSEnabled bool) {
	setSupervisorCapability(ctx, client, dynamicClient, svSSHCommandRunner, asyncSupervisorFSSEnabled, capability, true)
}

// DisableSupervisorCapability disables the capability on the given Supervisor *if the capability exists*.
func DisableSupervisorCapability(ctx context.Context, client clientset.Interface, dynamicClient dynamic.Interface, svSSHCommandRunner e2essh.SSHCommandRunner, capability string, asyncSupervisorFSSEnabled bool) {
	setSupervisorCapability(ctx, client, dynamicClient, svSSHCommandRunner, asyncSupervisorFSSEnabled, capability, false)
}

func IsWcpClusterCapabilityEnabled(ctx context.Context, client clientset.Interface, capability string) bool {
	enabled, _ := getWcpClusterCapabilityFromCM(ctx, client, capability)
	return enabled
}

func IsClientAuthToWebhookEnabled(ctx context.Context, client clientset.Interface) bool {
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
func getWcpClusterCapabilityFromCM(ctx context.Context, client clientset.Interface, capabilityKey string) (bool, bool) {
	cm := getWcpClusterCapabilitiesCM(ctx, client)

	val, ok := cm.Data[capabilityKey]
	if !ok {
		return false, false
	}

	return val == "true", true
}

// return whether the capability enabled, whether the capability exists.
func getSupervisorCapabilityFromCR(ctx context.Context, dynamicClient dynamic.Interface, capabilityKey string) (bool, bool) {
	cr := getSupervisorCapabilitiesCR(ctx, dynamicClient)

	status, found, err := unstructured.NestedMap(cr.Object, "status", "supervisor", capabilityKey)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error extracting nested field: %v", err))

	if !found {
		return false, false
	}

	return status["activated"].(bool), true
}

func setSupervisorCapability(ctx context.Context, client clientset.Interface, dynamicClient dynamic.Interface, svSSHCommandRunner e2essh.SSHCommandRunner, asyncSupervisorFSSEnabled bool, capability string, enable bool) {
	if !asyncSupervisorFSSEnabled {
		setSupervisorCapabilityFromCM(ctx, client, capability, enable)
	}

	setSupervisorCapabilityFromCR(ctx, dynamicClient, svSSHCommandRunner, capability, enable)
}

func setSupervisorCapabilityFromCM(ctx context.Context, client clientset.Interface, capability string, enable bool) {
	cm := getWcpClusterCapabilitiesCM(ctx, client)
	if _, ok := cm.Data[capability]; !ok {
		e2eframework.Logf("Capability %s doesn't exist, skip setting it to %t", capability, enable)
		return
	}

	cm.Data[capability] = strconv.FormatBool(enable)
	updateWcpClusterCapabilitiesCM(ctx, client, cm)
}

func setSupervisorCapabilityFromCR(ctx context.Context, client dynamic.Interface, svSSHCommandRunner e2essh.SSHCommandRunner, capabilityKey string, enable bool) {
	cr := getSupervisorCapabilitiesCR(ctx, client)
	crMap := cr.UnstructuredContent()

	supervisorCapList := crMap["spec"].(map[string]any)["supervisor"].([]any)
	capabilityExists := false

	for _, item := range supervisorCapList {
		itemMap := item.(map[string]any)
		if itemMap["name"] == capabilityKey {
			itemMap["enabled"] = enable
			capabilityExists = true

			break
		}
	}

	if !capabilityExists {
		e2eframework.Logf("Capability %s doesn't exist, skip setting it to %t", capabilityKey, enable)
		return
	}

	cr.SetUnstructuredContent(crMap)
	updateSupervisorCapabilitiesCR(svSSHCommandRunner, cr)
}

func getWcpClusterCapabilitiesCM(ctx context.Context, client clientset.Interface) *corev1.ConfigMap {
	cm, err := client.CoreV1().ConfigMaps(capabilityConfigMapNs).Get(ctx, capabilityConfigMapName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error getting ConfigMap(%s/%s): %v", capabilityConfigMapNs, capabilitiesCRName, err))

	return cm
}

func updateWcpClusterCapabilitiesCM(ctx context.Context, client clientset.Interface, cm *corev1.ConfigMap) {
	_, err := client.CoreV1().ConfigMaps(capabilityConfigMapNs).Update(ctx, cm, metav1.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error patching ConfigMap(%s/%s): %v", capabilityConfigMapNs, capabilitiesCRName, err))
}

func getSupervisorCapabilitiesCR(ctx context.Context, client dynamic.Interface) *unstructured.Unstructured {
	cr, err := client.Resource(CapabilityGRV).Get(ctx, capabilitiesCRName, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error getting Capabilities %s: %v", capabilitiesCRName, err))

	return cr
}

// User "sso:Administrator@vsphere.local" cannot update resource "capabilities" in API group "iaas.vmware.com" at the cluster scope
// Patching the Capability as root user.
func updateSupervisorCapabilitiesCR(svSSHCommandRunner e2essh.SSHCommandRunner, cr *unstructured.Unstructured) {
	yamlData, err := yaml.Marshal(cr)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("error marshaling YAML: %v", err))

	kubeCtlExecCmd := fmt.Sprintf("cat <<EOF | kubectl apply -f -\n%s\nEOF", yamlData)
	output, err := svSSHCommandRunner.RunCommand(kubeCtlExecCmd)
	Expect(err).To(BeNil(), string(output))
}
