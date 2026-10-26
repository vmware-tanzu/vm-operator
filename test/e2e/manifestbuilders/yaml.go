// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// ToYAML renders objs as a "---"-separated, multi-document manifest suitable
// for kubectl. Each object must have its TypeMeta set. The status and
// metadata.creationTimestamp fields are omitted since they are never part of
// a desired-state manifest.
func ToYAML(objs ...ctrlclient.Object) []byte {
	GinkgoHelper()
	out, err := toYAML(objs...)
	Expect(err).ToNot(HaveOccurred(), "Failed to render YAML")
	return out
}

func toYAML(objs ...ctrlclient.Object) ([]byte, error) {
	var buf bytes.Buffer

	for i, obj := range objs {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %T %s to unstructured: %w",
				obj, ctrlclient.ObjectKeyFromObject(obj), err)
		}

		delete(u, "status")
		unstructured.RemoveNestedField(u, "metadata", "creationTimestamp")

		doc, err := yaml.Marshal(u)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal %T %s to YAML: %w",
				obj, ctrlclient.ObjectKeyFromObject(obj), err)
		}

		if i > 0 {
			buf.WriteString("---\n")
		}
		buf.Write(doc)
	}

	return buf.Bytes(), nil
}
