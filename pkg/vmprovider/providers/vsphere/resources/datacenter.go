package resources

import (
	"context"
	"github.com/golang/glog"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
)

type Datacenter struct {
	client     govmomi.Client
	name       string
	Datacenter *object.Datacenter
	finder     *find.Finder
}

func NewDatacenter(client govmomi.Client, name string) (*Datacenter, error) {
	return &Datacenter{client: client, name: name}, nil
}

func (dc *Datacenter) Lookup() error {

	if dc.finder == nil {
		dc.finder = find.NewFinder(dc.client.Client, false)
	}

	// Datacenter is not required (ls command for example).
	// Set for relative func if dc flag is given or
	// if there is a single (default) Datacenter
	ctx := context.TODO()
	var err error = nil
	if dc.Datacenter, err = dc.finder.Datacenter(ctx, dc.name); err != nil {
		return err
	}

	dc.finder.SetDatacenter(dc.Datacenter)

	return nil
}

func (dc *Datacenter) ListVms(ctx context.Context, path string) ([]*object.VirtualMachine, error) {
	glog.Infof("Listing VMs for folder: %s", dc.name)

	vms, err := dc.finder.VirtualMachineList(ctx, path)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError:
			glog.Infof("No Vms were found")
			return vms, nil
		default:
			return nil, err
		}
	}

	return vms, nil
}
