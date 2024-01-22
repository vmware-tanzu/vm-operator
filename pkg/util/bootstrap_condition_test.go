// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("GetBootstrapConditionValuesTest", func() {
	var (
		configInfo *vimTypes.VirtualMachineConfigInfo
		status     bool
		reason     string
		msg        string
		ok         bool
	)
	JustBeforeEach(func() {
		status, reason, msg, ok = util.GetBootstrapConditionValues(configInfo)
	})
	AfterEach(func() {
		configInfo = nil
		status = false
		reason = ""
		msg = ""
		ok = false
	})
	assertResult := func(s bool, r, m string, o bool) {
		ExpectWithOffset(1, status).To(Equal(s))
		ExpectWithOffset(1, reason).To(Equal(r))
		ExpectWithOffset(1, msg).To(Equal(m))
		ExpectWithOffset(1, ok).To(Equal(o))
	}
	When("configInfo is nil", func() {
		It("should return status=false, reason=\"\", msg=\"\", ok=false", func() {
			assertResult(false, "", "", false)
		})
	})
	When("extraConfig is zero-length", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{}
		})
		It("should return status=false, reason=\"\", msg=\"\", ok=false", func() {
			assertResult(false, "", "", false)
		})
	})
	When("extraConfig is missing guestinfo key", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   "key1",
						Value: "val1",
					},
				},
			}
		})
		It("should return status=false, reason=\"\", msg=\"\", ok=false", func() {
			assertResult(false, "", "", false)
		})
	})
	When("guestinfo val is empty", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   "key1",
						Value: "val1",
					},
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "",
					},
				},
			}
		})
		It("should return status=false, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(false, "", "", true)
		})
	})
	When("status is 1", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "1",
					},
				},
			}
		})
		It("should return status=true, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(true, "", "", true)
		})
	})
	When("status is TRUE", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "TRUE",
					},
				},
			}
		})
		It("should return status=true, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(true, "", "", true)
		})
	})
	When("status is true", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "true",
					},
				},
			}
		})
		It("should return status=true, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(true, "", "", true)
		})
	})
	When("status is true with a reason", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "true,my-reason",
					},
				},
			}
		})
		It("should return status=true, reason=\"my-reason\", msg=\"\", ok=true", func() {
			assertResult(true, "my-reason", "", true)
		})
	})
	When("status is true with a reason and message", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "true,my-reason,my,comma,delimited,message",
					},
				},
			}
		})
		It("should return status=true, reason=\"my-reason\", msg=\"my,comma,delimited,message\", ok=true", func() {
			assertResult(true, "my-reason", "my,comma,delimited,message", true)
		})
	})
	When("status is true with a reason and message with leading or trailing whitespace", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "  true ,  my-reason ,   my,comma,delimited,message ",
					},
				},
			}
		})
		It("should return status=true, reason=\"my-reason\", msg=\"my,comma,delimited,message\", ok=true", func() {
			assertResult(true, "my-reason", "my,comma,delimited,message", true)
		})
	})
	When("status is empty with empty reason and message", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "  ,,  ",
					},
				},
			}
		})
		It("should return status=false, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(false, "", "", true)
		})
	})
	When("status is 0", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "0",
					},
				},
			}
		})
		It("should return status=false, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(false, "", "", true)
		})
	})
	When("status is FALSE", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "FALSE",
					},
				},
			}
		})
		It("should return status=false, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(false, "", "", true)
		})
	})
	When("status is false", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "false",
					},
				},
			}
		})
		It("should return status=false, reason=\"\", msg=\"\", ok=true", func() {
			assertResult(false, "", "", true)
		})
	})
	When("status is false with a reason", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "false,my-reason",
					},
				},
			}
		})
		It("should return status=false, reason=\"my-reason\", msg=\"\", ok=true", func() {
			assertResult(false, "my-reason", "", true)
		})
	})
	When("status is empty with a reason", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "  ,my-reason",
					},
				},
			}
		})
		It("should return status=false, reason=\"my-reason\", msg=\"\", ok=true", func() {
			assertResult(false, "my-reason", "", true)
		})
	})
	When("status is false with a reason and message", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "false,my-reason,my,comma,delimited,message",
					},
				},
			}
		})
		It("should return status=false, reason=\"my-reason\", msg=\"my,comma,delimited,message\", ok=true", func() {
			assertResult(false, "my-reason", "my,comma,delimited,message", true)
		})
	})
	When("status is false with a reason and message with leading or trailing whitespace", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "  false ,  my-reason ,   my,comma,delimited,message ",
					},
				},
			}
		})
		It("should return status=false, reason=\"my-reason\", msg=\"my,comma,delimited,message\", ok=true", func() {
			assertResult(false, "my-reason", "my,comma,delimited,message", true)
		})
	})
	When("status is empty with a reason and message with leading or trailing whitespace", func() {
		BeforeEach(func() {
			configInfo = &vimTypes.VirtualMachineConfigInfo{
				ExtraConfig: []vimTypes.BaseOptionValue{
					&vimTypes.OptionValue{
						Key:   util.GuestInfoBootstrapCondition,
						Value: "   ,  my-reason ,   my,comma,delimited,message ",
					},
				},
			}
		})
		It("should return status=false, reason=\"my-reason\", msg=\"my,comma,delimited,message\", ok=true", func() {
			assertResult(false, "my-reason", "my,comma,delimited,message", true)
		})
	})
})
