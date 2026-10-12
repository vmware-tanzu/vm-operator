// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

// NamespaceAccess is part of the response returned by wcpsvc when getting the
//
//	privileges assigned to a given principal on a namespace.
//
// It's technically used in the VMODL definition to assign privileges as well, but the
//
//	WorkloadManagementAPI interface in gce2e simplifies those methods to take in
//	the principal, namespace (entity), and access type (privilege), to look more
//	like other VC authorization manager methods.
type NamespaceAccess struct {
	Role        string `json:"role"`
	SubjectType string `json:"subject_type"`
}

// SubjectType defines whether the subject principal privileges is a user or a group.
type SubjectType string

// AccessType controls the kind of privileges a user receives on a supervisor namespace.
type AccessType string

const (
	UserSubjectType  SubjectType = "USER"
	GroupSubjectType SubjectType = "GROUP"
	EditAccessType   AccessType  = "EDIT"
	ViewAccessType   AccessType  = "VIEW"
)

// Principal represents an entity to be granted privileges (for instance, a user or a group).
type Principal struct {
	Type   SubjectType `json:"subject_type"`
	Name   string      `json:"name"`
	Domain string      `json:"domain"`
}
