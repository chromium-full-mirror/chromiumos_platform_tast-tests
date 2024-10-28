// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "diagnosticsPrep",
		Desc: "Ensure relevant service is running before diagnostics ui test",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
		},
		// ChromeOS > Platform > Enablement > Health
		BugComponent:    "b:982097",
		Impl:            newDiagnosticsPrepFixture( /*disableTabletMode*/ false),
		SetUpTimeout:    chrome.LoginTimeout + 15*time.Second,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PreTestTimeout:  15 * time.Second,
		PostTestTimeout: 5 * time.Second,
		Parent:          "crosHealthdRunning",
	})

	testing.AddFixture(&testing.Fixture{
		Name: "diagnosticsPrepForInputDiagnostics",
		Desc: "Ensure relevant service is running before diagnostics ui test",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
		},
		// ChromeOS > Platform > Enablement > Health
		BugComponent:    "b:982097",
		Impl:            newDiagnosticsPrepFixture( /*disableTabletMode*/ true),
		SetUpTimeout:    chrome.LoginTimeout + 15*time.Second,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		PreTestTimeout:  15 * time.Second,
		PostTestTimeout: 5 * time.Second,
		Parent:          "crosHealthdRunning",
	})
}

const appURL = "chrome://diagnostics/"

// FixtureData contains the data available for use in diagnostics tests
type FixtureData struct {
	Cr    *chrome.Chrome
	Tconn *chrome.TestConn
}

// diagnosticsPrepFixture is a fixture to ensure relevant service is running
// before diagnostics ui test.
type diagnosticsPrepFixture struct {
	cr                *chrome.Chrome
	api               *MojoAPI
	tconn             *chrome.TestConn
	disableTabletMode bool
}

func newDiagnosticsPrepFixture(disableTabletMode bool) testing.FixtureImpl {
	return &diagnosticsPrepFixture{disableTabletMode: disableTabletMode}
}

func (f *diagnosticsPrepFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	success := false

	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if !success {
			cr.Close(ctx)
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if f.disableTabletMode {
		if err := ash.SetTabletModeEnabled(ctx, tconn, false); err != nil {
			s.Error("Failed to set the system mode: ", err)
		}
	}

	success = true
	f.cr = cr
	f.tconn = tconn

	return &FixtureData{Cr: cr, Tconn: tconn}
}

func (f *diagnosticsPrepFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome: ", err)
	}

	f.cr = nil
	f.tconn = nil
}

func (f *diagnosticsPrepFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *diagnosticsPrepFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if _, err := diagnosticsapp.Launch(ctx, f.tconn); err != nil {
		s.Fatal("Failed to launch diagnostics app: ", err)
	}

	// Make sure mojo API is connected.
	success := false
	var api *MojoAPI
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		if api, err = SystemDataProviderMojoAPI(ctx, f.cr); err != nil {
			return errors.Wrap(err, "unable to get systemDataProvider mojo API")
		}

		if err := api.RunFetchSystemInfo(ctx); err != nil {
			return errors.Wrap(err, "failed to fetch system info")
		}

		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to connect to mojo API: ", err)
	}

	defer func() {
		if !success {
			api.Release(ctx)
		}
	}()

	success = true
	f.api = api
}

func (f *diagnosticsPrepFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, f.cr, "ui_dump")

	if err := f.api.Release(ctx); err != nil {
		s.Log("Error releasing systemDataProvider mojo API: ", err)
	}

	if err := diagnosticsapp.Close(ctx, f.tconn); err != nil {
		s.Log("Failed to close diagnostics app: ", err)
	}

	f.api = nil
}
