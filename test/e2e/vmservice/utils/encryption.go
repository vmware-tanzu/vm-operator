// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

// EnsureEncryptionKeyProviders sets up encryption key providers for E2E tests.
// This is idempotent and can be run multiple times safely.
//
// For pipeline automation, this uses the proven vks-gce2e hack/kms.sh script which:
// 1. Native provider: govc kms.add -tmp=false -N gce2e-native && govc kms.export  
// 2. Standard provider: PyKMIP on gateway VM + govc kms.add + certificate trust
//
// Falls back to minimal native-only setup if the vks-gce2e script is not available.
func EnsureEncryptionKeyProviders(ctx context.Context, clusterProxy wcpframework.ClusterProxyInterface) {
	const (
		nativeKeyProviderID   = "gce2e-native"
		standardKeyProviderID = "gce2e-standard"
	)

	// Try automated setup first (for pipeline use)
	if tryAutomatedKMSSetup() {
		framework.Byf("Automated KMS setup completed successfully")
	} else {
		framework.Byf("Automated KMS setup not available, using fallback native-only setup")
		// Fallback to minimal native setup for compatibility
		ensureNativeProviderFallback(ctx, clusterProxy, nativeKeyProviderID)
	}

	// Get vCenter client for verification operations
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	cryptoManager := crypto.NewManagerKmip(vCenterClient)

	// Verify native provider is accessible
	verifyKeyProviderStatus(ctx, cryptoManager, nativeKeyProviderID, "Native")

	// Check for standard key provider availability
	if keyProviderExists(ctx, cryptoManager, standardKeyProviderID) {
		verifyKeyProviderStatus(ctx, cryptoManager, standardKeyProviderID, "Standard")
	} else {
		framework.Byf("Standard key provider %s not found - tests requiring it will be skipped", standardKeyProviderID)
		framework.Byf("To enable standard provider, run: hack/setup-kms.sh standard")
	}
}

// tryAutomatedKMSSetup attempts to run the vks-gce2e compatible KMS setup script
// Returns true if successful, false if the script is not available or fails
func tryAutomatedKMSSetup() bool {
	// Find the vm-operator root directory
	workingDir, err := os.Getwd()
	if err != nil {
		framework.Byf("Failed to get working directory: %v", err)
		return false
	}

	// Look for hack/kms.sh (vks-gce2e compatible script) relative to current directory or parent directories
	var scriptPath string
	for dir := workingDir; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "hack", "kms.sh")
		if _, err := os.Stat(candidate); err == nil {
			scriptPath = candidate
			break
		}
	}

	if scriptPath == "" {
		framework.Byf("KMS setup script not found at hack/kms.sh")
		return false
	}

	framework.Byf("Running vks-gce2e compatible KMS setup: %s", scriptPath)

	// Run the setup script with "install" command (sets up both providers)
	cmd := exec.Command(scriptPath, "install")
	cmd.Env = os.Environ() // Inherit GOVC_* and GATEWAY_VM_* environment variables

	output, err := cmd.CombinedOutput()
	if err != nil {
		framework.Byf("KMS setup script failed: %v\nOutput: %s", err, string(output))
		return false
	}

	framework.Byf("KMS setup completed:\n%s", string(output))
	return true
}

// ensureNativeProviderFallback provides minimal native provider setup when automation is not available
func ensureNativeProviderFallback(ctx context.Context, clusterProxy wcpframework.ClusterProxyInterface, providerID string) {
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	cryptoManager := crypto.NewManagerKmip(vCenterClient)

	framework.Byf("Ensuring native key provider %s exists (fallback method)", providerID)
	if !keyProviderExists(ctx, cryptoManager, providerID) {
		framework.Byf("Creating native key provider %s using SOAP API", providerID)
		Expect(cryptoManager.RegisterKmsCluster(ctx, types.KmipClusterInfo{
			ClusterId:      types.KeyProviderId{Id: providerID},
			ManagementType: string(types.KmipClusterInfoKmsManagementTypeNativeProvider),
			UseAsDefault:   types.NewBool(false),
		})).To(Succeed(), "failed to create native key provider %s", providerID)
		
		framework.Byf("WARNING: Native provider created but may need manual backup/export for encryption use")
		framework.Byf("Consider running: govc kms.export -f /dev/null %s", providerID)
	}
}

// verifyKeyProviderStatus verifies a key provider is accessible and logs its status
func verifyKeyProviderStatus(ctx context.Context, cryptoManager *crypto.ManagerKmip, providerID, providerType string) {
	framework.Byf("Verifying %s key provider %s is accessible", providerType, providerID)
	
	Eventually(func(g Gomega) {
		status, err := cryptoManager.GetClusterStatus(ctx, providerID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get status for key provider %s", providerID)
		
		// Native providers may show "red" until first use, which is acceptable
		// Standard providers should be "green" when properly configured
		expectedStatuses := []types.ManagedEntityStatus{types.ManagedEntityStatusGreen}
		if providerType == "Native" {
			expectedStatuses = append(expectedStatuses, types.ManagedEntityStatusRed)
		}
		
		g.Expect(status.OverallStatus).To(BeElementOf(expectedStatuses),
			"key provider %s has unexpected status: %s", providerID, status.OverallStatus)
	}, "1m", "5s").Should(Succeed(), "%s key provider %s is not accessible", providerType, providerID)

	// Log final status
	if status, err := cryptoManager.GetClusterStatus(ctx, providerID); err == nil {
		framework.Byf("%s key provider %s status: %s", providerType, providerID, status.OverallStatus)
	}
}

// keyProviderExists checks if a key provider with the given ID already exists
func keyProviderExists(ctx context.Context, cryptoManager *crypto.ManagerKmip, keyProviderID string) bool {
	_, err := cryptoManager.GetClusterStatus(ctx, keyProviderID)
	return err == nil
}

// IsStandardKeyProviderAvailable checks if the gce2e-standard key provider is available and ready.
// Tests requiring the standard provider should call this and skip if it returns false.
func IsStandardKeyProviderAvailable(ctx context.Context, clusterProxy wcpframework.ClusterProxyInterface) bool {
	const standardKeyProviderID = "gce2e-standard"
	
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	cryptoManager := crypto.NewManagerKmip(vCenterClient)
	
	if !keyProviderExists(ctx, cryptoManager, standardKeyProviderID) {
		return false
	}
	
	// Check if it's actually accessible (green status)
	status, err := cryptoManager.GetClusterStatus(ctx, standardKeyProviderID)
	if err != nil {
		return false
	}
	
	return status.OverallStatus == types.ManagedEntityStatusGreen
}