// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides fixtures for mantis tast tests.
package fixture

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	commonfixture "go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var mantisPowerTestOptions = powersetup.PowerTestOptions{
	UpdateEngine:       powersetup.DoNotChangeUpdateEngine,
	NightLight:         powersetup.DisableNightLight,
	DarkTheme:          powersetup.EnableLightTheme,
	KeyboardBrightness: powersetup.SetKbBrightnessToZero,
}

const (
	resetTimeout    = 30 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

// List of fixture names for Mantis
const (
	// PowerAshGaiaWithUpdateEngine is a fixture for power test that depends on update-engine
	PowerAshGaiaWithUpdateEngine string = "powerAshGaiaWithUpdateEngine"
	LoggedInWithUpdateEngine     string = "loggedInWithUpdateEngine"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         PowerAshGaiaWithUpdateEngine,
		Desc:         "PowerAsh fixture for tests that depends on update engine",
		Contacts:     []string{"cros-mantis@google.com", "nurlitadf@google.com"},
		BugComponent: "b:1445284",
		Impl: powersetup.NewPowerUIFixture(mantisPowerTestOptions, powersetup.PowerFixtureOptions{
			EnableGAIALogin: true,
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + powersetup.SetUpTimeout,
		ResetTimeout:    powersetup.ResetTimeout,
		TearDownTimeout: powersetup.TearDownTimeout,
		PreTestTimeout:  powersetup.PreTestTimeout,
		PostTestTimeout: powersetup.PostTestTimeout,
		Parent:          commonfixture.UpdateEngine, // Ensure update engine is reset. go/cros-tast-updateengine-not-ready-error
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LoggedInWithUpdateEngine,
		Desc:            "Logged into a user session with update engine setup",
		Contacts:        []string{"cros-mantis@google.com", "nurlitadf@google.com"},
		BugComponent:    "b:1445284", // ChromeOS > Platform > Technologies > Machine Learning > On-Device ML
		Impl:            &fixture{},
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          commonfixture.UpdateEngine,
	})
}

type fixture struct {
	cr       *chrome.Chrome
	recorder *uiauto.ScreenRecorder
	tconn    *chrome.TestConn
}

// Data is the struct exposed to tests.
type Data struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
}

func gaiaLoginOption(ctx context.Context) (chrome.Option, error) {
	const (
		// TODO(crbug.com/404376751): Use the dedicated account pool with DMA consent enabled once available.
		pltpBaseURL = "https://storage.googleapis.com/chromiumos-test-assets-public/power_LoadTest/account"
		pltuURL     = pltpBaseURL + "/pltu_rand"
		pltpURL     = pltpBaseURL + "/pltp_rand"
	)

	usernames, err := utils.FetchFromURL(ctx, pltuURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch usernames")
	}
	password, err := utils.FetchFromURL(ctx, pltpURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch password")
	}

	names := strings.Split(usernames, "\n")
	password = strings.TrimSuffix(password, "\n")
	var loginPool strings.Builder
	// loginPool is a string containing multiple credentials separated by newlines:
	//
	// user1:pass1
	// user2:pass2
	// user3:pass3
	for _, n := range names {
		fmt.Fprintf(&loginPool, "%s:%s\n", n, password)
	}

	return chrome.GAIALoginPool(loginPool.String()), nil
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	loginPool, err := gaiaLoginOption(ctx)
	if err != nil {
		s.Fatal("Failed to get login pool: ", err)
	}
	// Prepare Chrome browser.
	opts := []chrome.Option{loginPool}

	f.cr, err = chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	f.tconn, err = f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	return Data{
		Chrome:      f.cr,
		TestAPIConn: f.tconn,
	}
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to tear down Chrome: ", err)
	}
	f.cr = nil
	f.tconn = nil
}

func (f *fixture) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	recorder, err := uiauto.NewScreenRecorder(ctx, f.tconn)
	if err != nil {
		s.Log("Failed to create screen recorder: ", err)
		return
	}
	if err := recorder.Start(ctx, f.tconn); err != nil {
		s.Log("Failed to start screen recorder: ", err)
		return
	}
	f.recorder = recorder
}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}
