// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mahiutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"time"
	"unicode/utf8"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	longUITimeout      = 10 * time.Second
	mockQuestionString = "This is a mock question"
	mockResponseString = "This is a fake response with text"
)

var (
	consentTryItButton         = nodewith.Name("Try it").ClassName("MdTextButton")
	consentGotItButton         = nodewith.Name("Got it").ClassName("MdTextButton")
	contextMenu                = nodewith.ClassName("SubmenuView").Role("menu")
	mahiMenuView               = nodewith.ClassName("MahiMenuView")
	mahiPanelView              = nodewith.ClassName("MahiPanelView")
	mahiQAView                 = nodewith.ClassName("MahiQuestionAnswerView").Ancestor(mahiPanelView)
	compactSummaryButton       = nodewith.Name("Summarize with Help me read").ClassName("MahiCondensedMenuButton")
	summaryElucidationSection  = nodewith.ClassName("SummaryOutlinesElucidationSection").Ancestor(mahiPanelView)
	summaryIndicatorLabel      = nodewith.Name("Summary").ClassName("Label").Role("staticText").Ancestor(summaryElucidationSection)
	simplifyIndicatorLabel     = nodewith.Name("Simplified text").ClassName("Label").Role("staticText").Ancestor(summaryElucidationSection)
	anySummaryText             = nodewith.NameRegex(regexp.MustCompile(`^.{20,}$`)).ClassName("Label").Role("staticText").Ancestor(summaryElucidationSection)
	mockSummaryText            = nodewith.Name(mockResponseString).ClassName("Label").Role("staticText").Ancestor(summaryElucidationSection)
	mahiErrorStatus            = nodewith.ClassName("MahiErrorStatusView").Ancestor(mahiPanelView)
	mahiCloseButton            = nodewith.Name("Close").ClassName("IconButton").Ancestor(mahiPanelView)
	questionTextInputOnWidget  = nodewith.ClassName("Textfield").Role("textField").Ancestor(mahiMenuView)
	questionSendButtonOnWidget = nodewith.Name("Send").ClassName("ImageButton").Role("button").Ancestor(mahiMenuView)
	questionTextInputOnPanel   = nodewith.ClassName("Textfield").Role("textField").Ancestor(mahiPanelView)
	questionSendButtonOnPanel  = nodewith.Name("Send").ClassName("IconButton").Role("button").Ancestor(mahiPanelView)
	mockQuestion               = nodewith.Name(mockQuestionString).ClassName("Label").Role("staticText").Ancestor(mahiQAView)
	mockAnswer                 = nodewith.Name(mockResponseString).ClassName("Label").Role("staticText").Ancestor(mahiQAView)

	// SimplifyButton is the button on the normal Mahi widget to request a
	// simplification for the selected text.
	SimplifyButton = nodewith.Name("Simplify").ClassName("LabelButton").Ancestor(mahiMenuView)
	// SummarizeButton is the button on the normal Mahi widget to request a summry.
	// It helps identify whether a normal widget or a compact one is shown.
	SummarizeButton = nodewith.Name("Summarize").ClassName("LabelButton").Ancestor(mahiMenuView)
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

// PrepareLocalServer unzips the local data zip and starts a local http server with the file path.
// TODO(b:358454189): try using zip package to simplify this.
func PrepareLocalServer(ctx context.Context, zipFilePath string) (string, []os.DirEntry, *httptest.Server, error) {
	localFilePath := path.Join(os.TempDir(), "mahi.local_files")
	if err := os.MkdirAll(localFilePath, 0755); err != nil {
		return "", nil, nil, errors.Wrap(err, "failed to make temp dir")
	}

	if err := testexec.CommandContext(ctx, "unzip", "-o", zipFilePath, "-d", localFilePath).Run(testexec.DumpLogOnError); err != nil {
		return "", nil, nil, errors.Wrap(err, "failed to unzip local files")
	}

	localFiles, err := os.ReadDir(localFilePath)
	if err != nil {
		return "", nil, nil, errors.Wrap(err, "failed to read file list from local file path")
	}

	localServer := httptest.NewServer(http.FileServer(http.Dir(localFilePath)))

	return localFilePath, localFiles, localServer, nil
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

		// Clicking mahiPanelView first to focus the panel can reduce the flakiness
		// compared to clicking the close button directly.
		if err := uiauto.Combine("Hide mahi panel",
			uiauto.IfSuccessThen(ui.Exists(mahiPanelView), ui.LeftClick(mahiPanelView)),
			uiauto.IfSuccessThen(ui.Exists(mahiCloseButton), ui.LeftClick(mahiCloseButton)),
			ui.WaitUntilGone(mahiCloseButton),
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
		if err := ui.WaitUntilAnyExists(SummarizeButton, compactSummaryButton)(ctx); err != nil {
			return errors.Wrap(err, "no consent flow nor summary button")
		}
	} else {
		if err := uiauto.Combine("Do consent flow",
			ui.Exists(consentTryItButton),
			ui.LeftClick(consentTryItButton),
			ui.WaitUntilExists(consentGotItButton),
			ui.LeftClick(consentGotItButton),
			ui.WaitUntilAnyExists(anySummaryText, mahiErrorStatus),
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
		return ui.WaitUntilAnyExists(SummarizeButton, compactSummaryButton)(ctx)
	}, &testing.PollOptions{
		Timeout:  5 * time.Second,
		Interval: time.Second,
	})
}

// DoSummary triggers summary and checks the result panel
func DoSummary(
	ctx context.Context,
	ui *uiauto.Context,
	expectMockResponse bool,
) error {
	expectAction := func() uiauto.Action {
		if expectMockResponse {
			return ui.WaitUntilExists(mockSummaryText)
		}
		return ui.WaitUntilAnyExists(anySummaryText, mahiErrorStatus)
	}

	return uiauto.Combine("Do summary and check the panel exists",
		uiauto.IfSucceedThenElse(ui.Exists(SummarizeButton), ui.LeftClick(SummarizeButton), ui.LeftClick(compactSummaryButton)),
		ui.WaitUntilExists(mahiCloseButton),
		expectAction(),
	)(ctx)
}

// MaybePassConsentFlowForGalleryPDF passes the one-off consent flow if the
// related elements exists on Gallery PDF surface.
func MaybePassConsentFlowForGalleryPDF(
	ctx context.Context,
	tconn *chrome.TestConn,
	window *ash.Window,
	ui *uiauto.Context,
	kb *input.KeyboardEventWriter) error {
	if err := mouse.Click(tconn, window.TargetBounds.CenterPoint(), mouse.RightButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to right click")
	}

	if err := ui.WaitUntilExists(consentTryItButton)(ctx); err != nil {
		if err := ui.WaitUntilExists(SummarizeButton)(ctx); err != nil {
			return errors.Wrap(err, "no consent flow nor summary button")
		}
	} else {
		if err := uiauto.Combine("Do consent flow",
			ui.Exists(consentTryItButton),
			ui.LeftClick(consentTryItButton),
			ui.WaitUntilExists(consentGotItButton),
			ui.LeftClick(consentGotItButton),
			ui.WaitUntilGone(consentGotItButton),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to pass the consent flow")
		}
	}

	return CleanUIElement(ctx, ui, kb)
}

// AskQuestionOnMahiPanel sends a question on the result panel
func AskQuestionOnMahiPanel(
	ctx context.Context,
	ui *uiauto.Context,
	kb *input.KeyboardEventWriter,
) error {
	return uiauto.Combine("Send a question on the result panel",
		ui.WaitUntilExists(questionTextInputOnPanel),
		ui.WaitUntilExists(questionSendButtonOnPanel),
		ui.EnsureFocused(questionTextInputOnPanel),
		kb.TypeAction(mockQuestionString),
		ui.LeftClick(questionSendButtonOnPanel),
		ui.WaitUntilExists(mockQuestion),
		ui.WaitUntilExists(mockAnswer),
	)(ctx)
}

// AskQuestionOnMahiWidget sends a question on the floating widget
func AskQuestionOnMahiWidget(
	ctx context.Context,
	ui *uiauto.Context,
	kb *input.KeyboardEventWriter,
) error {
	return uiauto.Combine("Send a question on the floating widget",
		ui.WaitUntilExists(questionTextInputOnWidget),
		ui.WaitUntilExists(questionSendButtonOnWidget),
		ui.EnsureFocused(questionTextInputOnWidget),
		kb.TypeAction(mockQuestionString),
		ui.LeftClick(questionSendButtonOnWidget),
		ui.WaitUntilGone(questionSendButtonOnWidget),
		ui.WaitUntilExists(mockQuestion),
		ui.WaitUntilExists(mockAnswer),
	)(ctx)
}

// ReadTextFile reads content from the given textFile as a string.
func ReadTextFile(textFile string) (string, error) {
	content, err := os.ReadFile(textFile)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read content from file %s", textFile)
	}
	return string(content), nil
}

// SelectContentAndRightClick selects the given content from the webview,
// righi-clicks it and checks expected_finder exists if not nil.
func SelectContentAndRightClick(
	ctx context.Context,
	ui *uiauto.Context,
	content string,
	expectedFinder *nodewith.Finder) error {
	// This assume the current browser tab is a plain text page with `content`.
	contentNode := nodewith.Role(role.StaticText).Ancestor(
		nodewith.Role(role.WebView)).First()
	if err := ui.WaitUntilExists(contentNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for content to load")
	}

	// Select the content and setup watcher to wait for text selection event.
	if err := ui.WaitForEvent(nodewith.Root(),
		event.DocumentSelectionChanged,
		ui.Select(
			contentNode, 0 /*startOffset*/, contentNode,
			utf8.RuneCountInString(content)-1 /*endOffset*/))(ctx); err != nil {
		return errors.Wrap(err, "failed to select query")
	}

	if err := uiauto.Combine("Right click selected text and do simplify",
		ui.RightClick(contentNode),
		ui.WaitUntilExists(contextMenu),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to right click and wait for the context menu")
	}

	if expectedFinder != nil {
		if err := ui.WaitUntilExists(expectedFinder)(ctx); err != nil {
			return errors.Wrapf(err, "failed to wait until expected finder: %v", expectedFinder.Pretty())
		}
	}

	return nil
}

// DoSimplify clicks the simplify button and checks the result panel exists.
func DoSimplify(
	ctx context.Context,
	ui *uiauto.Context,
	expectResponse bool,
) error {

	expectAction := func() uiauto.Action {
		if expectResponse {
			return ui.WaitUntilExists(simplifyIndicatorLabel)
		}
		return ui.WaitUntilAnyExists(simplifyIndicatorLabel, mahiErrorStatus)
	}

	return uiauto.Combine("Right click selected text and do simplify",
		ui.WaitUntilExists(SimplifyButton),
		ui.LeftClick(SimplifyButton),
		ui.WaitUntilExists(mahiCloseButton),
		expectAction(),
	)(ctx)

}
