package vsphere

import (
	"context"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
)

type Datastore struct {
	client     govmomi.Client
	name       string
	datacenter *object.Datacenter
	Datastore  *object.Datastore
	finder     *find.Finder
}

func NewDatastore(client govmomi.Client, datacenter *object.Datacenter, name string) (*Datastore, error) {
	return &Datastore{client: client, datacenter: datacenter, name: name}, nil
}

func (ds *Datastore) Lookup() error {

	if ds.finder == nil {
		ds.finder = find.NewFinder(ds.client.Client, false)
	}

	ds.finder.SetDatacenter(ds.datacenter)

	var err error = nil
	ds.Datastore, err = ds.finder.DatastoreOrDefault(context.TODO(), ds.name)

	if err != nil {
		return err
	}

	return nil

}
