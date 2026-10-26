// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

func TestAdminClientCreatesWithProxyCredentials(t *testing.T) {
	g := NewWithT(t)
	RegisterTestingT(t)

	// The self-signed server also verifies the TLS behavior of admin kubectl
	// calls, which use --insecure-skip-tls-verify with the admin kubeconfig.
	requests := make(chan string, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			_ = json.NewEncoder(w).Encode(&metav1.APIVersions{Versions: []string{"v1"}})
		case "/apis":
			version := metav1.GroupVersionForDiscovery{GroupVersion: byokv1.GroupVersion.String(), Version: "v1alpha1"}
			_ = json.NewEncoder(w).Encode(&metav1.APIGroupList{Groups: []metav1.APIGroup{{
				Name: byokv1.GroupVersion.Group, Versions: []metav1.GroupVersionForDiscovery{version}, PreferredVersion: version,
			}}})
		case "/apis/encryption.vmware.com/v1alpha1":
			_ = json.NewEncoder(w).Encode(&metav1.APIResourceList{
				GroupVersion: byokv1.GroupVersion.String(),
				APIResources: []metav1.APIResource{{
					Name: "encryptionclasses", Kind: "EncryptionClass", Namespaced: true,
					Verbs: metav1.Verbs{"get", "create"},
				}},
			})
		case "/apis/encryption.vmware.com/v1alpha1/namespaces/test-ns/encryptionclasses":
			if r.Method != http.MethodPost {
				http.Error(w, "expected create", http.StatusMethodNotAllowed)
				return
			}
			requests <- r.Header.Get("Authorization")
			var class byokv1.EncryptionClass
			if err := json.NewDecoder(r.Body).Decode(&class); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(&class)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	config := clientcmdapi.Config{
		CurrentContext: "selected-admin",
		Clusters: map[string]*clientcmdapi.Cluster{
			"supervisor": {Server: server.URL},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"selected-admin": {Cluster: "supervisor", AuthInfo: "admin"},
			"tenant":         {Cluster: "supervisor", AuthInfo: "tenant"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"admin":  {Token: "proxy-admin-token"},
			"tenant": {Token: "ambient-tenant-token"},
		},
	}
	path := filepath.Join(dir, "admin.kubeconfig")
	g.Expect(clientcmd.WriteToFile(config, path)).To(Succeed())
	config.CurrentContext = "tenant"
	ambientPath := filepath.Join(dir, "ambient.kubeconfig")
	g.Expect(clientcmd.WriteToFile(config, ambientPath)).To(Succeed())
	t.Setenv("KUBECONFIG", ambientPath)

	scheme := runtime.NewScheme()
	g.Expect(byokv1.AddToScheme(scheme)).To(Succeed())
	proxy := &common.VMServiceClusterProxy{
		WCPClusterProxy: &wcpframework.WCPClusterProxy{
			ClusterProxyInterface: framework.NewClusterProxy("admin", path, scheme),
		},
	}
	client, err := proxy.GetAdminClient()
	g.Expect(err).NotTo(HaveOccurred())
	class := &byokv1.EncryptionClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "test-ns"},
		Spec:       byokv1.EncryptionClassSpec{KeyProvider: "provider", KeyID: "key"},
	}
	g.Expect(client.Create(context.Background(), class)).To(Succeed())
	g.Expect(requests).To(Receive(Equal("Bearer proxy-admin-token")))
}
