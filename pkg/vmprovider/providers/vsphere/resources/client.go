package resources

import (
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/vim25/soap"
	"golang.org/x/net/context"
)

func NewClient(ctx context.Context, url string) (*govmomi.Client, error) {
	// Parse URL from string
	u, err := soap.ParseURL(url)
	if err != nil {
		return nil, err
	}

	return govmomi.NewClient(ctx, u, true)
}
