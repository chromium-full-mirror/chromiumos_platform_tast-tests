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

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/debug"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/internal"
	"go.chromium.org/tast-tests/cros/local/chrome"
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

// AP effect names.
const (
	apAEC = "echo_cancellation"
	apNC  = "noise_cancellation"
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
						{effects: 0x11, expectAPEffects: nil},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectEnabled,
						NC:  internal.EffectEnabled,
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
						{effects: 0x0, expectAPEffects: []string{apNC}},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled, // DSP AEC blocked.
						NC:  internal.EffectDisabled, // DSP AEC blocked.
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
						{effects: 0x10, expectAPEffects: []string{apNC}},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled, // DSP AEC blocked.
						NC:  internal.EffectDisabled, // DSP AEC blocked.
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
						{effects: 0x10, expectAPEffects: []string{apNC}},
						{effects: 0x11, expectAPEffects: []string{apAEC, apNC}},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled, // DSP AEC blocked.
						NC:  internal.EffectDisabled, // DSP AEC blocked.
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
						{
							effects:         0x0,
							clientType:      5,   // CRAS_CLIENT_TYPE_ARC
							expectAPEffects: nil, // Effects applied on DSP.
						},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled,
						NC:  internal.EffectEnabled, // NC enabled.
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
						{effects: 0x11, expectAPEffects: []string{apNC, apAEC}},
						{effects: 0x0, expectAPEffects: []string{apNC}},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled, // DSP AEC blocked.
						NC:  internal.EffectDisabled, // DSP AEC blocked.
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
						{effects: 0x11, expectAPEffects: nil},
						{
							effects:         0x0,
							clientType:      5, // CRAS_CLIENT_TYPE_ARC
							expectAPEffects: nil,
						},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectEnabled,
						NC:  internal.EffectEnabled, // NC enabled.
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
						{effects: 0x11, expectAPEffects: []string{apAEC, apNC}},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled, // Blocked by echo reference: user selection.
						NC:  internal.EffectDisabled, // Blocked by echo reference: user selection.
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
						{effects: 0x11, expectAPEffects: []string{apAEC, apNC}},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled, // Blocked by echo reference: playback.
						NC:  internal.EffectDisabled, // Blocked by echo reference: playback.
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
						{effects: 0x11, expectAPEffects: nil},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectEnabled,
						NC:  internal.EffectEnabled,
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
						{
							effects:         0x11, // Set AEC on to avoid blocking DSP NC.
							expectAPEffects: nil,
						},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectEnabled,
						NC:  internal.EffectEnabled,
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
						{
							effects:         0x11, // Set AEC on to avoid blocking DSP NC.
							expectAPEffects: nil,
						},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectEnabled,
						NC:  internal.EffectDisabled,
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
						{
							effects:         0x11, // Set AEC on to avoid blocking DSP NC.
							expectAPEffects: []string{apAEC, apNC},
						},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled,
						NC:  internal.EffectDisabled,
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
						{
							effects:         0x11, // Set AEC on to avoid blocking DSP NC.
							expectAPEffects: []string{apAEC},
						},
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectDisabled,
						NC:  internal.EffectDisabled,
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
						{effects: 0, expectAPEffects: nil}, // Effects=0 should not block.
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectUnavailable,
						NC:  internal.EffectEnabled,
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
						{effects: 0, expectAPEffects: nil}, // Effects=0 should not block.
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectUnavailable,
						NC:  internal.EffectDisabled,
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
						{effects: 0, expectAPEffects: nil}, // Effects=0 should not block.
					},
					addPlaybackPinDevice: "INTERNAL_SPEAKER", // Using internal speaker should block DSP NC.
					expectDSPEffects: dspEffects{
						AEC: internal.EffectUnavailable,
						NC:  internal.EffectDisabled,
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
						{effects: 0x1, expectAPEffects: []string{apAEC}}, // Effects=1 should not block.
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectUnavailable,
						NC:  internal.EffectEnabled,
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
						{effects: 0x1, expectAPEffects: []string{apAEC}}, // Effects=1 should not block.
					},
					expectDSPEffects: dspEffects{
						AEC: internal.EffectUnavailable,
						NC:  internal.EffectDisabled,
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
						{effects: 0x1, expectAPEffects: []string{apAEC}}, // Effects=1 should not block.
					},
					addPlaybackPinDevice: "INTERNAL_SPEAKER", // Using internal speaker should block DSP NC.
					expectDSPEffects: dspEffects{
						AEC: internal.EffectUnavailable,
						NC:  internal.EffectDisabled,
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
	expectDSPEffects         dspEffects
}

// dspEffects observed and expected.
type dspEffects struct {
	AEC internal.EffectState
	NC  internal.EffectState
}

type streamState struct {
	effects               uint
	joinedActiveAPEffects string
}

func canonicalActiveAPEffectsString(effects []string) string {
	effects = slices.Clone(effects)
	slices.Sort(effects)
	return strings.Join(effects, " ")
}

func newStreamStateFromDebug(stream debug.Stream) streamState {
	return streamState{
		// Effects but remove the PRIVATE_DONT_CARE_APM_EFFECTS bit.
		effects:               stream.Effects &^ (1 << 10),
		joinedActiveAPEffects: canonicalActiveAPEffectsString(stream.ActiveAPEffects),
	}
}

type effectState struct {
	dsp dspEffects
	// Count of streams in each stream state.
	streams map[streamState]int
}

// satisfies checks whether es satisfies requirement.
// Returns a non-nil error when not satisfied.
func (es *effectState) satisfies(requirement *effectState) error {
	if es.dsp != requirement.dsp {
		return errors.New("dsp effect mismatch")
	}
	for ss, count := range requirement.streams {
		if es.streams[ss] < count {
			return errors.Errorf("want at least %d streams with %+v", count, ss)
		}
	}
	return nil
}

type captureConfig struct {
	effects    uint
	clientType uint
	// The expected AP effects on this stream.
	expectAPEffects []string
}

func (cc captureConfig) flags() []string {
	flags := []string{
		fmt.Sprintf("--effects=0x%x", cc.effects),
	}
	if cc.clientType != 0 {
		flags = append(flags, fmt.Sprintf("--client_type=%d", cc.clientType))
	}
	return flags
}

func (cc captureConfig) streamState() streamState {
	return streamState{
		effects:               cc.effects,
		joinedActiveAPEffects: canonicalActiveAPEffectsString(cc.expectAPEffects),
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

	if err := audio.SelectIODevices(ctx, cras, param.inputDevice, param.outputDevice); err != nil {
		s.Fatal("Failed to select IO devices: ", err)
	}
	if err := cras.SetNoiseCancellationEnabled(ctx, param.noiseCancellationEnabled); err != nil {
		s.Fatal("Failed to set noise cancellation: ", err)
	}

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
			cmd.Args = append(cmd.Args, config.flags()...)
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

	requiredState := &effectState{
		dsp:     param.expectDSPEffects,
		streams: map[streamState]int{},
	}
	for _, client := range param.captureClients {
		requiredState.streams[client.streamState()]++
	}

	currentProcessingState := func(ctx context.Context) *effectState {
		dspAEC, err := internal.DSPEchoCancellationState(ctx)
		if err != nil {
			s.Fatal("Cannot get DSPEchoCancellationState: ", err)
		}
		dspNC, err := internal.DSPNoiseCancellationState(ctx)
		if err != nil {
			s.Fatal("Cannot get DSPNoiseCancellationState: ", err)
		}
		debugInfo, err := debug.Dump(ctx)
		if err != nil {
			s.Fatal("Cannot get audio thread dump: ", err)
		}
		es := &effectState{
			dsp: dspEffects{
				AEC: dspAEC,
				NC:  dspNC,
			},
			streams: map[streamState]int{},
		}
		for _, stream := range debugInfo.Streams {
			es.streams[newStreamStateFromDebug(stream)]++
		}
		return es
	}
	checkCurrentProcessingState := func(ctx context.Context) error {
		got := currentProcessingState(ctx)
		if err := got.satisfies(requiredState); err != nil {
			return errors.Errorf("%v:%s%+v%s%+v",
				err,
				"\n  got:", got,
				"\n  req:", requiredState,
			)
		}
		return nil
	}

	if err := testing.Poll(
		ctx,
		func(ctx context.Context) error {
			if err := checkCurrentProcessingState(ctx); err != nil {
				return err
			}
			s.Log("First check passed")

			const rechecks = 3
			for i := 0; i < rechecks; i++ {
				const sleepFor = 200 * time.Millisecond
				s.Logf("Sleeping for %v to recheck state to ensure it is stablized", sleepFor)
				// GoBigSleepLint: See above log.
				if err := testing.Sleep(ctx, sleepFor); err != nil {
					return errors.Wrap(err, "cannot sleep")
				}
				if err := checkCurrentProcessingState(ctx); err != nil {
					return errors.Wrap(err, "wrong effects running after sleep")
				}
			}
			return nil
		},
		&testing.PollOptions{
			Interval: time.Second,
			Timeout:  5 * time.Second,
		}); err != nil {
		s.Fatal("Wrong effects running: ", err)
	}

	// Stop CRAS clients and wait.
	s.Log("Waiting for cras_test_clients to terminate")
	cancelCapture()
	wg.Wait()
}
