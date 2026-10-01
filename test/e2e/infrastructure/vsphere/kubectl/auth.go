// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kubectl

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
)

// AssertKubectlUserCan uses kubectl auth can-i to verify a user's privileges.
func AssertKubectlUserCan(ctx context.Context, kubeconfigPath string, args ...string) {
	Eventually(func() bool {
		// We ignore errors as they can come from two possible sources-
		// 1) RBAC rules do not allow resource access.
		// 2) kubectl command failed for some reason. (eg. network blip)
		// In either case, the result will not be 'yes', and the
		// Eventually() means it'll be retried.
		kubectlArgs := []string{"can-i"}
		kubectlArgs = append(kubectlArgs, args...)
		result, _ := framework.KubectlAuth(ctx, kubeconfigPath, kubectlArgs...)

		return strings.TrimSpace(string(result)) == "yes"
	}, 1*time.Minute, 5*time.Second).Should(BeTrue(), "User should eventually be able to", args)
}

// AssertKubectlUserCannot uses kubectl auth can-i to verify the absence of a user's privileges.
func AssertKubectlUserCannot(ctx context.Context, kubeconfigPath string, args ...string) {
	// This is a bit of a hack, but passing in the flag twice causes the second one to be preferred.
	//  It's a cheap way of overriding kubectl.
	Eventually(func() bool {
		kubectlArgs := []string{"can-i"}
		kubectlArgs = append(kubectlArgs, args...)
		result, _ := framework.KubectlAuth(ctx, kubeconfigPath, kubectlArgs...)
		// Don't use an Expect() inside this function as that will prematurely fail the test.
		//  The assertion is that this function should eventually return true.
		return strings.TrimSpace(string(result)) == "no"
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "User should not be able to", args)
}
