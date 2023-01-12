// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/bundles/cros/spera/multitaskingapp"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/cuj/bluetooth"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

type multiTaskingParam struct {
	tier        cuj.Tier
	appName     string
	enableBT    bool // enable the bluetooth or not
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultiTaskingApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures the performance of multi-tasking app test",
		BugComponent: "b:1024862", // ChromeOS > EngProd > Platform > SPERA
		Contacts:     []string{"cienet-development@googlegroups.com", "jane.yang@cienet.com", "xibin@google.com"},
		SoftwareDeps: []string{"chrome", "arc"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Vars: []string{
			"spera.cuj_mute",                      // Optional. Mute the DUT during the test.
			"spera.cuj_mode",                      // Optional. Expecting "tablet" or "clamshell".
			"spera.collectTrace",                  // Optional. Expecting "enable" or "disable", default is "disable".
			"spera.bt_devicename",                 // Required for Bluetooth subtests.
			"spera.MultiTaskingApp.operateCamera", // Optional. Expecting "true" or "false", default is "true".
			"spera.MultiTaskingApp.web_source",    // Optional. Expecting "google" or "external", default is "external".
		},
		Data: []string{"cca_ui.js", cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "essential",
				Fixture: "loggedInAndKeepState",
				Timeout: 20 * time.Minute,
				Val: multiTaskingParam{
					tier:     cuj.Essential,
					appName:  multitaskingapp.YoutubeMusicAppName,
					enableBT: false,
				},
			}, {
				Name:              "essential_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           20 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: multiTaskingParam{
					tier:        cuj.Essential,
					appName:     multitaskingapp.YoutubeMusicAppName,
					enableBT:    false,
					browserType: browser.TypeLacros,
				},
			}, {
				Name:    "advanced",
				Fixture: "loggedInAndKeepState",
				Timeout: 30 * time.Minute,
				Val: multiTaskingParam{
					tier:     cuj.Advanced,
					appName:  multitaskingapp.YoutubeMusicAppName,
					enableBT: false,
				},
			}, {
				Name:              "advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacros",
				Timeout:           30 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: multiTaskingParam{
					tier:        cuj.Advanced,
					appName:     multitaskingapp.YoutubeMusicAppName,
					enableBT:    false,
					browserType: browser.TypeLacros,
				},
			},
		},
	})
}

func MultiTaskingApp(ctx context.Context, s *testing.State) {
	param := s.Param().(multiTaskingParam)
	tier := param.tier
	app := param.appName
	enableBT := param.enableBT

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	a := s.FixtValue().(cuj.FixtureData).ARC

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if _, ok := s.Var("spera.cuj_mute"); ok {
		if err := crastestclient.Mute(ctx); err != nil {
			s.Fatal("Failed to mute audio: ", err)
		}
		defer crastestclient.Unmute(cleanupCtx)
	}

	isBtEnabled, err := bluetooth.IsEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to get bluetooth status: ", err)
	}

	if enableBT {
		testing.ContextLog(ctx, "Start to connect bluetooth")
		deviceName := s.RequiredVar("spera.bt_devicename")
		if err := bluetooth.ConnectDevice(ctx, deviceName); err != nil {
			s.Fatal("Failed to connect bluetooth: ", err)
		}
		if !isBtEnabled {
			defer func(ctx context.Context) {
				if err := bluetooth.Disable(ctx); err != nil {
					s.Fatal("Failed to disable bluetooth: ", err)
				}
			}(cleanupCtx)
		}
	} else if isBtEnabled {
		testing.ContextLog(ctx, "Start to disable bluetooth")
		if err := bluetooth.Disable(ctx); err != nil {
			s.Fatal("Failed to disable bluetooth: ", err)
		}
		defer func(ctx context.Context) {
			if err := bluetooth.Enable(ctx); err != nil {
				s.Fatal("Failed to connect bluetooth: ", err)
			}
		}(cleanupCtx)
	}

	// The cras active node will change if the bluetooth has been connected or disconnected.
	// Wait until cras node change completes before doing output volume testing.
	condition := func(cn *audio.CrasNode) bool {
		if !cn.Active {
			return false
		}

		// According to src/third_party/adhd/cras/README.dbus-api,
		// the audio.CrasNode.Type should be "BLUETOOTH" when a BlueTooth device
		// is used as input or output.
		if enableBT {
			return cn.Type == "BLUETOOTH"
		}
		return cn.Type != "BLUETOOTH"
	}
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create cras: ", err)
	}
	if err := cras.WaitForDeviceUntil(ctx, condition, 40*time.Second); err != nil {
		s.Fatalf("Failed to wait for cras nodes to be in expected status: %v; please check the DUT log for possible audio device connection issue", err)
	}

	// Spotify login account.
	var account string
	if app == multitaskingapp.SpotifyAppName {
		account = cr.Creds().User
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

	webSource := cuj.ExternalWebSource
	if ws, ok := s.Var("spera.MultiTaskingApp.web_source"); ok {
		webSource = cuj.WebSourceType(strings.ToLower(ws))
	}

	testRunParams := multitaskingapp.NewRunParams(tier, ccaScriptPaths, s.OutDir(), app, account, traceConfigPath, tabletMode, enableBT, webSource)
	if err := multitaskingapp.Run(ctx, cr, param.browserType, a, testRunParams); err != nil {
		s.Fatal("Failed to run multi-tasking app test: ", err)
	}
}
