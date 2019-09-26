package resources

import (
	"context"
	"testing"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
)

// Test GetStatus(), verify we get a non-empty host
func TestVirtualMachineGetStatus(t *testing.T) {
	simulator.Test(func(ctx context.Context, c *vim25.Client) {
		// Pick a random VM
		svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
		// Set the IP address directly, as this field is not populated by default
		svm.Guest.IpAddress = "10.0.0.1"

		obj := object.NewVirtualMachine(c, svm.Reference())

		vm, err := NewVMFromObject(obj)
		if err != nil {
			t.Fatal(err)
		}

		vmstatus, err := vm.GetStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if vmstatus.Host == "" {
			t.Fatal(err)
		}
	})
}
