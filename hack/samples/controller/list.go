// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var testEnv *envtest.Environment

const namespace string = "default"

// List VirtualMachines in a target cluster to stdout using a controller client
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
	vmList := v1alpha1.VirtualMachineList{}
	err = vmOpClient.List(context.TODO(), &vmList)
	if err != nil {
		panic(err)
	}
	for _, vm := range vmList.Items {
		fmt.Printf("- %s\n", vm.GetName())
	}
}

// Get a vm-operator-api client from the generated clientset
func getVmopClient(config *rest.Config) (ctrlClient.Client, error) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	client, err := ctrlClient.New(config, ctrlClient.Options{
		Scheme: scheme,
	})
	return client, err
}

func startTestEnv() (*rest.Config, error) {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
	}

	return testEnv.Start()
}

func populateTestEnv(client ctrlClient.Client, name string) error {
	newVM := v1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return client.Create(context.TODO(), &newVM)
}
