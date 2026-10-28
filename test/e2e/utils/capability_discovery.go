// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

// LoadSupervisorCapabilities reads all capabilities from the selected
// representation. The returned map is a snapshot independent of future API
// changes. A successful load always returns a non-nil map, even if it is empty.
func LoadSupervisorCapabilities(ctx context.Context, client ctrlclient.Reader, asyncUpgrade bool) (map[string]bool, error) {
	capabilities := map[string]bool{}
	if asyncUpgrade {
		var cr capv1.Capabilities
		if err := client.Get(ctx, ctrlclient.ObjectKey{Name: capabilitiesCRName}, &cr); err != nil {
			return nil, fmt.Errorf("load Capabilities %s: %w", capabilitiesCRName, err)
		}
		for name, status := range cr.Status.Supervisor {
			capabilities[string(name)] = status.Activated
		}
	} else {
		var cm corev1.ConfigMap
		key := ctrlclient.ObjectKey{Namespace: capabilityConfigMapNs, Name: capabilityConfigMapName}
		if err := client.Get(ctx, key, &cm); err != nil {
			return nil, fmt.Errorf("load capability ConfigMap %s: %w", key, err)
		}
		for name, value := range cm.Data {
			capabilities[name] = value == "true"
		}
	}
	return capabilities, nil
}

// DiscoverSupervisorAsyncUpgrade opens and closes one SSH runner to read the
// stable FSS that selects the Supervisor capability representation. Connection,
// command, and unrecognized-response errors must not masquerade as disabled FSS.
func DiscoverSupervisorAsyncUpgrade(
	ctx context.Context,
	newRunner func(context.Context) (e2essh.SSHCommandRunner, error),
) (bool, error) {
	runner, err := newRunner(ctx)
	if err != nil {
		return false, fmt.Errorf("connect to discover %s: %w", SupervisorAsyncUpgradeFSS, err)
	}
	defer func() { _ = runner.Close() }()

	output, err := runner.RunCommand("python /usr/sbin/feature-state-wrapper.py " + SupervisorAsyncUpgradeFSS)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", SupervisorAsyncUpgradeFSS, err)
	}
	switch strings.TrimSpace(string(output)) {
	case "enabled":
		return true, nil
	case "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("read %s: unexpected response %q", SupervisorAsyncUpgradeFSS, output)
	}
}
