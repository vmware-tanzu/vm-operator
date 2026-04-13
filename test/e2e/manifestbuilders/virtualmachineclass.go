package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

// Util function to return a Namespaced VirtualMachineClass yaml from a templatized fixture.
func GetVirtualMachineClassYaml(namespace, vmClassName string) []byte {
	test := "test/e2e/fixtures/yaml/vmoperator/virtualmachineclasses"
	classYamlIn := fixtures.ReadFile(test, "vmclass.yaml.in")
	vmClassYaml, _ := ReadVirtualMachineClassBinding(namespace, vmClassName, classYamlIn)

	return vmClassYaml
}

func ReadVirtualMachineClass(ns, vmClassName, input string) ([]byte, error) {
	tmpl := template.Must(template.New("vmclass").Parse(input))

	config := struct {
		Namespace string
		Name      string
	}{
		ns,
		vmClassName,
	}

	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, config)
	if err != nil {
		e2eframework.Failf("Failed executing template: %v", err)
	}

	return parsed.Bytes(), nil
}
