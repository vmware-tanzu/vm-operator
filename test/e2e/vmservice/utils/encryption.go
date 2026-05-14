// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/crypto"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	nativeKeyProviderID   = "gce2e-native"
	standardKeyProviderID = "gce2e-standard"
)

// EnsureEncryptionKeyProviders verifies that the KMS key providers expected by
// the encryption tests are accessible. The providers are set up by
// hack/e2e/kms.sh (run during testbed setup); this function only checks their
// status so that failures are surfaced early rather than inside individual It
// blocks.
//
// If gce2e-native is not present the suite logs a warning — individual
// encryption tests call vcenter.EnsureNativeKeyProvider in their own BeforeEach
// so they will still self-heal and report a clear error. If gce2e-standard is
// absent we only warn since it is only available on VDS testbeds with a gateway.
func EnsureEncryptionKeyProviders(ctx context.Context, clusterProxy wcpframework.WCPClusterProxyInterface) {
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	defer vcenter.LogoutVimClient(vCenterClient)

	cryptoManager := crypto.NewManagerKmip(vCenterClient)

	framework.Byf("Checking KMS key provider %q", nativeKeyProviderID)
	if keyProviderExists(ctx, cryptoManager, nativeKeyProviderID) {
		status, _ := cryptoManager.GetClusterStatus(ctx, nativeKeyProviderID)
		framework.Byf("Native key provider %q status: %s", nativeKeyProviderID, status.OverallStatus)
	} else {
		framework.Byf("WARNING: native key provider %q not found — "+
			"encryption tests will attempt to create it in their own BeforeEach", nativeKeyProviderID)
	}

	framework.Byf("Checking KMS key provider %q", standardKeyProviderID)
	if keyProviderExists(ctx, cryptoManager, standardKeyProviderID) {
		status, _ := cryptoManager.GetClusterStatus(ctx, standardKeyProviderID)
		framework.Byf("Standard key provider %q status: %s", standardKeyProviderID, status.OverallStatus)
	} else {
		framework.Byf("Standard key provider %q not found — "+
			"tests requiring it will be skipped (VDS testbed with gateway needed)", standardKeyProviderID)
	}
}

// IsStandardKeyProviderAvailable returns true when the gce2e-standard key
// provider exists and has a green status. Tests that require an external KMIP
// server should skip when this returns false.
func IsStandardKeyProviderAvailable(ctx context.Context, clusterProxy wcpframework.WCPClusterProxyInterface) bool {
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	defer vcenter.LogoutVimClient(vCenterClient)
	cryptoManager := crypto.NewManagerKmip(vCenterClient)

	if !keyProviderExists(ctx, cryptoManager, standardKeyProviderID) {
		return false
	}
	status, err := cryptoManager.GetClusterStatus(ctx, standardKeyProviderID)
	if err != nil {
		return false
	}
	return status.OverallStatus == types.ManagedEntityStatusGreen
}

// keyProviderExists returns true when a key provider with the given ID is
// registered in vCenter.
func keyProviderExists(ctx context.Context, cryptoManager *crypto.ManagerKmip, keyProviderID string) bool {
	existing, err := cryptoManager.ListKmipServers(ctx, nil)
	if err != nil {
		return false
	}
	for _, c := range existing {
		if c.ClusterId.Id == keyProviderID {
			return true
		}
	}
	return false
}

// VerifyKeyProviderStatus asserts that the named key provider is registered and
// has an acceptable status. Native providers may show "red" before first use.
func VerifyKeyProviderStatus(ctx context.Context, cryptoManager *crypto.ManagerKmip, providerID, providerType string) {
	framework.Byf("Verifying %s key provider %q", providerType, providerID)
	Eventually(func(g Gomega) {
		status, err := cryptoManager.GetClusterStatus(ctx, providerID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get status for key provider %s", providerID)

		// Native providers start as "red" until a key is generated on first use.
		expectedStatuses := []types.ManagedEntityStatus{types.ManagedEntityStatusGreen}
		if providerType == "Native" {
			expectedStatuses = append(expectedStatuses, types.ManagedEntityStatusRed)
		}
		g.Expect(status.OverallStatus).To(BeElementOf(expectedStatuses),
			"key provider %s has unexpected status: %s", providerID, status.OverallStatus)
	}, "1m", "5s").Should(Succeed(), "%s key provider %q is not accessible", providerType, providerID)
}
