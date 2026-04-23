// Copyright (c) 2021-2024 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

// Datastore represents VMODL output for datastore get on a vCenter.
type Datastore struct {
	Datastore string `json:"datastore"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	FreeSpace int    `json:"free_space"`
	Capacity  int    `json:"capacity"`
}

// BackingInfo represents datastore backing used for storing VMImages of the Content Library.
type BackingInfo struct {
	DatastoreID string `json:"datastore_id"`
	Type        string `json:"type"`
}

// StorageBackingInfo represents storage backing used for storing VMImages of the Content Library.
type StorageBackingInfo struct {
	StorageBackings []BackingInfo
}

// SecurityPolicyInfo represents datastore backing used for storing VMImages of the Content Library.
type SecurityPolicyInfo struct {
	PolicyID      string            `json:"policy"`
	Name          string            `json:"name"`
	ItemTypeRules map[string]string `json:"item_type_rules"`
}

// SubscriptionInfo represents subscribed Content Library related information.
type SubscriptionInfo struct {
	AuthenticationMethod string `json:"authentication_method"`
	Password             string `json:"password"`
	SubscriptionURL      string `json:"subscription_url"`
	AutomaticSyncEnabled bool   `json:"automatic_sync_enabled"`
	OnDemand             bool   `json:"on_demand"`
}

type PublishInfo struct {
	AuthenticationMethod string `json:"authentication_method"`
	UserName             string `json:"user_name"`
	Password             string `json:"password"`
	Published            bool   `json:"published"`
	PublishURL           string `json:"publish_url"`
	PersistJSONEnabled   bool   `json:"persist_json_enabled"`
}

// ContentLibraryInfo represents VMODL output for content library get on a vCenter.
type ContentLibraryInfo struct {
	StorageBackingInfo

	CreationTime     string           `json:"creation_time"`
	LastModifiedTime string           `json:"last_modified_time"`
	ServerGUID       string           `json:"server_guid"`
	Description      string           `json:"description"`
	Type             string           `json:"type"`
	Version          string           `json:"version"`
	SubscriptionInfo SubscriptionInfo `json:"subscription_info"`
	PublishInfo      PublishInfo      `json:"publish_info"`
	Name             string           `json:"name"`
	ID               string           `json:"id"`
}

// ContentLibraryItemInfo represents VMODL output for a Content Library Item on a vCenter.
type ContentLibraryItemInfo struct {
	CreationTime       string                 `json:"creation_time"`
	LastModifiedTime   string                 `json:"last_modified_time"`
	Description        string                 `json:"description"`
	Type               ContentLibraryItemType `json:"type"`
	Version            string                 `json:"version"`
	ContentVersion     string                 `json:"content_version"`
	LibraryID          string                 `json:"library_id"`
	Size               int                    `json:"size"`
	Cached             bool                   `json:"cached"`
	Name               string                 `json:"name"`
	ID                 string                 `json:"id"`
	SourceID           string                 `json:"source_id"`
	SecurityCompliance bool                   `json:"security_compliance"`
}

type ProbeResult struct {
	Status        string   `json:"status"`
	SSLThumbprint string   `json:"ssl_thumbprint"`
	ErrorMessages []string `json:"error_messages"`
}

// ContentLibraryItemType represents different types of supported Content Library Objects.
type ContentLibraryItemType string

const (
	OVF ContentLibraryItemType = "ovf"
	ISO ContentLibraryItemType = "iso"
)
