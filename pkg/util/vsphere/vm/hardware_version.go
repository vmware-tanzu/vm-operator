// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

const (
	HardwareVersionProperty = "config.version"
	PowerStateProperty      = "runtime.powerState"
)

type ReconcileMinHardwareVersionResult uint8

const (
	ReconcileMinHardwareVersionResultUpgraded = iota + 1
	ReconcileMinHardwareVersionResultNotPoweredOff
	ReconcileMinHardwareVersionResultAlreadyUpgraded
	ReconcileMinHardwareVersionResultMinHardwareVersionZero
)

func ReconcileMinHardwareVersion(
	ctx context.Context,
	client *vim25.Client,
	mo mo.VirtualMachine,
	fetchProperties bool,
	minHardwareVersion int32) (ReconcileMinHardwareVersionResult, error) {

	if ctx == nil {
		return 0, fmt.Errorf("invalid ctx: nil")
	}

	if client == nil {
		return 0, fmt.Errorf("invalid client: nil")
	}

	if minHardwareVersion == 0 {
		return ReconcileMinHardwareVersionResultMinHardwareVersionZero, nil
	}
	targetHardwareVersion := vimtypes.HardwareVersion(minHardwareVersion) //nolint:gosec // disable G115
	if !targetHardwareVersion.IsSupported() {
		return 0, fmt.Errorf("invalid minHardwareVersion: %d", minHardwareVersion)
	}

	var (
		configVersion string
		log           = logr.FromContextOrDiscard(ctx)
		obj           = object.NewVirtualMachine(client, mo.Self)
		powerState    = mo.Runtime.PowerState
	)

	if mo.Config != nil {
		configVersion = mo.Config.Version
	}

	log = log.WithValues(
		"fetchProperties", fetchProperties,
		"minHardwareVersion", minHardwareVersion,
		"targetHardwareVersion", targetHardwareVersion,
		HardwareVersionProperty, configVersion,
		PowerStateProperty, powerState)

	// Fetch the VM's current hardware version if it is not already known or if
	// explicitly requested.
	if configVersion == "" || powerState == "" || fetchProperties {
		if err := obj.Properties(
			ctx,
			mo.Self,
			[]string{HardwareVersionProperty, PowerStateProperty},
			&mo); err != nil {
			return 0, fmt.Errorf("failed to retrieve properties %w", err)
		}
		configVersion = mo.Config.Version
		powerState = mo.Runtime.PowerState
		log = log.WithValues(
			HardwareVersionProperty, configVersion,
			PowerStateProperty, powerState)
		log.Info("Fetched hardware version and power state")
	}

	currentHardwareVersion := vimtypes.MustParseHardwareVersion(configVersion)
	log = log.WithValues("currentHardwareVersion", currentHardwareVersion)

	// If the current hardware version is already the same or newer than the
	// specified minimum hardware version, then there is nothing to do.
	if currentHardwareVersion >= targetHardwareVersion {
		return ReconcileMinHardwareVersionResultAlreadyUpgraded, nil
	}

	// If the VM is not powered off, then there is nothing to do.
	if mo.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOff {
		return ReconcileMinHardwareVersionResultNotPoweredOff, nil
	}

	// Upgrade the hardware version.
	log.Info("Upgrading hardware version")
	t, err := obj.UpgradeVM(ctx, targetHardwareVersion.String())
	if err != nil {
		return 0, fmt.Errorf("failed to invoke upgrade vm: %w", err)
	}

	// Wait for the upgrade to complete.
	if err := t.WaitEx(ctx); err != nil {
		return 0, fmt.Errorf("failed to upgrade vm: %w", err)
	}

	return ReconcileMinHardwareVersionResultUpgraded, nil
}
