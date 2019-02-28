/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import "vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"

type ResourceContext struct {
	Datacenter   *resources.Datacenter
	Folder       *resources.Folder
	ResourcePool *resources.ResourcePool
	Datastore    *resources.Datastore
}
