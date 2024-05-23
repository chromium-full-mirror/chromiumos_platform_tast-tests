// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TTSExtensionSettings,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the accessbility extensions settings can be opened in PWA Kiosk",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"neis@google.com",
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration + time.Minute,
		Params: []testing.Param{{
			Name: "ash",
			Val: kioskmode.TestData{
				IsLacros: false,
				Policies: []policy.Policy{
					&policy.FloatingAccessibilityMenuEnabled{Val: true},
				},
			},
		}, {
			Name: "lacros",
			Val: kioskmode.TestData{
				IsLacros: true,
				Policies: []policy.Policy{
					&policy.LacrosAvailability{Val: "lacros_only"},
					&policy.FloatingAccessibilityMenuEnabled{Val: true},
				},
			},
			ExtraSoftwareDeps: []string{"lacros"},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.FloatingAccessibilityMenuEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func TTSExtensionSettings(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(kioskmode.TestData)

	cleanupKioskCtx := ctx
	ctx, cancelKioskCtx := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancelKioskCtx()

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		kioskmode.PublicAccountPolicies(kioskmode.KioskAppAccountID, param.Policies),
	)
	if err != nil {
		s.Fatal("Failed to create Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupKioskCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if param.IsLacros {
		if _, err = lacrosproc.Root(ctx, tconn); err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
		s.Log("http://b/281993208 sleep 5 seconds before checking UI tree in lacros")
		// GoBigSleepLint: TODO(b/281993208) lacros needs some time before we can check the UI tree.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep before checking UI tree in lacros: ", err)
		}
	}

	// Mute the device to avoid noisiness.
	cleanupMuteCtx := ctx
	ctx, cancelMuteCtx := ctxutil.Shorten(ctx, time.Second)
	defer cancelMuteCtx()
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	defer crastestclient.Unmute(cleanupMuteCtx)

	// Force-enable ChromeVox for the duration of this test.
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SpokenFeedback, true); err != nil {
		s.Fatal("Failed to enable ChromeVox: ", err)
	}
	defer a11y.ClearFeature(cleanupMuteCtx, tconn, a11y.SpokenFeedback)

	ui := uiauto.New(tconn)

	for i, extension := range []struct {
		name         string
		openSettings uiauto.Action
	}{{
		name: "ChromeVox",
		openSettings: uiauto.Combine("open ChromeVox settings",
			ui.DoDefault(nodewith.Name("ChromeVox settings").Role(role.Link)),
			ui.WaitUntilExists(nodewith.
				Name("Enable verbose descriptions").
				Ancestor(nodewith.Name("Settings - ChromeVox").Role(role.RootWebArea)).
				First())),
	}, {
		name: "eSpeak-NG",
		openSettings: uiauto.Combine("open eSpeak-NG settings",
			ui.DoDefault(nodewith.NameStartingWith("Text-to-Speech voice settings").Role(role.Link)),
			ui.DoDefault(nodewith.Name("Settings").Role(role.Button).ClassName("tast-eSpeakNG text-to-speech extension")),
			ui.WithTimeout(30*time.Second).WaitUntilExists(nodewith.
				Name("Enabled Languages").
				Ancestor(nodewith.Name("eSpeak-NG Options").Role(role.RootWebArea)).
				First())),
	}, {
		name: "Google TTS",
		openSettings: uiauto.Combine("open Google TTS settings",
			ui.DoDefault(nodewith.NameStartingWith("Text-to-Speech voice settings").Role(role.Link)),
			ui.DoDefault(nodewith.Name("Settings").Role(role.Button).ClassName("tast-Chrome OS built-in text-to-speech extension")),
			ui.WithTimeout(30*time.Second).WaitUntilExists(nodewith.
				Name("Search voices").
				Ancestor(nodewith.Name("Google TTS Settings").Role(role.RootWebArea)).
				First())),
	}} {
		s.Run(ctx, extension.name, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, fmt.Sprintf("ui_tree_%d", i))

			if err := uiauto.Combine("open accessibility settings",
				ui.DoDefault(nodewith.Name("Open accessibility settings menu").Role(role.ToggleButton)),
				ui.DoDefault(nodewith.Name("Accessibility settings").Role(role.Button)),
				ui.DoDefault(nodewith.NameStartingWith("Text-to-Speech").Role(role.Link)),
			)(ctx); err != nil {
				s.Fatal("Failed: ", err)
			}

			if err := extension.openSettings(ctx); err != nil {
				s.Fatal("Failed to open extension's settings: ", err)
			}
		})
	}
}
