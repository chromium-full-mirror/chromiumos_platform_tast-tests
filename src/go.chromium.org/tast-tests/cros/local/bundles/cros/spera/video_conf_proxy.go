// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/spera/videoconfproxy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	videoConfProxyTimeout = 5 * time.Minute
	videoConfHTMLFile     = "video_conf_proxy_local.html"
	docHTMLFile           = "power_video_call_doc.html"
	video720P15FPS        = "720p15fpsVP9.webm"
	video1080P30FPS       = "1080p30fpsVP9.webm"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VideoConfProxy,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A test case that simulates the video call testing",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "chicheny@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Vars: []string{
			"spera.cuj_mode",                  // Optional. Expecting "tablet" or "clamshell".
			"spera.collectTrace",              // Optional. Expecting "enable" or "disable", default is "disable".
			"spera.VideoConfProxy.web_source", // Optional. Expecting "google" or "local", default is "google".
		},
		Data: []string{cujrecorder.SystemTraceConfigFile, videoConfHTMLFile, docHTMLFile, video720P15FPS, video1080P30FPS},
		Params: []testing.Param{
			{
				Name:    "vp9_essential",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeAsh,
					Tier:        cuj.Essential,
				},
			},
			{
				Name:    "vp9_advanced",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeAsh,
					Tier:        cuj.Advanced,
				},
			},
			{
				Name:              "vp9_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeLacros,
					Tier:        cuj.Essential,
				},
			},
			{
				Name:              "vp9_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeLacros,
					Tier:        cuj.Advanced,
				},
			},
			{
				Name:    "vp9_grid_essential",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeAsh,
					Tier:        cuj.Essential,
					IsGrid:      true,
				},
			},
			{
				Name:    "vp9_grid_advanced",
				Fixture: "loggedInAndKeepStateWithFakeCamera",
				Timeout: 5 * time.Minute,
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeAsh,
					Tier:        cuj.Advanced,
					IsGrid:      true,
				},
			},
			{
				Name:              "vp9_grid_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeLacros,
					Tier:        cuj.Essential,
					IsGrid:      true,
				},
			},
			{
				Name:              "vp9_grid_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithFakeCamera",
				Timeout:           5 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: videoconfproxy.TestParams{
					BrowserType: browser.TypeLacros,
					Tier:        cuj.Advanced,
					IsGrid:      true,
				},
			},
		},
	})
}

func VideoConfProxy(ctx context.Context, s *testing.State) {
	p := s.Param().(videoconfproxy.TestParams)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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
	p.TabletMode = tabletMode
	p.OutDir = s.OutDir()

	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		p.TraceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}

	webSource := cuj.GoogleWebSource
	ws, ok := s.Var("spera.VideoConfProxy.web_source")
	if ok && cuj.WebSourceType(strings.ToLower(ws)) == cuj.LocalWebSource {
		webSource = cuj.LocalWebSource
	}

	var baseURL string
	switch webSource {
	case cuj.GoogleWebSource:
		baseURL = cuj.VideoCallURL
		p.DocsURL = cuj.VideoCallDocsURL
	case cuj.LocalWebSource:
		videoConfDirPath := path.Join(os.TempDir(), "spera.video_conf_proxy")
		if err := os.MkdirAll(videoConfDirPath, 0755); err != nil {
			s.Fatal("Failed to create video conf proxy directory: ", err)
		}
		defer os.RemoveAll(videoConfDirPath)

		for _, file := range []string{videoConfHTMLFile, docHTMLFile, video720P15FPS, video1080P30FPS} {
			if err := testexec.CommandContext(ctx, "cp", s.DataPath(file), videoConfDirPath).Run(testexec.DumpLogOnError); err != nil {
				s.Fatalf("Failed to copy %q to video conf proxy directory: %v", file, err)
			}
		}

		videoConfDir := http.Dir(videoConfDirPath)
		localServer := httptest.NewTLSServer(http.FileServer(videoConfDir))
		defer localServer.Close()

		baseURL = path.Join(localServer.URL, videoConfHTMLFile)
		p.DocsURL = path.Join(localServer.URL, docHTMLFile)
	default:
		s.Fatal("Unknown web source: ", webSource)
	}

	const (
		essentialPreset   = "essential"
		advancedPreset    = "advanced"
		normalVideoNumber = "4"
		gridVideoNumber   = "16"
	)

	preset := essentialPreset
	if p.Tier == cuj.Advanced {
		preset = advancedPreset
	}
	videoNumber := normalVideoNumber
	if p.IsGrid {
		videoNumber = gridVideoNumber
	}
	p.VideoCallURL = fmt.Sprintf("%s?preset=%s&&numVideo=%s", baseURL, preset, videoNumber)

	if err := videoconfproxy.Run(ctx, cr, p); err != nil {
		if err := crastestclient.DumpAudioDiagnostics(cleanupCtx, s.OutDir()); err != nil {
			s.Error("Failed to dump audio diagnostics: ", err)
		}
		s.Fatal("Failed to run video conference proxy cuj: ", err)
	}
}
