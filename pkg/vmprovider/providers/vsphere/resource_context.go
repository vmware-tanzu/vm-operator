package vsphere

type ResourceContext struct {
	datacenter   *Datacenter
	folder       *Folder
	resourcePool *ResourcePool
	datastore    *Datastore
}
