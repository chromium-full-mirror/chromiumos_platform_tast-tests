// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/internal"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
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
		Fixture: fixture.Chrome(
			chrome.GuestLogin(),
			chrome.EnableFeatures("AudioSettingsPage", "QsRevamp"),
		),
		HardwareDeps: hwdep.D(hwdep.Model(internal.DSPAECModels...)),
		Timeout:      30 * time.Second,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name: "dsp_aec",
				Val: crasEffectsParam{
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x11"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectEnabled,
						CrasAPM: internal.EffectEnabled,  // For DSP AEC.
						APNC:    internal.EffectDisabled, // NC fallback not implemented.
					},
				},
			},
			{
				Name: "dsp_0x0_conflict",
				Val: crasEffectsParam{
					captureClients: []captureConfig{
						{flags: []string{"--effects=0x0"}},
					},
					expectEffects: effects{
						DSPNC:   internal.EffectDisabled, // DSP AEC blocked.
						CrasAPM: internal.EffectEnabled,  // Constructed with empty effects to block DSP AEC.
						APNC:    internal.EffectDisabled, // NC fallback not implemented.
					},
				},
			},
			{
				Name: "dsp_0x0_dont_care",
				Val: crasEffectsParam{
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
			},
			{
				Name: "dsp_aec_0x0_conflict",
				Val: crasEffectsParam{
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
			},
			{
				Name: "dsp_aec_0x0_dont_care",
				Val: crasEffectsParam{
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
			},
		},
	})
}

type crasEffectsParam struct {
	captureClients []captureConfig
	expectEffects  effects
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

func CrasEffects(ctx context.Context, s *testing.State) {
	param := s.Param().(crasEffectsParam)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to cr.TestAPIConn: ", err)
	}

	for _, device := range []string{"Microphone (internal)", "Speaker (internal)"} {
		if err := quicksettings.SelectAudioOption(ctx, tconn, device); err != nil {
			s.Fatalf("Failed to select %q in UI: %v", device, err)
		}
	}
	if err := quicksettings.ToggleNoiseCancellation(ctx, tconn, true); err != nil {
		s.Fatal("Failed to enable noise cancellation: ", err)
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
	captureCtx, cancelCapture := context.WithCancel(ctx)
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
				"--block_size=48000",
			)
			cmd.Args = append(cmd.Args, config.flags...)
			s.Log("Running capture with: ", cmd)
			if err := cmd.Run(); err != nil && ctx.Err() == nil {
				// Error happened not due to context cancelled.
				s.Error("Failed to run capture:", cmd.Args)
			}
		}(captureCtx)
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
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		got := currentProcessingState(ctx)
		if diff := cmp.Diff(param.expectEffects, got); diff != "" {
			return errors.Errorf("-want; +got: %s%s", "\n", diff)
		}
		return nil
	}, &testing.PollOptions{
		Interval: time.Second,
		Timeout:  5 * time.Second,
	}); err != nil {
		s.Error("Wrong effects running: ", err)
	}

	// Stop capture clients and wait.
	s.Log("Waiting for capture clients to terminate")
	cancelCapture()
	wg.Wait()
}
