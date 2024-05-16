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
	DeviceSignals     *clientSignals
	KeyTrustLevel     *string
}

type clientSignals struct {
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
	OperatingSystem                  *string
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
	expectedDiskEncryption         = "DISK_ENCRYPTION_ENCRYPTED"
	expectedScreenLockSecured      = "SCREEN_LOCK_SECURED_ENABLED"
)

// Array signals which might be empty and thus omitted from the proto.
var optionalSignals = map[string]bool{"IMEI": true, "MEID": true, "ProfileAffiliationIDs": true}

// checkIfSignalsAreFilled checks if all signals were transmitted by checking for nil values at the struct fields.
// It will fail, if a signal was missing or the value was "null".
func checkIfSignalsAreFilled(signals any) error {
	signalValues := reflect.ValueOf(signals)

	for i := 0; i < signalValues.NumField(); i++ {
		field := signalValues.Field(i)
		fieldName := signalValues.Type().Field(i).Name

		// Skip optional signals.
		if optionalSignals[fieldName] {
			continue
		}

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

// verifyNonDeviceIdentifyingSignalValues verifies that non device identifying signals are in their value ranges or have pre-known values.
func verifyNonDeviceIdentifyingSignalValues(parsedServerSignals serverSignals, parsedClientSignals clientSignals, isInSession bool) error {
	// Checking pre-known values.
	if *parsedServerSignals.KeyTrustLevel != expectedKeyTrustLevelDev && *parsedServerSignals.KeyTrustLevel != expectedKeyTrustLevelVerified {
		return errors.Errorf("unexpected value for serverSignals.keyTrustLevel: got %q, want %q or %q", *parsedServerSignals.KeyTrustLevel, expectedKeyTrustLevelDev, expectedKeyTrustLevelVerified)
	}

	if *parsedClientSignals.OperatingSystem != expectedOS {
		return errors.Errorf("unexpected value for clientSignals.os: got %q, want %q", *parsedClientSignals.OperatingSystem, expectedOS)
	}

	if *parsedClientSignals.DiskEncryption != expectedDiskEncryption {
		return errors.Errorf("unexpected value for clientSignals.diskEncrypted: got %q, want %q", *parsedClientSignals.DiskEncryption, expectedDiskEncryption)
	}

	if *parsedClientSignals.ScreenLockSecured != expectedScreenLockSecured {
		return errors.Errorf("unexpected value for clientSignals.screenLockSecured: got %q, want %q", *parsedClientSignals.ScreenLockSecured, expectedScreenLockSecured)
	}

	// Checking the expected os firewall which depends on the key trust level.
	var expectedOSFirewall string
	if *parsedServerSignals.KeyTrustLevel == expectedKeyTrustLevelDev {
		expectedOSFirewall = "OS_FIREWALL_DISABLED"
	} else if *parsedServerSignals.KeyTrustLevel == expectedKeyTrustLevelVerified {
		expectedOSFirewall = "OS_FIREWALL_ENABLED"
	} else {
		expectedOSFirewall = "OS_FIREWALL_UNSPECIFIED"
	}

	if *parsedClientSignals.OSFirewall != expectedOSFirewall {
		return errors.Errorf("unexpected value for clientSignals.osFirewall: got %q, want %q", *parsedClientSignals.OSFirewall, expectedOSFirewall)
	}

	// Checking the signal for the trigger which generated the device signals.
	var expectedTrigger string
	if isInSession {
		expectedTrigger = "TRIGGER_BROWSER_NAVIGATION"
	} else {
		expectedTrigger = "TRIGGER_LOGIN_SCREEN"
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
	if parsedClientSignals.HostName != nil {
		return errors.New("key parsedClientSignals.hostName should not exist")
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

// Verify tries to parse the signal strings to JSON and checks the signals for completeness and validity.
func Verify(serverSignalsString, clientSignalsString []byte, isInSession, isDeviceManaged bool) error {
	parsedServerSignals, err := parseServerSignals(serverSignalsString)
	if err != nil {
		return errors.Wrap(err, "failed to parse server signals")
	}

	if isDeviceManaged {
		if err = verifySignalValuesManagedDevice(*parsedServerSignals, *parsedServerSignals.DeviceSignals, isInSession); err != nil {
			return errors.Wrap(err, "failed to verify signal values for a managed device")
		}
	} else {
		if err = verifySignalValuesUnmanagedDevice(*parsedServerSignals, *parsedServerSignals.DeviceSignals, isInSession); err != nil {
			return errors.Wrap(err, "failed to verify signal values for an unmanaged device")
		}
	}

	return nil
}
