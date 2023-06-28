// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TTSExtensionSettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test that each Text-to-Speech extension's settings can be opened",
		Contacts:     []string{"chromeos-a11y-eng@google.com", "neis@chromium.org"},
		BugComponent: "b:1272672",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		Params: []testing.Param{{
			Name:    "ash",
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}},
	})
}

func TTSExtensionSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	bt := s.Param().(browser.Type)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Mute the device to avoid noisiness.
	ctxCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	defer cancel()
	defer crastestclient.Unmute(ctxCleanup)

	// Force-enable ChromeVox for the duration of this test.
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SpokenFeedback, true); err != nil {
		s.Fatal("Failed to enable ChromeVox: ", err)
	}
	defer a11y.ClearFeature(ctxCleanup, tconn, a11y.SpokenFeedback)

	ui := uiauto.New(tconn)
	ttsLink := nodewith.NameStartingWith("Text-to-Speech").Role(role.Link)

	for i, extension := range []struct {
		settingsWindowTitle string
		openSettings        func(ctx context.Context, ui *uiauto.Context) error
	}{{
		settingsWindowTitle: "Settings - ChromeVox",
		openSettings: func(ctx context.Context, ui *uiauto.Context) (retErr error) {
			return uiauto.Combine("open ChromeVox settings",
				ui.DoDefault(nodewith.Name("ChromeVox settings").Role(role.Link)),
			)(ctx)
		},
	}, {
		settingsWindowTitle: "eSpeak-NG Options",
		openSettings: func(ctx context.Context, ui *uiauto.Context) (retErr error) {
			return uiauto.Combine("open eSpeak-NG settings",
				ui.DoDefault(nodewith.NameStartingWith("Text-to-Speech voice settings").Role(role.Link)),
				ui.DoDefault(nodewith.Name("Settings").Role(role.Button).ClassName("tast-eSpeakNG text-to-speech extension")),
			)(ctx)
		},
	}, {
		settingsWindowTitle: "Google TTS Settings",
		openSettings: func(ctx context.Context, ui *uiauto.Context) (retErr error) {
			return uiauto.Combine("open Google TTS settings",
				ui.DoDefault(nodewith.NameStartingWith("Text-to-Speech voice settings").Role(role.Link)),
				ui.DoDefault(nodewith.Name("Settings").Role(role.Button).ClassName("tast-Chrome OS built-in text-to-speech extension")),
			)(ctx)
		},
	}} {
		s.Run(ctx, extension.settingsWindowTitle, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, fmt.Sprintf("ui_tree_%d", i))

			if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "osAccessibility", ui.Exists(ttsLink)); err != nil {
				s.Fatal("Failed to launch OS accessibility settings: ", err)
			}
			if err := ui.DoDefault(ttsLink)(ctx); err != nil {
				s.Fatal("Failed to open TTS settings: ", err)
			}
			if err := extension.openSettings(ctx, ui); err != nil {
				s.Fatal("Failed to open extension's settings: ", err)
			}
			if err := ash.WaitForCondition(ctx, tconn, settingsWindowMatch(ctx, bt, extension.settingsWindowTitle), &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
				s.Fatalf("Failed to find `%s` window: %v", extension.settingsWindowTitle, err)
			}
			ws, err := ash.GetAllWindows(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get all windows: ", err)
			}
			if len(ws) != 2 {
				s.Fatalf("Unexpected number of windows, want 2, got %d", len(ws))
			}
		})
		if err := cr.ResetState(ctx); err != nil {
			s.Fatal("Failed to reset Chrome: ", err)
		}
	}
}

func settingsWindowMatch(ctx context.Context, bt browser.Type, titlePrefix string) func(w *ash.Window) bool {
	if bt == browser.TypeLacros {
		return osURLHandlerWindowMatch(ctx, titlePrefix)
	}
	return ash.BrowserTitleMatch(browser.TypeAsh, titlePrefix)
}

func osURLHandlerWindowMatch(ctx context.Context, titlePrefix string) func(w *ash.Window) bool {
	titlePrefix = "ChromeOS-URLs - " + titlePrefix
	return func(w *ash.Window) bool {
		return w.WindowType == ash.WindowTypeExtension && strings.HasPrefix(w.Title, titlePrefix)
	}
}
