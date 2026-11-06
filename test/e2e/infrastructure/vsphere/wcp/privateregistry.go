// Copyright (c) 2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

import (
	"context"

	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
)

type PrivateRegistryInput struct {
	Kubeconfig       string
	WCPClient        WorkloadManagementAPI
	RegistryName     string
	Hostname         string
	Port             int
	Username         string
	Password         string
	CertificateChain string
	DefaultRegistry  bool
}

func CreatePrivateRegistry(ctx context.Context, input PrivateRegistryInput) ContainerImageRegistryInfo {
	wcpClient := input.WCPClient
	svKubeConfig := input.Kubeconfig

	// Get supervisor ID from kubeconfig
	supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, svKubeConfig)
	Expect(supervisorID).NotTo(BeEmpty(), "Unable to determine supervisor ID")

	// Delete existing registry if it exists
	existingRegistry, err := wcpClient.GetContainerImageRegistry(supervisorID, input.RegistryName)
	if err == nil {
		// Registry exists, delete it first
		err = wcpClient.DeleteContainerImageRegistry(supervisorID, existingRegistry.ID)
		Expect(err).NotTo(HaveOccurred(), "Failed to delete existing container image registry %s", input.RegistryName)
	}

	registry := ContainerImageRegistry{
		Name:            input.RegistryName,
		DefaultRegistry: input.DefaultRegistry,
		ImageRegistry: ImageRegistry{
			Hostname:         input.Hostname,
			Port:             input.Port,
			Username:         input.Username,
			Password:         input.Password,
			CertificateChain: input.CertificateChain,
		},
	}

	registryInfo, err := wcpClient.CreateContainerImageRegistry(supervisorID, registry)
	Expect(err).NotTo(HaveOccurred(), "Failed to create container image registry %s", input.RegistryName)

	return registryInfo
}

func GetPrivateRegistry(ctx context.Context, input PrivateRegistryInput) ContainerImageRegistryInfo {
	wcpClient := input.WCPClient
	svKubeConfig := input.Kubeconfig

	// Get supervisor ID from kubeconfig
	supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, svKubeConfig)
	Expect(supervisorID).NotTo(BeEmpty(), "Unable to determine supervisor ID")

	registryInfo, err := wcpClient.GetContainerImageRegistry(supervisorID, input.RegistryName)
	Expect(err).NotTo(HaveOccurred(), "Failed to get container image registry %s", input.RegistryName)

	return registryInfo
}

func DeletePrivateRegistry(ctx context.Context, input PrivateRegistryInput) error {
	wcpClient := input.WCPClient
	svKubeConfig := input.Kubeconfig

	// Get supervisor ID from kubeconfig
	supervisorID := vcenter.GetSupervisorIDFromKubeconfig(ctx, svKubeConfig)

	registryInfo, err := wcpClient.GetContainerImageRegistry(supervisorID, input.RegistryName)
	if err != nil {
		return err
	}

	return wcpClient.DeleteContainerImageRegistry(supervisorID, registryInfo.ID)
}
