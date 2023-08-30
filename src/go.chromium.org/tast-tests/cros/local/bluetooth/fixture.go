// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/bluetooth/floss"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWithBlueZ",
		Desc: "Logs into a user session with the Floss feature flag disabled",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            fixtureImplWithFeatures([]string{"QsRevamp"}, []string{"Floss"}, false),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWithFloss",
		Desc: "Logs into a user session with the Floss feature flag enabled",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            fixtureImplWithFeatures([]string{"QsRevamp", "Floss"}, []string{}, false),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "oobeWithBlueZ",
		Desc: "Enter Chrome OOBE with Floss feature flag disabled",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            fixtureImplWithFeatures([]string{"QsRevamp"}, []string{"Floss"}, true),
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "oobeWithFloss",
		Desc: "Enter Chrome OOBE with Floss feature flag enabled",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            fixtureImplWithFeatures([]string{"QsRevamp", "Floss"}, []string{}, true),
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "bluetoothEnabledWithBlueZ",
		Desc: "Logs into Chrome with Floss disabled, and enables Bluetooth during set up and tear down",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            &bluetoothEnabledFixt{btImpl: &bluez.BlueZ{}},
		Parent:          "chromeLoggedInWithBlueZ",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "bluetoothEnabledWithFloss",
		Desc: "Logs into Chrome with Floss enabled, and enables Bluetooth during set up and tear down",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            &bluetoothEnabledFixt{btImpl: &floss.Floss{}},
		Parent:          "chromeLoggedInWithFloss",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "bluetoothEnabledInOobeWithBlueZ",
		Desc: "Enter Chrome OOBE with Floss disabled, and enables Bluetooth during set up and tear down",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            &bluetoothEnabledFixt{btImpl: &bluez.BlueZ{}, isOobe: true},
		Parent:          "oobeWithBlueZ",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "bluetoothEnabledInOobeWithFloss",
		Desc: "Enter Chrome OOBE with Floss enabled, and enables Bluetooth during set up and tear down",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl:            &bluetoothEnabledFixt{btImpl: &floss.Floss{}, isOobe: true},
		Parent:          "oobeWithFloss",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

// fixtureImplWithFeatures returns an implementation of a fixture with specified features be enabled/disabled.
// noLoginAndEnterOobe indicates do not login but enter the OOBE instead.
func fixtureImplWithFeatures(enableFeatures, disableFeatures []string, noLoginAndEnterOobe bool) testing.FixtureImpl {
	return chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		opts := []chrome.Option{
			chrome.EnableFeatures(enableFeatures...), chrome.DisableFeatures(disableFeatures...),
		}
		if noLoginAndEnterOobe {
			opts = append(opts,
				chrome.NoLogin(),
				chrome.DontSkipOOBEAfterLogin(),
				chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
			)
		}
		return opts, nil
	})
}

// HasChrome is an interface for fixture values that contain a Chrome instance.
// It allows retrieval of the underlying Chrome object.
type HasChrome interface {
	Chrome() *chrome.Chrome
}

// HasTconn is an interface for fixture values that contain a Test API connection instance.
// It allows retrieval of the underlying Test API connection object.
type HasTconn interface {
	Tconn() *chrome.TestConn
}

// HasBluetoothImpl is an interface for fixture values that contain a Bluetooth implementation.
// It allows retrieval of the underlying Bluetooth implementation.
type HasBluetoothImpl interface {
	BluetoothImpl() Bluetooth
}

// bluetoothEnabledFixt provides an interface for Bluetooth tests
// that provides both access to the Chrome session and a Bluetooth
// implementation.
type bluetoothEnabledFixt struct {
	chrome *chrome.Chrome
	isOobe bool
	tconn  *chrome.TestConn
	btImpl Bluetooth
}

// Chrome returns the Chrome instance.
// It implements the chrome.HasChrome interface.
func (f *bluetoothEnabledFixt) Chrome() *chrome.Chrome { return f.chrome }

// Chrome returns the Test API connection.
// It implements the HasTconn interface.
func (f *bluetoothEnabledFixt) Tconn() *chrome.TestConn { return f.tconn }

// HasBluetoothImpl returns the Bluetooth implementation.
// It implements the HasBluetoothImpl interface.
func (f *bluetoothEnabledFixt) BluetoothImpl() Bluetooth { return f.btImpl }

// Reset is called between tests to reset state.
func (f *bluetoothEnabledFixt) Reset(ctx context.Context) error {
	if err := f.btImpl.Enable(ctx); err != nil {
		return errors.Wrap(err, "failed to enable Bluetooth")
	}
	return nil
}

// PreTest is called before each test to perform required setup.
func (*bluetoothEnabledFixt) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest is called after each test to perform required cleanup.
func (*bluetoothEnabledFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

// SetUp is called before any tests using this fixture are run to perform fixture setup.
func (f *bluetoothEnabledFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	f.chrome = s.ParentValue().(*chrome.Chrome)

	getTestAPIConn := f.chrome.TestAPIConn
	if f.isOobe {
		getTestAPIConn = f.chrome.SigninProfileTestAPIConn

		// Waits for OOBE to be ready for testing.
		oobeConn, err := f.chrome.WaitForOOBEConnection(ctx)
		if err != nil {
			s.Fatal("Failed to wait for OOBE connection: ", err)
		}
		defer oobeConn.Close()
	}

	var err error
	if f.tconn, err = getTestAPIConn(ctx); err != nil {
		s.Fatal("Failed to get Test API connection: ", err)
	}

	// The adapter may not be available immediately in OOBE, waits for an available adapter before enabling it.
	if err := f.btImpl.PollForAdapterAvailable(ctx); err != nil {
		s.Fatal("Failed to wait for available bluetooth adapter: ", err)
	}

	if err := f.btImpl.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}
	return f
}

// TearDown is called after all tests using this fixture have run to perform fixture cleanup.
func (f *bluetoothEnabledFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.btImpl.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}
}
