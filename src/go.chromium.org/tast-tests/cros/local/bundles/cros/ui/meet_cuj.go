// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/meetcuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: MeetCUJ,
		Desc: "Measures the performance of critical user journey for Google Meet",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@chromium.org",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		HardwareDeps: hwdep.D(
			hwdep.SkipOnModel("kaisa"),
			hwdep.SkipOnModel("kench"),
		),
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			cujrecorder.SystemTraceConfigFile,
			meetcuj.FakeCameraVideoFile720p,
		},
		Vars: []string{
			"mute",
			"ui.MeetCUJ.doc",

			// Boolean string to disable experiments.
			"ui.MeetCUJ.skipDisableExperiments",

			// Parsable test duration, like 10m or 60s, to run the Meet call.
			// Values can be added over 10 minutes, but this duration flag
			// is only supported up to 10 minutes, given that internal timeouts
			// might start to fail once the value becomes too high. Test time
			// defaults to 10 minutes.
			"ui.MeetCUJ.duration",
		},
		VarDeps: []string{
			"ui.MeetCUJ.bond_credentials",
		},
		Params: []testing.Param{
			{
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "present",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetcuj.MeetTest{
					Bots:           []int{15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Slides:         true,
					Sheets:         true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					TypingDuration: meetcuj.DefaultMeetTimeout / 3,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "docs",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "group:crosbolt", "crosbolt_release_gates"},
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "docs_arc_disabled",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingDisableARC",
			},
			{
				Name:      "docs_echo_measured",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					// Restrict meetings to a single bot participant. Echo cancellation algorithms often struggle to process multiple simultaneous bot voices.
					Bots:    []int{1},
					Layout:  googlemeet.TiledLayout,
					Present: true,
					Docs:    true,
					Split:   true,
					Cam:     true,
					ZoomOut: true,
					Effects: true,
					// The meeting bot must be enabled with human speech as the only audio source (no other noise) to accurately evaluate the echo RMS.
					MeasureEcho:   true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
					BotsOptions:   []bond.AddBotsOption{bond.WithAudio("sample_speech_10secs_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			},
			{
				Name:      "docs_nc_echo_measured",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					// Restrict meetings to a single bot participant. Echo cancellation algorithms often struggle to process multiple simultaneous bot voices.
					Bots:           []int{1},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					Effects:        true,
					StudioStyleMic: true,
					// The meeting bot must be enabled with human speech as the only audio source (no other noise) to accurately evaluate the echo RMS.
					MeasureEcho:   true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
					BotsOptions:   []bond.AddBotsOption{bond.WithAudio("sample_speech_10secs_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			},
			{
				Name:      "docs_no_effects",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "group:crosbolt", "crosbolt_perbuild"},
				// Platform VC effects become available at feature level 1. Refer to
				// the feature database in platform/feature-management{,-private}.
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
					BotsOptions:   []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "docs_no_effects_npu",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				// Platform VC effects become available at feature level 1. Refer to
				// the feature database in platform/feature-management{,-private}.
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				ExtraSoftwareDeps: []string{"npu"},
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					NpuInference:  true,
					ZoomOut:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
					BotsOptions:   []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:              "docs_audio_effects",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					LiveCaptions:   true,
					StudioStyleMic: true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
					BotsOptions:    []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_audio_effects_studio_mic",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					LiveCaptions:  true,
					StudioMic:     true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
					BotsOptions:   []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffectsAndStudioMic",
			}, {
				Name:              "docs_noise_cancellation",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					StudioStyleMic: true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
					BotsOptions:    []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_studio_mic",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj", "group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					StudioMic:     true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
					BotsOptions:   []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffectsAndStudioMic",
			}, {
				Name:              "docs_live_captions",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					LiveCaptions:  true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
					BotsOptions:   []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_background_blur",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					BackgroundBlur: true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_background_blur_and_meet_effects",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					Effects:        true,
					BackgroundBlur: true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_background_blur_npu_and_meet_effects",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				ExtraSoftwareDeps: []string{"npu"},
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					Effects:        true,
					BackgroundBlur: true,
					NpuInference:   true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_adjust_lighting",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj", "group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					AdjustLighting: true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_adjust_lighting_npu",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				ExtraSoftwareDeps: []string{"npu"},
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					AdjustLighting: true,
					NpuInference:   true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_retouch",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj", "group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					Retouch:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_adjust_lighting_and_retouch",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj", "group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					AdjustLighting: true,
					Retouch:        true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_video_effects",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					BackgroundBlur: true,
					AdjustLighting: true,
					Retouch:        true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_video_effects_npu",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				ExtraSoftwareDeps: []string{"npu"},
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					BackgroundBlur: true,
					AdjustLighting: true,
					Retouch:        true,
					NpuInference:   true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_platform_effects",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					BackgroundBlur: true,
					AdjustLighting: true,
					Retouch:        true,
					LiveCaptions:   true,
					StudioStyleMic: true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
					BotsOptions:    []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_platform_effects_npu",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				ExtraSoftwareDeps: []string{"npu"},
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					BackgroundBlur: true,
					AdjustLighting: true,
					Retouch:        true,
					NpuInference:   true,
					LiveCaptions:   true,
					StudioStyleMic: true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
					BotsOptions:    []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_platform_effects_studio_mic",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj", "group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					BackgroundBlur: true,
					AdjustLighting: true,
					Retouch:        true,
					LiveCaptions:   true,
					StudioMic:      true,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
					BotsOptions:    []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffectsAndStudioMic",
			}, {
				Name:    "docs_battery_saver",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithBatterySaver",
			}, {
				Name:    "docs_scx_central",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithScxCentral",
			}, {
				Name:      "docs_enterprise",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Enterprise:    true,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			},
			{
				Name:      "docs_field_trials",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithFieldTrialsAndWebRTCEventLogging",
			},
			// Inactive variants. No group should be specified for these tests.
			// 4p Meet variants.
			{
				Name:    "4p_present_notes_split",
				Timeout: meetcuj.DefaultTestTimeout,
				// Skip devices without cameras to see which devices in lab can run this variant.
				ExtraSoftwareDeps: []string{caps.BuiltinOrVividCamera},
				Val: meetcuj.MeetTest{
					Bots:          []int{3},
					Layout:        googlemeet.TiledLayout,
					Present:       true,
					Docs:          true,
					Split:         true,
					Cam:           true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			},
			{
				Name:    "coral",
				Timeout: meetcuj.DefaultTestTimeout,
				// No attrs because this test can only be triggered manually currently.
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					FakeCamHALCfg: meetcuj.FakeCamHALCfg720p,
				},
				Fixture:           "loggedInToCUJUserWithCoralEnabledAndWebRTCEventLogging",
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
			{
				Name:    "present_overview",
				Timeout: meetcuj.DefaultTestTimeout,
				// No attrs because this test is only for comparing with overview_coral, which can only be triggered manually currently.
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					Effects:        true,
					TypingDuration: meetcuj.DefaultMeetTimeout / 3,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
					TabsForOverview: []string{
						"chrome://device-log",
						"chrome://version",
						"chrome://histograms",
						"chrome://settings",
					},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			},
			{
				Name:    "present_overview_coral",
				Timeout: meetcuj.DefaultTestTimeout,
				// No attrs because this test can only be triggered manually currently.
				Val: meetcuj.MeetTest{
					Bots:           []int{1, 3, 15},
					Layout:         googlemeet.TiledLayout,
					Present:        true,
					Docs:           true,
					Split:          true,
					Cam:            true,
					ZoomOut:        true,
					Effects:        true,
					TypingDuration: meetcuj.DefaultMeetTimeout / 3,
					FakeCamHALCfg:  meetcuj.FakeCamHALCfg720p,
					TabsForOverview: []string{
						"chrome://device-log",
						"chrome://version",
						"chrome://histograms",
						"chrome://settings",
					},
				},
				Fixture:           "loggedInToCUJUserWithCoralEnabledAndWebRTCEventLogging",
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
			},
		},
	})
}

// MeetCUJ measures the performance of critical user journeys for Google Meet.
// Journeys for Google Meet are specified by testing parameters.
//
// Pre-preparation:
//   - Open a Meet window.
//   - Create and enter the meeting code.
//   - Open a Google Docs window (if necessary).
//   - Enter split mode (if necessary).
//   - Turn off camera (if necessary).
//
// During recording:
//   - Join the meeting.
//   - Add participants(bots) to the meeting.
//   - Set up the layout.
//   - Max out the number of the maximum tiles (if necessary).
//   - Start to present (if necessary).
//   - Input notes to Google Docs file (if necessary).
//   - Navigate to Google Slides and input notes to file (if necessary).
//   - Navigate to Google Sheets and input notes to file (if necessary).
//   - Wait for 30 seconds before ending the meeting.
//
// After recording:
//   - Record and save metrics.
func MeetCUJ(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Ensure that the Meet test parameters are properly formed.
	meet := s.Param().(meetcuj.MeetTest)

	var skipDisableExperiments bool
	skipDisableExperimentsStr, ok := s.Var("ui.MeetCUJ.skipDisableExperiments")
	if ok {
		var err error
		skipDisableExperiments, err = strconv.ParseBool(skipDisableExperimentsStr)
		if err != nil {
			s.Fatal("Failed to parse ui.MeetCUJ.skipDisableExperiments: ", err)
		}
	}

	// Disable experiments if the skipDisableExperiments flag isn't set.
	if !skipDisableExperiments {
		if meet.DisabledExperiments == nil {
			disabledExperiments, err := meetcuj.GetDisabledExperiments(ctx, s.CloudStorage())
			if err != nil {
				s.Fatal("Failed to get disabled experiments (if you don't care about disabling Meet experiments, rerun this test with -var=ui.MeetCUJ.skipDisableExperiments=true): ", err)
			}
			meet.DisabledExperiments = disabledExperiments
		}
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	creds := s.RequiredVar("ui.MeetCUJ.bond_credentials")

	if _, err := meetcuj.Run(ctx, meet, cr, s.Var, s.DataPath, s.OutDir(), creds); err != nil {
		s.Fatal("Failed to run MeetCUJ: ", err)
	}
}
