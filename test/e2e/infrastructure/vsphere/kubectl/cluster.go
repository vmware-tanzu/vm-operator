// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kubectl

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
)

const (
	getContextClusterNameJSONPath = `jsonpath={.contexts[?(@.name == "%s")].context.cluster}`
	getClusterAddressJSONPath     = `jsonpath={.clusters[?(@.name == "%s")].cluster.server}`
	getClusterCADataJSONPath      = `jsonpath={.clusters[?(@.name == "%s")].cluster.certificate-authority-data}`
)

// Get the current cluster from the current context
// kubectl config view -o jsonpath='{.contexts[?(@.name == "test-gc-e2e-demo-ns")].context.cluster}
// Return the address of the cluster associated with the current context.
func GetKubectlClusterForCurrentContext(ctx context.Context, kubeconfigPath string) string {
	stdout, err := framework.KubectlConfig(ctx, kubeconfigPath, "current-context")
	Expect(err).NotTo(HaveOccurred(), "get current context failed with kubeconfig %q", kubeconfigPath)

	currentContext := strings.TrimSpace(string(stdout))

	return GetKubectlClusterAPIServerFromContext(ctx, currentContext, kubeconfigPath)
}

// GetClusterAddressFromCluster returns the server address for the given context in a kubeconfig file.
func GetKubectlClusterAPIServerFromContext(ctx context.Context, contextName string, kubeconfigPath string) string {
	kubectlArgs := []string{"view", "-o", fmt.Sprintf(getContextClusterNameJSONPath, contextName)}
	stdout, err := framework.KubectlConfig(ctx, kubeconfigPath, kubectlArgs...)
	Expect(err).NotTo(HaveOccurred(), "get context cluster failed with kubeconfig %q", kubeconfigPath)

	currentCluster := strings.TrimSpace(string(stdout))

	return GetClusterAddressFromCluster(ctx, currentCluster, kubeconfigPath)
}

// GetClusterAddressFromCluster returns the server address for the given cluster in a kubeconfig file.
func GetClusterAddressFromCluster(ctx context.Context, clusterName string, kubeconfigPath string) string {
	kubectlArgs := []string{"view", "-o", fmt.Sprintf(getClusterAddressJSONPath, clusterName), "--kubeconfig", kubeconfigPath}
	stdout, err := framework.KubectlConfig(ctx, kubeconfigPath, kubectlArgs...)
	Expect(err).NotTo(HaveOccurred(), "get cluster server address failed with kubeconfig %q", kubeconfigPath)

	return strings.TrimSpace(string(stdout))
}

// GetClusterCertificateAuthorityDataFromCluster returns the CA data for the given context.
func GetClusterCertificateAuthorityDataFromContext(ctx context.Context, contextName string, kubeconfigPath string) string {
	kubectlArgs := []string{"view", "-o", fmt.Sprintf(getContextClusterNameJSONPath, contextName)}
	stdout, err := framework.KubectlConfig(ctx, kubeconfigPath, kubectlArgs...)
	Expect(err).NotTo(HaveOccurred(), "get cluster context with kubeconfig %q", kubeconfigPath)

	clusterName := strings.TrimSpace(string(stdout))

	return GetClusterCertificateAuthorityDataFromCluster(ctx, clusterName, kubeconfigPath)
}

// GetClusterCertificateAuthorityDataFromCluster returns the CA data for the given cluster.
func GetClusterCertificateAuthorityDataFromCluster(ctx context.Context, clusterName string, kubeconfigPath string) string {
	kubectlArgs := []string{"view", "-o", fmt.Sprintf(getClusterCADataJSONPath, clusterName)}
	stdout, err := framework.KubectlConfig(ctx, kubeconfigPath, kubectlArgs...)
	Expect(err).NotTo(HaveOccurred(), "get CA failed with kubeconfig %q", kubeconfigPath)

	return strings.TrimSpace(string(stdout))
}
