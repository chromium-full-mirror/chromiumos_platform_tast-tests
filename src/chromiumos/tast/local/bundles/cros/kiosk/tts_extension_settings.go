// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/a11y"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/chrome/lacros/lacrosproc"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/kioskmode"
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
			"group:complementary"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.KioskAutoLaunchCleanup,
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
		Timeout: 5 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.FloatingAccessibilityMenuEnabled{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func TTSExtensionSettings(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(kioskmode.TestData)

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		kioskmode.DefaultLocalAccounts(),
		kioskmode.AutoLaunch(kioskmode.KioskAppAccountID),
		kioskmode.PublicAccountPolicies(kioskmode.KioskAppAccountID, param.Policies),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if param.IsLacros {
		if _, err = lacrosproc.Root(ctx, tconn); err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}

	// Mute the device to avoid noisiness.
	ctxCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	defer crastestclient.Unmute(ctxCleanup)

	// Force-enable ChromeVox for the duration of this test.
	if err := a11y.SetFeatureEnabled(ctx, tconn, a11y.SpokenFeedback, true); err != nil {
		s.Fatal("Failed to enable ChromeVox: ", err)
	}
	defer a11y.ClearFeature(ctxCleanup, tconn, a11y.SpokenFeedback)

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
				Ancestor(nodewith.Name("ChromeVox Options").Role(role.RootWebArea)).
				First())),
	}, {
		name: "eSpeak-NG",
		openSettings: uiauto.Combine("open eSpeak-NG settings",
			ui.DoDefault(nodewith.NameStartingWith("Text-to-Speech voice settings").Role(role.Link)),
			ui.DoDefault(nodewith.Name("Settings").Role(role.Button).ClassName("tast-eSpeakNG text-to-speech extension")),
			ui.WaitUntilExists(nodewith.
				Name("Enabled Languages").
				Ancestor(nodewith.Name("eSpeak-NG Options").Role(role.RootWebArea)).
				First())),
	}, {
		name: "Google TTS",
		openSettings: uiauto.Combine("open Google TTS settings",
			ui.DoDefault(nodewith.NameStartingWith("Text-to-Speech voice settings").Role(role.Link)),
			ui.DoDefault(nodewith.Name("Settings").Role(role.Button).ClassName("tast-Chrome OS built-in text-to-speech extension")),
			ui.WaitUntilExists(nodewith.
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
			)(ctx); err != nil {
				s.Fatal("Failed: ", err)
			}

			if err := extension.openSettings(ctx); err != nil {
				s.Fatal("Failed to open extension's settings: ", err)
			}
		})
	}
}
