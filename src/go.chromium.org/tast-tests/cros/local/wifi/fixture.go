// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "hiddenNetworkMigration",
		Desc: "Logs into a user session where hidden networks are migrated at a more quick/test-friendly interval",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
		},
		Impl:            &hiddenNetworkMigrationFixture{},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

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
