// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"
	"regexp"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
)

type guestInfoProber struct {
	prober vmProviderGuestInfoProber
}

func NewGuestInfoProber(prober vmProviderGuestInfoProber) Probe {
	return &guestInfoProber{
		prober: prober,
	}
}

func (gip guestInfoProber) Probe(ctx *context.ProbeContext) (Result, error) {

	numProbes := len(ctx.VM.Spec.ReadinessProbe.GuestInfo)
	if numProbes == 0 {
		return Unknown, nil
	}

	// Build the list of property paths to retrieve based on the guestinfo keys.
	var (
		propertyPaths   = make([]string, numProbes)
		propertyKeyVals = make(map[string]string, numProbes)
	)
	for i := range ctx.VM.Spec.ReadinessProbe.GuestInfo {
		gi := ctx.VM.Spec.ReadinessProbe.GuestInfo[i]
		pp := fmt.Sprintf(`config.extraConfig["guestinfo.%s"]`, gi.Key)
		propertyPaths[i] = pp
		propertyKeyVals[pp] = gi.Value
	}

	results, err := gip.prober.GetVirtualMachineProperties(ctx, ctx.VM, propertyPaths)
	if err != nil {
		return Unknown, err
	}

	for i := range propertyPaths {
		key := propertyPaths[i]

		valObj, ok := results[key]
		if !ok {
			return Failure, nil
		}

		expectedVal := propertyKeyVals[key]

		if expectedVal == "" {
			// Matches everything.
			continue
		}

		expectedValRx, err := regexp.Compile(expectedVal)
		if err != nil {
			// Treat an invalid expressions as a wildcard too.
			continue
		}

		obj, ok := valObj.(vimtypes.OptionValue)
		if !ok {
			return Failure, nil
		}

		// If the value is not a string, then format it as a string.
		val, ok := obj.Value.(string)
		if !ok {
			val = fmt.Sprintf("%v", obj.Value)
		}

		if !expectedValRx.MatchString(val) {
			return Failure, nil
		}
	}

	return Success, nil
}
