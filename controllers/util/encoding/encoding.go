// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package encoding

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// DecodeYAML unmarshals a YAML document or multidoc YAML as unstructured
// objects, placing each decoded object into a channel.
func DecodeYAML(data []byte) (<-chan *unstructured.Unstructured, <-chan error) {
	var (
		chanErr        = make(chan error)
		chanObj        = make(chan *unstructured.Unstructured)
		multidocReader = utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	)

	go func() {
		defer close(chanErr)
		defer close(chanObj)

		// Iterate over the data until Read returns io.EOF. Every successful
		// read returns a complete YAML document.
		for {
			buf, err := multidocReader.Read()
			if err != nil {
				if err == io.EOF {
					return
				}
				chanErr <- errors.Wrap(err, "failed to read yaml data")
				return
			}

			// Do not use this YAML doc if it is unkind.
			var typeMeta runtime.TypeMeta
			if err := yaml.Unmarshal(buf, &typeMeta); err != nil {
				continue
			}
			if typeMeta.Kind == "" {
				continue
			}

			// Define the unstructured object into which the YAML document will be
			// unmarshaled.
			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{},
			}

			// Unmarshal the YAML document into the unstructured object.
			if err := yaml.Unmarshal(buf, &obj.Object); err != nil {
				chanErr <- errors.Wrap(err, "failed to unmarshal yaml data")
				return
			}

			// Place the unstructured object into the channel.
			chanObj <- obj
		}
	}()

	return chanObj, chanErr
}
