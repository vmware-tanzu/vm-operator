// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("OptionValues", func() {

	const (
		sza   = "a"
		szb   = "b"
		szc   = "c"
		szd   = "d"
		sze   = "e"
		sz1   = "1"
		sz2   = "2"
		sz3   = "3"
		i32_1 = int32(1)
		u64_2 = uint64(2)
		f32_3 = float32(3)
		f64_4 = float64(4)
		b_5   = byte(5) //nolint:revive
	)

	var (
		psz1   = &[]string{sz1}[0]
		pu64_2 = &[]uint64{u64_2}[0]
		pf32_3 = &[]float32{f32_3}[0]
		pb_5   = &[]byte{b_5}[0] //nolint:revive
	)

	Context("OptionValuesFromMap", func() {
		When("input is nil", func() {
			It("should return nil", func() {
				out := pkgutil.OptionValuesFromMap[any](nil)
				Expect(out).To(BeNil())
			})
		})
		When("source values are all strings", func() {
			It("should return an OptionValues", func() {
				in := map[string]string{
					sza: sz1,
					szb: sz2,
					szc: sz3,
				}
				out := pkgutil.OptionValuesFromMap(in)
				Expect(out).To(ConsistOf(
					&vimtypes.OptionValue{Key: sza, Value: sz1},
					&vimtypes.OptionValue{Key: szb, Value: sz2},
					&vimtypes.OptionValue{Key: szc, Value: sz3},
				))
			})
		})
		When("source values include numbers", func() {
			It("should return an OptionValues", func() {
				in := map[string]any{
					sza: i32_1,
					szb: u64_2,
					szc: f32_3,
				}
				out := pkgutil.OptionValuesFromMap(in)
				Expect(out).To(ConsistOf(
					&vimtypes.OptionValue{Key: sza, Value: i32_1},
					&vimtypes.OptionValue{Key: szb, Value: u64_2},
					&vimtypes.OptionValue{Key: szc, Value: f32_3},
				))
			})
		})
		When("source values include pointers", func() {
			It("should return an OptionValues", func() {
				in := map[string]any{
					sza: psz1,
					szb: pu64_2,
					szc: pf32_3,
				}
				out := pkgutil.OptionValuesFromMap(in)
				Expect(out).To(ConsistOf(
					&vimtypes.OptionValue{Key: sza, Value: psz1},
					&vimtypes.OptionValue{Key: szb, Value: pu64_2},
					&vimtypes.OptionValue{Key: szc, Value: pf32_3},
				))
			})
		})
	})

	Context("Delete", func() {
		When("receiver is nil", func() {
			It("should return nil", func() {
				var (
					left, out pkgutil.OptionValues
				)

				left = nil

				Expect(func() { out = left.Delete("") }).ToNot(Panic())
				Expect(out).To(BeNil())
			})
		})

		When("receiver is not nil", func() {
			When("key does not exist", func() {
				It("should return same list", func() {
					var (
						left, out pkgutil.OptionValues
					)

					left = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: sz2},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
					}

					out = left.Delete(szd)
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: sz2},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
					))
				})
			})
			When("key does exist", func() {
				It("should return list with key removed", func() {
					var (
						left, out pkgutil.OptionValues
					)

					left = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: sz2},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
					}

					out = left.Delete(sza)
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: szb, Value: sz2},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
					))
				})
			})
		})
	})

	Context("Get", func() {
		When("receiver is nil", func() {
			It("should return nil, false", func() {
				var (
					left pkgutil.OptionValues
					val  any
					ok   bool
				)

				left = nil

				Expect(func() { val, ok = left.Get("") }).ToNot(Panic())
				Expect(val).To(BeNil())
				Expect(ok).To(BeFalse())
			})
		})
		When("receiver is not nil", func() {
			When("key does not exist", func() {
				It("should return nil, false", func() {
					var (
						left pkgutil.OptionValues
						val  any
						ok   bool
					)

					left = pkgutil.OptionValues{}

					val, ok = left.Get("")
					Expect(val).To(BeNil())
					Expect(ok).To(BeFalse())
				})
			})
			When("key does exist", func() {
				It("should return the value, true", func() {
					var (
						left pkgutil.OptionValues
						val  any
						ok   bool
					)

					left = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: sza, Value: sz1},
					}

					val, ok = left.Get(sza)
					Expect(val).To(Equal(sz1))
					Expect(ok).To(BeTrue())
				})
			})
			When("data includes a nil interface value", func() {
				It("should return the value, true", func() {
					var (
						left pkgutil.OptionValues
						val  any
						ok   bool
					)

					left = pkgutil.OptionValues{
						nillableOptionValue{},
						&vimtypes.OptionValue{Key: sza, Value: sz1},
					}

					val, ok = left.Get(sza)
					Expect(val).To(Equal(sz1))
					Expect(ok).To(BeTrue())
				})
			})
		})
	})

	Context("GetString", func() {
		When("receiver is nil", func() {
			It("should return \"\", false", func() {
				var (
					left pkgutil.OptionValues
					val  string
					ok   bool
				)

				left = nil

				Expect(func() { val, ok = left.GetString("") }).ToNot(Panic())
				Expect(val).To(BeEmpty())
				Expect(ok).To(BeFalse())
			})
		})
		When("receiver is not nil", func() {
			When("key does not exist", func() {
				It("should return \"\", false", func() {
					var (
						left pkgutil.OptionValues
						val  string
						ok   bool
					)

					left = pkgutil.OptionValues{}

					val, ok = left.GetString("")
					Expect(val).To(BeEmpty())
					Expect(ok).To(BeFalse())
				})
			})
			When("key does exist", func() {
				It("should return the value, true", func() {
					var (
						left pkgutil.OptionValues
						val  string
						ok   bool
					)

					left = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: sza, Value: sz1},
					}

					val, ok = left.GetString(sza)
					Expect(val).To(Equal(sz1))
					Expect(ok).To(BeTrue())
				})
			})
			When("data includes a nil interface value", func() {
				It("should return the value, true", func() {
					var (
						left pkgutil.OptionValues
						val  string
						ok   bool
					)

					left = pkgutil.OptionValues{
						nillableOptionValue{},
						&vimtypes.OptionValue{Key: sza, Value: sz1},
					}

					val, ok = left.GetString(sza)
					Expect(val).To(Equal(sz1))
					Expect(ok).To(BeTrue())
				})
			})
			When("data includes a *string", func() {
				Context("that is nil", func() {
					It("should return \"\", true", func() {
						var (
							left pkgutil.OptionValues
							val  string
							ok   bool
						)

						left = pkgutil.OptionValues{
							&vimtypes.OptionValue{Key: sza, Value: (*string)(nil)},
						}

						val, ok = left.GetString(sza)
						Expect(val).To(BeEmpty())
						Expect(ok).To(BeTrue())
					})
				})
				Context("that is not nil", func() {
					It("should return the value, true", func() {
						var (
							left pkgutil.OptionValues
							val  string
							ok   bool
						)

						left = pkgutil.OptionValues{
							nillableOptionValue{},
							&vimtypes.OptionValue{Key: sza, Value: psz1},
						}

						val, ok = left.GetString(sza)
						Expect(val).To(Equal(sz1))
						Expect(ok).To(BeTrue())
					})
				})
			})
			When("data includes a number", func() {
				It("should return the value, true", func() {
					var (
						left pkgutil.OptionValues
						val  string
						ok   bool
					)

					left = pkgutil.OptionValues{
						nillableOptionValue{},
						&vimtypes.OptionValue{Key: sza, Value: i32_1},
					}

					val, ok = left.GetString(sza)
					Expect(val).To(Equal(sz1))
					Expect(ok).To(BeTrue())
				})
			})
		})
	})

	Context("Map", func() {
		When("receiver is nil", func() {
			It("should return nil", func() {
				var (
					left pkgutil.OptionValues
					out  map[string]any
				)

				left = nil

				Expect(func() { out = left.Map() }).ToNot(Panic())
				Expect(out).To(BeNil())
			})
		})
		When("receiver is not nil", func() {
			When("there is data", func() {
				It("should return a map", func() {
					var (
						left pkgutil.OptionValues
						out  map[string]any
					)

					left = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: sza, Value: sz1},
					}

					out = left.Map()
					Expect(out).To(HaveLen(1))
					Expect(out).To(HaveKeyWithValue(sza, sz1))
				})
			})
			When("data is just a nil interface value", func() {
				It("should return nil", func() {
					var (
						left pkgutil.OptionValues
						out  map[string]any
					)

					left = pkgutil.OptionValues{
						nillableOptionValue{},
					}

					out = left.Map()
					Expect(out).To(BeNil())
				})
			})
		})
	})

	Context("StringMap", func() {
		When("receiver is nil", func() {
			It("should return nil", func() {
				var (
					left pkgutil.OptionValues
					out  map[string]string
				)

				left = nil

				Expect(func() { out = left.StringMap() }).ToNot(Panic())
				Expect(out).To(BeNil())
			})
		})
		When("receiver is not nil", func() {
			When("there is data", func() {
				It("should return a map", func() {
					var (
						left pkgutil.OptionValues
						out  map[string]string
					)

					left = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: u64_2},
						&vimtypes.OptionValue{Key: szc, Value: f32_3},
					}

					out = left.StringMap()
					Expect(out).To(HaveLen(3))
					Expect(out).To(HaveKeyWithValue(sza, sz1))
					Expect(out).To(HaveKeyWithValue(szb, sz2))
					Expect(out).To(HaveKeyWithValue(szc, sz3))
				})
			})
			When("data is just a nil interface value", func() {
				It("should return nil", func() {
					var (
						left pkgutil.OptionValues
						out  map[string]string
					)

					left = pkgutil.OptionValues{
						nillableOptionValue{},
					}

					out = left.StringMap()
					Expect(out).To(BeNil())
				})
			})
		})
	})

	Context("Additions", func() {
		When("receiver is nil", func() {
			When("input is nil", func() {
				It("should return nil", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = nil

					Expect(func() { out = left.Additions(right...) }).ToNot(Panic())
					Expect(out).To(BeNil())
				})
			})
			When("input is not nil", func() {
				It("should return additions", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					Expect(func() { out = left.Additions(right...) }).ToNot(Panic())
					Expect(out).To(HaveLen(3))
					Expect(out).To(HaveExactElements(
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
		When("receiver is not nil", func() {
			When("input is nil", func() {
				It("should return nil", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = nil

					out = left.Additions(right...)
					Expect(out).To(BeNil())
				})
			})
			When("input is not nil", func() {
				It("should return additions", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = pkgutil.OptionValues{

						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					out = left.Additions(right...)
					Expect(out).To(HaveLen(2))
					Expect(out).To(HaveExactElements(
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
	})

	Context("Diff", func() {
		When("receiver is nil", func() {
			When("input is nil", func() {
				It("should return nil", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = nil

					Expect(func() { out = left.Diff(right...) }).ToNot(Panic())
					Expect(out).To(BeNil())
				})
			})
			When("input is not nil", func() {
				It("should return diff", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					Expect(func() { out = left.Diff(right...) }).ToNot(Panic())
					Expect(out).To(HaveLen(3))
					Expect(out).To(HaveExactElements(
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
		When("receiver is not nil", func() {
			When("input is nil", func() {
				It("should return nil", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = nil

					out = left.Diff(right...)
					Expect(out).To(BeNil())
				})
			})
			When("input is not nil", func() {
				It("should return diff", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					out = left.Diff(right...)
					Expect(out).To(HaveLen(3))
					Expect(out).To(HaveExactElements(
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
	})

	Context("Merge", func() {
		When("receiver is nil", func() {
			When("input is nil", func() {
				It("should not panic", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = nil

					Expect(func() { out = left.Merge(right...) }).ToNot(Panic())
					Expect(out).To(BeNil())
				})
			})
			When("input is not nil", func() {
				It("should not panic", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					Expect(func() { out = left.Merge(right...) }).ToNot(Panic())
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
		When("receiver is not nil", func() {
			When("input is nil", func() {
				It("should not modify the original data", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = nil

					out = left.Merge(right...)
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: sz2},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
					))

				})
			})
			When("input is not nil", func() {
				It("should merge the data", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					out = left.Merge(right...)
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
	})

	Context("Append", func() {
		When("receiver is nil", func() {
			When("input is nil", func() {
				It("should not panic", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = nil

					Expect(func() { out = left.Append(right...) }).ToNot(Panic())
					Expect(out).To(BeNil())
				})
			})
			When("input is not nil", func() {
				It("should not panic", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = nil
					right = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					Expect(func() { out = left.Append(right...) }).ToNot(Panic())
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
		When("receiver is not nil", func() {
			When("input is nil", func() {
				It("should not modify the original data", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = nil

					out = left.Append(right...)
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: sz2},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
					))

				})
			})
			When("input is not nil", func() {
				It("should append the data", func() {
					var (
						left  pkgutil.OptionValues
						right pkgutil.OptionValues
						out   pkgutil.OptionValues
					)

					left = pkgutil.OptionValuesFromMap(map[string]string{
						sza: sz1,
						szb: sz2,
						szc: sz3,
					})
					right = pkgutil.OptionValues{
						&vimtypes.OptionValue{Key: szb, Value: ""},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					}

					out = left.Append(right...)
					Expect(out).To(ConsistOf(
						&vimtypes.OptionValue{Key: sza, Value: sz1},
						&vimtypes.OptionValue{Key: szb, Value: sz2},
						&vimtypes.OptionValue{Key: szc, Value: sz3},
						&vimtypes.OptionValue{Key: szd, Value: f64_4},
						&vimtypes.OptionValue{Key: sze, Value: pb_5},
					))
				})
			})
		})
	})

})

type nillableOptionValue struct{}

func (ov nillableOptionValue) GetOptionValue() *vimtypes.OptionValue {
	return nil
}
