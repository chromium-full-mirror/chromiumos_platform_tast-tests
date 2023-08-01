// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devicetrust

import (
	"encoding/json"
	"reflect"

	"go.chromium.org/tast/core/errors"
)

// Fields are sorted in alphabetical order.
// Pointers are used to detect missing or null signals.
type serverSignals struct {
	CustomerID        *string
	DevicePermanentID *string
	DeviceSignal      *string
	DeviceSignals     *clientSignalsProto
	KeyTrustLevel     *string
}

// Fields are sorted in alphabetical order.
// Pointers are used to detect missing or null signals.
type clientSignals struct {
	AllowScreenLock                  *bool
	BrowserVersion                   *string
	BuiltInDNSClientEnabled          *bool
	ChromeRemoteDesktopAppBlocked    *bool
	DeviceAffiliationIDs             *[]string
	DeviceEnrollmentDomain           *string
	DeviceHostName                   *string
	DeviceManufacturer               *string
	DeviceModel                      *string
	DiskEncrypted                    *int
	DisplayName                      *string
	IMEI                             *[]string
	MACAddresses                     *[]string
	MEID                             *[]string
	OS                               *string
	OSFirewall                       *int
	OSVersion                        *string
	PasswordProtectionWarningTrigger *int
	ProfileAffiliationIDs            *[]string
	RealtimeURLCheckMode             *int
	SafeBrowsingProtectionLevel      *int
	ScreenLockSecured                *int
	SerialNumber                     *string
	SiteIsolationEnabled             *bool
	SystemDNSServers                 *[]string
	Trigger                          *int
}

type clientSignalsProto struct {
	AllowScreenLock                  *bool
	BrowserVersion                   *string
	BuiltInDNSClientEnabled          *bool
	ChromeRemoteDesktopAppBlocked    *bool
	DeviceAffiliationIDs             *[]string
	DeviceEnrollmentDomain           *string
	HostName                         *string
	DeviceManufacturer               *string
	DeviceModel                      *string
	DiskEncryption                   *string
	DisplayName                      *string
	IMEI                             *[]string
	MACAddresses                     *[]string
	MEID                             *[]string
	OS                               *string
	OSFirewall                       *string
	OSVersion                        *string
	PasswordProtectionWarningTrigger *string
	ProfileAffiliationIDs            *[]string
	RealtimeURLCheckMode             *string
	SafeBrowsingProtectionLevel      *string
	ScreenLockSecured                *string
	SerialNumber                     *string
	SiteIsolationEnabled             *bool
	SystemDNSServers                 *[]string
	Trigger                          *string
}

const (
	expectedKeyTrustLevelDev       = "CHROME_OS_DEVELOPER_MODE"
	expectedKeyTrustLevelVerified  = "CHROME_OS_VERIFIED_MODE"
	expectedOS                     = "ChromeOS"
	expectedDeviceEnrollmentDomain = "managedchrome.com"
	expectedAffiliationIDLength    = 1
	expectedDiskEncryption         = 2
	expectedScreenLockSecured      = 2
)

// List of JSON signals which are converted to an enum by Verified Access and thus should not be compared with the corresponding proto signal.
var enumSignals = map[string]bool{"DiskEncrypted": true, "OS": true, "OSFirewall": true, "PasswordProtectionWarningTrigger": true, "RealtimeURLCheckMode": true, "SafeBrowsingProtectionLevel": true, "ScreenLockSecured": true, "Trigger": true}

// checkIfSignalsAreFilled checks if all signals were transmitted by checking for nil values at the struct fields.
// It will fail, if a signal was missing or the value was "null". Empty values of the datatype are allowed (empty string or array).
func checkIfSignalsAreFilled(signals any) error {
	signalValues := reflect.ValueOf(signals)

	for i := 0; i < signalValues.NumField(); i++ {
		field := signalValues.Field(i)
		if field.IsNil() {
			return errors.Errorf("signal field not found: %s", signalValues.Type().Field(i).Name)
		}
	}

	return nil
}

func parseServerSignals(signalsString []byte) (*serverSignals, error) {
	if !json.Valid(signalsString) {
		return nil, errors.New("signals json invalid")
	}

	var signals serverSignals
	// json.Unmarshal verifies that signals have the right data type, if they do exist.
	if err := json.Unmarshal(signalsString, &signals); err != nil {
		return nil, errors.Wrap(err, "failed to marshal the server signals")
	}

	return &signals, nil
}

func parseClientSignals(signalsString []byte) (*clientSignals, error) {
	if !json.Valid(signalsString) {
		return nil, errors.New("signals json invalid")
	}

	var signals clientSignals
	// json.Unmarshal verifies that signals have the right data type, if they do exist.
	if err := json.Unmarshal(signalsString, &signals); err != nil {
		return nil, errors.Wrap(err, "failed to marshal the client signals")
	}

	return &signals, nil
}

// verifyIsInRange checks if values is inclusively in range of minValue and maxValue.
func verifyIsInRange(value, minValue, maxValue int) error {
	if value < minValue || value > maxValue {
		return errors.Errorf("value %v is not in range (%v, %v)", value, minValue, maxValue)
	}

	return nil
}

// verifyIsSettingEnum verifies the value is in the valid enum values range.
// Enum defined at: chrome/browser/enterprise/signals/signals_common.h
func verifyIsSettingEnum(value int) error {
	return verifyIsInRange(value, 0, 2)
}

// verifyNonDeviceIdentifyingSignalValues verifies that non device identifying signals are in their value ranges or have pre-known values.
func verifyNonDeviceIdentifyingSignalValues(parsedServerSignals serverSignals, parsedClientSignals clientSignals, isInSession bool) error {
	// Checking value ranges for settings.
	if err := verifyIsSettingEnum(*parsedClientSignals.DiskEncrypted); err != nil {
		return errors.Wrap(err, "failed to verify clientSignals.diskEncrypted")
	}

	if err := verifyIsSettingEnum(*parsedClientSignals.OSFirewall); err != nil {
		return errors.Wrap(err, "failed to verify clientSignals.osFirewall")
	}

	if err := verifyIsSettingEnum(*parsedClientSignals.ScreenLockSecured); err != nil {
		return errors.Wrap(err, "failed to verify clientSignals.screenLockSecured")
	}

	if err := verifyIsInRange(*parsedClientSignals.PasswordProtectionWarningTrigger, 0, 3); err != nil {
		return errors.Wrap(err, "failed to verify clientSignals.passwordPotectionWarningTrigger")
	}

	if err := verifyIsInRange(*parsedClientSignals.RealtimeURLCheckMode, 0, 1); err != nil {
		return errors.Wrap(err, "failed to verify clientSignals.realtimeUrlCheckMode")
	}

	if err := verifyIsInRange(*parsedClientSignals.SafeBrowsingProtectionLevel, 0, 2); err != nil {
		return errors.Wrap(err, "failed to verify clientSignals.safeBrowsingProtectionLevel")
	}

	// Checking pre-known values.
	if *parsedServerSignals.KeyTrustLevel != expectedKeyTrustLevelDev && *parsedServerSignals.KeyTrustLevel != expectedKeyTrustLevelVerified {
		return errors.Errorf("unexpected value for serverSignals.keyTrustLevel: got %q, want %q or %q", *parsedServerSignals.KeyTrustLevel, expectedKeyTrustLevelDev, expectedKeyTrustLevelVerified)
	}

	if *parsedClientSignals.OS != expectedOS {
		return errors.Errorf("unexpected value for clientSignals.os: got %q, want %q", *parsedClientSignals.OS, expectedOS)
	}

	if *parsedClientSignals.DiskEncrypted != expectedDiskEncryption {
		return errors.Errorf("unexpected value for clientSignals.diskEncrypted: got %q, want %q", *parsedClientSignals.DiskEncrypted, expectedDiskEncryption)
	}

	if *parsedClientSignals.ScreenLockSecured != expectedScreenLockSecured {
		return errors.Errorf("unexpected value for clientSignals.screenLockSecured: got %q, want %q", *parsedClientSignals.ScreenLockSecured, expectedScreenLockSecured)
	}

	// Checking the expected os firewall which depends on the key trust level.
	var expectedOSFirewall int
	if *parsedServerSignals.KeyTrustLevel == expectedKeyTrustLevelDev {
		expectedOSFirewall = 1
	} else if *parsedServerSignals.KeyTrustLevel == expectedKeyTrustLevelVerified {
		expectedOSFirewall = 2
	} else {
		expectedOSFirewall = -1
	}

	if *parsedClientSignals.OSFirewall != expectedOSFirewall {
		return errors.Errorf("unexpected value for clientSignals.osFirewall: got %q, want %q", *parsedClientSignals.OSFirewall, expectedOSFirewall)
	}

	// Checking the signal for the trigger which generated the device signals.
	var expectedTrigger int
	if isInSession {
		expectedTrigger = 1
	} else {
		expectedTrigger = 2
	}

	if *parsedClientSignals.Trigger != expectedTrigger {
		return errors.Errorf("unexpected value for clientSignals.trigger: got %q, want %q", *parsedClientSignals.Trigger, expectedTrigger)
	}

	// Checking profileAffiliationIDs.
	if isInSession && len(*parsedClientSignals.ProfileAffiliationIDs) != expectedAffiliationIDLength {
		return errors.Errorf("unexpected value for len(clientSignals.profileAffiliationIDs): got %v, want %v", len(*parsedClientSignals.ProfileAffiliationIDs), expectedAffiliationIDLength)
	}

	if !isInSession && len(*parsedClientSignals.ProfileAffiliationIDs) != 0 {
		return errors.New("clientSignals.profileAffiliationIDs should be empty")
	}

	return nil
}

// verifySignalValuesManagedDevice verifies that certain signals are in their value ranges or have pre-known values.
// The function also verifies that the signal structs have no fields with a nil value.
func verifySignalValuesManagedDevice(parsedServerSignals serverSignals, parsedClientSignals clientSignals, isInSession bool) error {
	// Check if the server and client signals have a valid format.
	if err := checkIfSignalsAreFilled(parsedServerSignals); err != nil {
		return errors.Wrap(err, "invalid format for server signals")
	}

	if err := checkIfSignalsAreFilled(parsedClientSignals); err != nil {
		return errors.Wrap(err, "invalid format for client signals")
	}

	verifyNonDeviceIdentifyingSignalValues(parsedServerSignals, parsedClientSignals, isInSession)

	// Checking non empty values.
	if *parsedServerSignals.DevicePermanentID == "" {
		return errors.New("serverSignals.devicePermanentId should not be empty")
	}

	if len(*parsedClientSignals.MACAddresses) == 0 {
		return errors.New("clientSignals.macAddresses should not be empty")
	}

	if *parsedClientSignals.SerialNumber == "" {
		return errors.New("clientSignals.serialNumber should not be empty")
	}

	// Checking pre-known value DeviceEnrollmentDomain.
	if *parsedClientSignals.DeviceEnrollmentDomain != expectedDeviceEnrollmentDomain {
		return errors.Errorf("unexpected value for clientSignals.deviceEnrollmentDomain: got %q, want %q", *parsedClientSignals.DeviceEnrollmentDomain, expectedDeviceEnrollmentDomain)
	}

	// Checking affiliation IDs.
	if len(*parsedClientSignals.DeviceAffiliationIDs) != expectedAffiliationIDLength {
		return errors.Errorf("unexpected value for len(clientSignals.deviceAffiliationIDs): got %v, want %v", len(*parsedClientSignals.DeviceAffiliationIDs), expectedAffiliationIDLength)
	}

	if *parsedServerSignals.CustomerID == "" || *parsedServerSignals.CustomerID != (*parsedClientSignals.DeviceAffiliationIDs)[0] {
		return errors.Errorf("serverSignals.customerId and clientSignals.deviceAffiliationIDs needs to be the same and non empty, values were %s and %s", *parsedServerSignals.CustomerID, (*parsedClientSignals.DeviceAffiliationIDs)[0])
	}

	// For the in-session case, check if the user is affiliated; i.e., the user affiliated ID is the same as the device affiliated ID.
	if isInSession {
		if (*parsedClientSignals.ProfileAffiliationIDs)[0] != (*parsedClientSignals.DeviceAffiliationIDs)[0] {
			return errors.Errorf("clientSignals.profileAffilationIds and clientSignals.deviceAffiliationIDs needs to be the same, values were %s and %s", (*parsedClientSignals.ProfileAffiliationIDs)[0], (*parsedClientSignals.DeviceAffiliationIDs)[0])
		}
	}

	return nil
}

// verifySignalValuesUnmanagedDevice verifies that certain signals are in their value ranges or have pre-known values.
// The function also verifies that device identifying signals are not part of the signal payload.
func verifySignalValuesUnmanagedDevice(parsedServerSignals serverSignals, parsedClientSignals clientSignals, isInSession bool) error {
	verifyNonDeviceIdentifyingSignalValues(parsedServerSignals, parsedClientSignals, isInSession)

	// Checking if device identifying server signals don't exist.
	if parsedServerSignals.DevicePermanentID != nil {
		return errors.New("key serverSignals.devicePermanentId should not exist")
	}

	// Checking if device identifying client signals don't exist.
	if parsedClientSignals.DeviceHostName != nil {
		return errors.New("key parsedClientSignals.deviceHostName should not exist")
	}

	if parsedClientSignals.DisplayName != nil {
		return errors.New("key parsedClientSignals.displayName should not exist")
	}

	if parsedClientSignals.IMEI != nil {
		return errors.New("key parsedClientSignals.imei should not exist")
	}

	if parsedClientSignals.MEID != nil {
		return errors.New("key parsedClientSignals.meid should not exist")
	}

	if parsedClientSignals.MACAddresses != nil {
		return errors.New("key parsedClientSignals.macAddresses should not exist")
	}

	if parsedClientSignals.SerialNumber != nil {
		return errors.New("key parsedClientSignals.serialNumber should not exist")
	}

	if parsedClientSignals.SystemDNSServers != nil {
		return errors.New("key parsedClientSignals.systemDNSServers should not exist")
	}

	return nil
}

// verifyIsSame checks whether both signal fields either contain the same value or are both empty.
func verifyIsSame(jsonField, protoField reflect.Value) error {
	// Both fields are empty.
	if jsonField.IsNil() && protoField.IsNil() {
		return nil
	}

	// Only one of both fields is empty.
	if jsonField.IsNil() || protoField.IsNil() {
		// Proto field is nil in case of an empty string array.
		if jsonField.Type().String() == "*[]string" {
			return nil
		}

		return errors.Errorf("both fields should be either empty or non empty, values were %v and %v", jsonField.Interface(), protoField.Interface())
	}

	// Fields have different values.
	if !reflect.DeepEqual(jsonField.Interface(), protoField.Interface()) {
		return errors.Errorf("values needs to be the same for JSON and proto signal, values were %v and %v", jsonField.Interface(), protoField.Interface())
	}

	return nil
}

// compareClientSignalsJSONAndProto compares the clients signals from the JSON and proto payload with each other which were not converted to enum by Verified Access.
func compareClientSignalsJSONAndProto(jsonSignals clientSignals, protoSignals clientSignalsProto) error {
	jsonValues := reflect.ValueOf(jsonSignals)
	protoValues := reflect.ValueOf(protoSignals)

	for i := 0; i < jsonValues.NumField(); i++ {
		jsonName := jsonValues.Type().Field(i).Name
		jsonField := jsonValues.Field(i)
		protoField := protoValues.Field(i)

		// Skip signals which were converted to an enum.
		if enumSignals[jsonName] == true {
			continue
		}

		if err := verifyIsSame(jsonField, protoField); err != nil {
			return errors.Wrapf(err, "JSON and proto signals for %s needs to be the same", jsonName)
		}
	}

	return nil
}

// Verify tries to parse the signal strings to JSON and checks the signals for completeness and validity.
func Verify(serverSignalsString, clientSignalsString []byte, isInSession, isDeviceManaged bool) error {
	parsedServerSignals, err := parseServerSignals(serverSignalsString)
	if err != nil {
		return errors.Wrap(err, "failed to parse server signals")
	}

	parsedClientSignals, err := parseClientSignals(clientSignalsString)
	if err != nil {
		return errors.Wrap(err, "failed to parse client signals")
	}

	if err = compareClientSignalsJSONAndProto(*parsedClientSignals, *parsedServerSignals.DeviceSignals); err != nil {
		return errors.Wrap(err, "mismatch for client signals in JSON and proto")
	}

	if isDeviceManaged {
		if err = verifySignalValuesManagedDevice(*parsedServerSignals, *parsedClientSignals, isInSession); err != nil {
			return errors.Wrap(err, "failed to verify signal values for a managed device")
		}
	} else {
		if err = verifySignalValuesUnmanagedDevice(*parsedServerSignals, *parsedClientSignals, isInSession); err != nil {
			return errors.Wrap(err, "failed to verify signal values for an unmanaged device")
		}
	}

	return nil
}
