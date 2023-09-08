// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("Base64Decode", func() {

	b64 := func(src []byte) []byte {
		return []byte(base64.StdEncoding.EncodeToString(src))
	}

	Context("Valid input", func() {
		It("Should decode successfully", func() {
			src := []byte("Hello, world.")
			data, err := util.Base64Decode(b64(src))
			Expect(err).ToNot(HaveOccurred())
			Expect(data).To(Equal(src))
		})
	})

	Context("Invalid input", func() {
		It("Should return an error", func() {
			src := []byte("Hello, world.")
			data, err := util.Base64Decode(src)
			Expect(err).To(HaveOccurred())
			Expect(data).To(BeNil())
		})
	})
})

var _ = Describe("TryToDecodeBase64Gzip", func() {

	b64 := func(count int, data []byte) []byte {
		for i := 0; i <= count; i++ {
			data = []byte(base64.StdEncoding.EncodeToString(data))
		}
		return data
	}

	gz := func(data []byte) []byte {
		var w bytes.Buffer
		gzw := gzip.NewWriter(&w)
		//nolint:errcheck,gosec
		gzw.Write(data)
		//nolint:errcheck,gosec
		gzw.Close()
		return w.Bytes()
	}

	var (
		inData    []byte
		inString  string
		outString string
		err       error
	)

	BeforeEach(func() {
		inString = "Hello, world."
	})

	JustBeforeEach(func() {
		outString, err = util.TryToDecodeBase64Gzip(inData)
	})

	Context("plain-text", func() {
		BeforeEach(func() {
			inData = []byte(inString)
		})
		It("should return expected value", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(outString).To(Equal(inString))
		})

		Context("base64-encoded once", func() {
			BeforeEach(func() {
				inData = b64(1, inData)
			})
			It("should return expected value", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(outString).To(Equal(inString))
			})
		})

		Context("base64-encoded twice", func() {
			BeforeEach(func() {
				inData = b64(2, inData)
			})
			It("should return expected value", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(outString).To(Equal(inString))
			})
		})

		Context("base64-encoded thrice", func() {
			BeforeEach(func() {
				inData = b64(3, inData)
			})
			It("should return expected value", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(outString).To(Equal(inString))
			})
		})

		Context("gzipped", func() {
			BeforeEach(func() {
				inData = gz([]byte(inString))
			})

			It("should return expected value", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(outString).To(Equal(inString))
			})

			Context("base64-encoded once", func() {
				BeforeEach(func() {
					inData = b64(1, inData)
				})
				It("should return expected value", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(outString).To(Equal(inString))
				})
			})

			Context("base64-encoded twice", func() {
				BeforeEach(func() {
					inData = b64(2, inData)
				})
				It("should return expected value", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(outString).To(Equal(inString))
				})
			})

			Context("base64-encoded thrice", func() {
				BeforeEach(func() {
					inData = b64(3, inData)
				})
				It("should return expected value", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(outString).To(Equal(inString))
				})
			})
		})
	})
})

var _ = Describe("EncodeGzipBase64", func() {
	It("Encodes a string correctly", func() {
		input := "HelloWorld"
		output, err := util.EncodeGzipBase64(input)
		Expect(err).NotTo(HaveOccurred())
		// Decode the base64 output.
		decoded, err := base64.StdEncoding.DecodeString(output)
		Expect(err).NotTo(HaveOccurred())
		// Ungzip the decoded output.
		reader := bytes.NewReader(decoded)
		gzipReader, err := gzip.NewReader(reader)
		Expect(err).NotTo(HaveOccurred())
		defer Expect(gzipReader.Close()).To(Succeed())

		ungzipped, err := ioutil.ReadAll(gzipReader)
		Expect(err).NotTo(HaveOccurred())
		Expect(input).Should(Equal(string(ungzipped)))
	})
})
