// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

// decryptK8PwdScript prints the Supervisor control plane credentials stored in VCDB. It runs on vCenter.
const decryptK8PwdScript = "/usr/lib/vmware-wcp/decryptK8Pwd.py"

// supervisorPasswordRecord is one Supervisor reported by decryptK8PwdScript.
type supervisorPasswordRecord struct {
	InstanceID string `json:"instance_id"`
	Cluster    string `json:"cluster"`
	// IP is "<ipv4>,<ipv6>" on dual-stack testbeds.
	IP string `json:"ip"`
	// Username is absent on older vCenters.
	Username string `json:"username"`
	Password string `json:"pwd"`
}

// supervisorPasswords is the output of decryptK8PwdScript.
type supervisorPasswords struct {
	// KubeInstSupervisors are legacy KubeInstance Supervisors.
	KubeInstSupervisors []supervisorPasswordRecord `json:"kubeInstSupervisors"`
	// SelfManagedSupervisors are self-managed Supervisors. They are not supported yet.
	SelfManagedSupervisors []supervisorPasswordRecord `json:"selfManagedSupervisors"`
}

// getSupervisorPasswords runs decryptK8PwdScript on vCenter and parses its --json output, falling back to the
// plain-text output on vCenters whose script has no --json flag.
func getSupervisorPasswords(vcSSHCommandRunner e2essh.SSHCommandRunner) (supervisorPasswords, error) {
	// The runner combines stdout and stderr, and the script can print warnings to stderr.
	out, err := vcSSHCommandRunner.RunCommand("python3 " + decryptK8PwdScript + " --json 2>/dev/null")
	if err == nil {
		supervisors, parseErr := parseSupervisorPasswordsJSON(out)
		if parseErr == nil {
			return supervisors, nil
		}
		err = parseErr
	}
	e2eframework.Logf("%s --json is unavailable (%v), parsing its plain-text output", decryptK8PwdScript, err)

	out, err = vcSSHCommandRunner.RunCommand("python3 " + decryptK8PwdScript)
	if err != nil {
		return supervisorPasswords{}, fmt.Errorf("error running %s over ssh: %w", decryptK8PwdScript, err)
	}
	return parseSupervisorPasswordsText(out), nil
}

// parseSupervisorPasswordsJSON parses the --json output of decryptK8PwdScript.
func parseSupervisorPasswordsJSON(out []byte) (supervisorPasswords, error) {
	start := bytes.IndexByte(out, '{')
	if start < 0 {
		return supervisorPasswords{}, errors.New("output is not JSON")
	}
	var supervisors supervisorPasswords
	if err := json.Unmarshal(out[start:], &supervisors); err != nil {
		// Do not wrap err: a decode error can quote the output, which holds passwords.
		return supervisorPasswords{}, errors.New("output is not valid JSON")
	}
	return supervisors, nil
}

// parseSupervisorPasswordsText parses the plain-text output of decryptK8PwdScript. Each Supervisor is a block of
// "<label>: <value>" lines ended by a dashed line, and only legacy KubeInstance Supervisors have a Cluster line.
func parseSupervisorPasswordsText(out []byte) supervisorPasswords {
	var supervisors supervisorPasswords
	var record supervisorPasswordRecord
	var inRecord bool

	appendRecord := func() {
		if !inRecord {
			return
		}
		if record.Cluster != "" {
			supervisors.KubeInstSupervisors = append(supervisors.KubeInstSupervisors, record)
		} else {
			supervisors.SelfManagedSupervisors = append(supervisors.SelfManagedSupervisors, record)
		}
		record, inRecord = supervisorPasswordRecord{}, false
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "----------") {
			appendRecord()
			continue
		}
		label, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch label {
		case "Supervisor ID":
			record.InstanceID = value
		case "Cluster":
			record.Cluster = value
		case "IP":
			record.IP = value
		case "Username":
			record.Username = value
		case "PWD":
			record.Password = value
		default:
			continue
		}
		inRecord = true
	}
	appendRecord()

	return supervisors
}

// connectionDetails returns the SSH connection details of the legacy KubeInstance Supervisor whose IP matches
// managementIP.
func (s supervisorPasswords) connectionDetails(managementIP string) (ControlPlaneVMConnectionDetails, error) {
	reportedIPs := make([]string, 0, len(s.KubeInstSupervisors))
	for _, record := range s.KubeInstSupervisors {
		reportedIPs = append(reportedIPs, record.IP)
		for _, ip := range strings.Split(record.IP, ",") {
			if strings.TrimSpace(ip) != managementIP {
				continue
			}
			if record.Password == "" {
				return ControlPlaneVMConnectionDetails{}, fmt.Errorf("%s reported no password for Supervisor %s", decryptK8PwdScript, managementIP)
			}
			username := record.Username
			if username == "" {
				username = APIServerSSHUser
			}
			return ControlPlaneVMConnectionDetails{
				ManagementAPIServerIP: managementIP,
				Username:              username,
				Password:              record.Password,
			}, nil
		}
	}

	if len(s.KubeInstSupervisors) == 0 && len(s.SelfManagedSupervisors) > 0 {
		return ControlPlaneVMConnectionDetails{}, fmt.Errorf("%s reported only self-managed Supervisors (%d), which are not supported yet",
			decryptK8PwdScript, len(s.SelfManagedSupervisors))
	}
	return ControlPlaneVMConnectionDetails{}, fmt.Errorf("%s reported no Supervisor with IP %s (legacy Supervisor IPs: %v, self-managed Supervisors: %d)",
		decryptK8PwdScript, managementIP, reportedIPs, len(s.SelfManagedSupervisors))
}
