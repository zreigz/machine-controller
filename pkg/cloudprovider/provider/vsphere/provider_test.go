// +build integration

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
	"os"
)

var (
	vSphereDatacenter = os.Getenv("VSPHERE_E2E_TEST_DATACENTER")
	vSphereDatastore  = os.Getenv("VSPHERE_E2E_DATASTORE")
	vSphereEndpoint   = os.Getenv("VSPHERE_E2E_ADDRESS")
	vSphereUsername   = os.Getenv("VSPHERE_E2E_USERNAME")
	vSpherePassword   = os.Getenv("VSPHERE_E2E_PASSWORD")
)

func getConfig() *Config {
	return &Config{
		VSphereURL: vSphereEndpoint,
		Datacenter: vSphereDatacenter,
		Username:   vSphereUsername,
		Password:   vSpherePassword,
		Datastore:  vSphereDatastore,
	}
}
