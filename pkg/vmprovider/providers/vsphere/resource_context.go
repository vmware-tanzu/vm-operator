/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package vsphere

import "vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"

type ResourceContext struct {
	datacenter   *resources.Datacenter
	folder       *resources.Folder
	resourcePool *resources.ResourcePool
	datastore    *resources.Datastore
}
