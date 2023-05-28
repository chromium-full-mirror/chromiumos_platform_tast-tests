// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/spera/multitaskingapp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type multiTaskingParam struct {
	tier        cuj.Tier
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultiTaskingApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of multi-tasking app test",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "chicheny@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"spera.cuj_mute",                      // Optional. Mute the DUT during the test.
			"spera.cuj_mode",                      // Optional. Expecting "tablet" or "clamshell".
			"spera.collectTrace",                  // Optional. Expecting "enable" or "disable", default is "disable".
			"spera.MultiTaskingApp.operateCamera", // Optional. Expecting "true" or "false", default is "true".
			"spera.MultiTaskingApp.web_source",    // Optional. Expecting "google" or "external", default is "google".
		},
		Data: []string{"cca_ui.js", cujrecorder.SystemTraceConfigFile},
		Attr: []string{"group:camera_dependent"},
		Params: []testing.Param{
			{
				Name:    "essential",
				Fixture: "loggedInAndKeepState",
				Timeout: 20 * time.Minute,
				Val: multiTaskingParam{
					tier: cuj.Essential,
				},
			}, {
				Name:              "essential_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: multiTaskingParam{
					tier:        cuj.Essential,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:    "advanced",
				Fixture: "loggedInAndKeepState",
				Timeout: 30 * time.Minute,
				Val: multiTaskingParam{
					tier: cuj.Advanced,
				},
			}, {
				Name:              "advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           30 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: multiTaskingParam{
					tier:        cuj.Advanced,
					browserType: browser.TypeLacros,
				},
			},
		},
	})
}

func MultiTaskingApp(ctx context.Context, s *testing.State) {
	param := s.Param().(multiTaskingParam)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if _, ok := s.Var("spera.cuj_mute"); ok {
		if err := crastestclient.Mute(ctx); err != nil {
			s.Fatal("Failed to mute audio: ", err)
		}
		defer crastestclient.Unmute(cleanupCtx)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "spera.cuj_mode")
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer resetTabletMode(cleanupCtx)

	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to rotate display to landscape: ", err)
		}
		defer cleanup(cleanupCtx)
	}
	traceConfigPath := ""
	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	var ccaScriptPaths []string
	if v, ok := s.Var("spera.MultiTaskingApp.operateCamera"); !ok || strings.ToLower(v) != "false" {
		// If there is no variable, the camera should be operated.
		ccaScriptPaths = []string{s.DataPath("cca_ui.js")}
	}

	webSource := cuj.GoogleWebSource
	ws, ok := s.Var("spera.MultiTaskingApp.web_source")
	if ok && strings.ToLower(ws) == string(cuj.ExternalWebSource) {
		webSource = cuj.ExternalWebSource
	}

	params := &multitaskingapp.TestParams{
		Tier:            param.tier,
		BrowserType:     param.browserType,
		WebSource:       webSource,
		CCAScriptPaths:  ccaScriptPaths,
		OutDir:          s.OutDir(),
		TraceConfigPath: traceConfigPath,
		TabletMode:      tabletMode,
	}

	if err := multitaskingapp.Run(ctx, cr, params); err != nil {
		s.Fatal("Failed to run multi-tasking app test: ", err)
	}
}
