package fault

import (
	"testing"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFault(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fault Util Suite")
}

var _ = Describe("LocalizedMessagesFromFault", func() {

	Context("when LocalizedMethodFault has no localized message and fault is nil", func() {
		It("should return empty slice", func() {
			result := LocalizedMessagesFromFault(vimtypes.LocalizedMethodFault{
				LocalizedMessage: "",
				Fault:            nil,
			})
			Expect(result).To(BeEmpty())
		})
	})

	Context("when LocalizedMethodFault has only localized message", func() {
		It("should return the localized message", func() {
			result := LocalizedMessagesFromFault(vimtypes.LocalizedMethodFault{
				LocalizedMessage: "Operation failed",
				Fault:            nil,
			})
			Expect(result).To(Equal([]string{"Operation failed"}))
		})
	})

	Context("when LocalizedMessage has trailing whitespaces and colons", func() {
		It("should trim them from the extracted message", func() {
			result := LocalizedMessagesFromFault(vimtypes.LocalizedMethodFault{
				LocalizedMessage: "Operation failed due to: \n\t",
				Fault:            nil,
			})
			Expect(result).To(Equal([]string{"Operation failed due to"}))
		})
	})

	Describe("when LocalizedMethodFault is NoCompatibleHost", func() {
		var (
			baseFault *vimtypes.NoCompatibleHost
			lmf       vimtypes.LocalizedMethodFault
		)

		BeforeEach(func() {
			baseFault = &vimtypes.NoCompatibleHost{
				Error: []vimtypes.LocalizedMethodFault{
					{
						LocalizedMessage: "First error",
						Fault:            nil,
					},
					{
						LocalizedMessage: "Second error",
						Fault:            nil,
					},
				},
			}
			lmf = vimtypes.LocalizedMethodFault{
				LocalizedMessage: "Top level",
				Fault:            baseFault,
			}
		})

		Context("when error slice is empty", func() {
			It("should only return top level message", func() {
				baseFault.Error = []vimtypes.LocalizedMethodFault{}
				result := LocalizedMessagesFromFault(lmf)
				Expect(result).To(Equal([]string{"Top level"}))
			})
		})

		Context("when fault has NoCompatibleHost with nested errors", func() {
			It("should recursively collect all localized messages", func() {
				lmf.LocalizedMessage = "No compatible host"
				result := LocalizedMessagesFromFault(lmf)
				Expect(result).To(Equal([]string{
					"No compatible host",
					"First error",
					"Second error",
				}))
			})
		})

		Context("when nested fault has empty localized message", func() {
			It("should skip the empty message", func() {
				baseFault.Error[0].LocalizedMessage = ""
				baseFault.Error[1].LocalizedMessage = "Valid message"
				result := LocalizedMessagesFromFault(lmf)
				Expect(result).To(Equal([]string{
					"Top level",
					"Valid message",
				}))
			})
		})

		Context("when deeply nested with mixed nil and non-nil faults", func() {
			It("should handle gracefully", func() {
				lmf.LocalizedMessage = "Level 0"
				baseFault.Error = []vimtypes.LocalizedMethodFault{
					{
						LocalizedMessage: "Level 1a",
						Fault:            nil,
					},
					{
						LocalizedMessage: "Level 1b",
						Fault: &vimtypes.NoCompatibleHost{
							Error: []vimtypes.LocalizedMethodFault{
								{
									LocalizedMessage: "Level 2a",
									Fault:            nil,
								},
								{
									LocalizedMessage: "",
									Fault:            nil,
								},
								{
									LocalizedMessage: "Level 2b",
									Fault:            nil,
								},
							},
						},
					},
				}
				result := LocalizedMessagesFromFault(lmf)
				Expect(result).To(Equal([]string{
					"Level 0",
					"Level 1a",
					"Level 1b",
					"Level 2a",
					"Level 2b",
				}))
			})
		})
	})
})

var _ = Describe("LocalizedMessagesFromFaults", func() {
	Describe("Basic batch processing", func() {
		Context("when faults is empty or nil", func() {
			It("should return empty slice for empty slice", func() {
				result := LocalizedMessagesFromFaults([]vimtypes.LocalizedMethodFault{})
				Expect(result).To(BeEmpty())
			})

			It("should return empty slice for nil slice", func() {
				var faults []vimtypes.LocalizedMethodFault
				result := LocalizedMessagesFromFaults(faults)
				Expect(result).To(BeEmpty())
			})
		})

		Context("when faults contains multiple entries", func() {
			It("should collect messages from all faults", func() {
				result := LocalizedMessagesFromFaults([]vimtypes.LocalizedMethodFault{
					{
						LocalizedMessage: "Fault 1 message",
						Fault:            nil,
					},
					{
						LocalizedMessage: "Fault 2 message",
						Fault:            nil,
					},
				})
				Expect(result).To(Equal([]string{
					"Fault 1 message",
					"Fault 2 message",
				}))
			})
		})

		Context("when some faults have nil messages", func() {
			It("should skip empty messages", func() {
				result := LocalizedMessagesFromFaults([]vimtypes.LocalizedMethodFault{
					{
						LocalizedMessage: "First",
						Fault:            nil,
					},
					{
						LocalizedMessage: "",
						Fault:            nil,
					},
					{
						LocalizedMessage: "Third",
						Fault:            nil,
					},
				})
				Expect(result).To(Equal([]string{
					"First",
					"Third",
				}))
			})
		})
	})
})
