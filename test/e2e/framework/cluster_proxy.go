/*
Copyright 2020 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

const (
	maxRetries      = 5
	initialDuration = 1 * time.Minute
	retryFactor     = 2
	jitterFactor    = 0.1
)

// https://github.com/kubernetes-sigs/cluster-api/blob/master/test/framework/cluster_proxy.go
// ClusterProxy defines the behavior of a type that acts as an intermediary with an existing Kubernetes cluster.
// It should work with any Kubernetes cluster, no matter if the Cluster was created by a bootstrap.ClusterProvider,
// by Cluster API (a workload cluster or a self-hosted cluster) or else.

type ClusterProxyInterface interface {
	// GetName returns the name of the cluster.
	GetName() string

	// GetKubeconfigPath returns the path to the kubeconfig file to be used to access the Kubernetes cluster.
	GetKubeconfigPath() string

	// GetScheme returns the scheme defining the types hosted in the Kubernetes cluster.
	// It is used when creating a controller-runtime client.
	GetScheme() *runtime.Scheme

	// GetClient returns a controller-runtime client to the Kubernetes cluster.
	GetClient() client.Client

	// GetDynamicClient returns a client-go dynamic client for the cluster.
	GetDynamicClient() dynamic.Interface

	// GetClientSet returns a client-go client to the Kubernetes cluster.
	GetClientSet() *kubernetes.Clientset

	// GetRESTConfig returns the REST config for direct use with client-go if needed.
	GetRESTConfig() *rest.Config

	// Apply applies YAML to the Kubernetes cluster, `kubectl apply`.
	// Optional noRetryPatterns: if provided, doesn't retry on errors matching these patterns.
	Apply(context.Context, []byte, ...string) error

	// Delete to delete YAML to the Kubernetes cluster, `kubectl delete`.
	Delete(context.Context, []byte) error

	// GetWorkloadCluster returns a workload cluster proxy interface provisioned by the management cluster
	GetWorkloadCluster(ctx context.Context, namespace, name string) WorkloadClusterProxy

	// Dispose proxy's internal resources (the operation does not affects the Kubernetes cluster).
	// This should be implemented as a synchronous function.
	Dispose(context.Context)
}

// clusterProxy provides a base implementation of the ClusterProxy interface.
type clusterProxy struct {
	name                    string
	kubeconfigPath          string
	shouldCleanupKubeconfig bool
	scheme                  *runtime.Scheme
	// TODO:
	//   for collecting logs in namespace of a cluster
	// logCollector            ClusterLogCollector
}

// NewClusterProxy returns a clusterProxy given a KubeconfigPath and the scheme defining the types hosted in the cluster.
// If a kubeconfig file isn't provided, standard kubeconfig locations will be used (kubectl loading rules apply).
func NewClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme) *clusterProxy {
	Expect(scheme).NotTo(BeNil(), "scheme is required for NewClusterProxy")

	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	proxy := &clusterProxy{
		name:                    name,
		kubeconfigPath:          kubeconfigPath,
		scheme:                  scheme,
		shouldCleanupKubeconfig: false,
	}

	return proxy
}

// GetName returns the name of the cluster.
func (p *clusterProxy) GetName() string {
	return p.name
}

// GetKubeconfigPath returns the path to the kubeconfig file for the cluster.
func (p *clusterProxy) GetKubeconfigPath() string {
	return p.kubeconfigPath
}

// GetScheme returns the scheme defining the types hosted in the cluster.
func (p *clusterProxy) GetScheme() *runtime.Scheme {
	return p.scheme
}

// GetClient returns a controller-runtime client for the cluster.
func (p *clusterProxy) GetClient() client.Client {
	config := p.GetRESTConfig()

	var c client.Client

	Eventually(func() client.Client {
		var err error

		c, err = client.New(config, client.Options{Scheme: p.scheme})
		if err != nil {
			fmt.Printf("error getting controller-runtime client, error : %s\n", err.Error())
			return nil
		}

		return c
	}, 5*time.Minute, 10*time.Second).ShouldNot(BeNil(), "Failed to get controller-runtime client")

	return c
}

// GetClientSet returns a client-go client for the cluster.
func (p *clusterProxy) GetClientSet() *kubernetes.Clientset {
	restConfig := p.GetRESTConfig()

	cs, err := kubernetes.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred(), "Failed to get client-go client")

	return cs
}

// Apply wraps `kubectl apply` and prints the output so we can see what gets applied to the cluster.
// Optional noRetryPatterns: if provided, doesn't retry on errors matching these patterns.
func (p *clusterProxy) Apply(ctx context.Context, resources []byte, noRetryPatterns ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeNil(), "resources is required for Apply")

	backoff := wait.Backoff{
		Steps:    maxRetries,
		Duration: initialDuration,
		Factor:   retryFactor,
		Jitter:   jitterFactor,
	}

	return wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		err := KubectlApply(ctx, p.kubeconfigPath, resources)
		if err != nil {
			e2eframework.Logf("KubectlApply error: '%s'", err.Error())

			// Check if error matches any no-retry patterns
			errMsg := strings.ToLower(err.Error())
			for _, pattern := range noRetryPatterns {
				if strings.Contains(errMsg, strings.ToLower(pattern)) {
					e2eframework.Logf("No-retry pattern '%s' matched, failing immediately without retry", pattern)
					return false, err // Return error immediately, don't retry
				}
			}

			return false, nil // Retry on other errors
		}

		return true, nil // Success, stop retrying
	})
}

// ApplyWithArgs wraps `kubectl apply ...` and prints the output so we can see what gets applied to the cluster.
func (p *clusterProxy) ApplyWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeNil(), "resources is required for Apply")

	return KubectlApplyWithArgs(ctx, p.kubeconfigPath, resources, args...)
}

// Delete wraps `kubectl delete` and prints the output so we can see what gets deleted.
func (p *clusterProxy) Delete(ctx context.Context, resources []byte) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Delete")
	Expect(resources).NotTo(BeNil(), "resources is required for Delete")

	return KubectlDelete(ctx, p.kubeconfigPath, resources)
}

// GetRESTConfig retrieves RESTConfig from kubeconfig and if not provided, from default's client config.
func (p *clusterProxy) GetRESTConfig() *rest.Config {
	config, err := clientcmd.LoadFromFile(p.kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to load Kubeconfig file from %q", p.kubeconfigPath)

	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to get ClientConfig from %q", p.kubeconfigPath)

	restConfig.UserAgent = "e2e"

	return restConfig
}

// GetDynamicClient returns a client-go dynamic client for the cluster.
func (p *clusterProxy) GetDynamicClient() dynamic.Interface {
	restConfig := p.GetRESTConfig()

	ds, err := dynamic.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred(), "Failed to get client-go dynamic client")

	return ds
}

// newFromAPIConfig returns a clusterProxy given a api.Config and the scheme defining the types hosted in the cluster.
func newFromAPIConfig(name string, config *api.Config, scheme *runtime.Scheme) WorkloadClusterProxy {
	// NB. the ClusterProvider is responsible for the cleanup of this file
	f, err := os.CreateTemp("", "e2e-kubeconfig")
	Expect(err).ToNot(HaveOccurred(), "Failed to create kubeconfig file for the kind cluster %q")

	kubeconfigPath := f.Name()

	err = clientcmd.WriteToFile(*config, kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to write kubeconfig for the kind cluster to a file %q")

	return newWorkloadClusterProxy(name, kubeconfigPath, scheme)
}

// getKubeconfig retrieves kubeconfig values from the secret of a workload cluster.
func (p *clusterProxy) getKubeconfig(ctx context.Context, namespace string, name string) *api.Config {
	cl := p.GetClient()

	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      fmt.Sprintf("%s-kubeconfig", name),
		Namespace: namespace,
	}
	Expect(cl.Get(ctx, key, secret)).To(Succeed(), "Failed to get %s", key)
	Expect(secret.Data).To(HaveKey("value"), "Invalid secret %s", key)

	config, err := clientcmd.Load(secret.Data["value"])
	Expect(err).ToNot(HaveOccurred(), "Failed to convert %s into a kubeconfig file", key)

	return config
}

// GetWorkloadCluster returns ClusterProxy for the workload cluster.
func (p *clusterProxy) GetWorkloadCluster(ctx context.Context, namespace, name string) WorkloadClusterProxy {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetWorkloadCluster")
	Expect(namespace).NotTo(BeEmpty(), "namespace is required for GetWorkloadCluster")
	Expect(name).NotTo(BeEmpty(), "name is required for GetWorkloadCluster")

	// gets the kubeconfig from the cluster
	config := p.getKubeconfig(ctx, namespace, name)

	return newFromAPIConfig(name, config, p.GetScheme())
}

// CollectWorkloadClusterLogs collects machines logs from the workload cluster.
func (p *clusterProxy) CollectWorkloadClusterLogs(ctx context.Context, namespace, name, outputPath string) {
}

// Dispose clean up the kubeconfig path in os environment.
func (p *clusterProxy) Dispose(ctx context.Context) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Dispose")

	if p.shouldCleanupKubeconfig {
		err := os.Remove(p.kubeconfigPath)
		if err != nil {
			fmt.Printf("deleting the kubeconfig file %q file. You may need to remove this by hand.\n", p.kubeconfigPath)
		}
	}
}

// TODO: Shrink the behavior of workload cluster proxy interface after the new E2E tests
//
//	has been re-written
type WorkloadClusterProxy interface {
	ClusterProxyInterface
	Get(ctx context.Context, args ...string) ([]byte, error)
	GetClientNoExpect(timeoutMins int) (client.Client, error)
	ApplyWithArgs(ctx context.Context, resources []byte, args ...string) error
	Create(ctx context.Context, resources []byte) error
	CreateWithArgs(ctx context.Context, resources []byte, args ...string) error
	Delete(ctx context.Context, resources []byte) error
	DeleteWithArgs(ctx context.Context, resources []byte, args ...string) error
	DeleteWithNamespacedName(ctx context.Context, resource, ns, name string) error
	Exec(ctx context.Context, args ...string) ([]byte, error)
	Config(ctx context.Context, args ...string) ([]byte, error)
}

type workloadClusterProxy struct {
	*clusterProxy
}

// NewClusterProxy returns a workloadClusterProxy given a KubeconfigPath and the scheme defining the types hosted in the cluster.
// If a kubeconfig file isn't provided, standard kubeconfig locations will be used (kubectl loading rules apply).
func newWorkloadClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme) *workloadClusterProxy {
	Expect(scheme).NotTo(BeNil(), "scheme is required for NewWorkloadClusterProxy")

	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	proxy := &clusterProxy{
		name:                    name,
		kubeconfigPath:          kubeconfigPath,
		scheme:                  scheme,
		shouldCleanupKubeconfig: false,
	}

	return &workloadClusterProxy{
		clusterProxy: proxy,
	}
}

// Get retrieves the status of a resource, `kubectl get`.
func (w *workloadClusterProxy) Get(ctx context.Context, args ...string) ([]byte, error) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Get")

	return KubectlGet(ctx, w.GetKubeconfigPath(), args...)
}

// GetClientNoExpect returns a controller-runtime client for the cluster.
// This function was made for retrying to work (when a client takes too long to load).
func (w *workloadClusterProxy) GetClientNoExpect(timeoutMins int) (client.Client, error) {
	var (
		err error
		c   client.Client
	)

	config := w.GetRESTConfig()

	c, err = client.New(config, client.Options{Scheme: w.scheme})

	return c, err
}

// Apply wraps `kubectl apply ...` and prints the output so we can see what gets applied to the cluster.
func (w *workloadClusterProxy) ApplyWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Apply")
	Expect(resources).NotTo(BeNil(), "resources is required for Apply")

	return KubectlApplyWithArgs(ctx, w.GetKubeconfigPath(), resources, args...)
}

// Create wraps `kubectl create` and prints the output so we can see what gets applied to the cluster.
func (w *workloadClusterProxy) Create(ctx context.Context, resources []byte) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Create")
	Expect(resources).NotTo(BeNil(), "resources is required for Create")

	return KubectlCreate(ctx, w.GetKubeconfigPath(), resources)
}

// Create wraps `kubectl create ...` and prints the output so we can see what gets applied to the cluster.
func (w *workloadClusterProxy) CreateWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Create")
	Expect(resources).NotTo(BeNil(), "resources is required for Create")

	return KubectlCreateWithArgs(ctx, w.GetKubeconfigPath(), resources, args...)
}

// Delete wraps `kubectl delete` and prints the output so we can see what gets deleted.
func (w *workloadClusterProxy) Delete(ctx context.Context, resources []byte) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Delete")
	Expect(resources).NotTo(BeNil(), "resources is required for Delete")

	return KubectlDelete(ctx, w.GetKubeconfigPath(), resources)
}

// DeleteWithArgs wraps `kubectl delete ...` and prints the output so we can see what gets deleted.
func (w *workloadClusterProxy) DeleteWithArgs(ctx context.Context, resources []byte, args ...string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Delete")
	Expect(resources).NotTo(BeNil(), "resources is required for Delete")

	return KubectlDeleteWithArgs(ctx, w.GetKubeconfigPath(), resources, args...)
}

// DeleteWithNamespacedName delete a manifestbuilders from the cluster by its namespace and name.
func (w *workloadClusterProxy) DeleteWithNamespacedName(ctx context.Context, resource, ns, name string) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Delete")

	return KubectlDeleteWithNamespacedName(ctx, w.GetKubeconfigPath(), resource, ns, name)
}

// Exec performs kubectl exec with following flags.
func (w *workloadClusterProxy) Exec(ctx context.Context, args ...string) ([]byte, error) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Exec")

	return KubectlExec(ctx, w.GetKubeconfigPath(), args...)
}

// Config performs kubectl exec with following flags.
func (w *workloadClusterProxy) Config(ctx context.Context, args ...string) ([]byte, error) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Config")

	return KubectlConfig(ctx, w.GetKubeconfigPath(), args...)
}
