// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// GetESXHostFQDN returns the ESX host's FQDN.
func GetESXHostFQDN(
	ctx context.Context,
	vimClient *vim25.Client,
	hostMoID string) (string, error) {

	hostMoRef := types.ManagedObjectReference{Type: "HostSystem", Value: hostMoID}
	networkSys, err := object.NewHostSystem(vimClient, hostMoRef).ConfigManager().NetworkSystem(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get HostNetworkSystem for hostMoID %s: %w", hostMoID, err)
	}

	var hostNetworkSys mo.HostNetworkSystem
	if err := networkSys.Properties(ctx, networkSys.Reference(), []string{"dnsConfig"}, &hostNetworkSys); err != nil {
		return "", fmt.Errorf("failed to get HostMoID %s DNSConfig prop: %w", hostMoID, err)
	}

	if hostNetworkSys.DnsConfig == nil {
		return "", fmt.Errorf("hostMoID %s HostNetworkSystem does not have DNSConfig", hostMoID)
	}

	hostDNSConfig := hostNetworkSys.DnsConfig.GetHostDnsConfig()
	hostFQDN := strings.TrimSuffix(hostDNSConfig.HostName+"."+hostDNSConfig.DomainName, ".")
	return strings.ToLower(hostFQDN), nil
}
