// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/session"
)

var _ = Describe("Test Session Utils", func() {

	Context("ExtraConfigToMap", func() {
		var (
			extraConfig    []vimTypes.BaseOptionValue
			extraConfigMap map[string]string
		)
		BeforeEach(func() {
			extraConfig = []vimTypes.BaseOptionValue{}
		})
		JustBeforeEach(func() {
			extraConfigMap = session.ExtraConfigToMap(extraConfig)
		})

		Context("Empty extraConfig", func() {
			It("Return empty map", func() {
				Expect(extraConfigMap).To(HaveLen(0))
			})
		})

		Context("With extraConfig", func() {
			BeforeEach(func() {
				extraConfig = append(extraConfig, &vimTypes.OptionValue{Key: "key1", Value: "value1"})
				extraConfig = append(extraConfig, &vimTypes.OptionValue{Key: "key2", Value: "value2"})
			})
			It("Return valid map", func() {
				Expect(extraConfigMap).To(HaveLen(2))
				Expect(extraConfigMap["key1"]).To(Equal("value1"))
				Expect(extraConfigMap["key2"]).To(Equal("value2"))
			})
		})
	})

	Context("MergeExtraConfig", func() {
		var (
			extraConfig []vimTypes.BaseOptionValue
			newMap      map[string]string
			merged      []vimTypes.BaseOptionValue
		)
		BeforeEach(func() {
			extraConfig = []vimTypes.BaseOptionValue{
				&vimTypes.OptionValue{Key: "existingkey1", Value: "existingvalue1"},
				&vimTypes.OptionValue{Key: "existingkey2", Value: "existingvalue2"},
			}
			newMap = map[string]string{}
		})
		JustBeforeEach(func() {
			merged = session.MergeExtraConfig(extraConfig, newMap)
		})

		Context("Empty newMap", func() {
			It("Return empty merged", func() {
				Expect(merged).To(BeEmpty())
			})
		})

		Context("NewMap with existing key", func() {
			BeforeEach(func() {
				newMap["existingkey1"] = "existingkey1"
			})
			It("Return empty merged", func() {
				Expect(merged).To(BeEmpty())
			})
		})

		Context("NewMap with new keys", func() {
			BeforeEach(func() {
				newMap["newkey1"] = "newvalue1"
				newMap["newkey2"] = "newvalue2"
			})
			It("Return merged map", func() {
				Expect(merged).To(HaveLen(2))
				mergedMap := session.ExtraConfigToMap(merged)
				Expect(mergedMap["newkey1"]).To(Equal("newvalue1"))
				Expect(mergedMap["newkey2"]).To(Equal("newvalue2"))
			})
		})
	})
})
