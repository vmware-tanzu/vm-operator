package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

type PodVMConfig struct {
	Metadata metadata `yaml:"metadata"`
}

type metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type PodVMTemplateConfig struct {
	Name                           string
	Namespace                      string
	PrivateKeySecretName           string `json:"privateKeySecretName,omitempty"`
	CustomUserPrivateKeySecretName string `json:"customUserPrivateKeySecretName,omitempty"`
	MemoryRequest                  string `json:"memoryRequest,omitempty"`
	MemoryLimit                    string `json:"memoryLimit,omitempty"`
	CPURequest                     string `json:"cpuRequest,omitempty"`
	CPULimit                       string `json:"cpuLimit,omitempty"`
}

func BuildPodVMYamlTemplate(inputConfig PodVMTemplateConfig) ([]byte, error) {
	podVMYaml := fixtures.ReadFile("test/e2e/fixtures/yaml/podvm", "podvm.yaml.in")

	inputTemplate := template.Must(template.New("podvm").Parse(podVMYaml))

	parsed := new(bytes.Buffer)

	err := inputTemplate.Execute(parsed, inputConfig)
	if err != nil {
		return nil, err
	}

	e2eframework.Logf("Generated PodVM yaml from template is:\n%s", parsed.String())

	return parsed.Bytes(), nil
}
