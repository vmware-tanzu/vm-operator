// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

const (
	AnnotationServiceExternalTrafficPolicyKey = "virtualmachineservice.vmoperator.vmware.com/service.externalTrafficPolicy"
	AnnotationServiceHealthCheckNodePortKey   = "virtualmachineservice.vmoperator.vmware.com/service.healthCheckNodePort"

	// AnnotationServiceExternalDNSHostnameAlphaKey is the alpha external-dns
	// annotation used to specify the FQDN(s) for a Service.
	AnnotationServiceExternalDNSHostnameAlphaKey = "external-dns.alpha.kubernetes.io/hostname"

	// AnnotationServiceExternalDNSHostnameKey is the stable form of
	// AnnotationServiceExternalDNSHostnameAlphaKey. Both are supported.
	AnnotationServiceExternalDNSHostnameKey = "external-dns.kubernetes.io/hostname"
)
