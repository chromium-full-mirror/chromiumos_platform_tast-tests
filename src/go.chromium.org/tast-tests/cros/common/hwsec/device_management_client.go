// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const installAttributesFinalizeSuccessOutput = "InstallAttributesFinalize(): 1"

// DeviceManagementClient wraps and the functions of deviceManagementBinary and parses the outputs to
// structured data.
type DeviceManagementClient struct {
	binary *deviceManagementBinary
}

// NewDeviceManagementClient creates a new DeviceManagementClient.
func NewDeviceManagementClient(r CmdRunner) *DeviceManagementClient {
	return &DeviceManagementClient{
		binary: newDeviceManagementBinary(r),
	}
}

// InstallAttributesStatus retrieves the a status string from device_management. The status string is in JSON format and holds the various device management related status.
func (u *DeviceManagementClient) InstallAttributesStatus(ctx context.Context) (string, error) {
	out, err := u.binary.installAttributesGetStatus(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get Install Attributes status with the following output %q", out)
	}
	out = strings.TrimSuffix(out, "\n")
	return out, err
}

// InstallAttributesGet retrieves the install attributes with the name of attributeName, and returns the tuple (value, error), whereby value is the value of the attributes, and error is nil iff the operation is successful, otherwise error is the error that occurred.
func (u *DeviceManagementClient) InstallAttributesGet(ctx context.Context, attributeName string) (string, error) {
	out, err := u.binary.installAttributesGet(ctx, attributeName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get Install Attributes with the following output %q", out)
	}
	out = strings.TrimSuffix(out, "\n")
	return out, err
}

// InstallAttributesSet sets the install attributes with the name of attributeName with the value attributeValue, and returns error, whereby error is nil iff the operation is successful, otherwise error is the error that occurred.
func (u *DeviceManagementClient) InstallAttributesSet(ctx context.Context, attributeName, attributeValue string) error {
	out, err := u.binary.installAttributesSet(ctx, attributeName, attributeValue)
	if err != nil {
		return errors.Wrapf(err, "failed to set Install Attributes with the following output %q", out)
	}
	return nil
}

// InstallAttributesFinalize finalizes the install attributes, and returns error encountered if any. error is nil iff the operation completes successfully.
func (u *DeviceManagementClient) InstallAttributesFinalize(ctx context.Context) error {
	out, err := u.binary.installAttributesFinalize(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to finalize Install Attributes with the following output %q", out)
	}
	if !strings.Contains(out, installAttributesFinalizeSuccessOutput) {
		return errors.Errorf("failed to finalize Install Attributes, incorrect output message %q", out)
	}
	return nil
}

// InstallAttributesCount retrieves the number of entries in install attributes. It returns count and error. error is nil iff the operation completes successfully, and in this case count holds the number of entries in install attributes.
func (u *DeviceManagementClient) InstallAttributesCount(ctx context.Context) (count int, err error) {
	out, err := u.binary.installAttributesCount(ctx)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to query install attributes count with the following output %q", out)
	}
	n, err := fmt.Sscanf(out, "InstallAttributesCount(): %d", &count)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to parse InstallAttributesCount output %q", out)
	}
	if n != 1 {
		return -1, errors.Errorf("invalid InstallAttributesCount output %q", out)
	}
	return count, nil
}

// installAttrBooleanHelper is a helper function that helps to parse the output of install attribute series of command that returns a boolean.
func installAttrBooleanHelper(out string, err error, methodName string) (bool, error) {
	if err != nil {
		return false, errors.Wrapf(err, "failed to run %s(), output %q", methodName, out)
	}
	var result int
	n, err := fmt.Sscanf(out, methodName+"(): %d", &result)
	if err != nil {
		return false, errors.Wrapf(err, "failed to parse %s(), output %q", methodName, out)
	}
	if n != 1 {
		return false, errors.Errorf("invalid %s() output %q", methodName, out)
	}
	return result != 0, nil
}

// InstallAttributesIsReady checks if install attributes is ready, returns isReady and error. error is nil iff the operation completes successfully, and in this case isReady is whether install attributes is ready.
func (u *DeviceManagementClient) InstallAttributesIsReady(ctx context.Context) (bool, error) {
	out, err := u.binary.installAttributesIsReady(ctx)
	return installAttrBooleanHelper(out, err, "InstallAttributesIsReady")
}

// InstallAttributesIsSecure checks if install attributes is secure, returns isSecure and error. error is nil iff the operation completes successfully, and in this case isSecure is whether install attributes is secure.
func (u *DeviceManagementClient) InstallAttributesIsSecure(ctx context.Context) (bool, error) {
	out, err := u.binary.installAttributesIsSecure(ctx)
	return installAttrBooleanHelper(out, err, "InstallAttributesIsSecure")
}

// InstallAttributesIsInvalid checks if install attributes is invalid, returns isInvalid and error. error is nil iff the operation completes successfully, and in this case isInvalid is whether install attributes is invalid.
func (u *DeviceManagementClient) InstallAttributesIsInvalid(ctx context.Context) (bool, error) {
	out, err := u.binary.installAttributesIsInvalid(ctx)
	return installAttrBooleanHelper(out, err, "InstallAttributesIsInvalid")
}

// InstallAttributesIsFirstInstall checks if install attributes is the first install state, returns isFirstInstall and error. error is nil iff the operation completes successfully, and in this case isFirstInstall is whether install attributes is in the first install state.
func (u *DeviceManagementClient) InstallAttributesIsFirstInstall(ctx context.Context) (bool, error) {
	out, err := u.binary.installAttributesIsFirstInstall(ctx)
	return installAttrBooleanHelper(out, err, "InstallAttributesIsFirstInstall")
}

// FirmwareManagementParametersError is a custom error type that conveys the error as well as parsed
// ErrorCode from device_management API.
type FirmwareManagementParametersError struct {
	*errors.E

	// ErrorCode is the error code from FWMP methods.
	ErrorCode string
}

// GetFirmwareManagementParameters retrieves the firmware parameter flags and hash.
func (u *DeviceManagementClient) GetFirmwareManagementParameters(ctx context.Context) (flags, hash string, returnedError *FirmwareManagementParametersError) {
	binaryMsg, err := u.binary.getFirmwareManagementParameters(ctx)
	msg := string(binaryMsg)

	// Parse for the error code and stuffs because we might need the error code in the return error in case it failed.
	const flagsPrefix = "flags=0x"
	const hashPrefix = "hash="
	const errorPrefix = "error: "
	prefixes := []string{flagsPrefix, hashPrefix, errorPrefix}
	params := make(map[string]string, len(prefixes))
	for _, line := range strings.Split(msg, "\n") {
		line := strings.TrimSpace(line)
		for _, prefix := range prefixes {
			if after, found := strings.CutPrefix(line, prefix); found {
				if _, existing := params[prefix]; existing {
					return "", "", &FirmwareManagementParametersError{E: errors.Errorf("duplicate attribute %q found GetFirmwareManagementParameters output", prefix)}
				}
				params[prefix] = after
			}
		}
	}

	if err != nil {
		testing.ContextLogf(ctx, "GetFirmwareManagementParameters failed with %q", msg)
		errorCode, haveError := params[errorPrefix]
		if haveError {
			return "", "", &FirmwareManagementParametersError{E: errors.Wrapf(err, "failed to call GetFirmwareManagementParameters command, error %q", errorCode), ErrorCode: errorCode}
		}
		return "", "", &FirmwareManagementParametersError{E: errors.Wrap(err, "failed to call GetFirmwareManagementParameters with unknown error")}
	}

	// return the hash and flags if they exist, and return error otherwise.
	paramsFlags, haveFlags := params[flagsPrefix]
	paramsHash, haveHash := params[hashPrefix]
	if !haveFlags {
		return "", "", &FirmwareManagementParametersError{E: errors.New("no flags in GetFirmwareManagementParameters output")}
	}
	if !haveHash {
		return "", "", &FirmwareManagementParametersError{E: errors.New("no hash in GetFirmwareManagementParameters output")}
	}

	return paramsFlags, paramsHash, nil
}

// SetFirmwareManagementParameters sets the firmware management parameters flags and hash (both as a hex string).
func (u *DeviceManagementClient) SetFirmwareManagementParameters(ctx context.Context, flags, hash string) (msg string, err error) {
	binaryMsg, err := u.binary.setFirmwareManagementParameters(ctx, "0x"+flags, hash)
	msg = string(binaryMsg)

	// Note that error code is not parsed because currently no tests requires it.

	if err != nil {
		testing.ContextLogf(ctx, "SetFirmwareManagementParameters failed with %q", msg)
		return msg, errors.Wrap(err, "failed to call SetFirmwareManagementParameters")
	}

	return msg, nil
}

// RemoveFirmwareManagementParameters removes the firmware management parameters.
func (u *DeviceManagementClient) RemoveFirmwareManagementParameters(ctx context.Context) (msg string, err error) {
	binaryMsg, err := u.binary.removeFirmwareManagementParameters(ctx)
	msg = string(binaryMsg)

	// Note that error code is not parsed because currently no tests requires it.

	if err != nil {
		testing.ContextLogf(ctx, "RemoveFirmwareManagementParameters failed with %q", msg)
		return msg, errors.Wrap(err, "failed to call RemoveFirmwareManagementParameters")
	}

	return msg, nil
}

// FWMPInfo contains the information regarding FWMP, so that it can be backed up and restored.
type FWMPInfo struct {
	// parametersExist is true iff the FWMP is set/exists on the DUT.
	parametersExist bool

	// flags contain the flags in the FWMP. This is valid iff parametersExist is true.
	flags string

	// hash contains the developer hash in the FWMP. This is valid iff parametersExist is true.
	hash string
}

// BackupFWMP backs up the current FWMP by returning the FWMP. The operation is successful iff error is nil.
func (u *DeviceManagementClient) BackupFWMP(ctx context.Context) (*FWMPInfo, error) {
	flags, hash, err := u.GetFirmwareManagementParameters(ctx)
	if err != nil {
		if err.ErrorCode != "DEVICE_MANAGEMENT_ERROR_FIRMWARE_MANAGEMENT_PARAMETERS_INVALID" {
			return nil, errors.Wrap(err, "failed to get FWMP for backup")
		}
		// FWMP doesn't exist.
		return &FWMPInfo{parametersExist: false}, nil
	}

	fwmp := FWMPInfo{parametersExist: true, flags: flags, hash: hash}
	return &fwmp, nil
}

// RestoreFWMP restores the FWMP from fwmp in parameter, and return nil iff the operation is successful.
func (u *DeviceManagementClient) RestoreFWMP(ctx context.Context, fwmp *FWMPInfo) error {
	if !fwmp.parametersExist {
		// Parameters doesn't exist, so let's clear it.
		if _, err := u.RemoveFirmwareManagementParameters(ctx); err != nil {
			return errors.Wrap(err, "failed to clear FWMP")
		}
		return nil
	}

	// FWMP exists, so let's set the correct values.
	if _, err := u.SetFirmwareManagementParameters(ctx, fwmp.flags, fwmp.hash); err != nil {
		return errors.Wrap(err, "failed to set FWMP")
	}

	return nil
}
