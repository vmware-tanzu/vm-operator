// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	fakeIP       = "192.0.2.10"
	fakePassword = "fake-Pass:1"
)

var _ = Describe("decryptK8Pwd.py output", func() {
	Describe("parseSupervisorPasswordsJSON", func() {
		It("parses legacy Supervisors with a username", func() {
			supervisors, err := parseSupervisorPasswordsJSON([]byte(`{
  "kubeInstSupervisors": [
    {"instance_id": "sv-1", "cluster": "domain-c9:abc", "ip": "192.0.2.10", "username": "root", "pwd": "fake-Pass:1"}
  ],
  "selfManagedSupervisors": []
}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(supervisors.KubeInstSupervisors).To(ConsistOf(supervisorPasswordRecord{
				InstanceID: "sv-1", Cluster: "domain-c9:abc", IP: fakeIP, Username: "root", Password: fakePassword,
			}))
			Expect(supervisors.SelfManagedSupervisors).To(BeEmpty())
		})

		It("skips text printed before the JSON", func() {
			supervisors, err := parseSupervisorPasswordsJSON([]byte("Warning: lstool exited with code 1\n" +
				`{"kubeInstSupervisors": [{"cluster": "domain-c9:abc", "ip": "192.0.2.10", "pwd": "fake-Pass:1"}]}`))
			Expect(err).ToNot(HaveOccurred())
			Expect(supervisors.KubeInstSupervisors).To(HaveLen(1))
		})

		It("fails on plain-text output", func() {
			_, err := parseSupervisorPasswordsJSON([]byte("Cluster: domain-c9:abc\nIP: 192.0.2.10\nPWD: fake-Pass:1\n"))
			Expect(err).To(MatchError("output is not JSON"))
		})

		It("does not include the output in the error for invalid JSON", func() {
			_, err := parseSupervisorPasswordsJSON([]byte(`{"kubeInstSupervisors": [{"pwd": "fake-Pass:1"`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).ToNot(ContainSubstring(fakePassword))
		})
	})

	Describe("parseSupervisorPasswordsText", func() {
		separator := "------------------------------------------------------------\n\n"

		It("parses the layout without a Username line", func() {
			supervisors := parseSupervisorPasswordsText([]byte("Read key from file\n\nConnected to PSQL\n\n" +
				"Cluster: domain-c9:abc\nIP: 192.0.2.10\nPWD: fake-Pass:1\n" + separator))
			Expect(supervisors.KubeInstSupervisors).To(ConsistOf(supervisorPasswordRecord{
				Cluster: "domain-c9:abc", IP: fakeIP, Password: fakePassword,
			}))
		})

		It("parses the layout with Supervisor ID and Username lines", func() {
			supervisors := parseSupervisorPasswordsText([]byte("Read key from file\n\nConnected to PSQL\n\n" +
				"Supervisor ID: sv-1\nCluster: domain-c9:abc\nIP: 192.0.2.10\nUsername: root\nPWD: fake-Pass:1\n" + separator +
				"Supervisor ID: sv-2\nIP: 192.0.2.20\nUsername: vmware-system-user\nPWD: fake-Pass:2\n" + separator))
			Expect(supervisors.KubeInstSupervisors).To(ConsistOf(supervisorPasswordRecord{
				InstanceID: "sv-1", Cluster: "domain-c9:abc", IP: fakeIP, Username: "root", Password: fakePassword,
			}))
			Expect(supervisors.SelfManagedSupervisors).To(ConsistOf(supervisorPasswordRecord{
				InstanceID: "sv-2", IP: "192.0.2.20", Username: "vmware-system-user", Password: "fake-Pass:2",
			}))
		})
	})

	Describe("connectionDetails", func() {
		legacy := supervisorPasswordRecord{Cluster: "domain-c9:abc", IP: fakeIP, Password: fakePassword}

		It("uses the reported username", func() {
			record := legacy
			record.Username = "admin"
			details, err := supervisorPasswords{KubeInstSupervisors: []supervisorPasswordRecord{record}}.connectionDetails(fakeIP)
			Expect(err).ToNot(HaveOccurred())
			Expect(details).To(Equal(ControlPlaneVMConnectionDetails{
				ManagementAPIServerIP: fakeIP, Username: "admin", Password: fakePassword,
			}))
		})

		It("defaults the username to root", func() {
			details, err := supervisorPasswords{KubeInstSupervisors: []supervisorPasswordRecord{legacy}}.connectionDetails(fakeIP)
			Expect(err).ToNot(HaveOccurred())
			Expect(details.Username).To(Equal(APIServerSSHUser))
		})

		It("matches either address of a dual-stack Supervisor", func() {
			record := legacy
			record.IP = "192.0.2.10,2001:db8::10"
			supervisors := supervisorPasswords{KubeInstSupervisors: []supervisorPasswordRecord{record}}
			for _, ip := range []string{fakeIP, "2001:db8::10"} {
				details, err := supervisors.connectionDetails(ip)
				Expect(err).ToNot(HaveOccurred())
				Expect(details.ManagementAPIServerIP).To(Equal(ip))
				Expect(details.Password).To(Equal(fakePassword))
			}
		})

		It("picks the Supervisor with the matching IP", func() {
			other := supervisorPasswordRecord{Cluster: "domain-c8:def", IP: "192.0.2.20", Password: "fake-Pass:2"}
			details, err := supervisorPasswords{KubeInstSupervisors: []supervisorPasswordRecord{other, legacy}}.connectionDetails(fakeIP)
			Expect(err).ToNot(HaveOccurred())
			Expect(details.Password).To(Equal(fakePassword))
		})

		It("does not match on an IP prefix", func() {
			record := legacy
			record.IP = "192.0.2.100"
			_, err := supervisorPasswords{KubeInstSupervisors: []supervisorPasswordRecord{record}}.connectionDetails(fakeIP)
			Expect(err).To(HaveOccurred())
		})

		It("reports the known IPs but no passwords when nothing matches", func() {
			_, err := supervisorPasswords{KubeInstSupervisors: []supervisorPasswordRecord{legacy}}.connectionDetails("192.0.2.99")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fakeIP))
			Expect(err.Error()).ToNot(ContainSubstring(fakePassword))
		})

		It("reports when only self-managed Supervisors exist", func() {
			selfManaged := supervisorPasswordRecord{InstanceID: "sv-2", IP: fakeIP, Username: "vmware-system-user", Password: fakePassword}
			_, err := supervisorPasswords{SelfManagedSupervisors: []supervisorPasswordRecord{selfManaged}}.connectionDetails(fakeIP)
			Expect(err).To(MatchError(ContainSubstring("only self-managed Supervisors")))
			Expect(err.Error()).ToNot(ContainSubstring(fakePassword))
		})

		It("fails when the matching Supervisor has no password", func() {
			record := legacy
			record.Password = ""
			_, err := supervisorPasswords{KubeInstSupervisors: []supervisorPasswordRecord{record}}.connectionDetails(fakeIP)
			Expect(err).To(MatchError(ContainSubstring("no password")))
		})
	})
})
