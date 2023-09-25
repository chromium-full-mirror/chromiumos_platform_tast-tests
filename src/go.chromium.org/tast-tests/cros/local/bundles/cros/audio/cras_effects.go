// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/internal"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasEffects,
		Desc:         "Check effects are executed on the right component",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
		},
		Fixture: fixture.AloopLoaded{
			Channels: 2,
			Parent: fixture.Chrome(
				chrome.GuestLogin(),
				chrome.EnableFeatures("AudioSettingsPage", "QsRevamp"),
			),
		}.Instance(),
		Timeout:      30 * time.Second,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "dsp_aec",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Speaker (internal)",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectEnabled,  // For DSP AEC.
						APNC:    internal.EffectDisabled, // NC fallback not implemented.
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_0x0_conflict",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Speaker (internal)",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x0"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectDisabled, // DSP AEC blocked.
						CrasAPM: internal.EffectDisabled,
						APNC:    internal.EffectDisabled, // NC fallback not implemented.
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_0x0_dont_care",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Speaker (internal)",
					captureClients: []captureConfig{
						{flags: []string{
							"--effects=0x0",
							"--client_type=5", // CRAS_CLIENT_TYPE_ARC
						}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectEnabled, // NC enabled.
						CrasAPM: internal.EffectDisabled,
						APNC:    internal.EffectDisabled,
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_aec_0x0_conflict",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Speaker (internal)",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
						{flags: []string{"--effects=0x0"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectDisabled, // DSP AEC blocked.
						CrasAPM: internal.EffectEnabled,  // Constructed with empty effects to block DSP AEC.
						APNC:    internal.EffectDisabled, // Fallback not implemented.
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_aec_0x0_dont_care",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Speaker (internal)",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
						{flags: []string{
							"--effects=0x0",
							"--client_type=5", // CRAS_CLIENT_TYPE_ARC
						}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectEnabled, // NC enabled.
						CrasAPM: internal.EffectEnabled, // For DSP AEC.
						APNC:    internal.EffectDisabled,
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_echo_ref_blocked_by_selection",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Loopback Playback",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectDisabled, // Blocked by echo reference: user selection.
						CrasAPM: internal.EffectEnabled,  // CRAS AEC fallback.
						APNC:    internal.EffectDisabled, // CRAS NC fallback not implemented.
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_echo_ref_blocked_by_playback",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Speaker (internal)",
					addPlaybackPinDevice:     "ALSA_LOOPBACK",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectDisabled, // Blocked by echo reference: playback.
						CrasAPM: internal.EffectEnabled,  // CRAS AEC fallback.
						APNC:    internal.EffectDisabled, // CRAS NC fallback not implemented.
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_echo_ref_not_blocked_by_playback",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "Microphone (internal)",
					outputDevice:             "Speaker (internal)",
					addPlaybackPinDevice:     "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectEnabled, // CRAS APM required by DSP AEC.
						APNC:    internal.EffectDisabled,
					},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
		},
	})
}

type crasEffectsParam struct {
	noiseCancellationEnabled bool
	inputDevice              string
	outputDevice             string
	addPlaybackPinDevice     string
	captureClients           []captureConfig
	expectEffects            effects
}

// effects observed and expected.
// A effect is set to EffectEnabled if it is enabled for any stream.
type effects struct {
	DSPNC   internal.EffectState
	CrasAPM internal.EffectState
	APNC    internal.EffectState
}

type captureConfig struct {
	flags []string
}

func toggleInputNoiseCancellation(ctx context.Context, s *testing.State, tconn *chrome.TestConn, state checked.Checked) {
	defer quicksettings.Hide(ctx, tconn)

	if err := ossettings.LaunchOsSettingsAudioPageFromQuickSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to open OS Settings audio page from Quick Settings: ", err)
	}

	ui := uiauto.New(tconn).WithTimeout(3 * time.Second)

	ncToggle := nodewith.Role(role.ToggleButton).NameContaining("Noise cancellation")

	if err := uiauto.Combine("Press noise cancellation toggle",
		ui.EnsureFocused(ncToggle),
		ui.DoDefault(ncToggle),
		ui.WaitUntilExists(ncToggle.Attribute("checked", state)))(ctx); err != nil {
		s.Fatal("Failed to press noise cancellation toggle: ", err)
	}
}

func resetNCState(ctx context.Context) error {
	ucmSuffix, err := crosconfig.Get(ctx, "/audio/main", "ucm-suffix")
	if err != nil && !crosconfig.IsNotFound(err) {
		return errors.Wrap(err, "cannot get ucm suffix")
	}

	if err := testexec.CommandContext(ctx,
		"alsaucm", "-csof-rt5682."+ucmSuffix,
		"set", "_verb", "HiFi",
		"set", "_enamod", "Internal Mic Noise Cancellation",
		"set", "_dismod", "Internal Mic Noise Cancellation",
	).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "alsaucm failed")
	}
	return nil
}

func CrasEffects(ctx context.Context, s *testing.State) {
	param := s.Param().(crasEffectsParam)

	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Cannot restart CRAS: ", err)
	}
	// b/301912218: This is needed because CRAS & the use case manager
	// assume that the modifiers are turned off initially,
	// e.g. on CRAS restart.
	if err := resetNCState(ctx); err != nil {
		s.Fatal("resetNCState failed: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to cr.TestAPIConn: ", err)
	}

	// Select internal mic/speaker initially so that NC UI is visible.
	for _, device := range []string{"Microphone (internal)", "Speaker (internal)"} {
		if err := quicksettings.SelectAudioOption(ctx, tconn, device); err != nil {
			s.Fatalf("Failed to select %q in UI: %v", device, err)
		}
	}
	if err := quicksettings.ToggleNoiseCancellation(ctx, tconn, param.noiseCancellationEnabled); err != nil {
		s.Fatal("Failed to enable noise cancellation: ", err)
	}
	for _, device := range []string{param.inputDevice, param.outputDevice} {
		if err := quicksettings.SelectAudioOption(ctx, tconn, device); err != nil {
			s.Fatalf("Failed to select %q in UI: %v", device, err)
		}
	}

	m, err := internal.NewCrasProcessingMonitor(ctx)
	if err != nil {
		s.Fatal("Cannot create CrasProcessingMonitor: ", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			s.Error("Cannot close CrasProcessingMonitor: ", err)
		}
	}()

	// Start capture clients.
	crasClientCtx, cancelCapture := context.WithCancel(ctx)
	defer cancelCapture()
	var wg sync.WaitGroup
	wg.Add(len(param.captureClients))
	for _, config := range param.captureClients {
		// Capture the variable.
		// See https://github.com/golang/go/issues/60078.
		config := config
		go func(ctx context.Context) {
			defer wg.Done()
			cmd := testexec.CommandContext(
				ctx,
				"cras_test_client",
				"-C", "/dev/null",
				"--block_size=480",
			)
			cmd.Args = append(cmd.Args, config.flags...)
			s.Log("Running capture with: ", cmd)
			if err := cmd.Run(); err != nil && ctx.Err() == nil {
				// Error happened not due to context cancelled.
				s.Error("Failed to run capture:", cmd.Args)
			}
		}(crasClientCtx)
	}

	if param.addPlaybackPinDevice != "" {
		node, err := cras.GetNodeByMatcher(ctx, audio.MatchNodeTypeDirection{Type: param.addPlaybackPinDevice, Direction: audio.OutputStream})
		if err != nil {
			s.Fatalf("Cannot find %q: %v", param.addPlaybackPinDevice, err)
		}
		deviceID := node.ID >> 32
		wg.Add(1)
		go func(ctx context.Context) {
			defer wg.Done()
			cmd := testexec.CommandContext(
				ctx,
				"cras_test_client",
				"-P", "/dev/zero",
				"--block_size=480",
				fmt.Sprintf("--pin_device=%d", deviceID),
			)
			s.Log("Running playback with: ", cmd)
			if err := cmd.Run(); err != nil && ctx.Err() == nil {
				// Error happened not due to context cancelled.
				s.Error("Failed to run playback:", cmd.Args)
			}
		}(crasClientCtx)
	}

	currentProcessingState := func(ctx context.Context) effects {
		dspNC, err := internal.DSPNoiseCancellationState(ctx)
		if err != nil {
			s.Fatal("Cannot get DSPNoiseCancellationState: ", err)
		}
		snap, err := m.Snapshot(ctx)
		if err != nil {
			s.Fatal("Cannot get CrasProcessingState: ", err)
		}
		return effects{
			DSPNC:   dspNC,
			CrasAPM: snap.CrasAPM,
			APNC:    snap.APNC,
		}
	}
	checkCurrentProcessingState := func(ctx context.Context) error {
		got := currentProcessingState(ctx)
		if diff := cmp.Diff(param.expectEffects, got); diff != "" {
			return errors.Errorf("-want; +got: %s%s", "\n", diff)
		}
		return nil
	}

	if err := testing.Poll(ctx, checkCurrentProcessingState, &testing.PollOptions{
		Interval: time.Second,
		Timeout:  5 * time.Second,
	}); err != nil {
		s.Fatal("Wrong effects running: ", err)
	}

	const rechecks = 3
	for i := 0; i < rechecks; i++ {
		const sleepFor = 200 * time.Millisecond
		s.Logf("Sleeping for %v to recheck state to ensure it is stablized", sleepFor)
		// GoBigSleepLint: See above log.
		if err := testing.Sleep(ctx, sleepFor); err != nil {
			s.Fatal("Cannot sleep: ", err)
		}
		if err := checkCurrentProcessingState(ctx); err != nil {
			s.Fatal("Wrong effects running after sleep: ", err)
		}
	}

	// Stop CRAS clients and wait.
	s.Log("Waiting for cras_test_clients to terminate")
	cancelCapture()
	wg.Wait()
}
