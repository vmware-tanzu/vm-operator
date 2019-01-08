package resources

import (
	"context"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
)

/*
import "github.com/vmware/govmomi/object"

func Folder(kind string) (*object.Folder, error) {

	// RootFolder, no dc required
	if kind == "/" {
		client, err := Client()
		if err != nil {
			return nil, err
		}

		folder := object.NewRootFolder(client.Client)
		return folder, nil
	}

	dc, err := Datacenter()
	if err != nil {
		return nil, err
	}

	folders, err := dc.Folders(context.TODO())
	if err != nil {
		return nil, err
	}

	switch kind {
	case "vm":
		flag.folder = folders.VmFolder
	case "host":
		flag.folder = folders.HostFolder
	case "datastore":
		flag.folder = folders.DatastoreFolder
	case "network":
		flag.folder = folders.NetworkFolder
	default:
		panic(kind)
	}

	return flag.folder, nil
}
*/

type Folder struct {
	client govmomi.Client
	name   string
	dc     *object.Datacenter
	Folder *object.Folder
	finder *find.Finder
}

func NewFolder(client govmomi.Client, dc *object.Datacenter, name string) (*Folder, error) {
	return &Folder{client: client, dc: dc, name: name}, nil
}

func (f *Folder) Lookup() error {

	if f.finder == nil {
		f.finder = find.NewFinder(f.client.Client, false)
	}

	folders, err := f.dc.Folders(context.TODO())
	if err != nil {
		return err
	}

	f.Folder = folders.VmFolder
	return nil
}


