// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/android/ui"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CastToClass,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measure the performance of casting to a screen connected to ADT-3. Additional chromecast hardware needs to be prepared before running this test",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "alstonhuang@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome", "arc"},
		Attr:         []string{"group:external-dependency"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"spera.cuj_mode",     // Optional. Expecting "tablet" or "clamshell". Other values will be be taken as "clamshell".
			"spera.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		// TODO(b/252870625): Port these tests to the new quick settings UI
		// (QsRevamp) by porting quicksettings.StartCast() and StopCast().
		Params: []testing.Param{
			{
				Name:    "essential",
				Timeout: 10 * time.Minute,
				Fixture: "enrolledLoggedInToCUJUserQsRevampDisabled",
				Val:     browser.TypeAsh,
			},
			{
				Name:              "essential_lacros",
				Timeout:           10 * time.Minute,
				Fixture:           "enrolledLoggedInToCUJUserLacrosQsRevampDisabled",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:               browser.TypeLacros,
			},
		},
	})
}

const (
	// targetResolution specifies the resolution to used for the YouTube video.
	targetResolution = "1080p"
	// accessCodeLength specifies the length of ADT-3 access code.
	accessCodeLength = 6

	// slideTab specifies the tab name for the new Google Slides.
	slideTab = "Google Slides"
	// title specifies the title to use for the new Google Slides.
	title = "Hello class"
	// subtitle specifies the subtitle to use for the new Google Slides.
	subtitle = "Welcome back"
)

var videoSrc = youtube.VideoSrc{
	URL:     cuj.YoutubeDeveloperKeynoteVideoURL,
	Title:   "Developer Keynote (Google I/O '21) - American Sign Language",
	Quality: targetResolution,
}

// CastToClass measures the system performance by casting to a screen connected to ADT-3.
func CastToClass(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	bt := s.Param().(browser.Type)
	outDir := s.OutDir()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Shorten context a bit to allow for cleanup if Run fails.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "spera.cuj_mode")
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer resetTabletMode(cleanupCtx)

	var uiHandler cuj.UIActionHandler
	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to rotate display to landscape: ", err)
		}
		defer cleanup(cleanupCtx)
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create tablet action handler: ", err)
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
	}
	defer uiHandler.Close(ctx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard input: ", err)
	}
	defer kb.Close(ctx)

	// Give 10 seconds to set initial settings. It is critical to ensure
	// cleanupSetting can be executed with a valid context so it has its
	// own cleanup context from other cleanup functions. This is to avoid
	// other cleanup functions executed earlier to use up the context time.
	cleanupSettingsCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanupSetting, err := cuj.InitializeSetting(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to set initial settings: ", err)
	}
	defer cleanupSetting(cleanupSettingsCtx)

	adbDevice, err := adb.WaitForDevice(ctx, func(device *adb.Device) bool { return !strings.HasPrefix(device.Serial, "ACHE-") }, 10*time.Second)
	if err != nil {
		s.Fatal("Failed to list adb devices: ", err)
	}

	device, err := ui.NewDevice(ctx, adbDevice)
	if err != nil {
		s.Fatal("Failed to setup device: ", err)
	}
	defer device.Close(ctx)

	accessCode, err := getAccessCode(ctx, device)
	if err != nil {
		s.Fatal("Failed to get the access code: ", err)
	}
	if len(accessCode) != accessCodeLength {
		s.Fatalf("Length of access code is incorrect; expected: %d; get: %d", accessCodeLength, len(accessCode))
	}

	testing.ContextLog(ctx, "Start to get browser start time")
	l, browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, tabletMode, bt)
	if err != nil {
		s.Fatal("Failed to get browser start time: ", err)
	}
	br := cr.Browser()
	if l != nil {
		br = l.Browser()
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatalf("Failed to create Test API connection for %v browser: %v", bt, err)
	}
	ac := uiauto.New(tconn)

	browserApp, err := apps.PrimaryBrowser(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find the primary browser: ", err)
	}

	youtubeWeb := youtube.NewYtWeb(br, tconn, kb, false, ac, uiHandler)
	defer youtubeWeb.Close(ctx)

	// Shorten the context to clean up the Google Slides created in the test case.
	cleanUpResourceCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer uiauto.Combine("remove the slide",
		uiHandler.SwitchToAppWindowByName(browserApp.Name, slideTab),
		googledocs.DeleteSlide(tconn),
	)(cleanUpResourceCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, s.HasError, cr, "ui_dump")

	// Shorten the context to cleanup cast setting.
	cleanupCastCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer uiauto.NamedAction("reset cast status", youtubeWeb.ResetCastStatus())(cleanupCastCtx)

	// Shorten the context to cleanup recorder.
	cleanupRecorderCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	options := cujrecorder.NewPerformanceCUJOptions()
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, options)
	if err != nil {
		s.Fatal("Failed to create the recorder: ", err)
	}
	defer recorder.Close(cleanupRecorderCtx)
	if err := cuj.AddPerformanceCUJMetrics(bt, tconn, bTconn, recorder); err != nil {
		s.Fatal("Failed to add metrics to recorder: ", err)
	}
	pv := perf.NewValues()
	if err = recorder.Run(ctx, func(ctx context.Context) error {
		// Start tracing now.
		if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
			if err := recorder.StartTracing(ctx, s.OutDir(), s.DataPath(cujrecorder.SystemTraceConfigFile)); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
			defer recorder.StopTracing(ctx)
		}

		if err := googledocs.NewGoogleSlides(ctx, tconn, br, uiHandler, false); err != nil {
			return err
		}
		castYoutubeVideo := uiauto.NamedCombine("cast youtube video",
			youtubeWeb.OpenAndPlayVideo(videoSrc),
			youtubeWeb.SwitchQuality(targetResolution),
			youtubeWeb.StartCast(accessCode),
		)
		editSlide := uiauto.NamedCombine("switch back to slide and edit",
			uiHandler.SwitchToAppWindowByName(browserApp.Name, slideTab),
			googledocs.EditSlideTitle(tconn, kb, title, subtitle),
		)
		if err := uiauto.NamedCombine("cast to class",
			castYoutubeVideo,
			editSlide,
			youtubeWeb.StopCast(),
		)(ctx); err != nil {
			return err
		}
		if err := cuj.GenerateADF(ctx, tconn, tabletMode); err != nil {
			return errors.Wrap(err, "failed to generate ADF")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to conduct the recorder task: ", err)
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to record the data: ", err)
	}
	if err := recorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}

	pv.Set(perf.Metric{
		Name:      "Browser.StartTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(browserStartTime.Milliseconds()))

	if err := pv.Save(outDir); err != nil {
		s.Fatal("Failed to save perf data: ", err)
	}

	if err := recorder.SaveHistograms(outDir); err != nil {
		s.Fatal("Failed to save histogram raw data: ", err)
	}
}

// getAccessCode get the access code from ADT-3 UI.
//
// It locates the access code with the following node hierarchy:
// <node index="0" text="" resource-id="com.google.android.apps.education.cast2class:id/access_code_container_left" class="android.widget.LinearLayout" ...>
//     <node index="0" text="P" resource-id="" class="android.widget.TextView" ...>
//     <node index="1" text="T" resource-id="" class="android.widget.TextView" ...>
//     <node index="2" text="V" resource-id="" class="android.widget.TextView" ...>
// </node>
// <node index="1" text="" resource-id="com.google.android.apps.education.cast2class:id/access_code_container_right" class="android.widget.LinearLayout" ...>
//     <node index="0" text="Q" resource-id="" class="android.widget.TextView" ...>
//     <node index="1" text="G" resource-id="" class="android.widget.TextView" ...>
//     <node index="2" text="R" resource-id="" class="android.widget.TextView" ...>
// </node>

func getAccessCode(ctx context.Context, device *androidui.Device) (accessCode string, err error) {
	var (
		packageName                = "com.google.android.apps.education.cast2class"
		showAccessCodeText         = "Show access code"
		linearLayoutClass          = "android.widget.LinearLayout"
		textViewClass              = "android.widget.TextView"
		accessCodeLeftContainerID  = packageName + ":id/access_code_container_left"
		accessCodeRightContainerID = packageName + ":id/access_code_container_right"
	)

	showAccessCode := device.Object(androidui.Text(showAccessCodeText), androidui.PackageName(packageName))
	if err := cuj.ClickIfExist(showAccessCode, 15*time.Second)(ctx); err != nil {
		return "", errors.Wrap(err, "failed to click Show access code")
	}

	accessCodeLeftContainer := device.Object(androidui.ID(accessCodeLeftContainerID), androidui.ClassName(linearLayoutClass))
	accessCodeRightContainer := device.Object(androidui.ID(accessCodeRightContainerID), androidui.ClassName(linearLayoutClass))
	containers := []*androidui.Object{accessCodeLeftContainer, accessCodeRightContainer}

	testing.ContextLog(ctx, "Get the access code through ADT-3 UI")
	// The access code has two parts, each containing three letters.
	for _, parent := range containers {
		for j := 0; j < 3; j++ {
			letter := device.Object(androidui.Index(j), androidui.ClassName(textViewClass), androidui.PackageName(packageName))
			if err := parent.GetChild(ctx, letter); err != nil {
				return "", errors.Wrapf(err, "failed to get %+v child", parent)
			}
			l, err := letter.GetText(ctx)
			if err != nil {
				return "", errors.Wrap(err, "failed to get the letter")
			}
			accessCode += l
		}
	}

	return accessCode, nil
}
