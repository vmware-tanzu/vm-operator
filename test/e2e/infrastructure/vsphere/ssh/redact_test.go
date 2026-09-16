// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ssh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

var _ = Describe("RedactSensitiveFlags", func() {
	DescribeTable("redacting CLI command strings",
		func(cmd, expected string) {
			Expect(ssh.RedactSensitiveFlags(cmd)).To(Equal(expected))
		},

		Entry("dcli +password flag",
			`dcli com vmware vcenter namespaces instances get --namespace vmsvc-e2e-4bl2l6 +formatter json +username 'Administrator@vsphere.local' +password 'OWJBcz6y**F2trzH'`,
			`dcli com vmware vcenter namespaces instances get --namespace vmsvc-e2e-4bl2l6 +formatter json +username 'Administrator@vsphere.local' +password '***'`,
		),

		Entry("dir-cli --login/--password admin credentials",
			`/usr/lib/vmware-vmafd/bin/dir-cli user find-by-name --account 'joe' --login 'Administrator@vsphere.local' --password 'AdminPass123'`,
			`/usr/lib/vmware-vmafd/bin/dir-cli user find-by-name --account 'joe' --login 'Administrator@vsphere.local' --password '***'`,
		),

		Entry("multiple distinct password flags in one command",
			`/usr/lib/vmware-vmafd/bin/dir-cli user create --account 'joe' --user-password 'JoePass456' --first-name 'joe First name' --last-name 'joe Last name' --login 'Administrator@vsphere.local' --password 'AdminPass123'`,
			`/usr/lib/vmware-vmafd/bin/dir-cli user create --account 'joe' --user-password '***' --first-name 'joe First name' --last-name 'joe Last name' --login 'Administrator@vsphere.local' --password '***'`,
		),

		Entry("hyphenated flag name containing password (image-registry-password)",
			`namespacemanagement/supervisors containerimageregistries create --name 'reg1' --supervisor 'sv1' --image-registry-password 'RegPass789' --image-registry-username 'reguser' --image-registry-hostname 'host'`,
			`namespacemanagement/supervisors containerimageregistries create --name 'reg1' --supervisor 'sv1' --image-registry-password '***' --image-registry-username 'reguser' --image-registry-hostname 'host'`,
		),

		Entry("flag named secret",
			`some-tool --client-secret 'topsecretvalue' --client-id 'abc123'`,
			`some-tool --client-secret '***' --client-id 'abc123'`,
		),

		Entry("flag named pwd",
			`some-tool --pwd 'shortformpassword' --user 'bob'`,
			`some-tool --pwd '***' --user 'bob'`,
		),

		Entry("is case-insensitive on the flag name",
			`some-tool --PASSWORD 'MixedCaseSecret'`,
			`some-tool --PASSWORD '***'`,
		),

		Entry("does not redact non-sensitive flags",
			`dcli com vmware vcenter namespaces instances get --namespace 'vmsvc-e2e-4bl2l6' +formatter 'json' +username 'Administrator@vsphere.local'`,
			`dcli com vmware vcenter namespaces instances get --namespace 'vmsvc-e2e-4bl2l6' +formatter 'json' +username 'Administrator@vsphere.local'`,
		),

		Entry("empty command string is a no-op",
			``,
			``,
		),
	)
})

var _ = Describe("RedactSensitiveOutput", func() {
	DescribeTable("redacting command output strings",
		func(output, expected string) {
			Expect(ssh.RedactSensitiveOutput(output)).To(Equal(expected))
		},

		Entry("decryptK8Pwd.py style Cluster/IP/PWD block",
			"Cluster: domain-c9:21d4eaa0-4c40-4aab-9a64-f45afa4b59ef\nIP: 10.144.29.123\nPWD: v4YQ_$/9eXgu2uIv\n",
			"Cluster: domain-c9:21d4eaa0-4c40-4aab-9a64-f45afa4b59ef\nIP: 10.144.29.123\nPWD: ***\n",
		),

		Entry("is case-insensitive on the key name",
			"password: SuperSecret1\n",
			"password: ***\n",
		),

		Entry("does not redact unrelated lines",
			"Cluster: domain-c9\nIP: 10.144.29.123\n",
			"Cluster: domain-c9\nIP: 10.144.29.123\n",
		),

		Entry("empty output string is a no-op",
			``,
			``,
		),
	)
})
