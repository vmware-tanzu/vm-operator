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
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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

	// GetRESTConfig returns the REST config for direct use with client-go if needed.
	GetRESTConfig() *rest.Config

	// Apply applies YAML to the Kubernetes cluster, `kubectl apply`.
	// Optional noRetryPatterns: if provided, doesn't retry on errors matching these patterns.
	Apply(context.Context, []byte, ...string) error

	// Delete to delete YAML to the Kubernetes cluster, `kubectl delete`.
	Delete(context.Context, []byte) error

	// Dispose proxy's internal resources (the operation does not affect the Kubernetes cluster).
	// This should be implemented as a synchronous function.
	Dispose(context.Context)
}

// clusterProxy provides a base implementation of the ClusterProxy interface.
type clusterProxy struct {
	name           string
	kubeconfigPath string
	scheme         *runtime.Scheme
}

// NewClusterProxy returns a clusterProxy given a KubeconfigPath and the scheme defining the types hosted in the cluster.
// If a kubeconfig file isn't provided, standard kubeconfig locations will be used (kubectl loading rules apply).
func NewClusterProxy(name string, kubeconfigPath string, scheme *runtime.Scheme) *clusterProxy {
	Expect(scheme).NotTo(BeNil(), "scheme is required for NewClusterProxy")

	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	proxy := &clusterProxy{
		name:           name,
		kubeconfigPath: kubeconfigPath,
		scheme:         scheme,
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

// GetClient returns a controller-runtime client for the cluster. The
// returned client retries Get, Create, Update, Patch, Delete, and
// DeleteAllOf on transient connectivity errors -- e.g. the conversion
// webhook being briefly unreachable during a vm-operator pod rollout --
// instead of failing an entire e2e spec on a single blip. See
// NewRetryableClient.
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

	return NewRetryableClient(c)
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

// Delete wraps `kubectl delete` and prints the output so we can see what gets deleted.
func (p *clusterProxy) Delete(ctx context.Context, resources []byte) error {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Delete")
	Expect(resources).NotTo(BeNil(), "resources is required for Delete")

	return KubectlDelete(ctx, p.kubeconfigPath, resources)
}

// GetRESTConfig loads the proxy's kubeconfig using the same explicit-path loading
// rules as kubectl --kubeconfig, including relative certificate and key paths.
// No context or user override is applied.
func (p *clusterProxy) GetRESTConfig() *rest.Config {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = p.kubeconfigPath
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{}).ClientConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to get ClientConfig from %q", p.kubeconfigPath)

	restConfig.UserAgent = "e2e"

	return restConfig
}

// Dispose releases proxy resources.
func (p *clusterProxy) Dispose(ctx context.Context) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for Dispose")
}
