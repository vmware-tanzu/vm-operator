// +build integration

/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere_test

import (
	"fmt"
	stdlog "log"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	vcSim  *integration.VcSimInstance
	err    error
	config *vsphere.VSphereVmProviderConfig
)

func TestVSphereIntegrationProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "vSphere Provider Suite")
}

var _ = BeforeSuite(func() {
	var err error
	stdlog.Print("setting up integration test env..")
	vcSim = integration.NewVcSimInstance()
	config, err = integration.SetupEnv(vcSim)
	Expect(err).NotTo(HaveOccurred())
	Expect(config).ShouldNot(Equal(nil))

	os.Setenv(vsphere.EnvContentLibApiWaitSecs, "1")
})

var _ = AfterSuite(func() {
	integration.CleanupEnv(vcSim)
})

func getVMClassInstance(vmName, namespace string) *vmoperatorv1alpha1.VirtualMachineClass {
	return &vmoperatorv1alpha1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-class", vmName),
		},
		Spec: vmoperatorv1alpha1.VirtualMachineClassSpec{
			Hardware: vmoperatorv1alpha1.VirtualMachineClassHardware{
				Cpus:   4,
				Memory: resource.MustParse("1Mi"),
			},
			Policies: vmoperatorv1alpha1.VirtualMachineClassPolicies{
				Resources: vmoperatorv1alpha1.VirtualMachineClassResources{
					Requests: vmoperatorv1alpha1.VirtualMachineResourceSpec{
						Cpu:    resource.MustParse("1000Mi"),
						Memory: resource.MustParse("100Mi"),
					},
					Limits: vmoperatorv1alpha1.VirtualMachineResourceSpec{
						Cpu:    resource.MustParse("2000Mi"),
						Memory: resource.MustParse("200Mi"),
					},
				},
			},
		},
	}
}

func getVirtualMachineInstance(name, namespace, imageName, className string) *vmoperatorv1alpha1.VirtualMachine {
	return &vmoperatorv1alpha1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSpec{
			ImageName:  imageName,
			ClassName:  className,
			PowerState: vmoperatorv1alpha1.VirtualMachinePoweredOn,
			Ports:      []vmoperatorv1alpha1.VirtualMachinePort{},
			VmMetadata: &vmoperatorv1alpha1.VirtualMachineMetadata{
				Transport: "ExtraConfig",
			},
		},
	}
}

func getVirtualMachineSetResourcePolicy(name, namespace string) *vmoperatorv1alpha1.VirtualMachineSetResourcePolicy {
	return &vmoperatorv1alpha1.VirtualMachineSetResourcePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-resourcepolicy", name),
		},
		Spec: vmoperatorv1alpha1.VirtualMachineSetResourcePolicySpec{
			ResourcePool: vmoperatorv1alpha1.ResourcePoolSpec{
				Name: fmt.Sprintf("%s-resourcepool", name),
				Reservations: vmoperatorv1alpha1.VirtualMachineResourceSpec{},
				Limits:       vmoperatorv1alpha1.VirtualMachineResourceSpec{},
			},
			Folder: vmoperatorv1alpha1.FolderSpec{
				Name: fmt.Sprintf("%s-folder", name),
			},
		},
	}
}
