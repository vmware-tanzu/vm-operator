// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package messages

const (
	UpdatingImmutableFieldsNotAllowed = "updates to immutable fields are not allowed"
	TypeNotSpecified                  = "spec.type must be specified"
	PortsNotSpecified                 = "spec.ports must be specified"
	SelectorNotSpecified              = "spec.selector must be specified"
	NameNotDNSComplaint               = "metadata.name %s does not conform to the kubernetes DNS_LABEL rules"
)
