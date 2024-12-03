// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast/core/testing"
)

// CrasSetUp is a ParameterizedFixture which sets up the CRAS, aloop and Chrome.
//
// Design note: why is this a fixture instead of a regular function?
//  1. Setting up Chrome is slow. It's beneficial to share the Chrome set up state
//     across multiple tests using a fixture.
//  2. To override a feature flag, we need to configure it in Chrome and verify
//     it in CRAS. If Chrome is set up inside a fixture, CRAS should be handled
//     by a fixture as well, to allow only specifying the flag for the fixture,
//     instead of having duplicated specifications for the test and the fixture.
//  3. As a bonus, fixtures' timeouts are separated from test timeouts.
type CrasSetUp struct {
	// Feature overrides that are visible to CRAS.
	CrasFeatures CrasFeatureOverrides

	// Aloop configuration.
	// nil for no aloop.
	// Aloop.Parent must not be set.
	Aloop *AloopLoaded

	// Set the VoiceIsolationUIEnabled D-Bus control.
	VoiceIsolationUIEnabled bool

	// The input device to select.
	InputDevice nodematch.Matcher
	// The output device to select.
	OutputDevice nodematch.Matcher
}

var _ ParameterizedFixture = CrasSetUp{}

var crasSetUpID int

// Instance implements ParameterizedFixture.
func (pf CrasSetUp) Instance() string {
	parent := ChromeForCras{
		CrasFeatures: pf.CrasFeatures,
	}.Instance()
	if pf.Aloop != nil {
		if pf.Aloop.Parent != "" {
			panic("Aloop.Parent must not be set")
		}
		aloopCopy := *pf.Aloop
		aloopCopy.Parent = parent
		parent = aloopCopy.Instance()
	}
	crasSetUpID++
	return maybeRegisterFixture(&testing.Fixture{
		Name:         fmt.Sprintf("crasSetUp%d", crasSetUpID),
		Desc:         fmt.Sprintf("Configure the ALSA loopback device with %v", pf),
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Impl: &crasSetUpFixtureImpl{
			config: &pf,
		},
		Parent:         parent,
		PreTestTimeout: 3 * time.Minute,
	})
}

// CrasFixtValue is the type of s.FixtValue() for CrasSetUp instances.
type CrasFixtValue interface {
	Cras() *audio.Cras
}

type crasSetUpFixtureImpl struct {
	config *CrasSetUp

	cras *audio.Cras
}

var _ testing.FixtureImpl = &crasSetUpFixtureImpl{}

var _ CrasFixtValue = &crasSetUpFixtureImpl{}

// Cras implements CrasFixtValue.
func (f *crasSetUpFixtureImpl) Cras() *audio.Cras {
	return f.cras
}

// SetUp implements FixtureImpl.
func (f *crasSetUpFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return f
}

// TearDown implements FixtureImpl.
func (f *crasSetUpFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}

// Reset implements FixtureImpl.
func (f *crasSetUpFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

// PreTest implements FixtureImpl.
func (f *crasSetUpFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	s.Logf("CrasSetUp.PreTest() for %s started", s.TestName())

	t0 := time.Now()

	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Cannot restart CRAS: ", err)
	}
	for feature, enabled := range f.config.CrasFeatures {
		if err := cras.WaitUntilFeatureFlagHasValue(ctx, string(feature), enabled); err != nil {
			s.Fatal("feature flag not propagated to CRAS: ", err)
		}
	}
	if err := cras.WaitForAudioEffectsReady(ctx); err != nil {
		s.Fatal("Faild to WaitForAudioEffectsReady(): ", err)
	}
	if err := audio.SelectIODevices(ctx, cras, f.config.InputDevice, f.config.OutputDevice); err != nil {
		s.Fatal("Failed to select IO devices: ", err)
	}
	if err := cras.SetVoiceIsolationUIEnabled(ctx, f.config.VoiceIsolationUIEnabled); err != nil {
		s.Fatal("Failed to set voice isolation enabled/disabled: ", err)
	}

	duration := time.Since(t0)
	s.Logf("CrasSetUp.PreTest() for %s completed in %v", s.TestName(), duration)

	f.cras = cras
}

// PostTest implements FixtureImpl.
func (f *crasSetUpFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	f.cras = nil
}
