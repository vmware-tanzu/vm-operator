//go:build vmop_tools
// +build vmop_tools

// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package tools manages the version of tooling used to build this project.
package tools

import (
	_ "github.com/AlekSi/gocov-xml"
	_ "github.com/axw/gocov/gocov"
	_ "github.com/elastic/crd-ref-docs"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/onsi/ginkgo/ginkgo"
	_ "github.com/wadey/gocovmerge"
	_ "golang.org/x/vuln/cmd/govulncheck"
	_ "k8s.io/code-generator"
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kind"
	_ "sigs.k8s.io/kubebuilder/v3/cmd"
	_ "sigs.k8s.io/kustomize/kustomize/v5"
)
