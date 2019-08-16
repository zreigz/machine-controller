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

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func getValueForField(ctx context.Context, vm *object.VirtualMachine, fieldName string) (string, error) {
	var mvm mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), nil, &mvm); err != nil {
		return "", fmt.Errorf("failed to get properties: %v", err)
	}

	var key int32
	for _, availableField := range mvm.AvailableField {
		if availableField.Name == fieldName {
			key = availableField.Key
			break
		}
	}

	for _, value := range mvm.Value {
		if value.GetCustomFieldValue().Key == key {
			stringVal, ok := value.(*types.CustomFieldStringValue)
			if ok {
				return stringVal.Value, nil
			}
			break
		}
	}

	return "", nil
}
