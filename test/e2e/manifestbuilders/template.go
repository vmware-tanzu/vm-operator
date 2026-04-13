// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"bytes"
	"text/template"

	e2eframework "k8s.io/kubernetes/test/e2e/framework"

	"github.com/vmware-tanzu/vm-operator/test/e2e/fixtures"
)

func GetYaml[T any](yamlBuilder T, path, file, newTemplateName string) []byte {
	yamlIn := fixtures.ReadFile(path, file)
	tmpl := template.Must(template.New(newTemplateName).Parse(yamlIn))
	parsed := new(bytes.Buffer)

	err := tmpl.Execute(parsed, yamlBuilder)
	if err != nil {
		e2eframework.Failf("Failed executing %s : %v", newTemplateName, err)
	}

	return parsed.Bytes()
}
