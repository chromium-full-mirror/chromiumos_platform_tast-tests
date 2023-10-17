// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/crostiniapps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/crostini/imetestutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppFirefoxIME,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Open a test webpage with an input box in Firefox and test IME inputs",
		Contacts:     []string{"clumptini@google.com", "sophialin@google.com"},
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "vm_host"},
		BugComponent: "b:1122570",
		Params: []testing.Param{
			// TODO(b/304160903,b/305579517): Removed from generation to promote bookworm tests.
			{
				Name:              "bullseye_clamshell_arabic_stable",
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppStable,
				Fixture:           "crostiniBullseyeLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "arabic",
			}, {
				Name:              "bullseye_clamshell_arabic_unstable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppUnstable,
				Fixture:           "crostiniBullseyeLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "arabic",
			}, {
				Name:              "bookworm_clamshell_arabic_stable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppStable,
				Fixture:           "crostiniBookwormLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "arabic",
			}, {
				Name:              "bookworm_clamshell_arabic_unstable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppUnstable,
				Fixture:           "crostiniBookwormLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "arabic",
			}, {
				Name:              "bullseye_clamshell_english_stable",
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppStable,
				Fixture:           "crostiniBullseyeLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "english",
			}, {
				Name:              "bullseye_clamshell_english_unstable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppUnstable,
				Fixture:           "crostiniBullseyeLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "english",
			}, {
				Name:              "bookworm_clamshell_english_stable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppStable,
				Fixture:           "crostiniBookwormLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "english",
			}, {
				Name:              "bookworm_clamshell_english_unstable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppUnstable,
				Fixture:           "crostiniBookwormLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "english",
			}, {
				Name:              "bullseye_clamshell_japanese_stable",
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppStable,
				Fixture:           "crostiniBullseyeLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "japanese",
			}, {
				Name:              "bullseye_clamshell_japanese_unstable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppUnstable,
				Fixture:           "crostiniBullseyeLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "japanese",
			}, {
				Name:              "bookworm_clamshell_japanese_stable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppStable,
				Fixture:           "crostiniBookwormLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "japanese",
			}, {
				Name:              "bookworm_clamshell_japanese_unstable",
				ExtraAttr:         []string{"informational"},
				ExtraSoftwareDeps: []string{"crostini_app", "dlc"},
				ExtraHardwareDeps: crostini.CrostiniAppUnstable,
				Fixture:           "crostiniBookwormLargeContainerClamshell",
				Timeout:           15 * time.Minute,
				Val:               "japanese",
			},
		},
	})
}

func AppFirefoxIME(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(crostini.FixtureData).Tconn
	cont := s.FixtValue().(crostini.FixtureData).Cont
	keyboard := s.FixtValue().(crostini.FixtureData).KB

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// TODO(b/304160903): Disable CSD to work around IME bug.
	if err := crostiniapps.DisableFirefoxCSD(ctx, cont); err != nil {
		s.Fatal("Failed to disable client-side decorations: ", err)
	}

	// Open Terminal app.
	terminalApp, err := terminalapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Terminal app: ", err)
	}
	defer terminalApp.Exit(keyboard)(cleanupCtx)

	// Switch back to default IME.
	defer imetestutil.ResetToDefaultIME(cleanupCtx, tconn)

	// Since defers are executed in a stack, this needs to be the last defer so it doesn't close the window before dumping the tree.
	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_tree")
	s.AttachErrorHandlers(handler, handler)

	imeName := s.Param().(string)
	imeData := imetestutil.IMETestCases[imeName]
	if err := testUseIMEInFirefox(ctx, terminalApp, keyboard, tconn, cont, imeData); err != nil {
		s.Fatal("Failed to open firefox and type using IMEs: ", err)
	}

}

func testUseIMEInFirefox(ctx context.Context, terminalApp *terminalapp.TerminalApp, keyboard *input.KeyboardEventWriter, tconn *chrome.TestConn, cont *vm.Container, imeData imetestutil.IMETestData) error {
	ui := uiauto.New(tconn)
	uda := uidetection.NewDefault(tconn)

	if err := crostiniapps.LaunchFirefoxWithTestPage(ctx, tconn, uda, ui, cont, terminalApp, keyboard); err != nil {
		return errors.Wrap(err, "failed to create Firefox test page")
	}

	if err := uiauto.Combine("enter text in Firefox",
		imeData.InputMethod.InstallAndActivate(tconn),
		imeData.InputMethod.WaitUntilActivated(tconn),
		imeData.EnterTestStringActionPK(keyboard, ui),
		crostini.TakeAppScreenshot("firefox"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to enter test string Firefox")
	}

	if err := imetestutil.CheckInputViaClipboard(ctx, keyboard, tconn, imeData.ExpectedText); err != nil {
		return err
	}

	if imeData.InputMethod == ime.Japanese {
		if err := imetestutil.TestJapaneseCandidatesBoxInFirefox(ctx, ui, uda, keyboard); err != nil {
			return err
		}
	}

	if err := crostiniapps.CloseFirefoxTestPage(ctx, uda, ui, cont, keyboard); err != nil {
		return errors.Wrap(err, "failed to close firefox")
	}

	return nil
}
