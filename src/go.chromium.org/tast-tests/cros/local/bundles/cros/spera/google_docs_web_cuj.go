// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/spera/productivitycuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleDocsWebCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of Google Docs web version CUJ",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "alstonhuang@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"spera.sampleGDocSheetURL", // Required. The URL of sample Google Sheet. It will be copied to create a new one to perform tests on.
			"spera.cuj_mode",           // Optional. Expecting "tablet" or "clamshell".
			"spera.collectTrace",       // Optional. Expecting "enable" or "disable", default is "disable".
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "basic",
				Fixture: "loggedInAndKeepState",
				Timeout: 15 * time.Minute,
				Val: productivitycuj.TestParams{
					Tier:        cuj.Basic,
					BrowserType: browser.TypeAsh,
				},
			},
			{
				Name:              "basic_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           15 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: productivitycuj.TestParams{
					Tier:        cuj.Basic,
					BrowserType: browser.TypeLacros,
				},
			},
			{
				Name:      "premium",
				Fixture:   "loggedInAndKeepState",
				ExtraData: []string{"productivity_cuj_voice_to_text_en.wav"},
				Timeout:   15 * time.Minute,
				Val: productivitycuj.TestParams{
					Tier:        cuj.Premium,
					BrowserType: browser.TypeAsh,
				},
			},
			{
				Name:              "premium_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				ExtraData:         []string{"productivity_cuj_voice_to_text_en.wav"},
				Timeout:           15 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: productivitycuj.TestParams{
					Tier:        cuj.Premium,
					BrowserType: browser.TypeLacros,
				},
			},
		},
	})
}

func GoogleDocsWebCUJ(ctx context.Context, s *testing.State) {
	p := s.Param().(productivitycuj.TestParams)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	sampleSheetURL, ok := s.Var("spera.sampleGDocSheetURL")
	if !ok {
		s.Fatal("Require variable spera.sampleGDocSheetURL is not provided")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "spera.cuj_mode")
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer resetTabletMode(cleanupCtx)

	var uiHdl cuj.UIActionHandler
	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to rotate display to landscape: ", err)
		}
		defer cleanup(cleanupCtx)
		if uiHdl, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create tablet action handler: ", err)
		}
	} else {
		if uiHdl, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
	}
	defer uiHdl.Close(ctx)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard input: ", err)
	}
	defer kb.Close(ctx)

	office := productivitycuj.NewGoogleDocs(tconn, kb, uiHdl, tabletMode)

	var expectedText, testFileLocation string
	if p.Tier == cuj.Premium {
		expectedText = "Mary had a little lamb whose fleece was white as snow And everywhere that Mary went the lamb was sure to go"
		testFileLocation = s.DataPath("productivity_cuj_voice_to_text_en.wav")
	}

	traceConfigPath := ""
	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	if err := productivitycuj.Run(ctx, cr, office, p.Tier, tabletMode, p.BrowserType, s.OutDir(), traceConfigPath, sampleSheetURL, expectedText, testFileLocation); err != nil {
		s.Fatal("Failed to run productivity cuj: ", err)
	}
}
