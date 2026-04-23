package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

type VirtualMachineWebConsoleRequestYaml struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	VMName    string `json:"virtualMachineName"`
}

func GetV1A1WebConsoleRequestYaml(vmWebConsoleRequestYaml VirtualMachineWebConsoleRequestYaml) []byte {
	test := "test/e2e/fixtures/yaml/vmoperator/virtualmachinewebconsolerequests"
	webConsoleRequestYamlIn := fixtures.ReadFile(test, "webconsolerequests.yaml.in")
	webConsoleRequestYamlBytes, _ := ReadVirtualMachineWebConsoleRequestTemplate(vmWebConsoleRequestYaml, webConsoleRequestYamlIn)

	return webConsoleRequestYamlBytes
}

func GetVirtualMachineWebConsoleRequestYaml(vmWebConsoleRequestYaml VirtualMachineWebConsoleRequestYaml) []byte {
	test := "test/e2e/fixtures/yaml/vmoperator/virtualmachinewebconsolerequests"
	vmWebConsoleRequestYamlIn := fixtures.ReadFile(test, "virtualmachinewebconsolerequests.yaml.in")
	vmWebConsoleRequestYamlBytes, _ := ReadVirtualMachineWebConsoleRequestTemplate(vmWebConsoleRequestYaml, vmWebConsoleRequestYamlIn)

	return vmWebConsoleRequestYamlBytes
}

func ReadVirtualMachineWebConsoleRequestTemplate(virtualMachineWebConsoleRequestYaml VirtualMachineWebConsoleRequestYaml, input string) ([]byte, error) {
	tmpl := template.Must(template.New("vmWebConsoleRequest").Parse(input))
	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, virtualMachineWebConsoleRequestYaml)
	if err != nil {
		e2eframework.Failf("Failed executing virtualMachineWebConsoleRequestYaml template: %v", err)
	}

	return parsed.Bytes(), nil
}
