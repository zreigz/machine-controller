/*
Copyright 2019 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vsphere

import (
	"context"
	"fmt"
	"path"

	"github.com/golang/glog"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	localTempDir     = "/tmp"
	metaDataTemplate = `instance-id: {{ .InstanceID}}
	local-hostname: {{ .Hostname }}`
)

var (
	diskMoveType = types.VirtualMachineRelocateDiskMoveOptionsMoveAllDiskBackingsAndConsolidate

	diskUUIDEnabled = true
)

func createClonedVM(ctx context.Context, vmName string, config *Config, session *Session, operatingSystem providerconfig.OperatingSystem, userdata string) (*object.VirtualMachine, error) {
	templateVM, err := session.Finder.VirtualMachine(ctx, config.TemplateVMName)
	if err != nil {
		return nil, fmt.Errorf("failed to get template vm: %v", err)
	}

	targetVMFolder, err := session.Finder.Folder(ctx, config.Folder)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM folder: %v", err)
	}

	targetVMPath := path.Join(targetVMFolder.InventoryPath, vmName)

	vmDevices, err := templateVM.Device(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices of tempalte VM: %v", err)
	}

	var deviceSpecs []types.BaseVirtualDeviceConfigSpec
	// Ubuntu wont boot with attached floppy device, because it tries to write to it
	// which fails, because the floppy device does not contain a floppy disk
	// Upstream issue: https://bugs.launchpad.net/cloud-images/+bug/1573095
	deviceSpecs = append(deviceSpecs, removeFloppyDevices(vmDevices)...)

	// Userdata handling:
	// CoreOS / ContainerLinux -> Set userdata via vApp config
	// Ubuntu, CentOS: Create & Upload an ISO containing the userdata
	var vAppPropertySpecs []types.VAppPropertySpec
	if operatingSystem == providerconfig.OperatingSystemCoreos {
		coreOSVAppSpecs, err := getCoreOSOptions(ctx, templateVM, userdata)
		if err != nil {
			return nil, fmt.Errorf("failed to build vApp options for CoreOS. Make sure you imported the correct OVA: %v", err)
		}
		vAppPropertySpecs = append(vAppPropertySpecs, coreOSVAppSpecs...)
	} else {
		datastoreISOPath, err := generateAndUploadUserDataISO(ctx, session, userdata, vmName)
		if err != nil {
			return nil, err
		}

		cdRomSpecs, err := getCDRomSpecs(vmDevices, datastoreISOPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get cdrom specifications for the userdata disk: %v", err)
		}

		deviceSpecs = append(deviceSpecs, cdRomSpecs...)
	}

	if config.DiskSizeGB != nil {
		diskSpec, err := getDiskSpec(vmDevices, *config.DiskSizeGB)
		if err != nil {
			return nil, fmt.Errorf("failed to get disk specification: %v", err)
		}
		deviceSpecs = append(deviceSpecs, diskSpec)
	}

	if config.VMNetName != "" || len(config.Networks) > 0 {
		networks := sets.NewString(config.Networks...)
		networks.Insert(config.VMNetName)
		networkSpecs, err := GetNetworkSpecs(ctx, session, vmDevices, networks.List())
		if err != nil {
			return nil, fmt.Errorf("failed to get network specifications: %v", err)
		}
		deviceSpecs = append(deviceSpecs, networkSpecs...)
	}

	cloneSpec := types.VirtualMachineCloneSpec{
		Location: types.VirtualMachineRelocateSpec{
			Datastore:    types.NewReference(session.Datastore.Reference()),
			DiskMoveType: string(diskMoveType),
			Folder:       types.NewReference(targetVMFolder.Reference()),
		},
		Config: &types.VirtualMachineConfigSpec{
			NumCPUs:      config.CPUs,
			MemoryMB:     config.MemoryMB,
			DeviceChange: deviceSpecs,
			VAppConfig: &types.VmConfigSpec{
				Property: vAppPropertySpecs,
			},
			Flags: &types.VirtualMachineFlagInfo{
				DiskUuidEnabled: &diskUUIDEnabled,
			},
		},
	}

	glog.V(2).Infof("Cloning the template VM %q to %q...", templateVM.InventoryPath, targetVMPath)
	cloneTask, err := templateVM.Clone(ctx, targetVMFolder, vmName, cloneSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to clone template vm: %v", err)
	}
	if err := cloneTask.Wait(ctx); err != nil {
		return nil, fmt.Errorf("error when waiting for result of clone task: %v", err)
	}
	glog.V(2).Infof("Successfully cloned the template VM %q to %q", templateVM.InventoryPath, targetVMPath)

	vm, err := session.Finder.VirtualMachine(ctx, vmName)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual machine object after cloning: %v", err)
	}

	return vm, nil
}
