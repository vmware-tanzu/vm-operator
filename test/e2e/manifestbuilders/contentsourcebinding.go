package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

// Util function to return a ContentSourceBinding yaml from a templatized fixture.
func GetContentSourceBindingYaml(namespace, contentSourceName string) []byte {
	test := "test/e2e/fixtures/yaml/vmoperator/contentsources"
	classBindingYamlIn := fixtures.ReadFile(test, "contentsourcebindings.yaml.in")
	contentSourceBindingYaml, _ := ReadContentSourceBinding(namespace, contentSourceName, classBindingYamlIn)

	return contentSourceBindingYaml
}

func ReadContentSourceBinding(ns, contentSourceName, input string) ([]byte, error) {
	tmpl := template.Must(template.New("contentsourcebinding").Parse(input))

	config := struct {
		Namespace string
		Name      string
	}{
		ns,
		contentSourceName,
	}

	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, config)
	if err != nil {
		e2eframework.Failf("Failed executing template: %v", err)
	}

	return parsed.Bytes(), nil
}
