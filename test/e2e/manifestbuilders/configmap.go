package manifestbuilders

import (
	"bytes"
	"text/template"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

const configMapFixtureBasePath = "test/e2e/fixtures/yaml/vmoperator/configmap"

type ConfigMap struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

func GetConfigMapYamlGOSC(configMap ConfigMap) []byte {
	configMapYamlIn := fixtures.ReadFile(configMapFixtureBasePath, "configmapgosc.yaml.in")
	configMapYaml, _ := ReadConfigMapTemplate(configMap, configMapYamlIn)

	return configMapYaml
}

func GetConfigMapYamlOvfEnv(configMap ConfigMap) []byte {
	configMapYamlIn := fixtures.ReadFile(configMapFixtureBasePath, "configmapOvfEnv.yaml.in")
	configMapYaml, _ := ReadConfigMapTemplate(configMap, configMapYamlIn)

	return configMapYaml
}

func GetConfigMapYamlVAppConfig(configMap ConfigMap) []byte {
	configMapYamlIn := fixtures.ReadFile(configMapFixtureBasePath, "configmapvapp.yaml.in")
	configMapYaml, _ := ReadConfigMapTemplate(configMap, configMapYamlIn)
	// templating cannot be parsed here only to keep its text
	dataYaml := fixtures.ReadFileBytes(configMapFixtureBasePath, "vappData.yaml")

	return append(configMapYaml, dataYaml...)
}

func ReadConfigMapTemplate(configMap ConfigMap, input string) ([]byte, error) {
	tmpl := template.Must(template.New("configmap").Parse(input))
	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, configMap)
	if err != nil {
		e2eframework.Failf("Failed executing configmap template: %v", err)
	}

	return parsed.Bytes(), nil
}
