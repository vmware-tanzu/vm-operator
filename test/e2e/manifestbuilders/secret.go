package manifestbuilders

import (
	"bytes"
	"text/template"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

const (
	secretFilePath = "test/e2e/fixtures/yaml/vmoperator/secret"
)

type Secret struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

func GetSecretYamlCloudConfig(secret Secret) []byte {
	secretYamlIn := fixtures.ReadFile(secretFilePath, "secretCloudConfig.yaml.in")
	secretYaml := ReadSecretTemplate(secret, secretYamlIn)

	return secretYaml
}

func GetSecretYamlInlineCloudInitData(secret Secret) []byte {
	secretYamlIn := fixtures.ReadFile(secretFilePath, "secretInlineCloudInitData.yaml.in")
	secretYaml := ReadSecretTemplate(secret, secretYamlIn)

	return secretYaml
}

func GetSecretYamlInlineSysprepData(secret Secret) []byte {
	secretYamlIn := fixtures.ReadFile(secretFilePath, "secretInlineSysprepData.yaml.in")
	secretYaml := ReadSecretTemplate(secret, secretYamlIn)

	return secretYaml
}

func GetSecretYamlOvfEnv(secret Secret) []byte {
	secretYamlIn := fixtures.ReadFile(secretFilePath, "secretOvfEnv.yaml.in")
	secretYaml := ReadSecretTemplate(secret, secretYamlIn)

	return secretYaml
}

func GetSecretYamlVAppConfig(secret Secret) []byte {
	secretYamlIn := fixtures.ReadFile(secretFilePath, "secretEmpty.yaml.in")
	secretYaml := ReadSecretTemplate(secret, secretYamlIn)
	// Add stringData in separate to keep its template text as is.
	stringDataYaml := fixtures.ReadFileBytes(secretFilePath, "vAppStringData.yaml")

	return append(secretYaml, stringDataYaml...)
}

func GetSecretYamlSysprepConfig(secret Secret) []byte {
	secretYamlIn := fixtures.ReadFile(secretFilePath, "secretEmpty.yaml.in")
	secretYaml := ReadSecretTemplate(secret, secretYamlIn)
	// Add stringData in separate to keep its template text as is.
	stringDataYaml := fixtures.ReadFileBytes(secretFilePath, "sysprepStringData.yaml")

	return append(secretYaml, stringDataYaml...)
}

func ReadSecretTemplate(secret Secret, input string) []byte {
	tmpl := template.Must(template.New("secret").Parse(input))
	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, secret)
	if err != nil {
		e2eframework.Failf("Failed executing secret template: %v", err)
	}

	return parsed.Bytes()
}
