// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const wifiDefaultTimeout = 30 * time.Second

// debugDataType keeps the output file name and the command to collect debug data.
var debugDataType = map[string]string{
	"dmesg.txt":    "dmesg",
	"ifconfig.txt": "ifconfig",
	"iw.txt":       "iw list",
	"lsmod.txt":    "lsmod",
	"lspci.txt":    "lspci -vvnn",
	"net.log":      "cat /var/log/net.log",
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "wiphyEnabled",
		Desc: "Ensure WiFi phy is enabled and otherwise collect debug logs",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
		},
		BugComponent:   "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Impl:           &wiphyEnabledFixture{},
		SetUpTimeout:   time.Minute + wifiDefaultTimeout,
		PreTestTimeout: time.Minute + wifiDefaultTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: "hiddenNetworkMigration",
		Desc: "Logs into a user session where hidden networks are migrated at a more quick/test-friendly interval",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
		},
		BugComponent:    "b:1131912", // ChromeOS > Software > Fundamentals > Connectivity > WiFi
		Impl:            &hiddenNetworkMigrationFixture{},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

type wiphyEnabledFixture struct {
	data *WiphyEnabledFixtureData
}

// WiphyEnabledFixtureData is the container for the wiphyEnabledFixture fixture
// that stores useful data to be shared among tests.
type WiphyEnabledFixtureData struct {
	ShillManager *shill.Manager
}

func (f *wiphyEnabledFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	s.Log("Connecting to shill manager proxy")
	sm, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}
	f.data = &WiphyEnabledFixtureData{
		ShillManager: sm,
	}
	return f.data
}

func (f *wiphyEnabledFixture) Reset(ctx context.Context) error { return nil }

func (f *wiphyEnabledFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	s.Log("Ensuring WiFi interface exists")
	_, err := shill.WifiInterface(ctx, f.data.ShillManager, wifiDefaultTimeout)
	if err != nil {
		for file, cmd := range debugDataType {
			SaveDebugData(ctx, s, file, cmd)
		}
		s.Fatal("Failed to get the WiFi interface: ", err)
	}
}

func (f *wiphyEnabledFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *wiphyEnabledFixture) TearDown(ctx context.Context, s *testing.FixtState) {}

// HiddenNetworkMigrationFixtureData provides a container for the data used by the
// hiddenNetworkMigrationFixture fixture and tests that use this fixture.
type HiddenNetworkMigrationFixtureData struct {
	Chrome *chrome.Chrome
}

type hiddenNetworkMigrationFixture struct {
	data *HiddenNetworkMigrationFixtureData
}

// Reset is called between tests to reset state.
func (f *hiddenNetworkMigrationFixture) Reset(ctx context.Context) error {
	if err := f.data.Chrome.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.data.Chrome.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

// PreTest is called before each test to perform required setup.
func (*hiddenNetworkMigrationFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest is called after each test to perform required cleanup.
func (*hiddenNetworkMigrationFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

// SetUp is called before any tests using this fixture are run to perform fixture setup.
func (f *hiddenNetworkMigrationFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(
		ctx,
		chrome.ExtraArgs("--hidden-network-migration-age=0"),
		chrome.ExtraArgs("--hidden-network-migration-interval=1"),
		chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.data = &HiddenNetworkMigrationFixtureData{
		Chrome: cr,
	}

	return f.data
}

// TearDown is called after all tests using this fixture have run to perform fixture cleanup.
func (f *hiddenNetworkMigrationFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.data.Chrome.Close(ctx); err != nil {
		s.Log("Failed to close Chrome: ", err)
	}
	f.data = nil
}
