// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides fixtures for mantis tast tests.
package fixture

import (
	"context"

	commonfixture "go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
)

var mantisPowerTestOptions = powersetup.PowerTestOptions{
	UpdateEngine:       powersetup.DoNotChangeUpdateEngine,
	NightLight:         powersetup.DisableNightLight,
	DarkTheme:          powersetup.EnableLightTheme,
	KeyboardBrightness: powersetup.SetKbBrightnessToZero,
}

// PowerAshGaiaWithUpdateEngine is a fixture for power test that depends on update-engine
const PowerAshGaiaWithUpdateEngine string = "powerAshGaiaWithUpdateEngine"

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         "powerAshGaiaWithUpdateEngine",
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
}

type fixture struct {
	cr *chrome.Chrome
}

// Data is the struct exposed to tests.
type Data struct {
	Chrome *chrome.Chrome
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr
	return Data{
		Chrome: cr,
	}
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to tear down Chrome: ", err)
	}
	f.cr = nil
}

func (f *fixture) Reset(ctx context.Context) error {
	return nil
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
