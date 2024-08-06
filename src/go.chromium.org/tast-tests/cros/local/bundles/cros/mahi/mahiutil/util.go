// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mahiutil

import (
	"context"
	"os"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	longUITimeout = 10 * time.Second
	// LocalHTMLZip is the name of the local html tarball.
	LocalHTMLZip = "mahi_html.zip"
)

var (
	consentTryItButton = nodewith.Name("Try it").ClassName("MdTextButton")
	consentGotItButton = nodewith.Name("Got it").ClassName("MdTextButton")
	contextMenu        = nodewith.ClassName("SubmenuView").Role("menu")
	// SummarizeButton is the button on the normal Mahi widget to request a summry.
	SummarizeButton = nodewith.Name("Summarize").ClassName("LabelButton")
	// CompactSummaryButton is the button on the compact Mahi widget to request a summry.
	CompactSummaryButton = nodewith.Name("Help me read this page").ClassName("MahiCondensedMenuButton")
	// SummaryOutlinesSection shows the summary & QA on the Mahi result panel.
	SummaryOutlinesSection = nodewith.ClassName("SummaryOutlinesSection")
	// SummaryText is the summary text on the Mahi result panel.
	SummaryText = nodewith.NameRegex(regexp.MustCompile(`^.{20,}$`)).ClassName("Label").Role("staticText").Ancestor(SummaryOutlinesSection)
	// MahiErrorStatus is the error message on the Mahi result panel.
	MahiErrorStatus = nodewith.ClassName("MahiErrorStatusView")
	// MahiCloseButton is to close the Mahi result panel.
	MahiCloseButton = nodewith.Name("Close").ClassName("IconButton")
)

// InstallScreenAIDLC installs and verifies the screen-ai dlc
func InstallScreenAIDLC(ctx context.Context) error {
	// Force install screen-ai dlc.
	if err := dlc.Install(ctx, "screen-ai", ""); err != nil {
		return errors.Wrap(err, "failed to install screen-ai dlc")
	}

	// Ensure screen2x is installed.
	if err := testing.Poll(ctx, a11y.VerifyScreenAIInstalled, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 10 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "fails to verify screen-ai dlc")
	}

	return nil
}

// PrepareLocalHTML unzips the local html files
// TODO(b:358454189): try using zip package to simplify this.
func PrepareLocalHTML(ctx context.Context, s *testing.State) (string, []os.DirEntry, error) {
	localHTMLPath := path.Join(os.TempDir(), "mahi.local_htmls")
	if err := os.MkdirAll(localHTMLPath, 0755); err != nil {
		return "", nil, errors.Wrap(err, "failed to make temp dir")
	}

	if err := testexec.CommandContext(ctx, "unzip", "-o", s.DataPath(LocalHTMLZip), "-d", localHTMLPath).Run(testexec.DumpLogOnError); err != nil {
		return "", nil, errors.Wrap(err, "failed to unzip local html files")
	}

	localHTMLFiles, err := os.ReadDir(localHTMLPath)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to read HTML file list from local html directory")
	}

	return localHTMLPath, localHTMLFiles, nil
}

// CleanUIElement cleans up Mahi widgets / panels if any, to prepare the screen for the next run.
func CleanUIElement(ctx context.Context, ui *uiauto.Context, kb *input.KeyboardEventWriter) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := uiauto.Combine("Hide context menu",
			uiauto.IfSuccessThen(ui.Exists(contextMenu), kb.AccelAction("Esc")),
			ui.WaitUntilGone(contextMenu),
		)(ctx); err != nil {
			return errors.Wrap(err, "fail to hide the context menu")
		}

		if err := uiauto.Combine("Hide mahi panel",
			uiauto.IfSuccessThen(ui.Exists(MahiCloseButton), ui.LeftClick(MahiCloseButton)),
			ui.WaitUntilGone(MahiCloseButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "fail to hide the mahi panel")
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  5 * time.Second,
		Interval: time.Second,
	})
}

// NavigateToURL opens URL and wait for quiescence
func NavigateToURL(ctx context.Context, conn *chrome.Conn, url string) error {
	if err := conn.Navigate(ctx, url); err != nil {
		return errors.Wrap(err, "failed to open url")
	}
	if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
		return errors.Wrapf(err, "failed to wait for quiescence, URL=%s", url)
	}
	return nil
}

// MaybePassConsentFlow passes the one-off consent flow if the related elements exists
func MaybePassConsentFlow(
	ctx context.Context,
	conn *chrome.Conn,
	tconn *chrome.TestConn,
	window *ash.Window,
	ui *uiauto.Context,
	kb *input.KeyboardEventWriter,
	url string) error {
	if err := NavigateToURL(ctx, conn, url); err != nil {
		return errors.Wrap(err, "failed to open a local html")
	}
	if err := mouse.Click(tconn, window.TargetBounds.CenterPoint(), mouse.RightButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to right click")
	}

	if err := ui.WaitUntilExists(consentTryItButton)(ctx); err != nil {
		if err := ui.WaitUntilAnyExists(SummarizeButton, CompactSummaryButton)(ctx); err != nil {
			return errors.Wrap(err, "no consent flow nor summary button")
		}
	} else {
		if err := uiauto.Combine("Do consent flow",
			ui.Exists(consentTryItButton),
			ui.LeftClick(consentTryItButton),
			ui.WaitUntilExists(consentGotItButton),
			ui.LeftClick(consentGotItButton),
			ui.WaitUntilAnyExists(SummaryText, MahiErrorStatus),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to pass the consent flow")
		}
	}

	return CleanUIElement(ctx, ui, kb)
}

// RightClickAndMaybeShowMahiWidget does a right click, and if expectedMahiWidget, waits for the mahi summary button.
func RightClickAndMaybeShowMahiWidget(
	ctx context.Context,
	tconn *chrome.TestConn,
	window *ash.Window,
	ui *uiauto.Context,
	expectMahiWidget bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := mouse.Click(tconn, window.TargetBounds.CenterPoint(), mouse.RightButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to right click")
		}
		if !expectMahiWidget {
			return ui.WaitUntilExists(contextMenu)(ctx)
		}
		return ui.WaitUntilAnyExists(SummarizeButton, CompactSummaryButton)(ctx)
	}, &testing.PollOptions{
		Timeout:  5 * time.Second,
		Interval: time.Second,
	})

}
