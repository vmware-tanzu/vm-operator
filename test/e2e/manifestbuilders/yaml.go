// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manifestbuilders

import (
	"bytes"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// MarshalObjects marshals one or more controller-runtime objects to a
// multi-document YAML byte slice suitable for piping to kubectl.
// It uses the scheme to populate TypeMeta on each object before marshaling.
func MarshalObjects(scheme *runtime.Scheme, objs ...ctrlclient.Object) ([]byte, error) {
	var buf bytes.Buffer
	for i, obj := range objs {
		gvks, _, err := scheme.ObjectKinds(obj)
		if err != nil {
			return nil, fmt.Errorf("getting GVK for %T: %w", obj, err)
		}
		obj.GetObjectKind().SetGroupVersionKind(gvks[0])

		b, err := yaml.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("marshaling %T: %w", obj, err)
		}
		if i > 0 {
			buf.WriteString("---\n")
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}

// MustMarshalObjects is like MarshalObjects but calls e2eframework.Failf on error.
func MustMarshalObjects(scheme *runtime.Scheme, objs ...ctrlclient.Object) []byte {
	b, err := MarshalObjects(scheme, objs...)
	if err != nil {
		e2eframework.Failf("MustMarshalObjects: %v", err)
	}
	return b
}
