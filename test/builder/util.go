// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

var (
	converter runtime.UnstructuredConverter = runtime.DefaultUnstructuredConverter
)

func ToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	content, err := converter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{}
	u.SetUnstructuredContent(content)
	return u, nil
}

func applyFeatureStateFnsToCRD(
	ctx context.Context,
	crd apiextensionsv1.CustomResourceDefinition,
	fns ...func(context.Context, apiextensionsv1.CustomResourceDefinition) apiextensionsv1.CustomResourceDefinition) apiextensionsv1.CustomResourceDefinition {

	for i := range fns {
		crd = fns[i](ctx, crd)
	}
	return crd
}

func updateImgRegStorageVersion(ctx context.Context, crd apiextensionsv1.CustomResourceDefinition) apiextensionsv1.CustomResourceDefinition {

	v1a1Idx := indexOfVersion(crd, "v1alpha1")
	v1a2Idx := indexOfVersion(crd, "v1alpha2")

	if pkgcfg.FromContext(ctx).Features.InventoryContentLibrary {
		if v1a1Idx >= 0 {
			crd.Spec.Versions[v1a1Idx].Storage = !(v1a2Idx >= 0) //nolint:staticcheck
			crd.Spec.Versions[v1a1Idx].Served = true
		}

		if v1a2Idx >= 0 {
			crd.Spec.Versions[v1a2Idx].Storage = true
			crd.Spec.Versions[v1a2Idx].Served = true
		}
	} else if v1a1Idx >= 0 {
		crd.Spec.Versions[v1a1Idx].Storage = true
		crd.Spec.Versions[v1a1Idx].Served = true

		if v1a2Idx >= 0 {
			var zeroVal apiextensionsv1.CustomResourceDefinitionVersion

			copy(crd.Spec.Versions[v1a2Idx:], crd.Spec.Versions[v1a2Idx+1:])
			crd.Spec.Versions[len(crd.Spec.Versions)-1] = zeroVal
			crd.Spec.Versions = crd.Spec.Versions[:len(crd.Spec.Versions)-1]
		}
	}

	return crd
}

// indexOfVersion returns the index of the specified schema version for a given
// CRD. This function is useful for writing the functions that are passed into
// the applyFeatureStateFnsToCRD function.
//
//nolint:unused
func indexOfVersion(
	crd apiextensionsv1.CustomResourceDefinition,
	version string) int {

	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Name == version {
			return i
		}
	}
	return -1
}

func LoadCRDs(rootFilePath string) ([]*apiextensionsv1.CustomResourceDefinition, error) {
	// Read the CRD files.
	files, err := os.ReadDir(rootFilePath)
	if err != nil {
		return nil, err
	}

	// Valid file extensions for CRDs.
	crdExts := sets.NewString(".json", ".yaml", ".yml")

	var out []*apiextensionsv1.CustomResourceDefinition
	for i := range files {
		if !crdExts.Has(filepath.Ext(files[i].Name())) {
			continue
		}

		docs, err := readDocuments(filepath.Join(rootFilePath, files[i].Name()))
		if err != nil {
			return nil, err
		}

		for _, d := range docs {
			var crd apiextensionsv1.CustomResourceDefinition
			if err = yaml.Unmarshal(d, &crd); err != nil {
				return nil, err
			}
			if crd.Spec.Names.Kind == "" || crd.Spec.Group == "" {
				continue
			}
			out = append(out, &crd)
		}
	}
	return out, nil
}

// readDocuments reads documents from file
// copied from https://github.com/kubernetes-sigs/controller-runtime/blob/5bf44d2ffd6201703508e11fbae74fcedc5ce148/pkg/envtest/crd.go#L434-L458
func readDocuments(fp string) ([][]byte, error) {
	//nolint:gosec
	b, err := os.ReadFile(fp)
	if err != nil {
		return nil, err
	}
	docs := [][]byte{}
	reader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(b)))
	for {
		// Read document
		doc, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
