// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUISettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Opens CCA and verifies the settings menu behavior",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Params: []testing.Param{{
			Fixture: "ccaTestBridgeReadyWithFakeHALCamera",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "ccaTestBridgeReadyWithFakeHALCameraLacros",
		}},
	})
}

// CCAUISettings verifies settings menu behavior.
func CCAUISettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(cca.FixtureData).Chrome
	bt := s.FixtValue().(cca.FixtureData).BrowserType
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp

	subTestTimeout := 40 * time.Second
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *chrome.Chrome, browser.Type, *cca.App) error
	}{{
		"testFeedback",
		testFeedback,
	}, {
		"testHelp",
		testHelp,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				cleanupCtx := ctx
				ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
				defer cancel()

				if err := app.OpenSettingMenu(ctx, cca.MainMenu); err != nil {
					s.Fatal("Failed to click settings button: ", err)
				}
				defer app.CloseSettingMenu(cleanupCtx, cca.MainMenu)

				return tst.testFunc(ctx, cr, bt, app)
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}
}

// testFeedback checks feedback button functionality.
func testFeedback(ctx context.Context, cr *chrome.Chrome, _ browser.Type, app *cca.App) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := app.Click(ctx, cca.FeedbackButton); err != nil {
		return errors.Wrap(err, "failed to click feedback button")
	}
	matcher := func(t *target.Info) bool {
		return strings.Contains(t.URL, "chrome://os-feedback")
	}
	fConn, err := cr.NewConnForTarget(ctx, matcher)
	if err != nil {
		return errors.Wrap(err, "failed to open feedback app")
	}
	defer fConn.Close()
	defer func(ctx context.Context) {
		if err := fConn.CloseTarget(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close the feedback page")
		}
	}(cleanupCtx)
	return nil
}

// testHelp checks help button functionality.
func testHelp(ctx context.Context, cr *chrome.Chrome, bt browser.Type, app *cca.App) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := app.Click(ctx, cca.HelpButton); err != nil {
		return errors.Wrap(err, "failed to click help button")
	}
	br, brCleanUp, err := browserfixt.Connect(ctx, cr, bt)
	if err != nil {
		return errors.Wrap(err, "failed to connect to browser")
	}
	defer brCleanUp(ctx)
	matcher := func(t *target.Info) bool {
		return strings.Contains(t.URL, "support.google.com") && t.Type == "page"
	}
	hConn, err := br.NewConnForTarget(ctx, matcher)
	if err != nil {
		return errors.Wrap(err, "failed to open help app")
	}
	defer hConn.Close()
	defer func(ctx context.Context) {
		if err := hConn.CloseTarget(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close the help page")
		}
	}(cleanupCtx)
	return nil
}
