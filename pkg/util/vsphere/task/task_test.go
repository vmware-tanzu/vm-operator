// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package task_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/task"
)

func TestTask(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Task Util Suite")
}

var _ = Describe("ErrorMessageFromTaskInfo", func() {
	Context("when taskInfo is nil", func() {
		It("should return empty string", func() {
			result := task.ErrorMessageFromTaskInfo(nil)
			Expect(result).To(BeEmpty())
		})
	})

	Context("when taskInfo.Error is nil", func() {
		It("should return empty string", func() {
			taskInfo := &vimtypes.TaskInfo{}
			result := task.ErrorMessageFromTaskInfo(taskInfo)
			Expect(result).To(BeEmpty())
		})
	})

	Context("when taskInfo has no fault messages", func() {
		It("should return the localized message", func() {
			taskInfo := &vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					LocalizedMessage: "Operation failed",
					Fault: &vimtypes.CustomizationFault{
						VimFault: vimtypes.VimFault{
							MethodFault: vimtypes.MethodFault{},
						},
					},
				},
			}
			result := task.ErrorMessageFromTaskInfo(taskInfo)
			Expect(result).To(Equal("Operation failed"))
		})
	})

	Context("when taskInfo has fault messages with trailing colon in localized message", func() {
		It("should combine localized message with fault messages and trim trailing colon", func() {
			taskInfo := &vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					LocalizedMessage: "Customization of the guest operating system is not supported due to the given reason: ",
					Fault: &vimtypes.CustomizationFault{
						VimFault: vimtypes.VimFault{
							MethodFault: vimtypes.MethodFault{
								FaultMessage: []vimtypes.LocalizableMessage{
									{
										Key:     "com.vmware.vim.vm.error.UnsupportedToolsVersion",
										Message: "Tools version 7.4.3 installed in the GuestOS is not supported for guest customization. Please upgrade to the latest version.",
									},
								},
							},
						},
					},
				},
			}
			result := task.ErrorMessageFromTaskInfo(taskInfo)
			Expect(result).To(Equal("Customization of the guest operating system is not supported due to the given reason: Tools version 7.4.3 installed in the GuestOS is not supported for guest customization. Please upgrade to the latest version."))
		})
	})

	Context("when taskInfo has multiple fault messages", func() {
		It("should join all fault messages with semicolons", func() {
			taskInfo := &vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					LocalizedMessage: "Multiple errors occurred:",
					Fault: &vimtypes.CustomizationFault{
						VimFault: vimtypes.VimFault{
							MethodFault: vimtypes.MethodFault{
								FaultMessage: []vimtypes.LocalizableMessage{
									{
										Key:     "error1",
										Message: "First error message",
									},
									{
										Key:     "error2",
										Message: "Second error message",
									},
									{
										Key:     "error3",
										Message: "Third error message",
									},
								},
							},
						},
					},
				},
			}
			result := task.ErrorMessageFromTaskInfo(taskInfo)
			Expect(result).To(Equal("Multiple errors occurred: First error message; Second error message; Third error message"))
		})
	})

	Context("when taskInfo has fault messages with empty messages", func() {
		It("should skip empty messages", func() {
			taskInfo := &vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					LocalizedMessage: "Error occurred:",
					Fault: &vimtypes.CustomizationFault{
						VimFault: vimtypes.VimFault{
							MethodFault: vimtypes.MethodFault{
								FaultMessage: []vimtypes.LocalizableMessage{
									{
										Key:     "error1",
										Message: "",
									},
									{
										Key:     "error2",
										Message: "Valid error message",
									},
									{
										Key:     "error3",
										Message: "",
									},
								},
							},
						},
					},
				},
			}
			result := task.ErrorMessageFromTaskInfo(taskInfo)
			Expect(result).To(Equal("Error occurred: Valid error message"))
		})
	})

	Context("when taskInfo has fault messages but no localized message", func() {
		It("should return only the fault messages", func() {
			taskInfo := &vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					LocalizedMessage: "",
					Fault: &vimtypes.CustomizationFault{
						VimFault: vimtypes.VimFault{
							MethodFault: vimtypes.MethodFault{
								FaultMessage: []vimtypes.LocalizableMessage{
									{
										Key:     "error1",
										Message: "First error",
									},
									{
										Key:     "error2",
										Message: "Second error",
									},
								},
							},
						},
					},
				},
			}
			result := task.ErrorMessageFromTaskInfo(taskInfo)
			Expect(result).To(Equal("First error; Second error"))
		})
	})

	Context("when taskInfo has localized message with various trailing characters", func() {
		It("should trim colons, spaces, newlines, and tabs", func() {
			testCases := []struct {
				localizedMsg string
				expected     string
			}{
				{
					localizedMsg: "Error: ",
					expected:     "Error: Fault message",
				},
				{
					localizedMsg: "Error:\n",
					expected:     "Error: Fault message",
				},
				{
					localizedMsg: "Error:\t",
					expected:     "Error: Fault message",
				},
				{
					localizedMsg: "Error: \n\r\t",
					expected:     "Error: Fault message",
				},
			}

			for _, tc := range testCases {
				taskInfo := &vimtypes.TaskInfo{
					Error: &vimtypes.LocalizedMethodFault{
						LocalizedMessage: tc.localizedMsg,
						Fault: &vimtypes.CustomizationFault{
							VimFault: vimtypes.VimFault{
								MethodFault: vimtypes.MethodFault{
									FaultMessage: []vimtypes.LocalizableMessage{
										{
											Key:     "error1",
											Message: "Fault message",
										},
									},
								},
							},
						},
					},
				}
				result := task.ErrorMessageFromTaskInfo(taskInfo)
				Expect(result).To(Equal(tc.expected))
			}
		})
	})

	Context("when taskInfo has only empty fault messages", func() {
		It("should return the localized message", func() {
			taskInfo := &vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					LocalizedMessage: "Operation failed",
					Fault: &vimtypes.CustomizationFault{
						VimFault: vimtypes.VimFault{
							MethodFault: vimtypes.MethodFault{
								FaultMessage: []vimtypes.LocalizableMessage{
									{
										Key:     "error1",
										Message: "",
									},
									{
										Key:     "error2",
										Message: "",
									},
								},
							},
						},
					},
				},
			}
			result := task.ErrorMessageFromTaskInfo(taskInfo)
			Expect(result).To(Equal("Operation failed"))
		})
	})
})
