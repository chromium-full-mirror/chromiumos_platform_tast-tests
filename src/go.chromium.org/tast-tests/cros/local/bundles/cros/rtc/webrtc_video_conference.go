// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rtc

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/rtc/webrtc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/pre"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         WebRTCVideoConference,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measure audio, camera, graphics and video stack performance in a video conference using WebRTC",
		Vars: []string{
			"rtc.WebRTCVideoConference.Mouse",
			"rtc.WebRTCVideoConference.NumPeople",
			"rtc.WebRTCVideoConference.Present",
			"rtc.WebRTCVideoConference.Text",
			"rtc.WebRTCVideoConference.Trace",
		},
		Contacts: []string{
			"hiroh@chromium.org", // Test Author.
			"chromeos-rtc@google.com",
		},
		BugComponent: "b:1401297", // ChromeOS > Platform > Technologies > RTC
		// TODO(hiroh): Look for a suite
		SoftwareDeps: []string{"chrome", caps.BuiltinCamera},
		HardwareDeps: hwdep.D(hwdep.Microphone()),
		Data:         webrtc.TestFiles(),
		// Extends the default time out by power.RecorderTimeout in order to
		// mitigate the failure of uploading power test metrics to power dashboard.
		Timeout: 5*time.Minute + power.RecorderTimeout,
		Params: []testing.Param{
			{
				Name: "custom",
				Val: webrtc.VCTestParams{
					NumPeople:   2,
					BrowserType: browser.TypeAsh,
				},
				Fixture: pre.ChromeRTCFixture(),
			},
			{
				Name: "step",
				Val: webrtc.VCTestParams{
					Step:        true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: pre.ChromeRTCFixture(),
			},
			{
				Name: "step_lacros",
				Val: webrtc.VCTestParams{
					Step:        true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           pre.ChromeRTCLacrosFixture(),
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "2p",
				Val: webrtc.VCTestParams{
					NumPeople:   2,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "9p",
				Val: webrtc.VCTestParams{
					NumPeople:   9,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "25p",
				Val: webrtc.VCTestParams{
					NumPeople:   25,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "49p",
				Val: webrtc.VCTestParams{
					NumPeople:   49,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_blur",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Blur:        true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:           pre.ChromeRTCFixture(),
				ExtraSoftwareDeps: []string{"camera_feature_effects"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_relight",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Relight:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:           pre.ChromeRTCFixture(),
				ExtraSoftwareDeps: []string{"camera_feature_effects"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_blur_relight",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Blur:        true,
					Relight:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:           pre.ChromeRTCFixture(),
				ExtraSoftwareDeps: []string{"camera_feature_effects"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Present:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_noise_cancellation",
				Val: webrtc.VCTestParams{
					NumPeople:         4,
					Present:           true,
					NoiseCancellation: true,
					BrowserType:       browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_noise_cancellation_style_transfer",
				Val: webrtc.VCTestParams{
					NumPeople:         4,
					Present:           true,
					NoiseCancellation: true,
					StyleTransfer:     true,
					BrowserType:       browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_text",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Text:        true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_text",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Present:     true,
					Text:        true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_text_noise_cancellation",
				Val: webrtc.VCTestParams{
					NumPeople:         4,
					Text:              true,
					Present:           true,
					NoiseCancellation: true,
					BrowserType:       browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_text_blur_relight",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Text:        true,
					Present:     true,
					Blur:        true,
					Relight:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:           pre.ChromeRTCFixture(),
				ExtraSoftwareDeps: []string{"camera_feature_effects"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_mouse",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Mouse:       true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_mouse",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Mouse:       true,
					Present:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_mouse_noise_cancellation",
				Val: webrtc.VCTestParams{
					NumPeople:         4,
					Mouse:             true,
					Present:           true,
					NoiseCancellation: true,
					BrowserType:       browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_mouse_blur_relight",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Mouse:       true,
					Present:     true,
					Blur:        true,
					Relight:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture:           pre.ChromeRTCFixture(),
				ExtraSoftwareDeps: []string{"camera_feature_effects"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_lacros",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           pre.ChromeRTCLacrosFixture(),
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_text_lacros",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Text:        true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           pre.ChromeRTCLacrosFixture(),
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_mouse_lacros",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Mouse:       true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           pre.ChromeRTCLacrosFixture(),
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_present_text_lacros",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					Text:        true,
					Present:     true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           pre.ChromeRTCLacrosFixture(),
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "4p_vsync_decoding",
				Val: webrtc.VCTestParams{
					NumPeople:   4,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(pre.VsyncDecoding),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "9p_vsync_decoding",
				Val: webrtc.VCTestParams{
					NumPeople:   9,
					BrowserType: browser.TypeAsh,
				},
				Fixture:   pre.ChromeRTCFixture(pre.VsyncDecoding),
				ExtraAttr: []string{"group:crosbolt", "crosbolt_perbuild"},
			},
			{
				Name: "9p_vsync_decoding_lacros",
				Val: webrtc.VCTestParams{
					NumPeople:   9,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           pre.ChromeRTCLacrosFixture(pre.VsyncDecoding),
				ExtraSoftwareDeps: []string{"lacros"},
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
			},
		},
	})
}

func WebRTCVideoConference(ctx context.Context, s *testing.State) {
	params := s.Param().(webrtc.VCTestParams)

	if val, ok := s.Var("rtc.WebRTCVideoConference.NumPeople"); ok {
		numPeople, err := strconv.ParseInt(val, 10, 32)
		if err != nil {
			s.Fatal("Failed to parse rtc.WebRTCVideoConference.NumPeople")
		}
		params.NumPeople = int(numPeople)
	}

	type ParamMap struct {
		args  string
		param *bool
	}
	for _, m := range []ParamMap{
		{"rtc.WebRTCVideoConference.Mouse", &params.Mouse},
		{"rtc.WebRTCVideoConference.Present", &params.Present},
		{"rtc.WebRTCVideoConference.Text", &params.Text},
		{"rtc.WebRTCVideoConference.Trace", &params.Trace},
	} {
		if val, ok := s.Var(m.args); ok {
			testing.ContextLog(ctx, (strings.ToLower(val) == "true"))
			*m.param = (strings.ToLower(val) == "true")
		}
	}

	if params.Step {
		if params.NumPeople > 0 {
			s.Fatal("Invalid parameters. The number of people cannot be set if Step is true")
		}
	} else {
		if params.NumPeople <= 1 {
			s.Fatal("Invalid parameters. The number of people cannot be less than 2")
		}
	}

	if params.Mouse && params.Text {
		s.Fatal("Invalid parameters: Cannot run both Mouse and Text in a single run")
	}

	tconn, err := s.FixtValue().(chrome.HasChrome).Chrome().TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Setup the device for power testing. This includes setting the battery to discharge mode in
	// order to be able to collect system power usage numbers.
	//
	// Note that we do this before possibly launching LaCrOS so that setup.PowerTest() doesn't
	// get confused with multiple windows.
	// Devices not supporting Chrome EC (e.g. drallion and sarien) fail to set the battery to discharge mode.
	// We ignore the error as this is expected. We simply won't collect the system power consumption metrics if the battery
	// is not set to discharge and the device does not support Chrome EC.
	// TODO(b/311061743): Set ignoreErr to false once all boards support chrome EC.
	cleanup, err := setup.PowerTest(ctx, tconn, setup.PowerTestOptions{
		Wifi:               setup.DisableWifiInterfaces,
		NightLight:         setup.DisableNightLight,
		DarkTheme:          setup.EnableLightTheme,
		KeyboardBrightness: setup.SetKbBrightnessToZero,
	}, setup.NewBatteryDischarge(true /*discharge*/, true /*ignoreErr*/, setup.DefaultDischargeThreshold))
	if err != nil {
		// This is not really an error: sometimes powerd is down or lost and setting
		// up the power test fails. Just don't provide any metric.
		s.Log("Skipping test, something went wrong during test set up: ", err)
		return
	}
	defer cleanup(ctx)

	cr, l, cs, err := lacros.Setup(ctx, s.FixtValue(), params.BrowserType)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}
	defer lacros.CloseLacros(ctx, l)

	var br *browser.Browser
	switch params.BrowserType {
	case browser.TypeAsh:
		br = cr.Browser()
	case browser.TypeLacros:
		br = l.Browser()
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API: ", err)
	}
	graphics.DisableSysLogCheck(s.TestName(),
		// Disable SysLogCheck for mediatek driver error because it is expected
		// that the driver fails decoding VP9 k-SVC and falls back to the
		// software decoder on MediaTek devices.
		graphics.SysLogMediatekVideoErrors,
		// Kernel splat happens on MediaTek 5.10+ due to the driver bug.
		// TODO(b/323065063): Remove this after the driver fixed.
		graphics.SysLogKernelSplats,
	)

	if err := webrtc.RunVideoConference(ctx, cs, tconn, bTconn, s, params); err != nil {
		s.Fatal("RunVideoConference failed: ", err)
	}
}
