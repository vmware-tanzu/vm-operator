package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

type EncryptionClass struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	KeyProvider string `json:"keyProvider"`
	KeyID       string `json:"keyID,omitempty"`
}

func GetEncryptionClassYaml(class EncryptionClass) []byte {
	input := `
apiVersion: encryption.vmware.com/v1alpha1
kind: EncryptionClass
metadata:
  namespace: {{.Namespace}}
  name: {{.Name}}
spec:
  keyProvider: {{.KeyProvider}}
  keyID: "{{.KeyID}}"
`

	tmpl := template.Must(template.New("EncryptionClass").Parse(input))
	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, class)
	if err != nil {
		e2eframework.Failf("Failed executing EncryptionClass template: %v", err)
	}

	return parsed.Bytes()
}
