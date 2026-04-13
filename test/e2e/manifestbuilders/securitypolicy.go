package manifestbuilders

import (
	"bytes"
	"text/template"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

const (
	securitypolicyFilePath = "test/e2e/fixtures/yaml/vmoperator/securitypolicy"
)

type SecurityPolicy struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

// Util function to return a Namespaced SecurityPolicy yaml from a templatized fixture.
func GetSecurityPolicyYaml(securitypolicy SecurityPolicy) []byte {
	securitypolicyYamlIn := fixtures.ReadFile(securitypolicyFilePath, "securitypolicy.yaml.in")
	securitypolicyYaml := ReadSecurityPolicy(securitypolicy, securitypolicyYamlIn)

	return securitypolicyYaml
}

func ReadSecurityPolicy(securitypolicy SecurityPolicy, input string) []byte {
	tmpl := template.Must(template.New("securitypolicy").Parse(input))

	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, securitypolicy)
	if err != nil {
		e2eframework.Failf("Failed executing securitypolicy template: %v", err)
	}

	return parsed.Bytes()
}
