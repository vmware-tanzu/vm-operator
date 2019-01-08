package resources

import (
	"context"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
)

type ResourcePool struct {
	client       govmomi.Client
	finder       *find.Finder
	Name         string
	datacenter   *object.Datacenter
	ResourcePool *object.ResourcePool
}

func NewResourcePool(client govmomi.Client, datacenter *object.Datacenter, name string) (*ResourcePool, error) {
	return &ResourcePool{client: client, datacenter: datacenter, Name: name}, nil
}

func (rp *ResourcePool) Lookup() error {

	if rp.finder == nil {
		rp.finder = find.NewFinder(rp.client.Client, false)
	}

	rp.finder.SetDatacenter(rp.datacenter)

	var err error = nil
	rp.ResourcePool, err = rp.finder.ResourcePoolOrDefault(context.TODO(), rp.Name)
	if err != nil {
		return err
	}

	return nil
}
