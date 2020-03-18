// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"path/filepath"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/pkg/client/clientset_generated/clientset/typed/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var testEnv *envtest.Environment

const namespace string = "default"

// List VirtualMachines in a target cluster to stdout using the generated client
func main() {
	fmt.Printf("Starting test env...\n")
	testClient, err := startTestEnv()
	if err != nil {
		panic(err)
	}
	defer func() {
		fmt.Printf("Stopping test env...\n")
		testEnv.Stop()
	}()

	vmOpClient, err := getVmopClient(testClient)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Populating test env...\n")
	err = populateTestEnv(vmOpClient, "test-vm1")
	if err != nil {
		panic(err)
	}
	err = populateTestEnv(vmOpClient, "test-vm2")
	if err != nil {
		panic(err)
	}

	fmt.Printf("Listing VMs:\n")
	vmList, err := vmOpClient.VirtualMachines(namespace).List(v1.ListOptions{})
	if err != nil {
		panic(err)
	}
	for _, vm := range vmList.Items {
		fmt.Printf("- %s\n", vm.GetName())
	}
}

// Get a vm-operator-api client from the generated clientset
func getVmopClient(client *rest.Config) (*vmopv1alpha1.VmoperatorV1alpha1Client, error) {
	return vmopv1alpha1.NewForConfig(client)
}

func startTestEnv() (*rest.Config, error) {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
	}

	return testEnv.Start()
}

func populateTestEnv(client *vmopv1alpha1.VmoperatorV1alpha1Client, name string) error {
	_, err := client.VirtualMachines(namespace).Create(&v1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	})
	return err
}
