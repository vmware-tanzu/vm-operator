package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
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

func TestIsCustomizationPending(t *testing.T) {
	t.Run("with no pending customizations exist", func(t *testing.T) {
		simulator.Test(func(ctx context.Context, c *vim25.Client) {
			svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
			obj := object.NewVirtualMachine(c, svm.Reference())
			vm, err := NewVMFromObject(obj)
			if err != nil {
				t.Fatal(err)
			}

			customizationPending, err := vm.IsGuestCustomizationPending(ctx)
			if customizationPending {
				t.Fatal(errors.New("VM should not have a customization pending"))
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("with pending customizations exist", func(t *testing.T) {
		simulator.Test(func(ctx context.Context, c *vim25.Client) {
			svm := simulator.Map.Any("VirtualMachine").(*simulator.VirtualMachine)
			toolsPkgFileNameKey := "tools.deployPkg.fileName"
			toolsPkgFileNameValue := "test-file-name"
			svm.Config.ExtraConfig = append(svm.Config.ExtraConfig, &types.OptionValue{Key: toolsPkgFileNameKey, Value: toolsPkgFileNameValue})

			obj := object.NewVirtualMachine(c, svm.Reference())

			vm, err := NewVMFromObject(obj)
			if err != nil {
				t.Fatal(err)
			}

			customizationPending, err := vm.IsGuestCustomizationPending(ctx)
			if !customizationPending {
				t.Fatal(errors.New("VM should have a customization pending"))
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	})

}
