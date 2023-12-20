// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"strings"
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
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var (
	crasEffectsHasAPNC = fixture.AloopLoaded{
		Channels: 2,
		Parent: fixture.Chrome(
			chrome.GuestLogin(),
			chrome.EnableFeatures("CrOSLateBootAudioAPNoiseCancellation"),
			chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
		),
	}.Instance()
	crasEffectsHasNoAPNC = fixture.AloopLoaded{
		Channels: 2,
		Parent: fixture.Chrome(
			chrome.GuestLogin(),
			chrome.ExtraArgs("--use-fake-cras-audio-client-for-dbus"),
		),
	}.Instance()
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
		Timeout:      30 * time.Second,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			// AEC provider tests.
			{
				Name: "dsp_aec",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectEnabled,
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectEnabled, // For DSP AEC.
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_0x0_conflict",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x0"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled, // DSP AEC blocked.
						DSPNC:   internal.EffectDisabled, // DSP AEC blocked.
						CrasAPM: internal.EffectEnabled,  // For AP NC.
						APNC:    internal.EffectEnabled,  // NC fallback.
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_0x10_conflict", // 0x10 is exactly the same as 0x0.
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x10"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled, // DSP AEC blocked.
						DSPNC:   internal.EffectDisabled, // DSP AEC blocked.
						CrasAPM: internal.EffectEnabled,  // For AP NC.
						APNC:    internal.EffectEnabled,  // NC fallback.
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_0x10_0x11_conflict",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x10"}},
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled, // DSP AEC blocked.
						DSPNC:   internal.EffectDisabled, // DSP AEC blocked.
						CrasAPM: internal.EffectEnabled,  // For AP NC.
						APNC:    internal.EffectEnabled,  // NC fallback.
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_0x0_dont_care",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{
							"--effects=0x0",
							"--client_type=5", // CRAS_CLIENT_TYPE_ARC
						}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled,
						DSPNC:   internal.EffectEnabled, // NC enabled.
						CrasAPM: internal.EffectDisabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_aec_0x0_conflict",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
						{flags: []string{"--effects=0x0"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled, // DSP AEC blocked.
						DSPNC:   internal.EffectDisabled, // DSP AEC blocked.
						CrasAPM: internal.EffectEnabled,  // For AP NC.
						APNC:    internal.EffectEnabled,  // AP NC fallback.
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_aec_0x0_dont_care",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
						{flags: []string{
							"--effects=0x0",
							"--client_type=5", // CRAS_CLIENT_TYPE_ARC
						}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectEnabled,
						DSPNC:   internal.EffectEnabled, // NC enabled.
						CrasAPM: internal.EffectEnabled, // For DSP AEC.
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_echo_ref_blocked_by_selection",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "ALSA_LOOPBACK",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled, // Blocked by echo reference: user selection.
						DSPNC:   internal.EffectDisabled, // Blocked by echo reference: user selection.
						CrasAPM: internal.EffectEnabled,  // CRAS AEC fallback.
						APNC:    internal.EffectEnabled,  // CRAS NC fallback.
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_echo_ref_blocked_by_playback",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					addPlaybackPinDevice:     "ALSA_LOOPBACK",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled, // Blocked by echo reference: playback.
						DSPNC:   internal.EffectDisabled, // Blocked by echo reference: playback.
						CrasAPM: internal.EffectEnabled,  // CRAS AEC fallback.
						APNC:    internal.EffectEnabled,  // CRAS NC fallback.
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			{
				Name: "dsp_echo_ref_not_blocked_by_playback",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					addPlaybackPinDevice:     "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectEnabled,
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectEnabled, // CRAS APM required by DSP AEC.
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
			},
			// NC provider tests with both DSP and AP NC.
			{
				Name: "nc_both_prefer_dsp",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}}, // Set AEC on to avoid blocking DSP NC.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectEnabled,
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectEnabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAPNCModels...)),
			},
			{
				Name: "nc_both_disabled",
				Val: crasEffectsParam{
					noiseCancellationEnabled: false,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}}, // Set AEC on to avoid blocking DSP NC.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectEnabled,
						DSPNC:   internal.EffectDisabled,
						CrasAPM: internal.EffectEnabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAPNCModels...)),
			},
			{
				Name: "nc_both_fallback_ap",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "ALSA_LOOPBACK", // Using non-internal speaker should block DSP AEC.
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}}, // Set AEC on to avoid blocking DSP NC.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled,
						DSPNC:   internal.EffectDisabled,
						CrasAPM: internal.EffectEnabled,
						APNC:    internal.EffectEnabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAPNCModels...)),
			},
			{
				Name: "nc_both_fallback_ap_disabled",
				Val: crasEffectsParam{
					noiseCancellationEnabled: false,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "ALSA_LOOPBACK", // Using non-internal speaker should block DSP AEC.
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}}, // Set AEC on to avoid blocking DSP NC.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectDisabled,
						DSPNC:   internal.EffectDisabled,
						CrasAPM: internal.EffectEnabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPAPNCModels...)),
			},
			// NC provider tests with only DSP NC.
			{
				Name: "nc_only_dsp_enabled",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "ALSA_LOOPBACK", // Using non-internal speaker should allow DSP NC.
					captureClients: []captureConfig{
						{flags: []string{"--effects=0"}}, // Effects=0 should not block.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectUnavailable,
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectDisabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasNoAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPNCOnlyModels...)),
			},
			{
				Name: "nc_only_dsp_block_select_internal_speaker",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER", // Using internal speaker should block DSP NC.
					captureClients: []captureConfig{
						{flags: []string{"--effects=0"}}, // Effects=0 should not block.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectUnavailable,
						DSPNC:   internal.EffectDisabled,
						CrasAPM: internal.EffectDisabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasNoAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPNCOnlyModels...)),
			},
			{
				Name: "nc_only_dsp_block_pin_internal_speaker",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "ALSA_LOOPBACK",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0"}}, // Effects=0 should not block.
					},
					addPlaybackPinDevice: "INTERNAL_SPEAKER", // Using internal speaker should block DSP NC.
					expectEffects: effects{
						DSPAEC:  internal.EffectUnavailable,
						DSPNC:   internal.EffectDisabled,
						CrasAPM: internal.EffectDisabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasNoAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPNCOnlyModels...)),
			},
			{
				Name: "nc_only_dsp_enabled_with_aec",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "ALSA_LOOPBACK", // Using non-internal speaker should allow DSP NC.
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x1"}}, // Effects=1 should not block.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectUnavailable,
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectEnabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasNoAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPNCOnlyModels...)),
			},
			{
				Name: "nc_only_dsp_block_select_internal_speaker_with_aec",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "INTERNAL_SPEAKER", // Using internal speaker should block DSP NC.
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x1"}}, // Effects=1 should not block.
					},
					expectEffects: effects{
						DSPAEC:  internal.EffectUnavailable,
						DSPNC:   internal.EffectDisabled,
						CrasAPM: internal.EffectEnabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasNoAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPNCOnlyModels...)),
			},
			{
				Name: "nc_only_dsp_block_pin_internal_speaker_with_aec",
				Val: crasEffectsParam{
					noiseCancellationEnabled: true,
					inputDevice:              "INTERNAL_MIC",
					outputDevice:             "ALSA_LOOPBACK",
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x1"}}, // Effects=1 should not block.
					},
					addPlaybackPinDevice: "INTERNAL_SPEAKER", // Using internal speaker should block DSP NC.
					expectEffects: effects{
						DSPAEC:  internal.EffectUnavailable,
						DSPNC:   internal.EffectDisabled,
						CrasAPM: internal.EffectEnabled,
						APNC:    internal.EffectDisabled,
					},
				},
				Fixture:           crasEffectsHasNoAPNC,
				ExtraHardwareDeps: hwdep.D(hwdep.Model(internal.DSPNCOnlyModels...)),
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
	DSPAEC  internal.EffectState
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

func internalCardName() (string, error) {
	cards, err := audio.GetSoundCards()
	if err != nil {
		return "", errors.Wrap(err, "audio.GetSoundCards")
	}
	for _, card := range cards {
		if ext, err := card.IsExternal(); err == nil && !ext && strings.HasPrefix(card.ShortName, "sof-") {
			return card.ShortName, nil
		}
	}
	return "", errors.Errorf("cannot get internal card name from %v", cards)
}

func resetNCState(ctx context.Context) error {
	cardName, err := internalCardName()
	if err != nil {
		return errors.Wrap(err, "cannot get internal card name")
	}
	ucmSuffix, err := crosconfig.Get(ctx, "/audio/main", "ucm-suffix")
	if err != nil && !crosconfig.IsNotFound(err) {
		return errors.Wrap(err, "cannot get ucm suffix")
	}

	if err := testexec.CommandContext(ctx,
		"alsaucm",
		fmt.Sprintf("-c%s.%s", cardName, ucmSuffix),
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

	if err := dlc.Install(ctx, "nc-ap-dlc", ""); err != nil {
		s.Fatal("Cannot install nc-ap-dlc: ", err)
	}

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

	if err := internal.SelectIODevices(ctx, cras, param.inputDevice, param.outputDevice); err != nil {
		s.Fatal("Failed to select IO devices: ", err)
	}
	if err := cras.SetNoiseCancellationEnabled(ctx, param.noiseCancellationEnabled); err != nil {
		s.Fatal("Failed to set noise cancellation: ", err)
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
		dspAEC, err := internal.DSPEchoCancellationState(ctx)
		if err != nil {
			s.Fatal("Cannot get DSPEchoCancellationState: ", err)
		}
		dspNC, err := internal.DSPNoiseCancellationState(ctx)
		if err != nil {
			s.Fatal("Cannot get DSPNoiseCancellationState: ", err)
		}
		snap, err := m.Snapshot(ctx)
		if err != nil {
			s.Fatal("Cannot get CrasProcessingState: ", err)
		}
		return effects{
			DSPAEC:  dspAEC,
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
