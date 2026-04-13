package vmservice

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	. "github.com/onsi/gomega"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"k8s.io/kubernetes/test/e2e/framework"
)

const supportBundleImage = "wcp-gc-docker-local.packages.vcfd.broadcom.net/utils/tkc-support-bundler:v1.2.0"

func runSupportBundleCLI(vm *vmopv1a3.VirtualMachine, svKubeconfig, testName string) error {
	output := fmt.Sprintf("${SUPPORT_BUNDLE_DIR}/%s-%s", vm.Name, testName)

	err := runCommand("mkdir -p " + output)
	if err != nil {
		return fmt.Errorf("unable to make output directory: %w", err)
	}
	// Note: tkc-support-bundler is specific to TKC. If a VM-specific tool exists, replace this command.
	cmdString := fmt.Sprintf("tkc-support-bundler create -k %s -o %s -c %s -n %s -i %s", svKubeconfig, output, vm.Name, vm.Namespace, supportBundleImage)
	framework.Logf("Running %s", cmdString)

	err = runCommand(cmdString)
	if err != nil {
		return fmt.Errorf("unable to collect support-bundle: %w", err)
	}

	return nil
}

func CollectSupportBundle(vm *vmopv1a3.VirtualMachine, svKubeconfig, testName string) {
	testName = strings.ReplaceAll(testName, " ", "-")

	supportBundleErr := runSupportBundleCLI(vm, svKubeconfig, testName)
	if supportBundleErr != nil {
		framework.Logf("Cannot collect support bundle \n %s\n", supportBundleErr.Error())
	}
}

func runCommand(cmdString string) error {
	cmd := exec.Command("/bin/sh", "-c", cmdString) //nolint:gosec // G204: E2E helper runs shell for support-bundle CLI
	stdout, err := cmd.StdoutPipe()
	Expect(err).ToNot(HaveOccurred())
	stderr, err := cmd.StderrPipe()
	Expect(err).ToNot(HaveOccurred())

	framework.Logf("Starting command %s", cmdString)

	err = cmd.Start()
	Expect(err).NotTo(HaveOccurred())

	stdoutBytes, _ := io.ReadAll(stdout)
	if len(stdoutBytes) > 0 {
		framework.Logf("Command standard output: %v\n", string(stdoutBytes))
	}

	stderrBytes, _ := io.ReadAll(stderr)
	if len(stderrBytes) > 0 {
		framework.Logf("Command standard error: %v\n", string(stderrBytes))
	}

	framework.Logf("Waiting for the command to finish")

	err = cmd.Wait()
	Expect(err).NotTo(HaveOccurred())

	return err
}
