// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"fmt"

	"golang.org/x/crypto/ssh"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
)

// LoadSupervisorCapabilities discovers and snapshots the Supervisor capabilities
// during suite setup. Tests reuse the returned map for the rest of the run.
func LoadSupervisorCapabilities(ctx context.Context, proxy framework.ClusterProxyInterface) (map[string]bool, error) {
	e2eframework.Logf("Loading Supervisor capability snapshot for %s", proxy.GetKubeconfigPath())
	asyncUpgrade, err := utils.DiscoverSupervisorAsyncUpgrade(ctx, func(ctx context.Context) (e2essh.SSHCommandRunner, error) {
		return e2essh.NewSSHCommandRunner(
			vcenter.GetVCPNIDFromKubeconfigFile(ctx, proxy.GetKubeconfigPath()),
			vcenter.VCSSHPort,
			testbed.RootUsername,
			[]ssh.AuthMethod{ssh.Password(testbed.RootPassword)},
		)
	})
	if err != nil {
		return nil, fmt.Errorf("discover Supervisor capability representation: %w", err)
	}
	return utils.LoadSupervisorCapabilities(ctx, proxy.GetClient(), asyncUpgrade)
}
