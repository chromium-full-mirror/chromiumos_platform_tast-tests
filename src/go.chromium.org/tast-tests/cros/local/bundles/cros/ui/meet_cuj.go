// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/async"
	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/meetcuj"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/loginstatus"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast-tests/cros/local/webrtcinternals"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// fakeCameraHALCfg defines parameters that are used to generate the fake
// camera HAL config.
type fakeCameraHALCfg struct {
	// videoFileName is the video used by the fake HAL to simulate the
	// real camera input.
	videoFileName string
	// formats defines the supported resolution / frame rates of the fake
	// camera HAL. Fake HAL will use whatever resolution / frame rates passed
	// down from upper layer, and this config only affects what's reported in
	// the camera static metadata.
	formats []*testutil.FakeCameraFormatsConfig
}

// meetTest specifies the setting of a Google Meet journey. More info at go/cros-meet-tests.
type meetTest struct {
	// bots is an increasing list of bot counts that should be in the call
	// during the test. There should be at least one value in this list, and
	// the first bot count value has to be greater than 0, to account for the
	// spotlight bot that is in every test.
	bots []int

	layout            googlemeet.LayoutOption // Type of the layout in the meeting.
	present           bool                    // Whether it is presenting the Google Docs/Jamboard window.
	docs              bool                    // Whether it is running with a Google Docs window.
	slides            bool                    // Whether it is running with a Google Slides window.
	sheets            bool                    // Whether it is running with a Google Sheets window.
	jamboard          bool                    // Whether it is running with a Jamboard window.
	split             bool                    // Whether it is in split screen mode. It can not be true if docs is false.
	cam               bool                    // Whether the camera is on or not.
	effects           bool                    // Whether to turn on visual effects within Meet.
	backgroundBlur    bool                    // Whether to turn on platform-level background blur.
	adjustLighting    bool                    // Whether to turn on the platform-level adjust lighting feature.
	liveCaptions      bool                    // Whether to turn on live captioning.
	noiseCancellation bool                    // Whether to turn on noise cancellation.
	zoomOut           bool                    // Whether to zoom out on both the browser and display.
	tabSwitchDocs     bool                    // Whether to switch between Docs and Meet. It cannot be true if docs is false.
	duration          time.Duration           // Duration of the meet call. Must be less than test timeout.
	typingDuration    time.Duration           // Duration of typing on Google Docs. Must be less than the duration of the meet call. If |typingDuration| is not given, it defaults to |meetTimeout|.
	browserType       browser.Type            // Ash Chrome browser or Lacros.
	botsOptions       []bond.AddBotsOption    // Customizes the meeting participant bots.
	fakeCamHALCfg     *fakeCameraHALCfg       // Enable Fake Camera HAL if the config is present.
}

const (
	defaultTestTimeout = 25 * time.Minute
	defaultMeetTimeout = 10 * time.Minute

	fakeCameraVideoFile720p = "camera_video_720p.y4m"
)

var fakeCamHALCfg720p = &fakeCameraHALCfg{
	// The video file used to simulate camera input.
	videoFileName: fakeCameraVideoFile720p,
	// Resolutions / frame rates that the fake HAL supports.
	formats: []*testutil.FakeCameraFormatsConfig{{
		Width:      320,
		Height:     180,
		FrameRates: []int{30}}, {
		Width:      640,
		Height:     360,
		FrameRates: []int{30}}, {
		Width:      1280,
		Height:     720,
		FrameRates: []int{30}},
	},
}

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
			fakeCameraVideoFile720p,
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
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetTest{
					bots:        []int{1, 3, 15},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					zoomOut:     true,
					effects:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// TODO (b/286531724): Remove this variant and apply fake HAL
				// to all variants after performance evaluation is done.
				Name:    "fake_cam_720p",
				Timeout: defaultTestTimeout,
				// group:camera_dependent is requested by tast lint for all
				// tests using camera libraries, so camera team can verify the
				// tests if there are camera lib changes.
				ExtraAttr: []string{"group:cuj", "group:camera_dependent"},
				Val: meetTest{
					bots:          []int{1, 3, 15},
					layout:        googlemeet.TiledLayout,
					cam:           true,
					zoomOut:       true,
					effects:       true,
					browserType:   browser.TypeAsh,
					fakeCamHALCfg: fakeCamHALCfg720p,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "present",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetTest{
					bots:           []int{15},
					layout:         googlemeet.TiledLayout,
					present:        true,
					docs:           true,
					slides:         true,
					sheets:         true,
					split:          true,
					cam:            true,
					zoomOut:        true,
					typingDuration: defaultMeetTimeout / 3,
					browserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "docs",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetTest{
					bots:        []int{1, 3, 15},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					effects:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			},
			{
				Name:      "docs_lacros",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetTest{
					bots:        []int{1, 3, 15},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					effects:     true,
					browserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			}, {
				Name:      "docs_no_effects",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				// Platform VC effects become available at feature level 1. Refer to
				// the feature database in platform/feature-management{,-private}.
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:        []int{1, 3, 15},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
					botsOptions: []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:              "docs_audio_effects",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:              []int{1, 3, 15},
					layout:            googlemeet.TiledLayout,
					present:           true,
					docs:              true,
					split:             true,
					cam:               true,
					zoomOut:           true,
					liveCaptions:      true,
					noiseCancellation: true,
					browserType:       browser.TypeAsh,
					botsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_noise_cancellation",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:              []int{1, 3, 15},
					layout:            googlemeet.TiledLayout,
					present:           true,
					docs:              true,
					split:             true,
					cam:               true,
					zoomOut:           true,
					noiseCancellation: true,
					browserType:       browser.TypeAsh,
					botsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_live_captions",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:         []int{1, 3, 15},
					layout:       googlemeet.TiledLayout,
					present:      true,
					docs:         true,
					split:        true,
					cam:          true,
					zoomOut:      true,
					liveCaptions: true,
					browserType:  browser.TypeAsh,
					botsOptions:  []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_background_blur",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:           []int{1, 3, 15},
					layout:         googlemeet.TiledLayout,
					present:        true,
					docs:           true,
					split:          true,
					cam:            true,
					zoomOut:        true,
					backgroundBlur: true,
					browserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_background_blur_and_meet_effects",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:           []int{1, 3, 15},
					layout:         googlemeet.TiledLayout,
					present:        true,
					docs:           true,
					split:          true,
					cam:            true,
					zoomOut:        true,
					effects:        true,
					backgroundBlur: true,
					browserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_adjust_lighting",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:           []int{1, 3, 15},
					layout:         googlemeet.TiledLayout,
					present:        true,
					docs:           true,
					split:          true,
					cam:            true,
					zoomOut:        true,
					adjustLighting: true,
					browserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_video_effects",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:           []int{1, 3, 15},
					layout:         googlemeet.TiledLayout,
					present:        true,
					docs:           true,
					split:          true,
					cam:            true,
					zoomOut:        true,
					backgroundBlur: true,
					adjustLighting: true,
					browserType:    browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_platform_effects",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:              []int{1, 3, 15},
					layout:            googlemeet.TiledLayout,
					present:           true,
					docs:              true,
					split:             true,
					cam:               true,
					zoomOut:           true,
					backgroundBlur:    true,
					adjustLighting:    true,
					liveCaptions:      true,
					noiseCancellation: true,
					browserType:       browser.TypeAsh,
					botsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffects",
			}, {
				Name:              "docs_platform_effects_lacros",
				Timeout:           defaultTestTimeout,
				ExtraAttr:         []string{"group:cuj"},
				ExtraHardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
				Val: meetTest{
					bots:              []int{1, 3, 15},
					layout:            googlemeet.TiledLayout,
					present:           true,
					docs:              true,
					split:             true,
					cam:               true,
					zoomOut:           true,
					backgroundBlur:    true,
					adjustLighting:    true,
					liveCaptions:      true,
					noiseCancellation: true,
					browserType:       browser.TypeLacros,
					botsOptions:       []bond.AddBotsOption{bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw")},
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingWithVCEffectsLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "docs_partial_low_end_mode",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetTest{
					bots:        []int{1, 3, 15},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					effects:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithPartialLowEndModeOnMidRangeDevices",
			},
			// 4p Meet variants.
			{
				Name:      "4p_present_notes_split",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				// Skip devices without cameras to see which devices in lab can run this variant.
				ExtraSoftwareDeps: []string{caps.BuiltinOrVividCamera},
				Val: meetTest{
					bots:        []int{3},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:      "lacros_4p_present_notes_split",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj"},
				Val: meetTest{
					bots:        []int{3},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					browserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "16p_enterprise",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			},

			// TODO(crbug/1410581): Consider deprecating this test once
			// analysis on 49p_maincompositing is complete.
			{
				Name:      "49p",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetTest{
					bots:        []int{48},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
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
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// 16p_present_notes_split variant with
				// MainThreadCompositingPriority enabled.
				// TODO(crbug/1410581): Remove this variant when done with testing.
				Name:      "16p_present_notes_split_maincompositing",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithMainThreadCompositingPriority",
			}, {
				// 49p variant with MainThreadCompositingPriority feature enabled.
				// TODO(crbug/1410581): Remove this variant when done with testing.
				Name:      "49p_maincompositing",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetTest{
					bots:        []int{48},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithMainThreadCompositingPriority",
				// Same target models as in the 49p variant.
				ExtraHardwareDeps: hwdep.D(hwdep.Model("gimble", "magpie", "lazor", "tomato", "volet")),
			}, {
				Name:      "16p_present_notes_split_field_trials",
				Timeout:   defaultTestTimeout,
				ExtraAttr: []string{"group:cuj", "cuj_experimental"},
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithFieldTrialsAndWebRTCEventLogging",
			},
			// Inactive variants. No group should be specified for these tests.
			{
				Name:    "4p_enterprise",
				Timeout: defaultTestTimeout,
				// Lower priority test, so run on fewer devices.
				Val: meetTest{
					bots:        []int{3},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			},
			{
				Name:    "4p",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{3},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "lacros_4p",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{3},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "2p",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{1},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			},
			{
				Name:    "lacros_2p",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{1},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "2p_enterprise",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{1},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			}, {
				// Long meeting to catch slow performance degradation.
				Name:    "2p_30m",
				Timeout: defaultTestTimeout + 30*time.Minute,
				Val: meetTest{
					bots:        []int{1},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					duration:    30 * time.Minute,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "4p_present_notes_split_enterprise",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{3},
					layout:      googlemeet.TiledLayout,
					present:     true,
					docs:        true,
					split:       true,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserEnterpriseWithWebRTCEventLogging",
			},
			{
				// 4p Meet call with Google Docs, with tab switching and
				// visual effects.
				Name:    "4p_notes_effects",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:          []int{3},
					layout:        googlemeet.TiledLayout,
					docs:          true,
					cam:           true,
					effects:       true,
					tabSwitchDocs: true,
					browserType:   browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// Lacros variant of 4p_notes_effects.
				Name:    "lacros_4p_notes_effects",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:          []int{3},
					layout:        googlemeet.TiledLayout,
					docs:          true,
					cam:           true,
					effects:       true,
					tabSwitchDocs: true,
					browserType:   browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			}, {
				// 16p with notes.
				Name:    "16p_notes",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					docs:        true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// 16p with jamboard test.
				Name:    "16p_jamboard",
				Timeout: defaultTestTimeout + 15*time.Minute,
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					jamboard:    true,
					split:       true,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				// 49p with vp8 video codec.
				Name:    "49p_vp8",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{48},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeAsh,
					botsOptions: []bond.AddBotsOption{bond.WithVP9(false, false)},
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "lacros_49p",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{48},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					zoomOut:     true,
					browserType: browser.TypeLacros,
				},
				Fixture:           "loggedInToCUJUserWithWebRTCEventLoggingLacros",
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:    "16p",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeAsh,
				},
				Fixture: "loggedInToCUJUserWithWebRTCEventLogging",
			}, {
				Name:    "lacros_16p",
				Timeout: defaultTestTimeout,
				Val: meetTest{
					bots:        []int{15},
					layout:      googlemeet.TiledLayout,
					cam:         true,
					browserType: browser.TypeLacros,
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
	const (
		// The addBotTimeout allows 3 2-minute BondAPI request retries by the
		// Bond lib.
		addBotTimeout = 6*time.Minute + 10*time.Second
		// addBotRetries is the local retry number for adding bots.
		addBotRetries  = 3
		defaultDocsURL = "https://docs.new/"
		jamboardURL    = "https://jamboard.google.com"
		newTabTitle    = "New Tab"
	)

	notes := strings.Split("Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.", "")

	// Ensure that the Meet test parameters are properly formed.
	meet := s.Param().(meetTest)
	if meet.docs && meet.jamboard {
		s.Fatal("Tried to open both Google Docs and Jamboard at the same time")
	}
	if meet.tabSwitchDocs && !meet.docs {
		s.Fatal("Cannot tab switch docs without opening a Google Doc")
	}
	if len(meet.bots) == 0 {
		s.Fatal("Must have at least 1 bot count")
	}
	if meet.bots[0] == 0 {
		s.Fatal("First bot count must have at least 1 bot, to add the spotlight bot")
	}

	// Determines the Meet call duration. Use the Meet duration specified in
	// test param if there is one. Otherwise, default to 10 minutes.
	meetTimeout := defaultMeetTimeout
	if meet.duration != 0 {
		meetTimeout = meet.duration
	}
	s.Log("Run meeting for ", meetTimeout)

	// Shorten context to allow for cleanup. Reserve one minute in case of power
	// test.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	if meet.fakeCamHALCfg != nil {
		const cameraService = "cros-camera"
		// Always restart cros-camera in the end.
		defer upstart.RestartJob(closeCtx, cameraService)

		// Configure CrOS to use only fake HAL camera.
		if err := testutil.SetupTestConfig(ctx, testutil.UseFakeHALCamera); err != nil {
			s.Fatal("Failed to set up camera test config: ", err)
		}
		defer testutil.RemoveTestConfig(closeCtx)

		// Copy the fake camera video to where the camera module can access.
		dutFakeHALPath, err := testutil.CopyFakeHALFrameImage(s.DataPath(meet.fakeCamHALCfg.videoFileName))
		if err != nil {
			s.Fatal("Failed to copy fake camera input: ", err)
		}
		defer os.Remove(dutFakeHALPath)

		// Write the fake HAL config to the system.
		fakeCameraConfig := testutil.FakeCameraConfig{
			ID:        1,
			Connected: true,
			Frames: &testutil.FakeCameraImageConfig{
				Path: dutFakeHALPath,
			},
			SupportedFormats: meet.fakeCamHALCfg.formats,
		}
		fakeHALConfig := testutil.FakeHALConfig{
			Cameras: []testutil.FakeCameraConfig{fakeCameraConfig},
		}
		if err := testutil.WriteFakeHALConfig(ctx, fakeHALConfig); err != nil {
			s.Fatal("Failed to configure HAL camera: ", err)
		}
		defer testutil.RemoveFakeHALConfig(closeCtx)

		if err := upstart.RestartJob(ctx, cameraService); err != nil {
			s.Fatalf("Failed to restart %s after camera setup: %v", cameraService, err)
		}
	}

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to the test API connection: ", err)
	}

	if meet.zoomOut {
		// Sets the display zoom factor to minimum, to ensure that all
		// meeting participants' video can be shown simultaneously.
		// The display zoom should be done before the Meet window is
		// opened, to prevent visual oddities on some devices. We also
		// zoom out on the browser after the Meet window is opened,
		// because on some boards the display zoom is not enough to
		// show all of the participants.
		revertZoom, err := display.MinimizePrimaryDisplayZoomFactor(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to set the zoom factor of the primary display to minimum: ", err)
		}
		defer revertZoom(closeCtx, tconn)
	}

	var cs ash.ConnSource
	var br *browser.Browser
	var bTconn *chrome.TestConn
	switch meet.browserType {
	case browser.TypeLacros:
		// Launch lacros.
		l, err := lacros.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch lacros: ", err)
		}
		defer l.Close(closeCtx)
		cs = l
		br = l.Browser()

		if bTconn, err = l.TestAPIConn(ctx); err != nil {
			s.Fatal("Failed to get lacros TestAPIConn: ", err)
		}
	case browser.TypeAsh:
		cs = cr
		br = cr.Browser()
		bTconn = tconn
	}

	creds := s.RequiredVar("ui.MeetCUJ.bond_credentials")
	bc, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
	if err != nil {
		s.Fatal("Failed to create a bond client: ", err)
	}
	defer bc.Close()

	meetingCode, err := bc.CreateConference(ctx)
	if err != nil {
		s.Fatal("Failed to create a conference room: ", err)
	}
	s.Log("Created a room with the code ", meetingCode)

	sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
	defer cancel()
	defer func(ctx context.Context) {
		s.Log("Removing all bots from the call")
		if _, _, err := bc.RemoveAllBots(ctx, meetingCode); err != nil {
			s.Log("Failed to remove all bots: ", err)
		}
	}(closeCtx)

	// Create a bot with spotlight layout to request HD video.
	spotlightBotList, _, err := bc.AddBots(sctx, meetingCode, 1, meetTimeout+30*time.Minute, append(meet.botsOptions, bond.WithLayout("SPOTLIGHT"))...)
	if err != nil {
		s.Fatal("Failed to create bot with spotlight layout: ", err)
	}
	if len(spotlightBotList) != 1 {
		s.Fatalf("Unexpected number of bots with spotlight layout successfully started; got %d, expected 1", len(spotlightBotList))
	}

	// Keep track of how many bots are already in the call, so we can add the
	// right number of bots later in the test.
	botsInCall := 1

	addBots := func(ctx context.Context, numBots int) error {
		sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
		defer cancel()

		testing.ContextLogf(ctx, "Adding %d bots to the call", numBots)

		if numBots == 0 {
			return nil
		}

		botsToAdd := numBots
		wait := 100 * time.Millisecond
		for i := 0; i < addBotRetries; i++ {
			// GoBigSleepLint: A short sleep before next call to Bond API.
			if err := testing.Sleep(ctx, wait); err != nil {
				s.Errorf("Failed to sleep for %v: %v", wait, err)
			}
			// Add 30 minutes to the bot duration, to ensure that the bots stay long
			// enough for the test to get info from chrome://webrtc-internals.
			botList, numFailures, err := bc.AddBots(sctx, meetingCode, botsToAdd, meetTimeout+30*time.Minute, meet.botsOptions...)
			if err != nil {
				s.Fatalf("Failed to create %d bots: %v", botsToAdd, err)
			}
			s.Logf("%d bots started, %d bots failed", len(botList), numFailures)
			botsToAdd -= len(botList)
			if botsToAdd <= 0 {
				break
			}
		}

		if botsToAdd > 0 {
			return errors.Errorf("failed to add all %d bots to the call; %d to be added after %d retries", numBots, botsToAdd, addBotRetries)
		}
		return nil
	}

	numBotsToAdd := meet.bots[0] - botsInCall
	if err := addBots(ctx, numBotsToAdd); err != nil {
		s.Fatalf("Failed to initially add %d bots: %v", numBotsToAdd, err)
	}
	botsInCall += numBotsToAdd

	tabChecker, err := cuj.NewTabCrashChecker(ctx, bTconn)
	if err != nil {
		s.Fatal("Failed to create TabCrashChecker: ", err)
	}

	// Ensure that even if the Meet call crashes outside of our direct
	// crash checks, we still log that the crash happened.
	caughtTabCrash := false
	defer func(ctx context.Context) {
		if caughtTabCrash {
			return
		}
		if err := tabChecker.Check(ctx); err != nil {
			s.Log("Tab renderer crashed: ", err)
		}
	}(ctx)

	assertTabActive := func(ctx context.Context) {
		if err := tabChecker.Check(ctx); err != nil {
			caughtTabCrash = true
			s.Fatal("Tab renderer crashed: ", err)
		}
	}

	meetHelper := googlemeet.NewHRTelemetryHelper(cs, tconn)
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}

	if err := recorder.AddCollectedMetrics(bTconn, meet.browserType,
		cujrecorder.NewCustomMetricConfig("Cras.MissedCallbackFrequencyInput", "millisecond", perf.SmallerIsBetter),
		cujrecorder.NewCustomMetricConfig("Cras.MissedCallbackFrequencyOutput", "millisecond", perf.SmallerIsBetter)); err != nil {
		s.Fatal("Failed to add metrics to recorder: ", err)
	}

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to recorder: ", err)
	}

	// Take a screenshot every 2 minutes up to a maximum of 5
	// screenshots, to ensure we capture any bots that drop during
	// the call and any issues that come up with the collab window.
	if err := recorder.AddScreenshotRecorder(ctx, 2*time.Minute, 5); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	defer func() {
		if err := recorder.Close(closeCtx); err != nil {
			s.Error("Failed to stop recorder: ", err)
		}
	}()

	if err := cuj.WaitForValidAccountInCookieJar(ctx, br, tconn); err != nil {
		s.Fatal("Failed to wait for valid account in cookie jar: ", err)
	}

	// Open chrome://webrtc-internals now so it will collect data on the meeting's streams.
	webrtcInternals, err := recorder.NewConn(ctx, br, "WebRTC_Internals", "chrome://webrtc-internals", browser.WithNewWindow())
	if err != nil {
		s.Fatal("Failed to open chrome://webrtc-internals: ", err)
	}
	defer webrtcInternals.Close()

	webRTCInternalsWindow, err := ash.FindOnlyWindow(ctx, tconn, ash.BrowserTitleMatch(meet.browserType, "WebRTC Internals"))
	if err != nil {
		s.Fatal("Failed to find the WebRTC Internals window: ", err)
	}

	// Maximize the WebRTC Internals window now, in preparation to take
	// a screenshot of it if DumpWebRTCInternals fails at the end of
	// the test. That screenshot is for investigation of b/255343902.
	// TODO(b/255343902): Remove this when the bug is fixed.
	if err := ash.SetWindowStateAndWait(ctx, tconn, webRTCInternalsWindow.ID, ash.WindowStateMaximized); err != nil {
		s.Log("Failed to ensure that the WebRTC Internals window is maximized: ", err)
	}

	// Lacros specific setup.
	if meet.browserType == browser.TypeLacros {
		// Close "New Tab" window after creating the chrome://webrtc-internals window.
		w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool {
			return strings.HasPrefix(w.Title, newTabTitle) && strings.HasPrefix(w.Name, "ExoShellSurface")
		})
		if err != nil {
			s.Fatal("Failed to find New Tab window: ", err)
		}
		if err := w.CloseWindow(ctx, tconn); err != nil {
			s.Fatal("Failed to close New Tab window: ", err)
		}
	}

	if meet.docs {
		// Ensure docs offline support is enabled to avoid docs page hitting
		// fatal network error. See http://b/254914987
		if err := cuj.EnsureDocsOfflineEnabled(ctx, br, tconn); err != nil {
			s.Fatal("Failed to enable docs offline support: ", err)
		}
	}

	// Ensure that we close the Meet window at the end of the test in case
	// the test fails. Closing the Meet window should occur after dumping
	// the UI tree, so the UI dump actually contains the proper failure.
	closedMeet := false
	defer func() {
		if closedMeet {
			return
		}
		// Close the windows to finish the meeting.
		if err := ash.CloseAllWindows(closeCtx, tconn); err != nil {
			s.Error("Failed to close all windows: ", err)
		}
	}()

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Autorelease the automation tree when it is not used so that excessive
	// automation events in lacros runs do not consumer too much cpu/power.
	// See b/278649596.
	automationAutoRelease, err := uiauto.NewScopedAutoRelease(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to create automation ScopedAutoRelease: ", err)
	}
	defer automationAutoRelease.Reset(ctx)

	if err := cuj.ExpandCreateDumpSection(ctx, tconn); err != nil {
		s.Fatal("Failed to expand Create Dump section of chrome://webrtc-internals: ", err)
	}

	var names []string
	for name := range cujrecorder.WebRTCMetricInfo {
		names = append(names, name)
	}
	webRTCMetricsRecorder, err := metrics.StartRecorder(ctx, bTconn, names...)
	if err != nil {
		s.Fatal("Failed to start recording WebRTC metrics: ", err)
	}

	if err := meetHelper.JoinMeeting(ctx, meetingCode, browser.WithNewWindow()); err != nil {
		s.Fatal("Failed to open the hangout meet website: ", err)
	}
	defer meetHelper.Close(closeCtx)

	// Match window titles `Google Meet` and `meet.google.com`.
	meetRE := regexp.MustCompile(`\bMeet\b|\bmeet\.\b`)
	meetWindow, err := ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return meetRE.MatchString(w.Title) })
	if err != nil {
		s.Fatal("Failed to find the Meet window: ", err)
	}

	inTabletMode, err := ash.TabletModeEnabled(ctx, tconn)
	s.Logf("Is in tablet-mode: %t", inTabletMode)
	if err != nil {
		s.Fatal("Failed to detect it is in tablet-mode or not: ", err)
	}
	var pc pointer.Context
	var mw *input.MouseEventWriter
	if inTabletMode {
		// If it is in tablet mode, ensure it it in landscape orientation.
		// TODO(crbug/1135239): test portrait orientation as well.
		orientation, err := display.GetOrientation(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get display orientation: ", err)
		}
		if orientation.Type == display.OrientationPortraitPrimary {
			info, err := display.GetPrimaryInfo(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get the primary display info: ", err)
			}
			s.Log("Rotating display 90 degrees")
			if err := display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
				s.Fatal("Failed to rotate display: ", err)
			}
			defer display.SetDisplayRotationSync(closeCtx, tconn, info.ID, display.Rotate0)
		}
		pc, err = pointer.NewTouch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to create a touch controller: ", err)
		}
	} else {
		// Make it into a maximized window if it is in clamshell-mode.
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
		}); err != nil {
			s.Fatal("Failed to turn all windows into maximized state: ", err)
		}
		pc = pointer.NewMouse(tconn)

		mw, err = input.Mouse(ctx)
		if err != nil {
			s.Fatal("Failed to create a mouse: ", err)
		}
		defer mw.Close(ctx)
	}
	defer pc.Close(ctx)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kw.Close(closeCtx)

	// Find the web view of Meet window.
	webview := nodewith.ClassName("ContentsWebView").Role(role.WebView)

	const longUITimeout = time.Minute
	// Check and grant permissions.
	if err := prompts.ClearPotentialPrompts(
		tconn,
		longUITimeout,
		prompts.ShowNotificationsPrompt,
		prompts.AllowAVPermissionPrompt,
		prompts.AllowMicrophoneAndCameraPermissionPrompt,
	)(ctx); err != nil {
		s.Fatal("Failed to grant permissions: ", err)
	}

	// Ensure to check that we are properly in the meeting before trying
	// to apply visual effects.
	if err := meetHelper.IsInMeeting(ctx, time.Minute); err != nil {
		s.Fatal("Failed to wait to enter the meeting: ", err)
	}

	expectedParticipantCount := botsInCall + 1
	checkParticipantCount := func(ctx context.Context, expectedCount int) error {
		participantCount, err := meetHelper.GetParticipantCount(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get participant count")
		}
		if participantCount != expectedCount {
			return errors.Errorf("got %d participants, expected %d", participantCount, expectedCount)
		}
		return nil
	}

	ui := uiauto.New(tconn)
	uiLongWait := ui.WithTimeout(longUITimeout)
	meetRootWebArea := nodewith.NameContaining("Meet").Role(role.RootWebArea)
	participantText := nodewith.NameRegex(regexp.MustCompile(`^[\d]+$`)).Role(role.StaticText).Ancestor(meetRootWebArea)
	if err := uiauto.NamedAction("wait for the number of participants to be loaded",
		// Some DUT models have poor performance. When joining a large conference
		// (over 15 participants), it would take much time to render DOM elements.
		// Set a longer timer here.
		uiLongWait.WaitUntilExists(participantText),
	)(ctx); err != nil {
		s.Fatal("Failed to wait for participant info: ", err)
	}
	if err := checkParticipantCount(ctx, expectedParticipantCount); err != nil {
		s.Fatal("The number of bots is unexpected: ", err)
	}

	doDefaultMoreOptions := func(ctx context.Context) error {
		moreOptionsFinder := nodewith.Name("More options").Role(role.PopUpButton)
		moreOptionsButtons, err := ui.NodesInfo(ctx, moreOptionsFinder)
		if err != nil || len(moreOptionsButtons) < 1 {
			return errors.Wrap(err, "failed to find more options button")
		}
		// Sometimes, the UI has two identical "More Options" buttons, which requires
		// selecting the last one to be the correct button.
		return ui.DoDefault(moreOptionsFinder.Nth(len(moreOptionsButtons) - 1))(ctx)
	}

	applyEffects := nodewith.Name("Apply visual effects").Role(role.MenuItem)
	blur := nodewith.Name("Blur your background").Role(role.ToggleButton).Focusable()
	turnOffBlur := nodewith.Name("Turn off visual effects").Role(role.ToggleButton).Focusable()
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(meetRootWebArea).Focusable()
	setEffect := func(ctx context.Context, effect *nodewith.Finder) error {
		return uiauto.Combine(
			fmt.Sprintf("set effect with node %v", effect),
			// Open the "More options" popup, and wait until we see
			// "Apply visual effects".
			doDefaultMoreOptions,
			uiLongWait.WaitUntilExists(applyEffects),

			// Open the visual effects section.
			ui.DoDefault(applyEffects),
			uiLongWait.WaitUntilExists(effect),

			// Toggle the effect.
			uiLongWait.DoDefaultUntil(effect,
				ui.WithTimeout(5*time.Second).WaitUntilCheckedState(effect, true)),

			// Close the visual effects section.
			ui.DoDefault(closeButton),
			uiLongWait.WaitUntilGone(effect),
		)(ctx)
	}
	if meet.effects {
		s.Log("Turn on visual effects")
		if err := setEffect(ctx, blur); err != nil {
			s.Fatal("Failed to turn on visual effects: ", err)
		}
	} else {
		if err := setEffect(ctx, turnOffBlur); err != nil {
			s.Fatal("Failed to turn off visual effects: ", err)
		}
	}

	if meet.liveCaptions ||
		meet.adjustLighting ||
		meet.backgroundBlur ||
		meet.noiseCancellation {
		s.Log("Toggling platform VC effects")
		vct := vctray.New(ctx, tconn)
		blur := vctray.BackgroundBlurOff
		if meet.backgroundBlur {
			blur = vctray.BackgroundBlurFull
		}
		if err := vct.ChangeSettingsInPanel(
			vct.SetLiveCaption(meet.liveCaptions),
			vct.SetAdjustLighting(meet.adjustLighting),
			vct.SetBackgroundBlur(blur),
			vct.SetNoiseCancellation(meet.noiseCancellation),
		)(ctx); err != nil {
			s.Fatal("Failed to configure platform VC effects: ", err)
		}
	}

	s.Log("Resetting browser zoom to 100%")
	zoomNode := nodewith.HasClass("ZoomView")
	if err := uiauto.Combine(
		"reset zoom and wait for zoom indicator to be absent",
		kw.AccelAction("Ctrl+0"),
		ui.WaitUntilGone(zoomNode),
	)(ctx); err != nil {
		s.Fatal("Failed to press Ctrl+0 to reset the zoom: ", err)
	}

	if meet.zoomOut {
		// Zoom out on the browser to maximize the number of visible video
		// feeds. This needs to be done before the final layout mode has been set,
		// so that Meet can properly recalculate how many inbound videos should
		// be visible. Pressing Ctrl+Minus 5 times results in the zoom going from
		// 100% -> 90% -> 80% -> 75% -> 67% -> 50%.
		if err := inputsimulations.RepeatKeyPress(ctx, kw, "Ctrl+-", 3*time.Second, 5); err != nil {
			s.Fatal("Failed to repeatedly press Ctrl+Minus to zoom out: ", err)
		}

		// Verify that we zoomed correctly.
		zoomInfo, err := ui.Info(ctx, zoomNode)
		if err != nil {
			s.Fatal("Failed to find the current browser zoom: ", err)
		}
		if zoomInfo.Name != "Zoom: 50%" {
			s.Fatalf(`Unexpected zoom value: got %s; want "Zoom: 50%%"`, zoomInfo.Name)
		}
		s.Log("Zoomed browser window to 50%")
	}

	// Make sure the Meet call window hasn't crashed before starting the recorder.
	assertTabActive(ctx)

	if err := recorder.Run(ctx, func(ctx context.Context) (retErr error) {
		// Open up the collab window inside the recorder to collect
		// PageLoad.PaintTiming.NavigationToFirstContentfulPaint.
		var collaborationRE *regexp.Regexp
		var collaborationConn *chrome.Conn
		if meet.docs {
			recorder.Annotate(ctx, "Open_Google_Doc")
			docsURL := defaultDocsURL
			if docsURLOverride, ok := s.Var("ui.MeetCUJ.doc"); ok {
				docsURL = docsURLOverride
			}

			// Create another browser window and open a Google Docs file.
			collaborationConn, err = recorder.NewConn(ctx, br, "Docs", docsURL, browser.WithNewWindow())
			if err != nil {
				return errors.Wrap(err, "failed to open the Google Docs website")
			}
			defer collaborationConn.Close()

			// Wait for tab quiescene to give any network requests time to
			// complete before forcing Google Docs offline. Only log the error,
			// because sometimes reaching quiescence can take a really long
			// time, even when the doc is interactable.
			if err := webutil.WaitForQuiescence(ctx, collaborationConn, 15*time.Second); err != nil {
				s.Log("Failed to wait for Google Docs to quiesce: ", err)
			}

			collaborationRE = regexp.MustCompile(`\bDocs\b`)

			// Enable docs blocker extension to force Docs in offline mode after docs
			// is loaded.
			docsBlockerConn, err := cuj.GetDocsBlockerConn(ctx, br)
			if err != nil {
				return errors.Wrap(err, "failed to get docs blocker conn")
			}
			defer docsBlockerConn.Close()

			if err := docsBlockerConn.Eval(ctx, "ForceDocsOffline(true)", nil); err != nil {
				return errors.Wrap(err, "failed to call docs blocker to block Docs")
			}
			docsBlockerCleanupCtx := ctx
			ctx, cancel = ctxutil.Shorten(ctx, 15*time.Second)
			defer cancel()
			defer func(ctx context.Context) {
				if err := docsBlockerConn.Eval(ctx, "ForceDocsOffline(false)", nil); err != nil {
					s.Log("Failed to call docs blocker to restore: ", err)
				}
			}(docsBlockerCleanupCtx)

			if err := googledocs.ShowTheDocMenus(tconn, kw)(ctx); err != nil {
				return errors.Wrap(err, "failed to show the doc menus")
			}
		} else if meet.jamboard {
			// Create another browser window and open a new Jamboard file.
			recorder.Annotate(ctx, "Open_Jamboard_window")
			collaborationConn, err = recorder.NewConn(ctx, br, "Jamboard", jamboardURL, browser.WithNewWindow())
			if err != nil {
				return errors.Wrap(err, "failed to open the Jamboard website")
			}
			defer collaborationConn.Close()
			s.Log("Creating a Jamboard window")
			if err := ui.LeftClick(nodewith.Name("New Jam").Role(role.Button))(ctx); err != nil {
				return errors.Wrap(err, "failed to click the new jam button")
			}
			collaborationRE = regexp.MustCompile(`\bJamboard\b`)
		}

		var collaborationWindow *ash.Window
		if meet.docs || meet.jamboard {
			collaborationWindow, err = ash.FindOnlyWindow(ctx, tconn, func(w *ash.Window) bool { return collaborationRE.MatchString(w.Title) })
			if err != nil {
				return errors.Wrap(err, "failed to find the collaboration window")
			}
		}

		if meet.split {
			// Start an annotation section for split screening each window.
			endSplitScreenSection := recorder.AnnotateSection(ctx, "Split_screen_windows")
			if collaborationRE == nil {
				return errors.New("need a collaboration window for split view")
			}
			if err := ash.SetWindowStateAndWait(ctx, tconn, collaborationWindow.ID, ash.WindowStatePrimarySnapped); err != nil {
				return errors.Wrap(err, "failed to snap the collaboration window to the left")
			}
			if err := ash.SetWindowStateAndWait(ctx, tconn, meetWindow.ID, ash.WindowStateSecondarySnapped); err != nil {
				return errors.Wrap(err, "failed to snap the Meet window to the right")
			}
			endSplitScreenSection(ctx)
		} else {
			if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to activate the Meet window")
			}
		}

		// Hide notifications so that they won't overlap with other UI components.
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all notifications")
		}
		shareMessage := "Share this info with people you want in the meeting"
		if err := ui.WaitUntilExists(nodewith.Name(shareMessage).Ancestor(webview))(ctx); err == nil {
			// "Share this code" popup appears, dismissing by close button.
			if err := uiauto.Combine(
				"click the close button and wait for the popup to disappear",
				pc.Click(nodewith.Name("Close").Role(role.Button).Ancestor(webview)),
				ui.WaitUntilGone(nodewith.Name(shareMessage).Ancestor(webview)),
			)(ctx); err != nil {
				return err
			}
		}

		if err := meetHelper.SetMicrophone(ctx, true); err != nil {
			return errors.Wrap(err, "failed to turn on microphone")
		}

		if err := meetHelper.SetCamera(ctx, meet.cam); err != nil {
			return errors.Wrapf(err, "failed to set camera off-status to %t", !meet.cam)
		}

		// Hide notifications so that they won't overlap with other UI components.
		if err := ash.CloseNotifications(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to close all notifications")
		}

		if err := meetHelper.ChangeLayoutOption(ctx, meet.layout); err != nil {
			return errors.Wrapf(err, "failed to set %s layout", meet.layout)
		}

		if err := meetHelper.SetSendResolution720p(ctx); err != nil {
			return errors.Wrap(err, "failed to request sending 720p")
		}

		if err := meetHelper.SetReceiveResolution720p(ctx); err != nil {
			return errors.Wrap(err, "failed to request receiving 720p")
		}
		// Direct the spotlight bot to pin the test user so
		// that the test user will have to provide HD video.
		login, err := loginstatus.GetLoginStatus(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get login status: ", err)
		}
		if !login.IsLoggedIn {
			s.Fatal("Expect to see a user is logged in in login status")
		}
		if err := bc.ExecuteScript(ctx, fmt.Sprintf("@b%d pin_participant_by_name %q", spotlightBotList[0], *login.DisplayName), meetingCode); err != nil {
			s.Fatal("Failed to direct the spotlight bot to pin the test user: ", err)
		}

		if meet.present {
			if !meet.docs && !meet.jamboard {
				return errors.New("need a Google Docs or Jamboard tab to present")
			}

			// Start an annotation section for opening the screen share window
			// and screen sharing the collaboration window.
			endPresentSection := recorder.AnnotateSection(ctx, "Screenshare")

			presentTabTitle := "Untitled document"
			if meet.jamboard {
				presentTabTitle = "Untitled Jam"
			}
			if err := startPresenting(ctx, collaborationConn, ui, meetHelper, kw, presentTabTitle); err != nil {
				return errors.Wrap(err, "failed to start screen sharing")
			}
			expectedParticipantCount++

			endPresentSection(ctx)
		}

		errc := make(chan error)
		s.Log("Keeping the meet session for ", meetTimeout)
		async.Run(ctx, func(ctx context.Context) {
			// Using goroutine to measure GPU counters asynchronously because:
			// - we will add some other test scenarios (controlling windows / meet sessions).
			// - graphics.MeasureGPUCounters may quit immediately when the hardware or
			//   kernel does not support the reporting mechanism.
			errc <- graphics.MeasureGPUCounters(ctx, meetTimeout, pv)
		}, "measure GPU counters")

		var addBotsErr error
		stopAddBotsC := make(chan struct{})
		defer func(ctx context.Context) {
			close(stopAddBotsC)
			if addBotsErr != nil {
				retErr = errors.Wrapf(retErr, "failed to run bot phases during the Meet call: %v", addBotsErr)
			}
		}(ctx)

		var stopSnapshot func(ctx context.Context) error
		numPhases := len(meet.bots)
		// If there are multiple phases, collect a shortened list of UMA
		// metrics for each phase.
		ashMetrics, browserMetrics := cujrecorder.GetShortenedPerformanceMetrics()
		async.Run(ctx, func(ctx context.Context) {
			if numPhases == 1 {
				return
			}

			// The call will be split into len(meet.bots) phases.
			// Each phase i will have meet.bots[i] number of bots in the call.
			// At this point in the test, the first set of bots have already
			// been added.
			currentPhase := 0
			startBotAddTime := time.Now()

			// The test is broken up into equal length phases with different
			// bot counts.
			phaseDuration := meetTimeout / time.Duration(numPhases)

			addingMoreBots := true
			for addingMoreBots {
				// Prefix the metric with the number of people in the call.
				// This is the number of bots in the call + the user themselves.
				stopSnapshot, err = recorder.StartSnapshot(ctx, fmt.Sprintf("%dp", botsInCall+1), ashMetrics, browserMetrics)
				if err != nil {
					addBotsErr = errors.Wrapf(err, "failed to start snapshot for phase %d", currentPhase)
					break
				}

				// End the goroutine early if we added the bots for the
				// last phase.
				if currentPhase == numPhases-1 {
					addingMoreBots = false
					break
				}

				select {
				// Subtract the time it took to add the bots from the phase
				// duration. For the first bot phase, the bots are added before
				// recorder.Run, so we subtract a negligible amount of time
				// from the phase duration.
				case <-time.After(phaseDuration - time.Since(startBotAddTime)):
					// Complete the snapshot of the last phase.
					if err := stopSnapshot(ctx); err != nil {
						addBotsErr = errors.Wrapf(err, "failed to stop snapshot for phase %d", currentPhase)
					}
					stopSnapshot = nil

					// Take a screenshot of the previous phase.
					recorder.CustomScreenshot(ctx)

					startBotAddTime = time.Now()

					currentPhase++
					numBotsToAdd = meet.bots[currentPhase] - botsInCall

					if err := addBots(ctx, numBotsToAdd); err != nil {
						addBotsErr = errors.Wrapf(err, "failed to add %d bots", numBotsToAdd)
						addingMoreBots = false
						break
					}
					botsInCall += numBotsToAdd
					recorder.Annotate(ctx, fmt.Sprintf("Added_%d_bots", numBotsToAdd))

					// Ensure to properly keep track of how many participants
					// are in the call, which would include the current user
					// and the optional screensharing connection.
					expectedParticipantCount += numBotsToAdd

				case <-stopAddBotsC:
					testing.ContextLog(ctx, "add_bots: Background signaled to stop")
					addBotsErr = errors.Errorf("failed to complete phase %d, background signaled to stop", currentPhase)
					addingMoreBots = false
					break
				}
			}
		}, "increasing bot count during test")
		defer func(ctx context.Context) {
			if stopSnapshot == nil {
				return
			}
			if err := stopSnapshot(ctx); err != nil {
				s.Log("Failed to stop final snapshot: ", err)
			}
		}(ctx)

		// Record trace for 30 seconds.
		// See go/trace-in-cuj-tests about rules for tracing.
		var tracingErr error
		traceDuration := 30 * time.Second
		stopTracingC := make(chan struct{})
		defer func(ctx context.Context) {
			close(stopTracingC)
			if tracingErr != nil {
				retErr = errors.Wrapf(retErr, "failed to complete tracing: %v", tracingErr)
			}
		}(ctx)

		startTracingRoutine := func(ctx context.Context) {
			async.Run(ctx, func(ctx context.Context) {
				if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
					tracingErr = errors.Wrap(err, "failed to start tracing")
					return
				}

				for {
					select {
					case <-time.After(traceDuration):
						if err := recorder.StopTracing(ctx); err != nil {
							tracingErr = errors.Wrap(err, "failed to stop tracing")
						}
						return
					case <-stopTracingC:
						testing.ContextLog(ctx, "tracing: Background signaled to stop")
						tracingErr = errors.New("failed to complete tracing, background signaled to stop")
						return
					}
				}
			}, /*prefix=*/ "Tracing")
		}

		meetEndTime := time.Now().Add(meetTimeout)
		if meet.docs {
			// Start an annotation section to interact with Google Docs.
			endDocsInteractions := recorder.AnnotateSection(ctx, "Docs_interactions")

			// Since adding bots also takes snapshots, to avoid collision, only start
			// snapshot for Google Docs if the number of bots won't vary during
			// meetTimeout.
			if numPhases == 1 {
				stopSnapshot, err = recorder.StartSnapshot(ctx, "Docs", ashMetrics, browserMetrics)
				if err != nil {
					return errors.Wrap(err, "failed to start snapshot for Google Docs")
				}
			}

			if err := collaborationWindow.ActivateWindow(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to activate the collaboration window")
			}

			// The UI elements might not immediately appear after activating the window.
			// Wait for the web area of the Google Docs website to appear
			// to ensure the security alert can be correctly dismissed.
			docsRootWebArea := nodewith.NameContaining("Google Docs").Role(role.RootWebArea)
			if err := ui.WaitUntilExists(docsRootWebArea)(ctx); err != nil {
				return errors.Wrap(err, "failed to wait for docs root web area to appear")
			}
			if err := cuj.DismissCriticalSecurityAlert(ctx, tconn, collaborationConn); err != nil {
				return errors.Wrap(err, "failed to dismiss critical security alert")
			}

			docsCanvas := nodewith.Role(role.Canvas).Ancestor(docsRootWebArea)
			if err := action.Combine("select and zoom document",
				pc.Click(docsCanvas),
				kw.AccelAction("Ctrl+Alt+["),
				kw.AccelAction("Ctrl+A"),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to select Google Docs")
			}

			typingDuration := meetTimeout
			if meet.typingDuration != 0 {
				typingDuration = meet.typingDuration
			}
			end := time.Now().Add(typingDuration)

			// By default, type a bolded header and paragraph, then sleep
			// for 5 seconds.
			cycleDescription := "type and sleep"
			cycleActions := []action.Action{
				// Ctrl+Alt+1 is the shortcut for activating Heading 1.
				kw.AccelAction("Ctrl+Alt+1"),

				inputsimulations.TypeSequenceWPMAction(ctx, kw, 120, strings.Split("my bolded header", "")),

				// Press enter to go to the next line and undo the bold lettering.
				kw.AccelAction("Enter"),

				// Type a paragraph in normal text.
				inputsimulations.TypeSequenceWPMAction(ctx, kw, 120, notes),
				kw.AccelAction("Enter"),

				// Add a small delay before typing again.
				action.Sleep(5 * time.Second),
			}

			// If tabSwitchDocs is true, Alt+Tab twice to switch to another window
			// and come back to the current window.
			if meet.tabSwitchDocs {
				cycleDescription = "sleep, type, and task switch"
				taskSwitch := kw.AccelAction("Alt+Tab")
				cycleActions = append(cycleActions,
					taskSwitch,
					action.Sleep(10*time.Second),
					taskSwitch,
				)
			}

			startTracingRoutine(ctx)

			// Start an annotation section for typing on the Google Doc.
			endTypingSection := recorder.AnnotateSection(ctx, "Type_on_docs")
			for time.Until(end) > 0 {
				if err := action.Combine(cycleDescription, cycleActions...)(ctx); err != nil {
					return err
				}
			}
			endTypingSection(ctx)

			// Toggle the Google Docs File menu button for press and
			// release metrics.
			if err := meetcuj.ToggleFileMenuButton(ctx, ui, pc, inTabletMode); err != nil {
				return errors.Wrap(err, "failed to toggle file menu button")
			}

			// Get the Google Docs window again to properly retrieve
			// the split-screen bounds.
			docsWindow, err := ash.GetWindow(ctx, tconn, collaborationWindow.ID)
			if err != nil {
				return errors.Wrap(err, "failed to get the Google Docs window")
			}

			// Highlight text on Google Docs to get mouse drag metrics.
			docsBounds := docsWindow.TargetBounds
			if !inTabletMode {
				if err := mouse.Move(tconn, docsBounds.CenterPoint(), 500*time.Millisecond)(ctx); err != nil {
					return errors.Wrap(err, "failed to move mouse to center of Google Docs window")
				}
			}
			if err := pc.Drag(
				docsBounds.CenterPoint(),
				pc.DragTo(docsBounds.TopLeft(), 500*time.Millisecond),
				pc.DragTo(docsBounds.BottomRight(), time.Second),
				pc.DragTo(docsBounds.CenterPoint(), 500*time.Millisecond),
			)(ctx); err != nil {
				return err
			}

			if inTabletMode {
				x := docsBounds.CenterX()
				topSwipePoint := coords.NewPoint(x, docsBounds.Height/4)
				bottomSwipePoint := coords.NewPoint(x, docsBounds.Height*3/4)

				// Scroll with 1-finger swipe.
				if err := pc.Drag(bottomSwipePoint,
					pc.DragTo(topSwipePoint, time.Second),
					pc.DragTo(bottomSwipePoint, time.Second),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to scroll with 1-finger swipe")
				}
			} else {
				// Scroll with mouse wheel.
				for _, scrollDown := range []bool{true, false} {
					if err := inputsimulations.RepeatMouseScroll(ctx, mw, scrollDown, 50*time.Millisecond, 30); err != nil {
						return errors.Wrap(err, "failed to repeat mouse scroll")
					}
				}
			}

			// Enable docsBlocker extension again to restore Google Docs.
			docsBlockerConn, err := cuj.GetDocsBlockerConn(ctx, br)
			if err != nil {
				return errors.Wrap(err, "failed to get docs blocker conn")
			}
			defer docsBlockerConn.Close()

			if err := docsBlockerConn.Eval(ctx, "ForceDocsOffline(false)", nil); err != nil {
				s.Log("Failed to call docs blocker to restore: ", err)
			}

			if err := kw.Accel(ctx, "Alt+Tab"); err != nil {
				return errors.Wrap(err, "failed to hit alt-tab and focus back to Meet tab")
			}
			endDocsInteractions(ctx)

			if numPhases == 1 {
				if err := stopSnapshot(ctx); err != nil {
					return errors.Wrap(err, "failed to stop snapshot for Google Docs")
				}
			}
		} else if meet.jamboard {
			// Simulate mouse input on jamboard.
			if err := ui.LeftClick(nodewith.Name("Pen").Role(role.ToggleButton))(ctx); err != nil {
				s.Fatal("Failed to click the pen toggle button: ", err)
			}
			contentArea, err := ui.Location(ctx, nodewith.ClassName("jam-content-area").Role(role.GenericContainer))
			if err != nil {
				s.Fatal("Failed to find the location of jamboard content area: ", err)
			}
			centerX, centerY, offsetX, offsetY := contentArea.CenterPoint().X, contentArea.CenterPoint().Y, 10, 10
			end := time.Now().Add(meetTimeout)

			startTracingRoutine(ctx)

			// Start an annotation section for interacting with the Jamboard.
			endJamboardInteractions := recorder.AnnotateSection(ctx, "Jamboard_interactions")
			for end.Sub(time.Now()).Seconds() > 42 {
				for i := 1; i <= 10; i++ {
					if err := uiauto.Combine(
						"simulate mouse movement",
						mouse.Move(tconn, coords.NewPoint(centerX-i*offsetX, centerY-i*offsetY), 0),
						mouse.Press(tconn, mouse.LeftButton),
						mouse.Move(tconn, coords.NewPoint(centerX-i*offsetX, centerY+i*offsetY), time.Second),
						mouse.Move(tconn, coords.NewPoint(centerX+i*offsetX, centerY+i*offsetY), time.Second),
						mouse.Move(tconn, coords.NewPoint(centerX+i*offsetX, centerY-i*offsetY), time.Second),
						mouse.Move(tconn, coords.NewPoint(centerX-i*offsetX, centerY-i*offsetY), time.Second),
						mouse.Release(tconn, mouse.LeftButton),
					)(ctx); err != nil {
						s.Fatal("Failed to simulate mouse movement on jamboard: ", err)
					}
				}
			}
			endJamboardInteractions(ctx)
		} else {
			startTracingRoutine(ctx)
		}

		// "Stop presenting" if the test wants to interact with
		// Google Slides or Google Sheets later.
		if meet.slides || meet.sheets {
			if err := stopPresenting(ctx, ui); err != nil {
				return errors.Wrap(err, "failed to stop presenting")
			}
			// When a participant share the screen, one more participant
			// is added to the meeting. Therefore, when screen sharing stops,
			// the number of participants should decrease by one.
			expectedParticipantCount--
		}

		// If we have a collaboration window open, navigate away from the page
		// to collect LCP metrics.
		if collaborationConn != nil {
			// If leaving the edit document page, the "Leave site?" window may pop up
			// which causing navigation to fail.
			// Set shortCtx to quickly check if it fails to navigate.
			shortCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if err := collaborationConn.Navigate(shortCtx, chrome.VersionURL); err != nil {
				testing.ContextLogf(ctx, "Failed to navigate to %s: %v", chrome.VersionURL, err)
				// If the "Leave site?" prompt pops up, click the leave button.
				if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(prompts.LeaveSitePrompt.PromptFinder)(ctx); err == nil {
					if err := prompts.ClearPotentialPrompts(tconn, time.Second, prompts.LeaveSitePrompt)(ctx); err != nil {
						return errors.Wrap(err, "failed to clear leave site prompt")
					}
					if err := webutil.WaitForQuiescence(ctx, collaborationConn, 15*time.Second); err != nil {
						testing.ContextLogf(ctx, "Failed to wait for %s to achieve quiescence: %v", chrome.VersionURL, err)
					}
					targets, err := br.FindTargets(ctx, chrome.MatchTargetURL(chrome.VersionURL))
					if err != nil || len(targets) == 0 {
						return errors.Wrapf(err, "failed to find URL %s", chrome.VersionURL)
					}
				} else {
					return errors.Wrapf(err, "failed to navigate to %s", chrome.VersionURL)
				}
			}
		}

		if meet.slides {
			// Start an annotation section to interact with Google Slides.
			endSlidesInteractions := recorder.AnnotateSection(ctx, "Slides_interactions")

			// Since adding bots also takes snapshots, to avoid collision, only start
			// snapshot for Google Slides if the number of bots won't vary during
			// meetTimeout.
			if numPhases == 1 {
				stopSnapshot, err = recorder.StartSnapshot(ctx, "Slides", ashMetrics, browserMetrics)
				if err != nil {
					return errors.Wrap(err, "failed to start snapshot for Google Slides")
				}
			}

			slidesURL, err := cuj.GetTestSlidesURL(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get Google Slides URL")
			}
			if err := navigate(ctx, collaborationConn, br, slidesURL); err != nil {
				return errors.Wrap(err, "failed to navigate to Google Slides website")
			}
			// We stopped presenting before navigating to the Google Slides page.
			// Start screen sharing again for the page.
			if err := startPresenting(ctx, collaborationConn, ui, meetHelper, kw, "Google Slides"); err != nil {
				return errors.Wrap(err, "failed to start screen sharing")
			}

			// Ensure the slides deck gets scrolled.
			if err := scrollDownPage(ctx, collaborationConn, kw, "document.getElementsByClassName('punch-filmstrip-scroll')[0]"); err != nil {
				return err
			}
			// Ensure MouseClick, LCP2 and ADF metrics are generated.
			if err := generateMetrics(ctx, collaborationConn, tconn, ui, pc, inTabletMode); err != nil {
				return err
			}

			// "Stop presenting" if the test wants to interact with
			// Google Sheets later.
			if err := stopPresenting(ctx, ui); err != nil {
				return errors.Wrap(err, "failed to stop presenting")
			}
			endSlidesInteractions(ctx)

			if numPhases == 1 {
				if err := stopSnapshot(ctx); err != nil {
					return errors.Wrap(err, "failed to stop snapshot for Google Slides")
				}
			}
		}

		if meet.sheets {
			// Start an annotation section to interact with Google Sheets.
			endSheetsInteractions := recorder.AnnotateSection(ctx, "Sheets_interactions")

			// Since adding bots also takes snapshots, to avoid collision, only start
			// snapshot for Google Sheets if the number of bots won't vary during
			// meetTimeout.
			if numPhases == 1 {
				stopSnapshot, err = recorder.StartSnapshot(ctx, "Sheets", ashMetrics, browserMetrics)
				if err != nil {
					return errors.Wrap(err, "failed to start snapshot for Google Sheets")
				}
			}

			sheetsURL, err := cuj.GetTestSheetsViewerURL(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get Google Sheets URL")
			}
			if err := navigate(ctx, collaborationConn, br, sheetsURL); err != nil {
				return errors.Wrap(err, "failed to navigate to Google Sheets website")
			}
			// We stopped presenting before navigating to the Google Sheets page.
			// Start screen sharing again for the page.
			if err := startPresenting(ctx, collaborationConn, ui, meetHelper, kw, "Google Sheets"); err != nil {
				return errors.Wrap(err, "failed to start screen sharing")
			}
			expectedParticipantCount++

			// Ensure the sheets deck gets scrolled.
			if err := scrollDownPage(ctx, collaborationConn, kw, "document.getElementsByClassName('native-scrollbar-y')[0]"); err != nil {
				return err
			}
			// Ensure MouseClick, LCP2 and ADF metrics are generated.
			if err := generateMetrics(ctx, collaborationConn, tconn, ui, pc, inTabletMode); err != nil {
				return err
			}
			endSheetsInteractions(ctx)

			if numPhases == 1 {
				if err := stopSnapshot(ctx); err != nil {
					return errors.Wrap(err, "failed to stop snapshot for Google Sheets")
				}
			}
		}

		moveMouseTimeout := meetEndTime.Sub(time.Now())
		// Ensures that meet session is long enough. graphics.MeasureGPUCounters
		// exits early without errors on ARM where there is no i915 counters.
		if err := inputsimulations.MoveMouseFor(ctx, tconn, moveMouseTimeout); err != nil {
			return errors.Wrap(err, "failed to simulate mouse movement")
		}
		if err := <-errc; err != nil {
			return errors.Wrap(err, "failed to collect GPU counters")
		}

		if err := checkParticipantCount(ctx, expectedParticipantCount); err != nil {
			return errors.Wrap(err, "the number of bots is unexpected, the bond server may have lost bots")
		}

		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	// Before recording the metrics, check if there is any tab crashed.
	assertTabActive(ctx)

	// Sometimes the nodes on the background window cannot be found.
	// Activate the window to download the dump from the WebRTC-internals window.
	if err := webRTCInternalsWindow.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to activate the WebRTC-internals window: ", err)
	}
	// Some DUTs need more time to wait for quiescence. Add log for debugging
	// loading duration.
	startTime := time.Now()
	if err := webutil.WaitForQuiescence(ctx, webrtcInternals, 2*time.Minute); err != nil {
		s.Fatal("Failed to wait for quiescence: ", err)
	}
	testing.ContextLog(ctx, "Loading page took: ", time.Since(startTime))

	// Report info from chrome://webrtc-internals.
	if path, err := cuj.DumpWebRTCInternals(ctx, tconn, ui, cr.NormalizedUser()); err != nil {
		s.Error("Failed to download dump from chrome://webrtc-internals: ", err)
		// Take a screenshot with the chrome://webrtc-internals tab in
		// the foreground, to facilitate investigation of b/255343902.
		// TODO(b/255343902): Remove this when the bug is fixed.
		recorder.CustomScreenshot(ctx)
	} else {
		dump, readErr := os.ReadFile(path)
		if readErr != nil {
			s.Error("Failed to read WebRTC internals dump from Downloads folder: ", readErr)
		}
		if err := os.Remove(path); err != nil {
			s.Error("Failed to remove WebRTC internals dump from Downloads folder: ", err)
		}
		if readErr == nil {
			if err := os.WriteFile(filepath.Join(s.OutDir(), "webrtc-internals.json"), dump, 0644); err != nil {
				s.Error("Failed to write WebRTC internals dump to test results folder: ", err)
			}
			webRTCInternalsPV, err := reportWebRTCInternals(ctx, dump, meetingCode, meet.bots[len(meet.bots)-1], meet.present)
			if err != nil {
				s.Error("Failed to report info from WebRTC internals dump to performance metrics: ", err)
			} else {
				pv.Merge(webRTCInternalsPV)
			}
		}
	}

	// Activate the Meet window to clean up browser zoom and effect settings.
	if err := meetWindow.ActivateWindow(ctx, tconn); err != nil {
		s.Fatal("Failed to activate the Meet window: ", err)
	}

	// Reset the browser zoom, because the browser retains the zoom
	// across test variants.
	if err := kw.Accel(ctx, "Ctrl+0"); err != nil {
		s.Log("Failed to reset browser zoom to 100%")
	}

	if meet.effects {
		if err := setEffect(ctx, turnOffBlur); err != nil {
			s.Log("Failed to turn off blur: ", err)
		}
	}

	// Report WebRTC metrics for video streams. Start by closing the Meet window and
	// waiting for the video streams to be gone (according to chrome://webrtc-internals),
	// because the metrics are recorded when the video streams are ended.
	closedMeet = true
	if err := meetWindow.CloseWindow(closeCtx, tconn); err != nil {
		s.Error("Failed to close the meeting: ", err)
	}
	if err := ui.WaitUntilGone(nodewith.NameContaining("VideoStream").First())(ctx); err != nil {
		s.Error("Failed to wait for video stream info to disappear: ", err)
	}
	if hists, err := webRTCMetricsRecorder.Histogram(ctx, bTconn); err != nil {
		s.Error("Failed to gather WebRTC metrics for video streams: ", err)
	} else {
		for _, hist := range hists {
			count := hist.TotalCount()
			if count == 0 {
				continue
			}

			info := cujrecorder.WebRTCMetricInfo[hist.Name]
			pv.Set(perf.Metric{
				Name:      hist.Name,
				Unit:      info.Unit,
				Direction: info.Direction,
			}, float64(hist.Sum))

			var bucketMinima []float64
			var bucketMaxima []float64
			for _, bucket := range hist.Buckets {
				// Only report the bucket max + mins if there's more than 1
				// element in the bucket.
				if bucket.Count <= 1 {
					continue
				}

				for i := int64(0); i < bucket.Count; i++ {
					bucketMinima = append(bucketMinima, float64(bucket.Min))
					bucketMaxima = append(bucketMaxima, float64(bucket.Max))
				}

				pv.Set(perf.Metric{
					Name:      hist.Name,
					Variant:   "bucket_minima",
					Unit:      info.Unit,
					Direction: info.Direction,
					Multiple:  true,
				}, bucketMinima...)
				pv.Set(perf.Metric{
					Name:      hist.Name,
					Variant:   "bucket_maxima",
					Unit:      info.Unit,
					Direction: info.Direction,
					Multiple:  true,
				}, bucketMaxima...)
			}
		}
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to save the perf data: ", err)
	}
}

// navigate navigates to the url and waits for the page to quiesce,
// and then focus on it.
func navigate(ctx context.Context, conn *chrome.Conn, br *browser.Browser, url string) error {
	if err := conn.Navigate(ctx, url); err != nil {
		return errors.Wrapf(err, "failed to navigate to %s", url)
	}

	// Some DUTs need more time to wait for quiescence. Add log for debugging
	// loading duration. If waiting for the page to quiesce fails, just print
	// the log.
	startTime := time.Now()
	if err := webutil.WaitForQuiescence(ctx, conn, 2*time.Minute); err != nil {
		testing.ContextLog(ctx, "Ignoring waiting for page to quiesce: ", err)
	} else {
		testing.ContextLog(ctx, "Loading page took: ", time.Since(startTime))
	}

	targets, err := br.FindTargets(ctx, chrome.MatchTargetURLPrefix(url))
	if err != nil || len(targets) == 0 {
		return errors.Wrapf(err, "failed to find URL %s", url)
	}
	return conn.ActivateTarget(ctx)
}

// scrollDownPage scrolls down the page by pressing Down key, and checks
// if the specified HTML element is scrolled.
func scrollDownPage(ctx context.Context, conn *chrome.Conn, kw *input.KeyboardEventWriter, element string) error {
	testing.ContextLog(ctx, "Going through the file")
	if err := inputsimulations.RepeatKeyPress(ctx, kw, "Down", 50*time.Millisecond, 60); err != nil {
		return errors.Wrap(err, `failed to repeatedly and rapidly press "Down" in between task switches`)
	}
	// Ensure the element gets scrolled.
	return meetcuj.EnsureElementGetsScrolled(ctx, conn, element)
}

// startPresenting starts to present |presentTabTitle| tab in Google Meet.
// It will only start present if there's nothing being shared now.
func startPresenting(ctx context.Context, conn *chrome.Conn, ui *uiauto.Context, meetHelper *googlemeet.HRTelemetryHelper, kw *input.KeyboardEventWriter, presentTabTitle string) error {
	// Only start sharing if it's not presenting anything now.
	stopSharing := nodewith.Name("Stop sharing").Role(role.Button).First()
	if err := ui.Exists(stopSharing)(ctx); err == nil {
		return nil
	}

	if err := meetHelper.OpenPresentDialog(ctx); err != nil {
		return errors.Wrap(err, "failed to start to present a tab")
	}

	// Select the tab to present. Avoid directly tapping on the screen
	// due to miscalculated node bounds for Lacros tablet devices.
	waitForPresentTabFocus := ui.WithTimeout(5 * time.Second).WaitUntilExists(nodewith.NameContaining(presentTabTitle).HasClass("AXVirtualView").Focused())
	stopPresenting := nodewith.Name("Stop presenting").Role(role.Button)
	if err := uiauto.NamedCombine(fmt.Sprintf("select tab %q to screenshare", presentTabTitle),
		ui.EnsureFocused(nodewith.Name("Chrome Tab").Role(role.ListGrid)),
		// If the presenting tab is not focused, press the down
		// arrow until it is.
		uiauto.IfFailThen(
			waitForPresentTabFocus,
			ui.RetryUntil(
				kw.AccelAction("Down"),
				waitForPresentTabFocus,
			),
		),
		kw.AccelAction("Enter"),
		// Some low-end DUTs may take a long time to actually get to
		// the presenting page. Wait for the "Stop presenting" to appear
		// to ensure the page is being shared.
		ui.WithTimeout(time.Minute).WaitUntilExists(stopPresenting),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select the tab to share")
	}

	startTime := time.Now()
	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		testing.ContextLog(ctx, "Ignoring waiting for page to quiesce: ", err)
	} else {
		testing.ContextLog(ctx, "Loading page took: ", time.Since(startTime))
	}

	return nil
}

// stopPresenting stops presenting in Google Meet.
func stopPresenting(ctx context.Context, ui *uiauto.Context) error {
	stopPresenting := nodewith.Name("Stop presenting").Role(role.Button)
	return uiauto.NamedCombine("stop presenting",
		ui.LeftClick(stopPresenting),
		ui.WithTimeout(30*time.Second).WaitUntilGone(stopPresenting),
	)(ctx)
}

// generateMetrics generates metrics by interacting with the page and the Ash UI.
func generateMetrics(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, ui *uiauto.Context, pc pointer.Context, inTabletMode bool) error {
	// Collect mouse events by toggling "File" button.
	if err := meetcuj.ToggleFileMenuButton(ctx, ui, pc, inTabletMode); err != nil {
		return errors.Wrap(err, "failed to toggle the File menu button")
	}
	// Navigate away to record PageLoad.PaintTiming.NavigationToLargestContentfulPaint2.
	if err := conn.Navigate(ctx, chrome.VersionURL); err != nil {
		return errors.Wrapf(err, "failed to navigate to %s", chrome.VersionURL)
	}
	// Perform Ash workflows to get ADF metrics.
	if err := inputsimulations.DoAshWorkflows(ctx, tconn, pc); err != nil {
		return errors.Wrap(err, "failed to do Ash workflows")
	}
	return nil
}

// reportWebRTCInternals reports info from a WebRTC internals dump to performance metrics.
func reportWebRTCInternals(ctx context.Context, dump []byte, meetingCode string, numBots int, present bool) (*perf.Values, error) {
	var webRTC webrtcinternals.Dump
	if err := json.Unmarshal(dump, &webRTC); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal WebRTC internals dump")
	}

	expectedConns := 1
	expectedScreenshareConns := 0
	if present {
		expectedConns = 2
		expectedScreenshareConns = 1
	}
	var inCountError, outCountErr error
	numPeerConns := 0
	numScreenshareConns := 0
	pv := perf.NewValues()
	for connID, peerConn := range webRTC.PeerConnections {
		// Only record peer connections that are related to our
		// currently open Meet window. This is to make our tests more
		// robust, by ignoring any peer connections that are hanging
		// around from previous tests.
		if !strings.Contains(peerConn.URL, meetingCode) {
			continue
		}
		numPeerConns++

		byType := peerConn.Stats.BuildIndex()
		inTotalCount, inScreenshareCount, err := cuj.ReportVideoStreams(pv, byType["inbound-rtp"], "framesReceived", ".Inbound", "bot%02d")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to report inbound-rtp video streams in peer connection %v", connID)
		}
		outTotalCount, outScreenshareCount, err := cuj.ReportVideoStreams(pv, byType["outbound-rtp"], "framesSent", ".Outbound", "stream%d")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to report outbound-rtp video streams in peer connection %v", connID)
		}

		if inScreenshareCount != 0 {
			return nil, errors.Errorf("unexpected number of inbound-rtp screenshare video streams in peer connection %v; got %d, want 0", connID, inScreenshareCount)
		}
		if outTotalCount == 0 {
			outCountErr = errors.Errorf("found no outbound-rtp video streams in peer connection %v", connID)
			continue
		} else {
			outCountErr = nil
		}
		expectedInTotalCount := 0
		switch outScreenshareCount {
		case 0: // This is the video chat connection.
			// Sometimes when the connection is unstable, there may be multiple peer connections.
			// Return failure only if none of the connections have correct inbound video data.
			expectedInTotalCount = numBots
			if inTotalCount != expectedInTotalCount {
				inCountError = errors.Errorf("unexpected number of inbound-rtp video streams in peer connection %v; got %d, want %d", connID, inTotalCount, expectedInTotalCount)
			} else {
				inCountError = nil
			}
		case outTotalCount: // This is the screen share connection.
			numScreenshareConns++
			if inTotalCount != expectedInTotalCount {
				return nil, errors.Errorf("unexpected number of inbound-rtp video streams in screenshare peer connection %v; got %d, want %d", connID, inTotalCount, expectedInTotalCount)
			}
		default:
			return nil, errors.Errorf("found %d screenshare(s) among %d outbound-rtp video streams in peer connection %v, expected all or none", outScreenshareCount, outTotalCount, connID)
		}
	}
	if outCountErr != nil {
		return nil, outCountErr
	}
	if inCountError != nil {
		return nil, inCountError
	}
	if numPeerConns < expectedConns {
		return nil, errors.Errorf("unexpected number of peer connections; got %d, want %d", numPeerConns, expectedConns)
	} else if numPeerConns > expectedConns {
		testing.ContextLogf(ctx, "Got more peer connections; got %d, want %d", numPeerConns, expectedConns)
	}

	if numScreenshareConns < expectedScreenshareConns {
		return nil, errors.Errorf("unexpected number of screenshare peer connections; got %d, want %d", numScreenshareConns, expectedScreenshareConns)
	} else if numScreenshareConns > expectedScreenshareConns {
		testing.ContextLogf(ctx, "Got more screenshare peer connections; got %d, want %d", numScreenshareConns, expectedScreenshareConns)
	}

	return pv, nil
}
