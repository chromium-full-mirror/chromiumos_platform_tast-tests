// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googledocs

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// slidesName represents the name of the Google Slides web area.
const slidesName = "Google Slides"

var (
	// SlidesWindow represents the window of the Google Slides.
	SlidesWindow = nodewith.NameContaining(slidesName).Role(role.Window).First()
	// SlidesWebArea represents the web area of the Google Slides.
	SlidesWebArea = nodewith.NameContaining(slidesName).Role(role.RootWebArea).First()
	navigation    = nodewith.Role(role.Navigation).Ancestor(SlidesWebArea)
)

// NewGoogleSlides returns an action that creates a new google slides from web.
func NewGoogleSlides(ctx context.Context, tconn *chrome.TestConn, uiHandler cuj.UIActionHandler, newWindow bool) error {
	testing.ContextLog(ctx, "Start to create google slide")
	conn, err := uiHandler.NewChromeTab(ctx, cuj.NewGoogleSlidesURL, newWindow)
	if err != nil {
		return errors.Wrap(err, "failed to open the google document")
	}
	defer conn.Close()
	if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
		return errors.Wrap(err, "failed to wait for page to finish loading")
	}

	ui := uiauto.New(tconn)
	return uiauto.Combine("confirm to enter Google Slides",
		ui.WithTimeout(longUITimeout).WaitUntilExists(navigation),
		clickGotItIfExists(ui),
	)(ctx)
}

// NewSlide returns an action that creates a new slide, edits its title and content.
func NewSlide(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, title, content, pageNumber string) action.Action {
	ui := uiauto.New(tconn)
	newSlide := nodewith.Name("New slide (Ctrl+M)").Role(role.Button).Ancestor(SlidesWebArea)
	titleNode := nodewith.Name("title").Role(role.StaticText).Ancestor(SlidesWebArea).First()
	pageNumberText := nodewith.Name(pageNumber).Role(role.StaticText).Ancestor(navigation)
	textNode := nodewith.Name("text").Role(role.StaticText).Ancestor(SlidesWebArea).First()

	return uiauto.NamedCombine(fmt.Sprintf("create a new slide with page number %s and edit its content", pageNumber),
		ui.WaitUntilExists(newSlide),
		ui.WithTimeout(longUITimeout).DoDefaultUntil(newSlide,
			ui.WithTimeout(25*time.Second).WaitUntilExists(pageNumberText)),
		ui.DoubleClick(titleNode),
		uiauto.Sleep(time.Second),
		kb.TypeAction(title),
		ui.DoubleClick(textNode),
		uiauto.Sleep(time.Second),
		kb.TypeAction(content),
		waitForSlideSaved(tconn),
	)
}

// RenameSlide returns an action that renames google slide.
func RenameSlide(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, title string) action.Action {
	ui := uiauto.New(tconn)
	renameTextbox := nodewith.Name("Rename").ClassName("docs-title-input").Ancestor(SlidesWebArea).Editable().Focusable()
	return uiauto.NamedAction("rename the slide",
		ui.Retry(5, uiauto.Combine("rename slide",
			ui.WaitUntilExists(SlidesWebArea),
			ShowTheSlideMenus(tconn),
			ui.LeftClickUntil(renameTextbox, ui.WithTimeout(5*time.Second).WaitUntilExists(renameTextbox.State("focused", true))),
			kb.AccelAction("Ctrl+A"),
			kb.TypeAction(title),
			waitForFieldTextToBe(tconn, renameTextbox, title),
			kb.AccelAction("Enter"),
			waitForSlideSaved(tconn),
		)),
	)
}

// PresentSlide returns an action that presents google slide.
func PresentSlide(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, slideCount int) action.Action {
	ui := uiauto.New(tconn)
	presentationOptionsButton := nodewith.Name("Presentation options").First()
	// There are two versions of ui to present slide.
	presentFromBeginningButton := nodewith.NameRegex(regexp.MustCompile("(Present|Start) from beginning.*")).Role(role.MenuItem).First()
	menuBar := nodewith.Name("Menu bar").Role(role.Banner).Ancestor(SlidesWebArea).First()
	return uiauto.NamedCombine("present slide",
		ShowTheSlideMenus(tconn),
		ui.WaitUntilExists(presentationOptionsButton),
		ui.DoDefaultUntil(presentationOptionsButton, ui.WithTimeout(5*time.Second).WaitUntilExists(presentFromBeginningButton)),
		ui.DoDefault(presentFromBeginningButton),
		ui.WithTimeout(40*time.Second).WaitUntilGone(presentationOptionsButton),
		func(ctx context.Context) error {
			testing.ContextLog(ctx, "Switch slides")
			for i := 0; i < slideCount; i++ {
				if err := uiauto.Combine("present Slide",
					kb.AccelAction("Enter"),   // Press enter to switch slide.
					uiauto.Sleep(time.Second), // Sleep to wait for slide switching.
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to switch slide")
				}
			}
			return nil
		},
		kb.AccelAction("Esc"), //Press Esc to leave presentation mode
		// Some of DUT models with poor performance need to wait a long time to leave presentation mode.
		ui.WithTimeout(longUITimeout).WaitUntilExists(menuBar),
	)
}

// EditSlideTitle returns an action that edits google slide title and subtitle.
func EditSlideTitle(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, title, subtitle string) action.Action {
	ui := uiauto.New(tconn)
	titleNode := nodewith.Name("title").First()
	subtitleNode := nodewith.Name("subtitle").First()
	return uiauto.NamedCombine("edit slide title and subtitle",
		ui.WaitUntilExists(titleNode),
		ui.DoubleClick(titleNode),
		uiauto.Sleep(time.Second),
		kb.TypeAction(title),
		ui.WaitUntilExists(subtitleNode),
		ui.DoubleClick(subtitleNode),
		uiauto.Sleep(time.Second),
		kb.TypeAction(subtitle),
		waitForSlideSaved(tconn),
	)
}

// EditSlide returns an action that edits google slide.
func EditSlide(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, text, expectedText string) action.Action {
	ui := uiauto.New(tconn)
	return uiauto.NamedCombine("edit slide",
		func(ctx context.Context) error {
			nodes := nodewith.Name(text)
			nodesInfo, err := ui.NodesInfo(ctx, nodes)
			if err != nil {
				return errors.Wrap(err, "failed to get nodes info")
			}
			return mouse.DoubleClick(tconn, nodesInfo[len(nodesInfo)-1].Location.CenterPoint(), 500*time.Millisecond)(ctx)
		},
		kb.TypeAction(expectedText),
		kb.AccelAction("Esc"),
		kb.AccelAction("Esc"),
		waitForSlideSaved(tconn),
	)
}

// DeleteSlide returns an action that deletes google slide.
func DeleteSlide(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	slideHomeWebArea := nodewith.Name(slidesName).Role(role.RootWebArea)
	application := nodewith.Role(role.Application).Ancestor(SlidesWebArea) // Google Slide application node.
	fileButton := nodewith.Name("File").Role(role.MenuItem).Ancestor(application)
	menu := nodewith.Role(role.Menu).Ancestor(application)
	moveToTrash := nodewith.NameContaining("Move to trash t").Role(role.MenuItem)
	goToSlidesHome := nodewith.Name("Go to Slides home screen").Role(role.Button)
	return uiauto.NamedCombine("delete slide",
		ShowTheSlideMenus(tconn),
		cuj.ExpandMenu(tconn, fileButton, menu, 470),
		ui.DoDefault(moveToTrash),
		ui.DoDefault(goToSlidesHome),
		// When leaving the edit slide, the popup "Leave site?" might appear.
		// Click the leave button if it exists.
		prompts.ClearPotentialPrompts(tconn, 5*time.Second, prompts.LeaveSitePrompt),
		ui.WithTimeout(longUITimeout).WaitUntilExists(slideHomeWebArea),
	)
}

// waitForSlideSaved waits for the slide document state to be saved.
func waitForSlideSaved(tconn *chrome.TestConn) action.Action {
	return waitForDocumentSaved(tconn, slidesName)
}

// ShowTheSlideMenus shows the hidden Slide menu.
func ShowTheSlideMenus(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	return uiauto.Combine("show the slide menus",
		clickGotItIfExists(ui),
		uiauto.IfFailThen(ui.Exists(SlidesWebArea),
			ui.DoDefaultUntil(SlidesWindow,
				ui.WithTimeout(5*time.Second).WaitUntilExists(SlidesWebArea),
			),
		),
		showTheMenus(ui),
	)
}

// ActivateTitleField makes slide title field editable.
func ActivateTitleField(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	webArea := nodewith.NameContaining("Slides").Role(role.RootWebArea)
	application := nodewith.Role(role.Application).Ancestor(webArea)
	titleNode := nodewith.Name("title").Role(role.StaticText).Ancestor(application)
	return ui.LeftClick(titleNode)
}

// ClickOnSlidesWebArea clicks on slide's web area.
func ClickOnSlidesWebArea(tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	return uiauto.Combine("click on slide web area",
		reloadSlidesIfLoadingIssueDialogAppears(ui),
		clickGotItIfExists(ui),
		ui.LeftClick(SlidesWebArea),
	)
}

// reloadSlidesIfLoadingIssueDialogAppears reloads the slide page if the
// "Loading issue" dialog appears.
func reloadSlidesIfLoadingIssueDialogAppears(ui *uiauto.Context) action.Action {
	return reloadIfLoadingIssueDialogAppears(ui, SlidesWebArea)
}
