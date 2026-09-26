// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resources_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/task"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/resources"
)

var _ = Describe("IsSharedBusControllerNotSupportedFault", func() {

	sharedBusFaultErr := func() error {
		return task.Error{
			LocalizedMethodFault: &vimtypes.LocalizedMethodFault{
				Fault: &vimtypes.SharedBusControllerNotSupported{
					DeviceNotSupported: vimtypes.DeviceNotSupported{
						Device: "key-1000",
					},
				},
				LocalizedMessage: "Device 'key-1000' is a SCSI controller engaged in bus-sharing.",
			},
		}
	}

	DescribeTable("classifies errors",
		func(err error, expected bool) {
			Expect(resources.IsSharedBusControllerNotSupportedFault(err)).To(Equal(expected))
		},

		Entry("nil error", nil, false),
		Entry("unrelated plain error", errors.New("boom"), false),
		Entry(
			"unrelated vSphere fault",
			task.Error{
				LocalizedMethodFault: &vimtypes.LocalizedMethodFault{
					Fault: &vimtypes.InvalidDeviceSpec{},
				},
			},
			false,
		),
		Entry("SharedBusControllerNotSupported fault", sharedBusFaultErr(), true),
		Entry(
			"SharedBusControllerNotSupported fault wrapped with fmt.Errorf",
			fmt.Errorf("reconfigure VM task failed: %w", sharedBusFaultErr()),
			true,
		),
	)
})
