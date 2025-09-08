// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path"

	"github.com/vmware-tanzu/vm-operator/test/testutil"
	"github.com/vmware/govmomi/ovf"
	"sigs.k8s.io/yaml"
)

func main() {
	f, err := os.Open(path.Join(
		testutil.GetRootDirOrDie(),
		"test", "builder", "testdata",
		"images", os.Args[1]))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ovfEnvelope, err := ovf.Unmarshal(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, err := yaml.Marshal(ovfEnvelope)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}
