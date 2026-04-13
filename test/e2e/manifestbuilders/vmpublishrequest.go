package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

type VirtualMachinePublishRequestSource struct {
	Name string `json:"name,omitempty"`
}

type VirtualMachinePublishRequestTarget struct {
	Item     VirtualMachinePublishRequestTargetItem     `json:"item,omitempty"`
	Location VirtualMachinePublishRequestTargetLocation `json:"location,omitempty"`
}

type VirtualMachinePublishRequestTargetItem struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type VirtualMachinePublishRequestTargetLocation struct {
	Name string `json:"name,omitempty"`
}

type VirtualMachinePublishRequestYaml struct {
	Namespace   string                             `json:"namespace,omitempty"`
	Name        string                             `json:"name,omitempty"`
	Labels      map[string]string                  `json:"labels,omitempty"`
	Annotations map[string]string                  `json:"annotations,omitempty"`
	Source      VirtualMachinePublishRequestSource `json:"source,omitempty"`
	Target      VirtualMachinePublishRequestTarget `json:"target,omitempty"`
}

func GetVirtualMachinePublishRequestYaml(vmPublishRequestYaml VirtualMachinePublishRequestYaml) []byte {
	test := "test/e2e/fixtures/yaml/vmoperator/virtualmachinepublishrequests"
	vmPublishRequestYamlIn := fixtures.ReadFile(test, "singlevirtualmachinepublishrequest.yaml.in")
	vmPublishRequestYamlBytes, _ := ReadVirtualMachinePublishRequestTemplate(vmPublishRequestYaml, vmPublishRequestYamlIn)

	return vmPublishRequestYamlBytes
}

func ReadVirtualMachinePublishRequestTemplate(virtualMachinePublishRequestYaml VirtualMachinePublishRequestYaml, input string) ([]byte, error) {
	tmpl := template.Must(template.New("vmPublishRequest").Parse(input))
	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, virtualMachinePublishRequestYaml)
	if err != nil {
		e2eframework.Failf("Failed executing virtualMachinePublishRequestYaml template: %v", err)
	}

	return parsed.Bytes(), nil
}
