// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfaillog"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// noUnexpectedUITestParam contains the options that configure NoUnexpectedUI tests.
type noUnexpectedUITestParam struct {
	// Whether this test uses lacros or ash.
	isLacros bool
}

const verifyPixelsTimeout = 5 * time.Second

func init() {
	testing.AddTest(&testing.Test{
		Func:         NoUnexpectedUI,
		Desc:         "Verifies there are no artifacts in the screen other than the Kiosk app",
		LacrosStatus: testing.LacrosVariantExists,
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"edmanp@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration + verifyPixelsTimeout,
		SoftwareDeps: []string{"reboot", "chrome", "lacros"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Params: []testing.Param{
			{
				Name: "ash",
				Val:  noUnexpectedUITestParam{isLacros: false},
			},
			{
				Name: "lacros",
				Val:  noUnexpectedUITestParam{isLacros: true},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Test auto-launched kiosk.
				Value: "screenplay-90708eaa-cdde-4060-8ccb-eeb305485531",
			},
			{
				Key: "feature_id",
				// Test launch PWA kiosk.
				Value: "screenplay-d93db63f-372e-4c3b-a1ca-3f23587dbac4",
			},
		},
	})
}

var greenApp = kioskmode.SimpleWebApp{
	Name:        "Green App",
	Description: "A Web app that is entirely green",
	IconColor:   color.RGBA{G: 255, A: 255},
	HTMLBody:    `<body style="background-color:#00ff00"></body>`,
}

// kioskModeOptions returns the option slice for this test parameter along a cleanup function.
func (param noUnexpectedUITestParam) kioskModeOptions(ctx context.Context) ([]kioskmode.Option, func()) {
	_, accounts, cleanup := greenApp.NewServerAndAccount(ctx)
	accountID := *accounts.Val[0].AccountID

	options := []kioskmode.Option{
		kioskmode.AutoLaunch(accountID),
		kioskmode.CustomLocalAccounts(&accounts),
	}

	if param.isLacros {
		options = append(options, kioskmode.PublicAccountPolicies(
			accountID,
			[]policy.Policy{&policy.LacrosAvailability{Val: "lacros_only"}},
		))
	}

	return options, cleanup
}

func waitUntilGreenAppStarted(ctx context.Context, cr *chrome.Chrome, outDir string) error {
	testing.ContextLog(ctx, "Waiting until Green app started")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Name(greenApp.Name).Role(role.RootWebArea))(ctx); err != nil {
		faillog.DumpUITree(ctx, filepath.Join(outDir, "green_app_faillog"), tconn)
		return errors.Wrap(err, "failed to find Green app node")
	}
	return nil
}

func verifyScreenIsAllGreen(ctx context.Context, screenshotPath string) error {
	testing.ContextLog(ctx, "Verifying screen is all green")
	// TODO(b/302098012): Lacros may briefly show the desktop. Poll until the condition passes.
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := screenshot.Capture(ctx, screenshotPath); err != nil {
			return errors.Wrap(err, "failed to take screenshot")
		}
		fd, err := os.Open(screenshotPath)
		if err != nil {
			return errors.Wrap(err, "error opening screenshot file")
		}
		img, _, err := image.Decode(fd)
		if err != nil {
			return errors.Wrap(err, "error decoding screenshot file")
		}
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
				if c := img.At(x, y); c != greenApp.IconColor {
					return errors.Errorf("mismatching color %v found at (%v,%v), expected %v", c, x, y, greenApp.IconColor)
				}
			}
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: verifyPixelsTimeout})
}

func verifyExpectedBrowser(ctx context.Context, cr *chrome.Chrome, isLacros bool) error {
	testing.ContextLog(ctx, "Verifying expected browser")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	_, err = lacrosproc.Root(ctx, tconn)
	if isLacros && err != nil {
		return errors.Wrap(err, "expected lacros but failed to get lacros process")
	} else if !isLacros && err == nil {
		return errors.New("expected ash but found lacros process")
	}
	return nil
}

func saveTestArtifacts(ctx context.Context, cr *chrome.Chrome, hasError bool, outDir string, isLacros bool) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	if hasError {
		faillog.DumpUITree(ctx, filepath.Join(outDir), tconn)
		if err := screenshot.Capture(ctx, filepath.Join(outDir, "NoUnexpectedUI_error.png")); err != nil {
			return errors.Wrap(err, "failed to take screenshot")
		}
	}
	if isLacros {
		lacrosfaillog.Save(ctx, tconn)
	}
	return nil
}

func NoUnexpectedUI(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(noUnexpectedUITestParam)
	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	options, cleanupOptions := param.kioskModeOptions(ctx)
	defer cleanupOptions()

	kiosk, cr, err := kioskmode.New(ctx, fdms, signinTestExtensionManifestKey, options...)
	if err != nil {
		s.Fatal("Failed to create Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := saveTestArtifacts(ctx, cr, s.HasError(), s.OutDir(), param.isLacros); err != nil {
			s.Error("Failed to save test artifacts: ", err)
		}
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	if err := waitUntilGreenAppStarted(ctx, cr, s.OutDir()); err != nil {
		s.Fatal("Kiosk launched but app did not start: ", err)
	}

	if err := verifyScreenIsAllGreen(ctx, filepath.Join(s.OutDir(), "GreenApp.png")); err != nil {
		s.Fatal("Failed to verify Kiosk is all green: ", err)
	}

	if err := verifyExpectedBrowser(ctx, cr, param.isLacros); err != nil {
		s.Fatal("Failed to verify expected browser: ", err)
	}
}
