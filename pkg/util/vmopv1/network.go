// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	// ErrInvalidHostName is returned by ValidateHostAndDomainName if
	// either the provided host name does not adhere to RFC-1123.
	ErrInvalidHostName = "host name must adhere to RFC-1123"

	// ErrInvalidDomainName is returned by ValidateHostAndDomainName if
	// either the provided domain name does not adhere to RFC-1123.
	ErrInvalidDomainName = "domain name must adhere to RFC-1123"

	// ErrInvalidHostNameIPWithDomainName is returned by
	// ValidateHostAndDomainName if the provided host name is an IP4 or IP6
	// address and the provided domain name is non-empty.
	ErrInvalidHostNameIPWithDomainName = "host name may not be IP address when domain name is non-empty" //nolint:gosec

	// ErrInvalidHostNameWindows is returned by ValidateHostAndDomainName if the
	// provided host name exceeds 15 characters and the guest is Windows.
	ErrInvalidHostNameWindows = "host name may not exceed 15 characters for Windows"

	// ErrInvalidHostNameWindowsDigits is returned by ValidateHostAndDomainName
	// if the provided host name consists only of digits and the guest is
	// Windows. A Windows computer name may not be composed entirely of digits.
	ErrInvalidHostNameWindowsDigits = "host name may not consist only of digits for Windows"

	// ErrInvalidHostAndDomainName is returned by ValidateHostAndDomainName if
	// the provided host name and domain name, combined, exceed 255 characters.
	ErrInvalidHostAndDomainName = "host name and domain name combined exceed 255 characters"
)

// ValidateHostAndDomainName returns nil if the provided host and domain names
// are valid; otherwise an error is returned. If the isWindows parameter is
// true, then the host name may not exceed 15 characters.
func ValidateHostAndDomainName(vm vmopv1.VirtualMachine) *field.Error {

	// When nil network spec, validate the name of the VM as the host name.
	var metadataNameField = field.NewPath("metadata", "name")
	if vm.Spec.Network == nil && !util.IsValidHostName(vm.Name) {
		return field.Invalid(metadataNameField, vm.Name, ErrInvalidHostName)
	}

	var (
		hostName        string
		domainName      string
		networkField    = field.NewPath("spec", "network")
		hostNameField   = networkField.Child("hostName")
		domainNameField = networkField.Child("domainName")
	)

	if vm.Spec.Network != nil {
		hostName = vm.Spec.Network.HostName
		domainName = vm.Spec.Network.DomainName
	}

	if hostName == "" {
		hostName = vm.Name
		hostNameField = metadataNameField
	}

	if !util.IsValidHostName(hostName) {
		return field.Invalid(
			hostNameField, hostName, ErrInvalidHostName)
	}

	if domainName != "" && net.ParseIP(hostName) != nil {
		return field.Invalid(
			hostNameField, hostName, ErrInvalidHostNameIPWithDomainName)
	}

	isWindowsVM := strings.HasPrefix(vm.Spec.GuestID, "win") ||
		(vm.Spec.Bootstrap != nil && vm.Spec.Bootstrap.Sysprep != nil)

	if isWindowsVM {
		if len(hostName) > 15 {
			return field.Invalid(
				hostNameField, hostName, ErrInvalidHostNameWindows)
		}

		// A Windows computer name may not be composed entirely of digits.
		// Guest customization rejects such a name, so reject it here instead
		// of letting the VM fail to customize later.
		if isAllDigits(hostName) {
			return field.Invalid(
				hostNameField, hostName, ErrInvalidHostNameWindowsDigits)
		}
	}

	if domainName != "" && !util.IsValidDomainName(domainName) {
		return field.Invalid(
			domainNameField, domainName, ErrInvalidDomainName)
	}

	if hostName != "" && domainName != "" {
		if fqdn := hostName + "." + domainName; len(fqdn) > 255 {
			return field.Invalid(
				networkField, fqdn, ErrInvalidHostAndDomainName)
		}
	}

	return nil
}

// isAllDigits reports whether s is non-empty and consists only of ASCII
// digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
