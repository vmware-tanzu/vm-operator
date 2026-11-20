package util

import (
	"fmt"
	"strings"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
)

func IsFSSEnabled(sshCommandRunner e2essh.SSHCommandRunner, fss string) (bool, string) {
	output, err := sshCommandRunner.RunCommand(fmt.Sprintf("python /usr/sbin/feature-state-wrapper.py %s", fss))
	if err != nil || strings.TrimSpace(string(output)) != "enabled" {
		msg := fmt.Sprintf("%s FSS is not enabled on the testbed", fss)
		if err != nil {
			msg = fmt.Sprintf("failed to get FSS for %s", fss)
			e2eframework.Logf("%s, err: %s", msg, err.Error())
		}

		return false, msg
	}

	return true, ""
}
