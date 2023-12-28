// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/ui/meetcuj"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeetCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of critical user journey for Google Meet",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"yichenz@chromium.org",
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
		},
		VarDeps: []string{
			"ui.MeetCUJ.bond_credentials",
		},
		Params: []testing.Param{
			{
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// TODO (b/286531724): Remove this variant and apply fake HAL
				// to all variants after performance evaluation is done.
				Name:    "fake_cam_720p",
				Timeout: meetcuj.DefaultTestTimeout,
				// group:camera_dependent is requested by tast lint for all
				// tests using camera libraries, so camera team can verify the
				// tests if there are camera lib changes.
				ExtraAttr: []string{"group:cuj", "group:camera_dependent"},
				Val: meetcuj.MeetTest{
					Bots:          []int{1, 3, 15},
					Layout:        googlemeet.TiledLayout,
					Cam:           true,
					ZoomOut:       true,
					Effects:       true,
					BrowserType:   browser.TypeAsh,
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
					BrowserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "docs",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			},
			{
				Name:      "docs_lacros",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			}, {
				Name:      "docs_no_effects",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				// Platform VC effects become available at feature level 1. Refer to
				// the feature database in platform/feature-management{,-private}.
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
					BotsOptions: []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:              "docs_audio_effects",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:              []int{1, 3, 15},
					Layout:            googlemeet.TiledLayout,
					Present:           true,
					Docs:              true,
					Split:             true,
					Cam:               true,
					ZoomOut:           true,
					LiveCaptions:      true,
					NoiseCancellation: true,
					BrowserType:       browser.TypeAsh,
					BotsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_noise_cancellation",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:              []int{1, 3, 15},
					Layout:            googlemeet.TiledLayout,
					Present:           true,
					Docs:              true,
					Split:             true,
					Cam:               true,
					ZoomOut:           true,
					NoiseCancellation: true,
					BrowserType:       browser.TypeAsh,
					BotsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_live_captions",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:         []int{1, 3, 15},
					Layout:       googlemeet.TiledLayout,
					Present:      true,
					Docs:         true,
					Split:        true,
					Cam:          true,
					ZoomOut:      true,
					LiveCaptions: true,
					BrowserType:  browser.TypeAsh,
					BotsOptions:  []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
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
					BrowserType:    browser.TypeAsh,
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
					BrowserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_adjust_lighting",
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
					AdjustLighting: true,
					BrowserType:    browser.TypeAsh,
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
					BrowserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_platform_effects",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:              []int{1, 3, 15},
					Layout:            googlemeet.TiledLayout,
					Present:           true,
					Docs:              true,
					Split:             true,
					Cam:               true,
					ZoomOut:           true,
					BackgroundBlur:    true,
					AdjustLighting:    true,
					LiveCaptions:      true,
					NoiseCancellation: true,
					BrowserType:       browser.TypeAsh,
					BotsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_platform_effects_lacros",
				Timeout:           meetcuj.DefaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetcuj.MeetTest{
					Bots:              []int{1, 3, 15},
					Layout:            googlemeet.TiledLayout,
					Present:           true,
					Docs:              true,
					Split:             true,
					Cam:               true,
					ZoomOut:           true,
					BackgroundBlur:    true,
					AdjustLighting:    true,
					LiveCaptions:      true,
					NoiseCancellation: true,
					BrowserType:       browser.TypeLacros,
					BotsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffectsLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "docs_partial_low_end_mode",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithPartialLowEndModeOnMidRangeDevices",
			},
			{
				Name:    "docs_blt_1gb",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingWithBackgroundLoad1GB",
			},
			{
				Name:    "docs_blt_2gb",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingWithBackgroundLoad2GB",
			},
			{
				Name:    "docs_blt_3gb",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingWithBackgroundLoad3GB",
			},
			{
				Name:    "docs_blt_4gb",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr:         []string{"group:cuj", "cuj_experimental"},
				ExtraHardwareDeps: hwdep.D(cuj.Experimental8GBModelConditions()...),
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingWithBackgroundLoad4GB",
			}, {
				Name:    "docs_battery_saver",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1, 3, 15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					Effects:     true,
					BrowserType: browser.TypeAsh,
				},
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Fixture:   "loggedInToCUJUserWithWebRTCEventLoggingWithBatterySaver",
			},
			{
				Name:      "16p_enterprise",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			},

			// TODO(crbug/1410581): Consider deprecating this test once
			// analysis on 49p_maincompositing is complete.
			{
				Name:      "49p",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:        []int{48},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
				// The list of targeted models which SPERA team uses to analyze.
				ExtraHardwareDeps: hwdep.D(hwdep.Model("gimble", "magpie", "lazor", "tomato", "volet")),
			},

			// Experimental Variants. These variants should only be run on
			// the minimized list of devices, and are running an A/B test for
			// particular feature.
			{
				// 16p call presenting a Google Doc. This is used as a baseline
				// for the following 16p_present_notes_split_* variants.
				Name:      "16p_present_notes_split",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// 16p_present_notes_split variant with
				// MainThreadCompositingPriority enabled.
				// TODO(crbug/1410581): Remove this variant when done with testing.
				Name:      "16p_present_notes_split_maincompositing",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithMainThreadCompositingPriority",
			}, {
				// 49p variant with MainThreadCompositingPriority feature enabled.
				// TODO(crbug/1410581): Remove this variant when done with testing.
				Name:      "49p_maincompositing",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:        []int{48},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithMainThreadCompositingPriority",
				// Same target models as in the 49p variant.
				ExtraHardwareDeps: hwdep.D(hwdep.Model("gimble", "magpie", "lazor", "tomato", "volet")),
			}, {
				Name:      "16p_present_notes_split_field_trials",
				Timeout:   meetcuj.DefaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
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
					Bots:        []int{3},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "lacros_4p_present_notes_split",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{3},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "4p_enterprise",
				Timeout: meetcuj.DefaultTestTimeout,
				// Lower priority test, so run on fewer devices.
				Val: meetcuj.MeetTest{
					Bots:        []int{3},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			},
			{
				Name:    "4p",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{3},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "lacros_4p",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{3},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "2p",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			},
			{
				Name:    "lacros_2p",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "2p_enterprise",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{1},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			}, {
				// Long meeting to catch slow performance degradation.
				Name:    "2p_30m",
				Timeout: meetcuj.DefaultTestTimeout + 30*time.Minute,
				Val: meetcuj.MeetTest{
					Bots:        []int{1},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					Duration:    30 * time.Minute,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "4p_present_notes_split_enterprise",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{3},
					Layout:      googlemeet.TiledLayout,
					Present:     true,
					Docs:        true,
					Split:       true,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			},
			{
				// 4p Meet call with Google Docs, with tab switching and
				// visual effects.
				Name:    "4p_notes_effects",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:          []int{3},
					Layout:        googlemeet.TiledLayout,
					Docs:          true,
					Cam:           true,
					Effects:       true,
					TabSwitchDocs: true,
					BrowserType:   browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// Lacros variant of 4p_notes_effects.
				Name:    "lacros_4p_notes_effects",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:          []int{3},
					Layout:        googlemeet.TiledLayout,
					Docs:          true,
					Cam:           true,
					Effects:       true,
					TabSwitchDocs: true,
					BrowserType:   browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			}, {
				// 16p with notes.
				Name:    "16p_notes",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Docs:        true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// 16p with jamboard test.
				Name:    "16p_jamboard",
				Timeout: meetcuj.DefaultTestTimeout + 15*time.Minute,
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Jamboard:    true,
					Split:       true,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// 49p with vp8 video codec.
				Name:    "49p_vp8",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{48},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeAsh,
					BotsOptions: []bond.AddBotsOption{bond.WithVP9(false, false)},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "lacros_49p",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{48},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					ZoomOut:     true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "16p",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "lacros_16p",
				Timeout: meetcuj.DefaultTestTimeout,
				Val: meetcuj.MeetTest{
					Bots:        []int{15},
					Layout:      googlemeet.TiledLayout,
					Cam:         true,
					BrowserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
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
//   - Open a Google Docs/Jamboard window (if necessary).
//   - Enter split mode (if necessary).
//   - Turn off camera (if necessary).
//
// During recording:
//   - Join the meeting.
//   - Add participants(bots) to the meeting.
//   - Set up the layout.
//   - Max out the number of the maximum tiles (if necessary).
//   - Start to present (if necessary).
//   - Input notes to Google Docs file or draw on Jamboard (if necessary).
//   - Navigate to Google Slides and input notes to file (if necessary).
//   - Navigate to Google Sheets and input notes to file (if necessary).
//   - Wait for 30 seconds before ending the meeting.
//
// After recording:
//   - Record and save metrics.
func MeetCUJ(ctx context.Context, s *testing.State) {
	meetcuj.Run(ctx, s)
}
