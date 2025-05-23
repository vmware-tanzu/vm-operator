// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = DescribeTable("GetBootstrapConditionValuesTest",
	func(extraConfig map[string]string,
		expectedStatus bool, expectedReason string, expectedMsg string, expectedOK bool) {

		status, reason, msg, ok := util.GetBootstrapConditionValues(extraConfig)
		Expect(status).To(Equal(expectedStatus))
		Expect(reason).To(Equal(expectedReason))
		Expect(msg).To(Equal(expectedMsg))
		Expect(ok).To(Equal(expectedOK))
	},
	func(extraConfig map[string]string,
		expectedStatus bool, expectedReason string, expectedMsg string, expectedOK bool) string {
		return fmt.Sprintf("ExtraConfig '%v' should return status=%v, reason=%q, msg=%q, ok=%v",
			extraConfig, expectedStatus, expectedReason, expectedMsg, expectedOK)
	},
	Entry(nil,
		nil,
		false, "", "", false,
	),
	Entry(nil,
		map[string]string{},
		false, "", "", false,
	),
	Entry(nil,
		map[string]string{
			"key1": "val1",
		},
		false, "", "", false,
	),
	Entry(nil,
		map[string]string{
			"key1":                           "val1",
			util.GuestInfoBootstrapCondition: "",
		},
		false, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "1",
		},
		true, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "TRUE",
		},
		true, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "true",
		},
		true, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "true,my-reason",
		},
		true, "my-reason", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "true,my-reason,my,comma,delimited,message",
		},
		true, "my-reason", "my,comma,delimited,message", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "  true ,  my-reason ,   my,comma,delimited,message ",
		},
		true, "my-reason", "my,comma,delimited,message", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "  ,,  ",
		},
		false, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "0",
		},
		false, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "FALSE",
		},
		false, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "false",
		},
		false, "", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "false,my-reason",
		},
		false, "my-reason", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "  ,my-reason",
		},
		false, "my-reason", "", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "false,my-reason,my,comma,delimited,message",
		},
		false, "my-reason", "my,comma,delimited,message", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "  false ,  my-reason ,   my,comma,delimited,message ",
		},
		false, "my-reason", "my,comma,delimited,message", true,
	),
	Entry(nil,
		map[string]string{
			util.GuestInfoBootstrapCondition: "   ,  my-reason ,   my,comma,delimited,message ",
		},
		false, "my-reason", "my,comma,delimited,message", true,
	),
)
