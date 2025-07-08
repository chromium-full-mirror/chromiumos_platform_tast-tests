// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crossdevice

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/crossdevice"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// NewCrossDeviceAndroid creates a fixture that sets up an Android device for crossdevice feature testing.
func NewCrossDeviceAndroid(feature Feature) testing.FixtureImpl {
	return &crossdeviceAndroidFixture{
		feature: feature,
	}
}

// resetTimeout is the timeout duration to trying reset of the current fixture.
const resetTimeout = 30 * time.Second

// Runtime variable names.
const (
	// Specify -var=skipAndroidLogin=true if the Android device is logged in to a personal account.
	// Otherwise we will attempt removing all Google accounts and adding a test account to the phone.
	// Adding/removing accounts requires ADB root access, so this will automatically be set to true if root is not available.
	skipAndroidLogin = "skipAndroidLogin"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "crossdeviceAndroidSetupPhoneHub",
		Desc: "Set up Android device for CrOS crossdevice testing",
		Impl: NewCrossDeviceAndroid(Feature{Name: PhoneHub}),
		Data: []string{AccountUtilZip, MultideviceSnippetZipName},
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"hansenmichael@google.com",
		},
		BugComponent: "b:1131837", // ChromeOS > Software > System Services > Cross Device > Phone Hub
		Vars: []string{
			skipAndroidLogin,
		},
		Parent:          "crossDeviceRemote",
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossdeviceAndroidSetupPhoneHubRerun",
		Desc: "Reset and Retry fixture for crossdeviceAndroidSetupPhoneHub",
		Impl: NewCrossDeviceAndroid(Feature{Name: PhoneHub}),
		Data: []string{AccountUtilZip, MultideviceSnippetZipName},
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"hansenmichael@google.com",
		},
		BugComponent: "b:1131837", // ChromeOS > Software > System Services > Cross Device > Phone Hub
		Vars: []string{
			skipAndroidLogin,
		},
		Parent:          "crossDeviceRemote",
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossdeviceAndroidSetupSmartLock",
		Desc: "Set up Android device for CrOS crossdevice testing of Smart Lock",
		Impl: NewCrossDeviceAndroid(Feature{Name: SmartLock}),
		Data: []string{AccountUtilZip, MultideviceSnippetZipName},
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"hansenmichael@google.com",
		},
		BugComponent: "b:1131772", // ChromeOS > Software > System Services > Cross Device > Smart Lock
		Vars: []string{
			skipAndroidLogin,
		},
		Parent:          "crossDeviceRemote",
		SetUpTimeout:    4 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: resetTimeout,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})
}

type crossdeviceAndroidFixture struct {
	adbDevice     *adb.Device
	androidDevice *AndroidDevice
	feature       Feature
	phoneIP       string
	ssid          string
	passphrase    string
}

func (f *crossdeviceAndroidFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	accountUtilZip := s.DataPath(AccountUtilZip)
	snippetZip := s.DataPath(MultideviceSnippetZipName)

	var networkDetails []string
	// Get the phone IP address from the remote fixture.
	if err := s.ParentFillValue(&networkDetails); err != nil {
		s.Fatal("Failed to deserialize fixture data with FixtFillValue: ", err)
	}
	s.Log("Parent fixture value is ", networkDetails)
	phoneIP := networkDetails[0]
	ssid := networkDetails[1]
	passphrase := networkDetails[2]

	// Set up adb, connect to the Android phone, and check if ADB root access is available.
	adbDevice, rooted, err := AdbSetup(ctx, phoneIP, ssid, passphrase)
	if err != nil {
		s.Fatal("Failed to set up an adb device: ", err)
	}
	f.adbDevice = adbDevice

	// Allocate time for saving logs in case of failure.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	// We want to ensure we have logs even if the Android device setup fails.
	fixtureLogcatPath := filepath.Join(s.OutDir(), "android_base_fixture_logcat.txt")
	defer adbDevice.DumpLogcat(cleanupCtx, fixtureLogcatPath)

	// Do some basic device set up like waking the screen and clearing logcat.
	if err := ConfigureDevice(ctx, adbDevice, rooted); err != nil {
		s.Fatal("Failed to prepare the Android device: ", err)
	}

	// Enable verbose logging for related modules.
	tags := []string{"ProximityAuth", "CryptauthV2", "NearbyConnections", "NearbyMediums"}
	if err := EnableVerboseLogging(ctx, adbDevice, rooted, tags...); err != nil {
		s.Fatal("Failed to enable verbose logs: ", err)
	}

	if err := OverrideFeatureFlags(ctx, adbDevice, f.feature); err != nil {
		s.Fatal("Failed to override required phenotype flags for feature ", f.feature.Name, ": ", err)
	}

	// Skip logging in to the test account on the Android device if specified in the runtime vars.
	// This lets you run the tests on a phone that's already signed in with your own account.
	loggedIn := false
	if val, ok := s.Var(skipAndroidLogin); ok {
		b, err := strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Unable to convert skipAndroidLogin var to bool: ", err)
		}
		loggedIn = b
	}
	androidUsername, androidPassword, err := GetLoginCredentials(ctx, s, f.feature)
	if err != nil {
		s.Fatal("Failed to get login credentials: ", err)
	}

	if !loggedIn {
		if rooted {
			if err := GAIALogin(ctx, adbDevice, accountUtilZip, androidUsername, androidPassword); err != nil {
				s.Fatal("Failed to log in on the Android device: ", err)
			}
		} else {
			s.Fatal("Cannot log in on Android on an unrooted phone")
		}
	}

	// Prepare the Multidevice Snippet.
	androidDevice, err := NewAndroidDevice(ctx, adbDevice, snippetZip)
	if err != nil {
		s.Fatal("Failed to prepare connected Android device for Multidevice testing: ", err)
	}
	f.androidDevice = androidDevice
	return &FixtData{
		AndroidDevice: androidDevice,
		Username:      androidUsername,
		Password:      androidPassword,
		PhoneIP:       phoneIP,
		SSID:          ssid,
		Passphrase:    passphrase,
	}
}
func (f *crossdeviceAndroidFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.androidDevice != nil {
		f.androidDevice.Cleanup(ctx)
	}
	if err := RemoveAccounts(ctx, f.adbDevice); err != nil {
		s.Log("Failed to remove accounts from the Android device: ", err)
	}
}
func (f *crossdeviceAndroidFixture) Reset(ctx context.Context) error                        { return nil }
func (f *crossdeviceAndroidFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (f *crossdeviceAndroidFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

// GetLoginCredentials returns the correct credentials to use based on the Cross Device feature being tested.
func GetLoginCredentials(ctx context.Context, s *testing.FixtState, feature Feature) (string, string, error) {
	var username, password string
	var err error

	switch feature.Name {
	case SmartLock:
		username, password, err = dma.UserPassFromPool(crossdevice.SmartLockPoolVarName)
		if err != nil {
			s.Fatal("Failed to get Smart Lock user and password: ", err)
		}
	case PhoneHub:
		username, password, err = dma.UserPassFromPool(crossdevice.DefaultCrossDevicePoolVarName)
		if err != nil {
			s.Fatal("Failed to get Phone Hub user and password: ", err)
		}
	default:
		return "", "", errors.New("unknown Cross Device feature specified")
	}

	s.Logf("GAIA account chosen: %s", username)
	return username, password, nil

}

// OverrideFeatureFlags overrides the required Phenotype flags for the given cross device feature.
func OverrideFeatureFlags(ctx context.Context, adbDevice *adb.Device, feature Feature) error {
	if err := adbDevice.OverridePhenotypeFlag(ctx, "com.google.android.gms.auth.proximity", "UnifiedBetterTogetherSetup__unify_better_together_host_set_feature_supported", "true", "boolean"); err != nil {
		return errors.Wrap(err, "failed to override required flag for Unified Better Together Setup")
	}
	switch feature.Name {
	case PhoneHub:
		// These flags need to be overridden before logging into the account so that they can have the desired values during CryptAuth enrollment.
		if err := adbDevice.OverridePhenotypeFlag(ctx, "com.google.android.gms.auth.proximity", "PhoneHub__enable_camera_roll", "true", "boolean"); err != nil {
			return errors.Wrap(err, "failed to override required flag for Phone Hub")
		}
		if err := adbDevice.OverridePhenotypeFlag(ctx, "com.google.android.gms.auth.proximity", "PhoneHub__set_camera_roll_host_supported", "true", "boolean"); err != nil {
			return errors.Wrap(err, "failed to override required flag for Phone Hub")
		}
		if err := adbDevice.OverridePhenotypeFlag(ctx, "com.google.android.gms.auth.proximity", "PhoneHub__enable_feature_setup_request", "true", "boolean"); err != nil {
			return errors.Wrap(err, "failed to override required flag for Phone Hub")
		}
		return nil
	case SmartLock:
		// These flags need to be overridden to ensure Nearby Share doesn't tear down Smart Lock's GATT connection when the phone's screen is unlocked (b/219981726).
		if err := adbDevice.OverridePhenotypeFlag(ctx, "com.google.android.gms.nearby", "connections_allow_control_ble_gatt_connection_in_advertising_option", "true", "boolean"); err != nil {
			return errors.Wrap(err, "failed to override required flag for SmartLock")
		}
		if err := adbDevice.OverridePhenotypeFlag(ctx, "com.google.android.gms.nearby", "sharing_enable_self_share_background_advertising", "true", "boolean"); err != nil {
			return errors.Wrap(err, "failed to override required flag for SmartLock")
		}
		return nil
	default:
		return nil
	}
}
