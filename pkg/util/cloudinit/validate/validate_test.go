// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validate_test

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	cloudinitvalidate "github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit/validate"
)

var _ = Describe("Validate CloudConfigJSONRawMessage", func() {
	var (
		cloudConfig vmopv1cloudinit.CloudConfig
		errs        field.ErrorList
	)

	BeforeEach(func() {
		errs = nil
		cloudConfig = vmopv1cloudinit.CloudConfig{
			Users: []vmopv1cloudinit.User{
				{
					Name: "bob.wilson",
					HashedPasswd: &common.SecretKeySelector{
						Name: "my-bootstrap-data",
						Key:  "cloud-init-user-bob.wilson-hashed_passwd",
					},
				},
			},
			RunCmd: []byte(`["ls /",["ls","-a","-l","/"],["echo","hello, world."]]`),
			WriteFiles: []vmopv1cloudinit.WriteFile{
				{
					Path:    "/file1",
					Content: []byte(`"single-line string"`),
				},
				{
					Path:    "/file2",
					Content: []byte(`"multi-line\nstring"`),
				},
				{
					Path:    "/file3",
					Content: []byte(`{"name":"my-bootstrap-data","key":"file3-content"}`),
				},
			},
		}
	})

	JustBeforeEach(func() {
		errs = cloudinitvalidate.CloudConfigJSONRawMessage(
			field.NewPath("spec").Child("bootstrap").Child("cloudInit"),
			cloudConfig)
	})

	When("The CloudConfig is valid", func() {
		It("Should not return any errors", func() {
			Expect(errs).To(HaveLen(0))
		})
	})

	When("The CloudConfig runcmd value is invalid", func() {
		BeforeEach(func() {
			cloudConfig.RunCmd = []byte(`{"foo":"bar"}`)
		})
		It("Should return a single error", func() {
			Expect(errs).To(HaveLen(1))
			Expect(errs.ToAggregate().Error()).To(Equal(
				`spec.bootstrap.cloudInit.cloudConfig.runcmds: Invalid value: "{\"foo\":\"bar\"}": value must be a list`))
		})
	})

	When("The CloudConfig's second runcmd is an invalid value", func() {
		BeforeEach(func() {
			cloudConfig.RunCmd = []byte(`["ls /",{"foo":"bar"},["echo","hello, world."]]`)
		})
		It("Should return a single error", func() {
			Expect(errs).To(HaveLen(1))
			Expect(errs.ToAggregate().Error()).To(Equal(
				`spec.bootstrap.cloudInit.cloudConfig.runcmds[1]: Invalid value: "{\"foo\":\"bar\"}": value must be a string or list of strings`))
		})

		When("The CloudConfig's first file content is an invalid value", func() {
			BeforeEach(func() {
				cloudConfig.WriteFiles[0].Content = []byte(`[ "ls", "-a", "-l", "/" ]`)
			})
			It("Should return two errors", func() {
				Expect(errs).To(HaveLen(2))
				Expect(errs.ToAggregate().Error()).To(Equal(
					`[spec.bootstrap.cloudInit.cloudConfig.runcmds[1]: Invalid value: "{\"foo\":\"bar\"}": value must be a string or list of strings, ` +
						`spec.bootstrap.cloudInit.cloudConfig.write_files[/file1]: Invalid value: "[ \"ls\", \"-a\", \"-l\", \"/\" ]": value must be a string, multi-line string, or SecretKeySelector]`))
			})
		})
	})
})

var _ = Describe("Validate CloudConfigYAML", func() {
	var (
		err             error
		cloudConfigYAML string
	)

	JustBeforeEach(func() {
		err = cloudinitvalidate.CloudConfigYAML(cloudConfigYAML)
	})

	When("The CloudConfig is valid", func() {
		BeforeEach(func() {
			data, err := os.ReadFile("./testdata/valid-cloud-config-1.yaml")
			Expect(err).ToNot(HaveOccurred())
			Expect(data).ToNot(HaveLen(0))
			cloudConfigYAML = string(data)
		})
		It("Should not return an error", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("The CloudConfig is invalid", func() {
		BeforeEach(func() {
			data, err := os.ReadFile("./testdata/invalid-cloud-config-1.yaml")
			Expect(err).ToNot(HaveOccurred())
			Expect(data).ToNot(HaveLen(0))
			cloudConfigYAML = string(data)
		})
		It("Should return an error", func() {
			Expect(err).To(HaveOccurred())
		})
	})
})
