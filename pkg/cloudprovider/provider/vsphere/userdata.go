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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"text/template"

	"github.com/golang/glog"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func datastoreISOFilename(machineName string) string {
	return path.Join(machineName, "cloud-init.iso")
}

func getCDRomSpecs(devices object.VirtualDeviceList, isoPath string) ([]types.BaseVirtualDeviceConfigSpec, error) {
	// Remove all IDE controllers & CDRom drives. We just add our own.
	// That way we don't have to fiddle around finding the correct one & we are sure we're the first IDE device
	var specs []types.BaseVirtualDeviceConfigSpec
	for _, dev := range devices.SelectByType((*types.VirtualCdrom)(nil)) {
		specs = append(specs, &types.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: types.VirtualDeviceConfigSpecOperationRemove,
		})
	}
	for _, dev := range devices.SelectByType((*types.VirtualIDEController)(nil)) {
		specs = append(specs, &types.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: types.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	ideController := &types.VirtualIDEController{}
	ideController.Key = devices.NewKey()
	specs = append(specs, &types.VirtualDeviceConfigSpec{
		Device:    ideController,
		Operation: types.VirtualDeviceConfigSpecOperationAdd,
	})

	cdrom, err := devices.CreateCdrom(ideController)
	if err != nil {
		return nil, err
	}
	cdrom.Backing = &types.VirtualCdromIsoBackingInfo{
		VirtualDeviceFileBackingInfo: types.VirtualDeviceFileBackingInfo{
			FileName: isoPath,
		},
	}
	specs = append(specs, &types.VirtualDeviceConfigSpec{
		Device:    cdrom,
		Operation: types.VirtualDeviceConfigSpecOperationAdd,
	})

	return specs, nil
}

func generateAndUploadUserDataISO(ctx context.Context, session *Session, userdata, machineName string) (string, error) {
	localIsoFilePath, err := generateLocalUserdataISO(userdata, machineName)
	if err != nil {
		return "", fmt.Errorf("failed to generate userdata ISO: %v", err)
	}
	defer os.Remove(localIsoFilePath)

	p := soap.DefaultUpload
	remoteIsoFilePath := datastoreISOFilename(machineName)
	glog.V(3).Infof("Uploading userdata ISO to datastore %s...", remoteIsoFilePath)
	if err := session.Datastore.UploadFile(ctx, localIsoFilePath, remoteIsoFilePath, &p); err != nil {
		return "", fmt.Errorf("failed to upload ISO: %v", err)
	}
	glog.V(3).Infof("Successfully uploaded userdata ISO to %s", remoteIsoFilePath)

	return session.Datastore.Path(remoteIsoFilePath), nil
}

func generateLocalUserdataISO(userdata, name string) (string, error) {
	// We must create a directory, because the iso-generation commands
	// take a directory as input
	userdataDir, err := ioutil.TempDir(localTempDir, name)
	if err != nil {
		return "", fmt.Errorf("failed to create local temp directory for userdata at %s: %v", userdataDir, err)
	}
	defer func() {
		if err := os.RemoveAll(userdataDir); err != nil {
			utilruntime.HandleError(fmt.Errorf("error cleaning up local userdata tempdir %s: %v", userdataDir, err))
		}
	}()

	userdataFilePath := fmt.Sprintf("%s/user-data", userdataDir)
	metadataFilePath := fmt.Sprintf("%s/meta-data", userdataDir)
	isoFilePath := fmt.Sprintf("%s/%s.iso", localTempDir, name)

	metadataTmpl, err := template.New("metadata").Parse(metaDataTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse metadata template: %v", err)
	}
	metadata := &bytes.Buffer{}
	templateContext := struct {
		InstanceID string
		Hostname   string
	}{
		InstanceID: name,
		Hostname:   name,
	}
	if err = metadataTmpl.Execute(metadata, templateContext); err != nil {
		return "", fmt.Errorf("failed to render metadata: %v", err)
	}

	if err := ioutil.WriteFile(userdataFilePath, []byte(userdata), 0644); err != nil {
		return "", fmt.Errorf("failed to locally write userdata file to %s: %v", userdataFilePath, err)
	}

	if err := ioutil.WriteFile(metadataFilePath, metadata.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to locally write metadata file to %s: %v", userdataFilePath, err)
	}

	var command string
	var args []string

	if _, err := exec.LookPath("genisoimage"); err == nil {
		command = "genisoimage"
		args = []string{"-o", isoFilePath, "-volid", "cidata", "-joliet", "-rock", userdataDir}
	} else if _, err := exec.LookPath("mkisofs"); err == nil {
		command = "mkisofs"
		args = []string{"-o", isoFilePath, "-V", "cidata", "-J", "-R", userdataDir}
	} else {
		return "", errors.New("system is missing genisoimage or mkisofs, can't generate userdata iso without it")
	}

	cmd := exec.Command(command, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("error executing command `%s %s`: output: `%s`, error: `%v`", command, args, string(output), err)
	}

	return isoFilePath, nil
}

func removeUserdataISO(ctx context.Context, session *Session, machineName string) error {
	filemanager := session.Datastore.NewFileManager(session.Datacenter, false)
	if err := filemanager.Delete(ctx, datastoreISOFilename(machineName)); err != nil {
		if err.Error() == fmt.Sprintf("File [%s] %s was not found", session.Datastore.Name(), machineName) {
			return nil
		}
		return fmt.Errorf("failed to delete userdate ISO of deleted machine %s: %v", machineName, err)
	}
	return nil
}
