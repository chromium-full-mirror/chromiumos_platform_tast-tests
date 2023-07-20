// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
)

// deviceManagementBinary is used to interact with the device_managementd process over
// 'device_management_client' executable. For more details of the arguments of the
// functions in this file, please check //src/platform2/device_management/client/tool.cc.
type deviceManagementBinary struct {
	runner CmdRunner
}

// newDeviceManagementBinary is a factory function to create a
// deviceManagementBinary instance.
func newDeviceManagementBinary(r CmdRunner) *deviceManagementBinary {
	return &deviceManagementBinary{r}
}

// call is a simple utility that helps to call device_management_client.
func (c *deviceManagementBinary) call(ctx context.Context, args ...string) ([]byte, error) {
	return c.runner.Run(ctx, "device_management_client", args...)
}

// getFirmwareManagementParameters calls "device_management_client --action=get_firmware_management_parameters"
func (c *deviceManagementBinary) getFirmwareManagementParameters(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--action=get_firmware_management_parameters")
}

// setFirmwareManagementParameters calls "device_management_client --action=set_firmware_management_parameters".
func (c *deviceManagementBinary) setFirmwareManagementParameters(ctx context.Context, flags, hash string) ([]byte, error) {
	return c.call(ctx, "--action=set_firmware_management_parameters", "--flags="+flags, "--developer_key_hash="+hash)
}

// removeFirmwareManagementParameters calls "device_management_client --action=remove_firmware_management_parameters".
func (c *deviceManagementBinary) removeFirmwareManagementParameters(ctx context.Context) ([]byte, error) {
	return c.call(ctx, "--action=remove_firmware_management_parameters")
}

// installAttributesGetStatus calls "device_management_client --action=install_attributes_get_status".
func (c *deviceManagementBinary) installAttributesGetStatus(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_get_status")
	return string(out), err
}

// installAttributesGet calls "device_management_client --action=install_attributes_get".
func (c *deviceManagementBinary) installAttributesGet(ctx context.Context, attributeName string) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_get", "--name="+attributeName)
	return string(out), err
}

// installAttributesSet calls "device_management_client --action=install_attributes_set".
func (c *deviceManagementBinary) installAttributesSet(ctx context.Context, attributeName, attributeValue string) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_set", "--name="+attributeName, "--value="+attributeValue)
	return string(out), err
}

// installAttributesFinalize calls "device_management_client --action=install_attributes_finalize".
func (c *deviceManagementBinary) installAttributesFinalize(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_finalize")
	return string(out), err
}

// installAttributesCount calls "device_management_client --action=install_attributes_count".
func (c *deviceManagementBinary) installAttributesCount(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_count")
	return string(out), err
}

// installAttributesIsReady calls "device_management_client --action=install_attributes_is_ready".
func (c *deviceManagementBinary) installAttributesIsReady(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_ready")
	return string(out), err
}

// installAttributesIsSecure calls "device_management_client --action=install_attributes_is_secure".
func (c *deviceManagementBinary) installAttributesIsSecure(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_secure")
	return string(out), err
}

// installAttributesIsInvalid calls "device_management_client --action=install_attributes_is_invalid".
func (c *deviceManagementBinary) installAttributesIsInvalid(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_invalid")
	return string(out), err
}

// installAttributesIsFirstInstall calls "device_management_client --action=install_attributes_is_first_install".
func (c *deviceManagementBinary) installAttributesIsFirstInstall(ctx context.Context) (string, error) {
	out, err := c.call(ctx, "--action=install_attributes_is_first_install")
	return string(out), err
}
