// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"regexp"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
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
	vmGuestInfo, err := gip.prober.GetVirtualMachineGuestInfo(ctx, ctx.VM)
	if err != nil {
		return Unknown, err
	}

	for _, info := range ctx.VM.Spec.ReadinessProbe.GuestInfo {
		key := "guestinfo." + info.Key

		val, ok := vmGuestInfo[key]
		if !ok {
			return Failure, nil
		}

		if info.Value == "" {
			// Matches everything.
			continue
		}

		ex, err := regexp.Compile(info.Value)
		if err != nil {
			// Treat an invalid expressions as a wildcard too.
			continue
		}

		if !ex.Match([]byte(val)) {
			return Failure, nil
		}
	}

	return Success, nil
}
