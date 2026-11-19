// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package framework_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
)

func TestClusterProxyUsesKubectlCredentials(t *testing.T) {
	g := NewWithT(t)
	RegisterTestingT(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "tenant.kubeconfig")
	config := clientcmdapi.Config{
		CurrentContext: "tenant",
		Clusters: map[string]*clientcmdapi.Cluster{
			"supervisor": {Server: "https://supervisor.example.test", InsecureSkipTLSVerify: true},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"tenant": {Cluster: "supervisor", AuthInfo: "tenant", Namespace: "tenant-ns"},
			"admin":  {Cluster: "supervisor", AuthInfo: "admin"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"tenant": {Token: "tenant-token"},
			"admin":  {Token: "admin-token"},
		},
	}
	g.Expect(clientcmd.WriteToFile(config, path)).To(Succeed())

	// Ambient configuration must not override the proxy's explicit kubeconfig.
	ambientPath := filepath.Join(dir, "ambient.kubeconfig")
	config.CurrentContext = "admin"
	g.Expect(clientcmd.WriteToFile(config, ambientPath)).To(Succeed())
	t.Setenv("KUBECONFIG", ambientPath)

	proxy := framework.NewClusterProxy("supervisor", path, runtime.NewScheme())
	for _, auth := range []struct {
		name string
		info *clientcmdapi.AuthInfo
	}{
		{name: "token", info: &clientcmdapi.AuthInfo{Token: "tenant-token"}},
		{name: "certificate", info: &clientcmdapi.AuthInfo{ClientCertificate: "tenant.crt", ClientKey: "tenant.key"}},
		{name: "exec", info: &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1", Command: "tenant-login",
			InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
		}}},
		{name: "impersonation", info: &clientcmdapi.AuthInfo{
			Token: "tenant-token", Impersonate: "tenant-user", ImpersonateGroups: []string{"tenant-group"},
		}},
	} {
		t.Run(auth.name, func(t *testing.T) {
			g := NewWithT(t)
			RegisterTestingT(t)
			config.CurrentContext = "tenant"
			config.AuthInfos["tenant"] = auth.info
			g.Expect(os.WriteFile(filepath.Join(dir, "tenant.crt"), []byte("certificate"), 0600)).To(Succeed())
			g.Expect(os.WriteFile(filepath.Join(dir, "tenant.key"), []byte("key"), 0600)).To(Succeed())
			g.Expect(clientcmd.WriteToFile(config, path)).To(Succeed())

			// Match kubectl's loading rules with --kubeconfig and no context or
			// user override. Compare TLS, exec, and impersonation fields too.
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			loadingRules.ExplicitPath = path
			expected, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				loadingRules, &clientcmd.ConfigOverrides{}).ClientConfig()
			g.Expect(err).NotTo(HaveOccurred())
			expected.UserAgent = "e2e"
			g.Expect(proxy.GetRESTConfig()).To(Equal(expected))
			g.Expect(proxy.GetKubeconfigPath()).To(Equal(path))
		})
	}
}
