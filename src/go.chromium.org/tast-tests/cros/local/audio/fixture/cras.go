// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
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
	// The name of the fixture that sets up Chrome.
	// Setting ChromeFixture removes the automatic Chrome configuration done by ChromeForCras.
	ChromeFixture string

	// Feature overrides that are visible to CRAS.
	CrasFeatures CrasFeatureOverrides

	// Aloop configuration.
	// nil for no aloop.
	// Aloop.Parent must not be set.
	Aloop *AloopLoaded

	// Set the VoiceIsolationUIEnabled D-Bus control.
	VoiceIsolationUIEnabled bool
	// SetVoiceIsolationUIPreferredEffect if not zero.
	VoiceIsolationUIPreferredEffect audio.VoiceIsolationPreferredEffect

	// The input device to select.
	InputDevice nodematch.Matcher
	// The output device to select.
	OutputDevice nodematch.Matcher
}

var _ ParameterizedFixture = CrasSetUp{}

var crasSetUpID int

// Instance implements ParameterizedFixture.
func (pf CrasSetUp) Instance() string {
	var parent string
	if pf.ChromeFixture != "" {
		parent = pf.ChromeFixture
	} else {
		parent = ChromeForCras{
			CrasFeatures: pf.CrasFeatures,
		}.Instance()
	}
	if pf.Aloop != nil {
		if pf.Aloop.Parent != "" {
			panic("Aloop.Parent must not be set")
		}
		aloopCopy := *pf.Aloop
		aloopCopy.Parent = parent
		parent = aloopCopy.internalInstance() // Use the internal aloop fixture. Our fixture restarts CRAS already.
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

// DoCras performs set up for the CRAS part.
//
// Most code should use CrasSetUp as a fixture as it also helps configure Chrome and aloop.
// Only use this when Chrome and aloop are already configured elsewhere.
func (pf CrasSetUp) DoCras(ctx context.Context) (*audio.Cras, error) {
	if pf.VoiceIsolationUIPreferredEffect == audio.VoiceIsolationEffectBeamforming {
		if err := audio.CheckBeamforming(ctx); err != nil {
			return nil, errors.Wrap(err, "audio.CheckBeamforming")
		}
	}

	// Stop CRAS and install DLCs.
	if err := upstart.StopJob(ctx, "cras"); err != nil {
		return nil, errors.Wrap(err, "cannot stop CRAS")
	}
	{
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute) // Retry DLC installation up to 2 minutes.
		defer cancel()
		if err := testexec.CommandContext(ctx, "cras_server_tool", "install-dlcs").Run(testexec.DumpLogOnError); err != nil {
			return nil, errors.Wrap(err, "cannot install CRAS DLCs")
		}
	}

	cras, err := audio.RestartCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "cannot restart CRAS")
	}
	for feature, enabled := range pf.CrasFeatures {
		if err := cras.WaitUntilFeatureFlagHasValue(ctx, string(feature), enabled); err != nil {
			return nil, errors.Wrap(err, "feature flag not propagated to CRAS")
		}
	}
	if err := cras.WaitForAudioEffectsReady(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to WaitForAudioEffectsReady()")
	}
	if err := audio.SelectIODevices(ctx, cras, pf.InputDevice, pf.OutputDevice); err != nil {
		return nil, errors.Wrap(err, "failed to select IO devices")
	}
	if pf.VoiceIsolationUIPreferredEffect != 0 {
		if err := cras.SetVoiceIsolationUIPreferredEffect(ctx, pf.VoiceIsolationUIPreferredEffect); err != nil {
			return nil, errors.Wrap(err, "failed to SetVoiceIsolationUIPreferredEffect")
		}
	}
	if err := cras.SetVoiceIsolationUIEnabled(ctx, pf.VoiceIsolationUIEnabled); err != nil {
		return nil, errors.Wrap(err, "failed to set voice isolation enabled/disabled")
	}
	return cras, nil
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

	cras, err := f.config.DoCras(ctx)
	if err != nil {
		s.Fatal("DoCras(): ", err)
	}

	duration := time.Since(t0)
	s.Logf("CrasSetUp.PreTest() for %s completed in %v", s.TestName(), duration)

	f.cras = cras
}

// PostTest implements FixtureImpl.
func (f *crasSetUpFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	f.cras = nil
}
