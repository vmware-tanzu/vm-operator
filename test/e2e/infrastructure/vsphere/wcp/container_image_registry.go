// Copyright (c) 2020-2024 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

type ImageRegistry struct {
	Hostname         string `json:"hostname"`
	Port             int    `json:"port"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	CertificateChain string `json:"certificate_chain"`
}

type ContainerImageRegistry struct {
	Name            string        `json:"name"`
	DefaultRegistry bool          `json:"default_registry"`
	ImageRegistry   ImageRegistry `json:"image_registry"`
}

type ContainerImageRegistryInfo struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	DefaultRegistry bool          `json:"default_registry"`
	ImageRegistry   ImageRegistry `json:"image_registry"`
}
