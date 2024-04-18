// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

var _ = Describe("Collections", func() {

	Describe("SliceToString", func() {
		var (
			in  []string
			out string
		)
		JustBeforeEach(func() {
			out = pkgcfg.SliceToString(in)
		})
		AfterEach(func() {
			in = nil
			out = ""
		})
		When("Input is nil", func() {
			BeforeEach(func() {
				in = nil
			})
			It("Should return an empty string", func() {
				Expect(out).To(BeEmpty())
			})
		})
		When("Input is one", func() {
			Context("empty element", func() {
				BeforeEach(func() {
					in = []string{""}
				})
				It("Should return an empty string", func() {
					Expect(out).To(BeEmpty())
				})
			})
			Context("element of nothing but white space", func() {
				BeforeEach(func() {
					in = []string{"     "}
				})
				It("Should return an empty string", func() {
					Expect(out).To(BeEmpty())
				})
			})
		})
		When("Input is multiple", func() {
			Context("elements, either empty or just white space", func() {
				BeforeEach(func() {
					in = []string{"     ", "", "\t\n", "\r   ", " \v "}
				})
				It("Should return an empty string", func() {
					Expect(out).To(BeEmpty())
				})
			})
			Context("non-empty elements", func() {
				BeforeEach(func() {
					in = []string{"a", "b", "c"}
				})
				It("Should return the correct value", func() {
					Expect(out).To(Equal("a,b,c"))
				})
				Context("where some include commas", func() {
					BeforeEach(func() {
						in = []string{"a", "b", "c,d"}
					})
					It("Should return the correct value", func() {
						Expect(out).To(Equal(`a,b,c\,d`))
					})
				})
			})
		})
	})

	Describe("StringToSlice", func() {
		var (
			in  string
			out []string
		)
		JustBeforeEach(func() {
			out = pkgcfg.StringToSlice(in)
		})
		AfterEach(func() {
			in = ""
			out = nil
		})
		When("Input is a zero-length string", func() {
			It("Should return a nil slice", func() {
				Expect(out).To(BeNil())
			})
		})
		When("Input is a string of just white-space", func() {
			BeforeEach(func() {
				in = "       "
			})
			It("Should return a nil slice", func() {
				Expect(out).To(BeNil())
			})
		})
		When("Input is a comma-delimited string of", func() {
			Context("elements that are just white-space", func() {
				BeforeEach(func() {
					in = "   ,\n\t,  \r, \v     ,    ,,  "
				})
				It("Should return a nil slice", func() {
					Expect(out).To(BeNil())
				})
			})
			Context("non-empty elements", func() {
				Context("with no surrounding white-space", func() {
					BeforeEach(func() {
						in = "a,b,c"
					})
					It("Should return the correct value", func() {
						Expect(out).To(Equal([]string{"a", "b", "c"}))
					})
				})
				Context("with surrounding white-space", func() {
					BeforeEach(func() {
						in = " a\v,\t\vb    ,c    \n"
					})
					It("Should return the correct value", func() {
						Expect(out).To(Equal([]string{"a", "b", "c"}))
					})
					Context("and white-space in between non-white-space characters", func() {
						BeforeEach(func() {
							in = " a\v,\t\vb\r b    ,c    \n"
						})
						It("Should return the correct value", func() {
							Expect(out).To(Equal([]string{"a", "b\r b", "c"}))
						})
					})
				})
				Context("that may include commas", func() {
					BeforeEach(func() {
						in = `a,b\,c,d`
					})
					It("Should return the correct value", func() {
						Expect(out).To(Equal([]string{"a", "b,c", "d"}))
					})
				})
			})
		})
	})

	// Please note the testing for this is rather minor has StringToSet is
	// essentially a wrapper for StringToSlice.
	Describe("StringToSet", func() {
		var (
			in  string
			out map[string]struct{}
		)
		JustBeforeEach(func() {
			out = pkgcfg.StringToSet(in)
		})
		AfterEach(func() {
			in = ""
			out = nil
		})
		When("Input is a zero-length string", func() {
			It("Should return a nil map", func() {
				Expect(out).To(BeNil())
			})
		})
		When("Input is a string of just white-space", func() {
			BeforeEach(func() {
				in = "       "
			})
			It("Should return a nil map", func() {
				Expect(out).To(BeNil())
			})
		})
		When("Input is a comma-delimited string of", func() {
			BeforeEach(func() {
				in = "   ,\n\t,  \r, \v     ,    ,,  "
			})
			It("Should return a nil map", func() {
				Expect(out).To(BeNil())
			})
			Context("non-empty elements", func() {
				Context("that do not repeat", func() {
					BeforeEach(func() {
						in = "a,b,c"
					})
					It("Should return the correct value", func() {
						Expect(out).To(HaveKey("a"))
						Expect(out).To(HaveKey("b"))
						Expect(out).To(HaveKey("c"))
					})
				})
				Context("where one repeats", func() {
					BeforeEach(func() {
						in = "a,a,b,c"
					})
					It("Should return the correct value", func() {
						Expect(out).To(HaveKey("a"))
						Expect(out).To(HaveKey("b"))
						Expect(out).To(HaveKey("c"))
					})
				})
				Context("where multiple repeat", func() {
					BeforeEach(func() {
						in = "a,a,b,b,c,d,d,d"
					})
					It("Should return the correct value", func() {
						Expect(out).To(HaveKey("a"))
						Expect(out).To(HaveKey("b"))
						Expect(out).To(HaveKey("c"))
						Expect(out).To(HaveKey("d"))
					})
				})
				Context("where there appears to be a repeating element but it is part of a single element with an escaped comma", func() {
					BeforeEach(func() {
						in = "a,a,b,b,c,d\\,d,d"
					})
					It("Should return the correct value", func() {
						Expect(out).To(HaveKey("a"))
						Expect(out).To(HaveKey("b"))
						Expect(out).To(HaveKey("c"))
						Expect(out).To(HaveKey("d,d"))
						Expect(out).To(HaveKey("d"))
					})
				})
			})
		})
	})
})
