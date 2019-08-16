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
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	CoreOSUserDataKey         = "guestinfo.coreos.config.data"
	CoreOSUserDataEncodingKey = "guestinfo.coreos.config.data.encoding"
)

func getCoreOSOptions(ctx context.Context, vm *object.VirtualMachine, userdata string) ([]types.VAppPropertySpec, error) {
	userdataBase64 := base64.StdEncoding.EncodeToString([]byte(userdata))

	// The properties describing userdata will already exist in the CoreOS VM template.
	// In order to overwrite them, we need to specify their numeric Key values,
	// which we'll extract from that template.
	var mvm mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"config", "config.vAppConfig", "config.vAppConfig.property"}, &mvm); err != nil {
		return nil, fmt.Errorf("failed to extract vapp properties for coreos: %v", err)
	}

	var propertySpecs []types.VAppPropertySpec
	if mvm.Config.VAppConfig.GetVmConfigInfo() == nil {
		return nil, errors.New("no VM config found in template")
	}

	properties := mvm.Config.VAppConfig.GetVmConfigInfo().Property
	for _, item := range properties {
		switch item.Id {
		case CoreOSUserDataKey:
			propertySpecs = append(propertySpecs, types.VAppPropertySpec{
				ArrayUpdateSpec: types.ArrayUpdateSpec{
					Operation: types.ArrayUpdateOperationEdit,
				},
				Info: &types.VAppPropertyInfo{
					Key:   item.Key,
					Id:    item.Id,
					Value: userdataBase64,
				},
			})
		case CoreOSUserDataEncodingKey:
			propertySpecs = append(propertySpecs, types.VAppPropertySpec{
				ArrayUpdateSpec: types.ArrayUpdateSpec{
					Operation: types.ArrayUpdateOperationEdit,
				},
				Info: &types.VAppPropertyInfo{
					Key:   item.Key,
					Id:    item.Id,
					Value: "base64",
				},
			})
		}
	}
	if len(propertySpecs) != 2 {
		return nil, fmt.Errorf("could not find the required VM config options. Required options: %q, %q", CoreOSUserDataKey, CoreOSUserDataEncodingKey)
	}

	return propertySpecs, nil
}
