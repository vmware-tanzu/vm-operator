// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

// hostMaintenanceModeTaskDescriptionIDs are the descriptionId values vCenter
// assigns to the tasks that transition a host's maintenance mode. A host
// mid-transition has not yet flipped runtime.inMaintenanceMode, so this is
// checked in addition to that flag.
var hostMaintenanceModeTaskDescriptionIDs = map[string]struct{}{
	"HostSystem.enterMaintenanceMode": {},
	"HostSystem.exitMaintenanceMode":  {},
}

// HostMaintenanceState describes whether a host is in, or actively
// transitioning into or out of, maintenance mode.
type HostMaintenanceState struct {
	// InMaintenanceMode reflects HostSystem.runtime.inMaintenanceMode.
	InMaintenanceMode bool
	// TransitioningMaintenanceMode is true if the host has an in-progress
	// EnterMaintenanceMode_Task or ExitMaintenanceMode_Task.
	TransitioningMaintenanceMode bool
}

// GetESXHostFQDN returns the ESX host's FQDN.
func GetESXHostFQDN(
	ctx context.Context,
	vimClient *vim25.Client,
	hostMoID string) (string, error) {

	hostMoRef := vimtypes.ManagedObjectReference{Type: "HostSystem", Value: hostMoID}
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

// GetHostMaintenanceState returns whether the given host is in, or actively
// transitioning into or out of, maintenance mode.
func GetHostMaintenanceState(
	ctx context.Context,
	vimClient *vim25.Client,
	hostMoRef vimtypes.ManagedObjectReference) (HostMaintenanceState, error) {

	ctx = pkgctx.WithVCOpID(ctx, nil, "getHostMaintenanceState")

	var host mo.HostSystem
	if err := object.NewHostSystem(vimClient, hostMoRef).Properties(
		ctx,
		hostMoRef,
		[]string{"runtime.inMaintenanceMode", "recentTask"},
		&host); err != nil {

		return HostMaintenanceState{}, fmt.Errorf(
			"failed to get HostSystem %s runtime/recentTask props: %w", hostMoRef.Value, err)
	}

	state := HostMaintenanceState{InMaintenanceMode: host.Runtime.InMaintenanceMode}

	if state.InMaintenanceMode || len(host.RecentTask) == 0 {
		return state, nil
	}

	pc := property.DefaultCollector(vimClient)

	var tasks []mo.Task
	if err := pc.Retrieve(
		ctx, host.RecentTask, []string{"info.descriptionId", "info.state"}, &tasks); err != nil {

		// A stale entry (task already expired out of vCenter) faults the
		// whole batch. Rather than retry per-task, just treat the host as
		// not transitioning and let the caller fall back to the plain
		// not-synced behavior.
		pkglog.FromContextOrDefault(ctx).Error(
			err, "failed to retrieve host recent task info", "host", hostMoRef.Value)
		return state, nil
	}

	for _, t := range tasks {
		if t.Info.State != vimtypes.TaskInfoStateRunning && t.Info.State != vimtypes.TaskInfoStateQueued {
			continue
		}
		if _, ok := hostMaintenanceModeTaskDescriptionIDs[t.Info.DescriptionId]; ok {
			state.TransitioningMaintenanceMode = true
			break
		}
	}

	return state, nil
}
