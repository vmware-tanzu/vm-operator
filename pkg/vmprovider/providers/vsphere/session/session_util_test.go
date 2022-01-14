// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

var _ = Describe("Test Session Utils", func() {

	Context("Convert CPU units from milli-cores to MHz", func() {
		Specify("return whole number for non-integer CPU quantity", func() {
			q, err := resource.ParseQuantity("500m")
			Expect(err).NotTo(HaveOccurred())
			freq := session.CPUQuantityToMhz(q, 3225)
			expectVal := int64(1613)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})

		Specify("return whole number for integer CPU quantity", func() {
			q, err := resource.ParseQuantity("1000m")
			Expect(err).NotTo(HaveOccurred())
			freq := session.CPUQuantityToMhz(q, 3225)
			expectVal := int64(3225)
			Expect(freq).Should(BeNumerically("==", expectVal))
		})
	})

	Context("Compute CPU Min Frequency in the Cluster", func() {
		Specify("return cpu min frequency when natural number of hosts attached the cluster", func() {
			// The default model used by simulator has one host and one cluster configured.
			res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
				find := find.NewFinder(c)
				cr, err := find.DefaultClusterComputeResource(ctx)
				Expect(err).ShouldNot(HaveOccurred())

				cpuMinFreq, err := session.ComputeCPUInfo(ctx, cr)
				Expect(err).ShouldNot(HaveOccurred())
				Expect(cpuMinFreq).Should(BeNumerically(">", 0))

				return nil
			})
			Expect(res).ShouldNot(HaveOccurred())
		})

		Specify("return cpu min frequency when the cluster contains no hosts", func() {
			// The model being used is configured to have 0 hosts. (Defined in vsphere_suite_test.go/BeforeSuite)
			c, _ := govmomi.NewClient(ctx, server.URL, true)
			si := object.NewSearchIndex(c.Client)
			ref, err := si.FindByInventoryPath(ctx, "/DC0/host/DC0_C0")
			Expect(err).NotTo(HaveOccurred())
			cr := object.NewClusterComputeResource(c.Client, ref.Reference())

			cpuMinFreq, err := session.ComputeCPUInfo(ctx, cr)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(cpuMinFreq).Should(BeNumerically(">", 0))
		})
	})

	Context("GetMergedvAppConfigSpec", func() {
		trueVar := true
		falseVar := false

		DescribeTable("calling GetMergedvAppConfigSpec",
			func(inProps map[string]string, vmProps []vimTypes.VAppPropertyInfo, expected *vimTypes.VmConfigSpec) {
				vAppConfigSpec := session.GetMergedvAppConfigSpec(inProps, vmProps)
				if expected == nil {
					Expect(vAppConfigSpec).To(BeNil())
				} else {
					Expect(len(vAppConfigSpec.Property)).To(Equal(len(expected.Property)))
					for i := range vAppConfigSpec.Property {
						Expect(vAppConfigSpec.Property[i].Info.Key).To(Equal(expected.Property[i].Info.Key))
						Expect(vAppConfigSpec.Property[i].Info.Id).To(Equal(expected.Property[i].Info.Id))
						Expect(vAppConfigSpec.Property[i].Info.Value).To(Equal(expected.Property[i].Info.Value))
						Expect(vAppConfigSpec.Property[i].ArrayUpdateSpec.Operation).To(Equal(vimTypes.ArrayUpdateOperationEdit))
					}
				}
			},
			Entry("return nil for absent vm and input props",
				map[string]string{},
				[]vimTypes.VAppPropertyInfo{},
				nil,
			),
			Entry("return nil for non UserConfigurable vm props",
				map[string]string{
					"one-id": "one-override-value",
					"two-id": "two-override-value",
				},
				[]vimTypes.VAppPropertyInfo{
					{Key: 1, Id: "one-id", Value: "one-value"},
					{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: &falseVar},
				},
				nil,
			),
			Entry("return nil for UserConfigurable vm props but no input props",
				map[string]string{},
				[]vimTypes.VAppPropertyInfo{
					{Key: 1, Id: "one-id", Value: "one-value"},
					{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: &trueVar},
				},
				nil,
			),
			Entry("return valid vAppConfigSpec for setting mixed UserConfigurable props",
				map[string]string{
					"one-id":   "one-override-value",
					"two-id":   "two-override-value",
					"three-id": "three-override-value",
				},
				[]vimTypes.VAppPropertyInfo{
					{Key: 1, Id: "one-id", Value: "one-value", UserConfigurable: nil},
					{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: &trueVar},
					{Key: 3, Id: "three-id", Value: "three-value", UserConfigurable: &falseVar},
				},
				&vimTypes.VmConfigSpec{
					Property: []vimTypes.VAppPropertySpec{
						{Info: &vimTypes.VAppPropertyInfo{Key: 2, Id: "two-id", Value: "two-override-value"}},
					},
				},
			),
		)
	})

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

	Context("EncodeGzipBase64", func() {
		It("Encodes a string correctly", func() {
			input := "HelloWorld"
			output, err := session.EncodeGzipBase64(input)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(output).To(Equal("H4sIAAAAAAAA//JIzcnJD88vykkBAAAA//8BAAD//3kMd3cKAAAA"))
		})
	})

	Context("EncryptWebMKS", func() {
		var (
			privateKey   *rsa.PrivateKey
			publicKey    rsa.PublicKey
			publicKeyPem string
		)

		BeforeEach(func() {
			privateKey, _ = rsa.GenerateKey(rand.Reader, 2048)
			publicKey = privateKey.PublicKey
			publicKeyPem = string(pem.EncodeToMemory(
				&pem.Block{
					Type:  "PUBLIC KEY",
					Bytes: x509.MarshalPKCS1PublicKey(&publicKey),
				},
			))
		})

		It("Encrypts a string correctly", func() {
			plaintext := "HelloWorld2"
			ciphertext, err := session.EncryptWebMKS(publicKeyPem, plaintext)
			Expect(err).ShouldNot(HaveOccurred())
			decrypted, err := session.DecryptWebMKS(privateKey, ciphertext)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(decrypted).To(Equal(plaintext))
		})

		It("Error on invalid public key", func() {
			plaintext := "HelloWorld3"
			_, err := session.EncryptWebMKS("invalid-pub-key", plaintext)
			Expect(err).Should(HaveOccurred())
		})

	})
})
