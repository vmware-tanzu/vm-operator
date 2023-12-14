// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validate_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/cloudinit/validate"
)

var _ = Describe("CloudConfig ValidateCloudConfig", func() {
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
			RunCmd: []json.RawMessage{
				[]byte("ls /"),
				[]byte(`[ "ls", "-a", "-l", "/" ]`),
				[]byte("- echo\n- \"hello, world.\""),
			},
			WriteFiles: []vmopv1cloudinit.WriteFile{
				{
					Path:    "/file1",
					Content: []byte("single-line string"),
				},
				{
					Path:    "/file2",
					Content: []byte("|\n  multi-line\n  string"),
				},
				{
					Path:    "/file3",
					Content: []byte("name: \"my-bootstrap-data\"\nkey: \"file3-content\""),
				},
			},
		}
	})

	JustBeforeEach(func() {
		errs = validate.CloudConfig(
			field.NewPath("spec").Child("bootstrap").Child("cloudInit"),
			cloudConfig)
	})

	When("The CloudConfig is valid", func() {
		It("Should not return any errors", func() {
			Expect(errs).To(HaveLen(0))
		})
	})

	When("The CloudConfig's second runcmd is an invalid value", func() {
		BeforeEach(func() {
			cloudConfig.RunCmd[1] = []byte("obj:\n  field1: value1")
		})
		It("Should return a single error", func() {
			Expect(errs).To(HaveLen(1))
			Expect(errs.ToAggregate().Error()).To(Equal(
				`spec.bootstrap.cloudInit.cloudConfig.runcmds[1]: Invalid value: "obj:\n  field1: value1": value must be a string or list of strings`))
		})

		When("The CloudConfig's first file content is an invalid value", func() {
			BeforeEach(func() {
				cloudConfig.WriteFiles[0].Content = []byte(`[ "ls", "-a", "-l", "/" ]`)
			})
			It("Should return two errors", func() {
				Expect(errs).To(HaveLen(2))
				Expect(errs.ToAggregate().Error()).To(Equal(
					`[spec.bootstrap.cloudInit.cloudConfig.runcmds[1]: Invalid value: "obj:\n  field1: value1": value must be a string or list of strings, ` +
						`spec.bootstrap.cloudInit.cloudConfig.write_files[/file1]: Invalid value: "[ \"ls\", \"-a\", \"-l\", \"/\" ]": value must be a string, multi-line string, or SecretKeySelector]`))
			})
		})
	})
})
