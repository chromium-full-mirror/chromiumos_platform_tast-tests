// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package productivitycuj

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"chromiumos/tast/common/action"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/checked"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/prompts"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

const (
	// myFiles indicates the "My files" item name in the navigation bar.
	myFiles = "My files"
	// recent indicates the "Recent" item label in the navigation bar.
	recent = "Recent"

	// oneDriveTab indicates the suffix of the tab name.
	oneDriveTab = "OneDrive"
	// myFiles indicates the tab name of the "My files - OneDrive".
	myFilesTab = "My files - OneDrive"
	// recentTab indicates the tab name of the "Recent - OneDrive".
	recentTab = "Recent - OneDrive"
	// wordTab indicates the tab name of the "Microsoft Word".
	wordTab = "Microsoft Word Online"
	// powerpointTab indicates the tab name of the "Microsoft PowerPoint".
	powerpointTab = "Microsoft PowerPoint Online"
	// excelTab indicates the tab name of the "Microsoft Excel".
	excelTab = "Microsoft Excel Online"

	// word indicates the label of the new document.
	word = "Word document"
	// powerpoint indicates the label of the new presentation.
	powerpoint = "PowerPoint presentation"
	// excel indicates the label of the new spreadsheet.
	excel = "Excel workbook"

	// listView indicates the "List view" option in the "View option" menu.
	listView = "List view"
	// modified indicates the "Modified" option in the "Sort" menu.
	modified = "Modified"
	// descending indicates the "Descending" option in the "Sort" menu.
	descending = "Descending"
)

var (
	homeTabPanel   = nodewith.Name("Home").Role(role.TabPanel)
	myFilesWebArea = nodewith.Name(myFilesTab).Role(role.RootWebArea)
	wordWebArea    = nodewith.Name("Word").Role(role.RootWebArea)
	excelWebArea   = nodewith.Name("Excel").Role(role.RootWebArea)
	canvas         = nodewith.Role(role.Canvas).Ancestor(excelWebArea).First()
)

// MicrosoftWebOffice implements the ProductivityApp interface.
type MicrosoftWebOffice struct {
	br              *browser.Browser
	tconn           *chrome.TestConn
	ui              *uiauto.Context
	kb              *input.KeyboardEventWriter
	uiHdl           cuj.UIActionHandler
	browserType     browser.Type
	tabletMode      bool
	username        string
	password        string
	documentCreated bool
	slideCreated    bool
	sheetCreated    bool
}

// CreateDocument creates a new document from microsoft web app.
func (app *MicrosoftWebOffice) CreateDocument(ctx context.Context) error {
	conn, err := app.openOneDrive(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open OneDrive")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	paragraph := nodewith.Role(role.Paragraph).Ancestor(wordWebArea).Editable()
	if err := uiauto.Combine("open a new document",
		app.openBlankDocument(word),
		// Make sure paragraph exists before typing. This is especially necessary on low-end DUTs.
		app.ui.WithTimeout(longerUIWaitTime).WaitUntilExists(paragraph),
		app.kb.TypeAction(docText),
		app.maybeCloseOneDriveTab(myFilesTab),
	)(ctx); err != nil {
		return err
	}
	// Since the file will only be saved automatically after the file is edited, mark the file created successfully here.
	app.documentCreated = true
	return nil
}

// CreateSlides creates a new presentation from microsoft web app.
func (app *MicrosoftWebOffice) CreateSlides(ctx context.Context) error {
	conn, err := app.openOneDrive(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open OneDrive")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	title := nodewith.Name("Click to add title").First()
	subtitle := nodewith.Name("Click to add subtitle").Role(role.StaticText)
	if err := uiauto.Combine("create a new presentation",
		app.openBlankDocument(powerpoint),
		// Make sure title exists before typing. This is especially necessary on low-end DUTs.
		app.ui.WithTimeout(longerUIWaitTime).WaitUntilExists(title),
		app.uiHdl.Click(title),
		app.kb.TypeAction(titleText),
		app.uiHdl.Click(subtitle),
		app.kb.TypeAction(subtitleText),
		app.kb.AccelAction("Enter"),
		app.maybeCloseOneDriveTab(myFilesTab),
	)(ctx); err != nil {
		return err
	}
	// Since the file will only be saved automatically after the file is edited, mark the file created successfully here.
	app.slideCreated = true
	return nil
}

// CreateSpreadsheet creates a new spreadsheet from the microsoft web app.
// and copy the content from the public shared document to return the sheet name.
// Since MS Office documents cannot directly copy view-only documents,
// we can only copy the contents of worksheets from public shared documents.
func (app *MicrosoftWebOffice) CreateSpreadsheet(ctx context.Context, cr *chrome.Chrome, sampleSheetURL, outDir string) (fileName string, err error) {
	var bTconn *browser.TestConn
	var connExcel, connOneDrive *browser.Conn

	bTconn, err = app.br.TestAPIConn(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to create test API connection")
	}

	connExcel, err = app.br.NewConn(ctx, sampleSheetURL)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open URL: %s", sampleSheetURL)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, func() bool { return err != nil }, cr, "ui_tree")
		cuj.CloseAllTabs(ctx, bTconn, app.browserType)
		connExcel.Close()
	}(cleanupCtx)

	if err = webutil.WaitForQuiescence(ctx, connExcel, longerUIWaitTime); err != nil {
		return "", errors.Wrap(err, "failed to wait for sample sheet page to finish loading")
	}

	// 1. If the account is already logged in, the "We are updating our terms" dialog may pop up after navigating to the sample sheet.
	// 2. If the account is not logged in, a dialog box may pop up after logging in.
	// So check the dialog before and after login.
	if err := uiauto.Combine("wait for sample sheet content appears",
		app.skipUpdatingTermsDialog(),
		app.signIn,
		app.ui.WaitUntilExists(canvas),
	)(ctx); err != nil {
		return "", errors.Wrap(err, "failed to wait sample sheet content appears")
	}

	connOneDrive, err = app.openOneDrive(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to open OneDrive")
	}
	defer connOneDrive.Close()

	if err = uiauto.NamedCombine("remove if the same file name exists",
		app.clickNavigationItem(myFiles),
		app.switchToListView(),
		app.sortByModified(),
		app.removeSheet(sheetName),
		app.clickNavigationItem(myFiles),
	)(ctx); err != nil {
		return "", errors.Wrap(err, "failed to check if the file already exists")
	}

	waitForRangeSelected := func(ctx context.Context) error {
		// Two columns should be selected: one contains the numbers and the other contains the sum of the numbers.
		const expectedColumnSelectedNumber = 2
		// The class of the selected excel column changes from "ewrch-col-nosel" to "ewrch-col-cellsel".
		excelColumnSelected := nodewith.Role(role.GenericContainer).HasClass("ewrch-col-cellsel").Ancestor(excelWebArea)
		start := time.Now()
		return testing.Poll(ctx, func(ctx context.Context) error {
			excelColumnSelectedInfo, err := app.ui.NodesInfo(ctx, excelColumnSelected)
			if err != nil {
				return errors.Wrap(err, "failed to get nodes info")
			}
			columnSelectedNumber := len(excelColumnSelectedInfo)
			if columnSelectedNumber != expectedColumnSelectedNumber {
				return errors.Errorf("got wrong selected columns number: got %d, want %d", columnSelectedNumber, expectedColumnSelectedNumber)
			}
			testing.ContextLog(ctx, "Range selected in ", time.Since(start))
			return nil
		}, &testing.PollOptions{Timeout: defaultUIWaitTime})
	}

	waitForCopiedData := func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			data, err := getClipboardText(ctx, app.tconn)
			if err != nil {
				return err
			}
			lines := strings.Fields(data)
			if len(lines) != 100 && !strings.HasPrefix(data, "1") {
				return errors.New("incorrect copied content")
			}
			return nil
		}, &testing.PollOptions{Timeout: defaultUIWaitTime})
	}

	copyAll := uiauto.Combine("copy all",
		app.selectBox("A1"),
		app.kb.AccelAction("Ctrl+A"),
		waitForRangeSelected,
		app.kb.AccelAction("Ctrl+C"),
		waitForCopiedData,
	)

	copyFromExistingSheet := uiauto.NamedCombine("copy from existing spreadsheet",
		app.openBlankDocument(excel),
		app.uiHdl.SwitchToChromeTabByName(excelTab),
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(canvas),
		copyAll,
	)

	pasteIntoNewSheet := uiauto.NamedCombine("paste into newly created spreadsheet",
		app.uiHdl.SwitchToChromeTabByName("Book"),
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(canvas),
		app.selectBox("A1"),
		app.kb.AccelAction("Ctrl+V"),
		waitForRangeSelected,
		app.checkFormula("A1", "1"),
		app.selectBox("H1"),
		app.kb.TypeAction(sheetText),
		app.kb.AccelAction("Enter"),
	)

	if err = uiauto.Combine("create the example spreadsheet",
		copyFromExistingSheet,
		pasteIntoNewSheet,
		app.renameDocument(sheetName),
	)(ctx); err != nil {
		return "", err
	}

	// Since the file will only be saved automatically after the file is edited, mark the file created successfully here.
	app.sheetCreated = true
	return sheetName, nil
}

// skipUpdatingTermsDialog skips the "We are updating our terms" dialog.
// The dialog might pop up in the following situations:
// 1. After navigating to OneDrive or Microsoft Excel.
// 2. After the sign-in process, before the stay sign-in dialog pops up.
func (app *MicrosoftWebOffice) skipUpdatingTermsDialog() uiauto.Action {
	updatingRootWebArea := nodewith.Name("We're updating our terms").Role(role.RootWebArea).Focusable()
	skipTermsButton := nodewith.Name("Next").Role(role.Button).Ancestor(updatingRootWebArea)
	return uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(skipTermsButton), app.uiHdl.Click(skipTermsButton))
}

// OpenSpreadsheet opens an existing spreadsheet from microsoft web app.
func (app *MicrosoftWebOffice) OpenSpreadsheet(ctx context.Context, fileName string) (err error) {
	testing.ContextLog(ctx, "Opening an existing spreadsheet: ", fileName)
	conn, err := app.openOneDrive(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open OneDrive")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	return uiauto.NamedCombine("search the sample spreadsheet",
		app.searchSampleSheet,
		app.maybeCloseOneDriveTab(myFilesTab),
	)(ctx)
}

// MoveDataFromDocToSheet moves data from document to spreadsheet.
func (app *MicrosoftWebOffice) MoveDataFromDocToSheet(ctx context.Context) error {
	paragraph := nodewith.Role(role.GenericContainer).Ancestor(wordWebArea).HasClass("EditingSurfaceBody").Focusable()
	if err := uiauto.NamedCombine("switch to Microsoft Word cut selected text from the document",
		app.uiHdl.SwitchToChromeTabByName(wordTab),
		uiauto.IfFailThen(app.ui.DoDefault(paragraph), app.ui.DoDefault(paragraph.Editable())),
		app.kb.AccelAction("Ctrl+A"),
		app.kb.AccelAction("Ctrl+C"),
		uiauto.Sleep(dataWaitTime), // Given time to select all data.
	)(ctx); err != nil {
		return err
	}

	return uiauto.NamedCombine("switch to Microsoft Excel and paste the content into a cell of the spreadsheet",
		app.uiHdl.SwitchToChromeTabByName(excelTab),
		app.ui.WaitUntilExists(canvas),
		app.selectBox("H3"),
		app.kb.AccelAction("Ctrl+V"),
	)(ctx)
}

// MoveDataFromSheetToDoc moves data from spreadsheet to document.
func (app *MicrosoftWebOffice) MoveDataFromSheetToDoc(ctx context.Context) error {
	if err := uiauto.Combine("cut selected text from cell",
		app.selectBox("H1"),
		app.kb.AccelAction("Ctrl+C"),
	)(ctx); err != nil {
		return err
	}

	paragraph := nodewith.Role(role.Paragraph).Ancestor(wordWebArea).Editable()
	return uiauto.NamedCombine("switch to Microsoft Word and paste the content",
		app.uiHdl.SwitchToChromeTabByName(wordTab),
		app.ui.WaitUntilExists(paragraph),
		app.kb.AccelAction("Ctrl+V"),
	)(ctx)
}

// ScrollPage scrolls the document and spreadsheet.
func (app *MicrosoftWebOffice) ScrollPage(ctx context.Context) error {
	testing.ContextLog(ctx, "Scrolling the document and spreadsheet")
	for _, tabName := range []string{wordTab, excelTab} {
		if err := scrollTabPageByName(ctx, app.uiHdl, tabName); err != nil {
			return err
		}
	}
	return nil
}

// SwitchToOfflineMode switches to offline mode.
func (app *MicrosoftWebOffice) SwitchToOfflineMode(ctx context.Context) error {
	return nil
}

// UpdateCells updates one of the independent cells and propagate values to dependent cells.
func (app *MicrosoftWebOffice) UpdateCells(ctx context.Context) error {
	if err := app.editBoxValue(ctx, "A3", "100"); err != nil {
		return errors.Wrap(err, "failed to edit the value of the cell")
	}

	val, err := app.getBoxValue(ctx, "B1")
	if err != nil {
		return errors.Wrap(err, "failed to get the value of the cell")
	}

	sum, err := strconv.Atoi(val)
	if err != nil {
		return errors.Wrap(err, "failed to convert type to integer")
	}

	if expectedSum := calculateSum(3, 100); sum != expectedSum {
		return errors.Errorf("failed to validate the sum %d rows: got: %v; want: %v", rangeOfCells, sum, expectedSum)
	}
	return nil
}

// VoiceToTextTesting uses the "Dictation" function to achieve voice-to-text (VTT) and directly input text into office documents.
func (app *MicrosoftWebOffice) VoiceToTextTesting(ctx context.Context, expectedText string, playAudio action.Action) error {

	// allowPermission allows microphone permission if requested by the browser.
	allowPermission := prompts.ClearPotentialPrompts(app.tconn, defaultUIWaitTime, prompts.AllowAVPermissionPrompt)

	paragraph := nodewith.Role(role.Paragraph).HasClass("Paragraph").Ancestor(wordWebArea).First()
	// checkDictationResult checks if the document contains the expected dictation results.
	checkDictationResult := func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			if err := uiauto.Combine("copy the content of the document to the clipboard",
				app.closeHelpPanel,
				app.uiHdl.Click(paragraph),
				app.kb.AccelAction("Ctrl+A"),
				uiauto.Sleep(dataWaitTime), // Wait for all text to be selected.
				app.kb.AccelAction("Ctrl+C"),
				uiauto.Sleep(dataWaitTime), // Wait for all text to be copied.
			)(ctx); err != nil {
				return err
			}

			clipData, err := getClipboardText(ctx, app.tconn)
			if err != nil {
				return err
			}
			clipData = strings.ReplaceAll(clipData, ",", "")
			clipData = strings.ReplaceAll(clipData, ".", "")
			ignoreCaseData := strings.TrimSuffix(strings.ToLower(clipData), "\n")
			if !strings.Contains(ignoreCaseData, strings.ToLower(expectedText)) {
				return errors.Errorf("failed to validate input value ignoring case: got: %s; want: %s", clipData, expectedText)
			}
			return nil
		}, &testing.PollOptions{Interval: time.Second, Timeout: 15 * time.Second})
	}

	// dictate operates the dictation process.
	dictate := uiauto.Combine("play an audio file and check dictation results",
		uiauto.NamedAction("turn on the dictation", app.turnOnDictation),
		allowPermission,
		uiauto.NamedAction("check if dictation is on", app.checkDictation),
		uiauto.NamedAction("play the audio", playAudio),
		uiauto.NamedAction("check if the result is as expected", checkDictationResult),
	)

	dictationToolbar := nodewith.Name("Dictation toolbar").Role(role.Toolbar)
	stopDictationButton := nodewith.Name("Stop Dictation").Role(role.Button).Ancestor(dictationToolbar).First()
	return uiauto.NamedCombine("turn on the voice typing",
		app.uiHdl.SwitchToChromeTabByName(wordTab), // Switch to Microsoft Word.
		app.ui.WaitUntilExists(wordWebArea),
		app.ui.Retry(retryTimes, dictate),
		uiauto.IfSuccessThen(app.ui.Exists(stopDictationButton), app.uiHdl.Click(stopDictationButton)),
	)(ctx)
}

// Cleanup cleans up the resources used by running the Microsoft Web Office testing.
// It removes the document and slide which we created in the test case and close all tabs after completing the test.
// This function should be called as deferred function after the app is created.
func (app *MicrosoftWebOffice) Cleanup(ctx context.Context, sheetName string) error {
	testing.ContextLog(ctx, "Cleaning up the tabs and files")

	conn, err := app.openOneDrive(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if err := app.clickNavigationItem(myFiles)(ctx); err != nil {
		return err
	}

	// Because we did not rename the document and the slide, the file names will be named as the default values.
	// documentCreated and slideCreated are variables that indicate whether the file has been successfully created.
	filesRemovedMap := map[string]bool{
		"Document":     app.documentCreated,
		"Presentation": app.slideCreated,
		sheetName:      app.sheetCreated,
	}

	for file, isCreated := range filesRemovedMap {
		if isCreated {
			if err := app.removeDocument(file)(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to removing: ", file)
			}
		}
	}

	return nil
}

// SetBrowser sets browser to chrome or lacros.
func (app *MicrosoftWebOffice) SetBrowser(br *browser.Browser) {
	app.br = br
}

// removeSheet checks the existence of the sheet and remove it if it exists.
func (app *MicrosoftWebOffice) removeSheet(sheetName string) uiauto.Action {
	return func(ctx context.Context) error {
		sheetFileRow := nodewith.NameContaining(sheetName).Role(role.Row)
		nodes, err := app.ui.NodesInfo(ctx, sheetFileRow)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			testing.ContextLogf(ctx, "Removing %s: ", node.Name)
			if err := app.removeDocument(sheetName)(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

// maybeCloseOneDriveTab closes the specified tab if it exists.
// Sometimes the tab name is just "OneDrive". Therefore, if the specified tab cannot be found, try to search for it.
func (app *MicrosoftWebOffice) maybeCloseOneDriveTab(tabName string) action.Action {
	return func(ctx context.Context) error {
		// Get a TestConn to active browser.
		bTconn, err := app.br.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create test API connection")
		}
		tabs, err := browser.CurrentTabs(ctx, bTconn)
		if err != nil {
			return err
		}
		tabID := 0
		found := false
		for _, tab := range tabs {
			if tab.Title == tabName || tab.Title == oneDriveTab {
				tabID = tab.ID
				found = true
				break
			}
		}
		if !found {
			testing.ContextLogf(ctx, "Cannot find the tab name containing %q or %q", tabName, oneDriveTab)
			return nil
		}
		return browser.CloseTabsByID(ctx, bTconn, []int{tabID})
	}
}

// signIn signs in to Microsoft Office account.
func (app *MicrosoftWebOffice) signIn(ctx context.Context) error {
	// There are four different sign in scenarios:
	// 1. On the Excel page, only signInButton exists.
	// 2. If the website does not cache account information, both signInButton and signInLink will be displayed on the homepage.
	// 3. If the website does cache account information, only signInLink will be displayed on the homepage.
	// 4. The website redirect to the sign-in page when navigating to the spreadsheet.
	accountManager := nodewith.NameContaining("Account manager for").Role(role.Button)
	msLoginWebArea := nodewith.NameContaining("Login | Microsoft").Role(role.RootWebArea)
	signInLink := nodewith.Name("Sign in to your account").Role(role.Link).Ancestor(msLoginWebArea)
	signInButton := nodewith.Name("Sign in").Role(role.Button).Ancestor(excelWebArea)
	oneDriveWebArea := nodewith.Name("OneDrive").Role(role.RootWebArea)
	signInHeading := nodewith.Name("Sign in").Role(role.Heading).Ancestor(oneDriveWebArea)

	signInStateNode, err := app.ui.FindAnyExists(ctx, accountManager, signInLink, signInButton, signInHeading)
	if err != nil {
		return errors.Wrap(err, "failed to find sign in state node")
	}

	// The account manager indicates the user has been logged in. Skip the login procedure.
	if signInStateNode == accountManager {
		testing.ContextLog(ctx, "Account has been logged in")
		return nil
	}

	// Clicking at the signInStateNode to navigate to the sign-in page if the website is not redirected.
	if signInStateNode != signInHeading {
		securityHeading := nodewith.Name("Is your security info still accurate?").Role(role.Heading)
		looksGoodButton := nodewith.Name("Looks good!").Role(role.Button)
		if err := uiauto.NamedCombine("navigate to sign-in page",
			app.ui.DoDefaultUntil(
				signInStateNode,
				app.ui.WithTimeout(defaultUIWaitTime).WaitUntilGone(signInStateNode)),
			uiauto.IfSuccessThen(
				app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(securityHeading),
				app.uiHdl.Click(looksGoodButton),
			),
		)(ctx); err != nil {
			return err
		}
	}

	accountField := nodewith.NameContaining("Enter your email").Role(role.TextField)
	enterAccount := uiauto.NamedCombine("enter the account",
		app.ui.DoDefaultUntil(accountField, app.ui.Exists(accountField.Focused())),
		app.kb.AccelAction("Ctrl+A"),
		app.kb.TypeAction(app.username),
		app.kb.AccelAction("Enter"),
		app.ui.WaitUntilGone(accountField),
	)

	passwordField := nodewith.Name("Enter the password for " + app.username).Role(role.TextField)
	enterPassword := uiauto.NamedCombine("enter the password",
		app.ui.DoDefaultUntil(passwordField, app.ui.Exists(passwordField.Focused())),
		app.kb.AccelAction("Ctrl+A"), // Prevent the field from already being populated.
		app.kb.TypeAction(app.password),
		app.kb.AccelAction("Enter"),
		app.ui.WaitUntilGone(passwordField),
	)

	accountList := nodewith.Name("Pick an account").Role(role.List)
	accountButton := nodewith.NameContaining(app.username).Role(role.Button).Ancestor(accountList)
	accountLocked := nodewith.Name("Your account has been locked").Role(role.StaticText)
	accountNode, err := app.ui.FindAnyExists(ctx, accountField, accountManager, accountButton, accountLocked)
	if err != nil {
		return errors.Wrap(err, "failed to find account node")
	}

	// After clicking the sign-in node, four situations might happen:
	// 1. No account is cached, and the account field appears.
	// 2. Multiple accounts are cached, and the "Pick an account" list appears.
	// 3. One account is cached, the account manager appears and the sign-in is skipped.
	// 4. The account is locked and required manually recover.
	var goThroughAccountPage uiauto.Action
	switch accountNode {
	case accountField:
		goThroughAccountPage = enterAccount
	case accountButton:
		goThroughAccountPage = app.uiHdl.Click(accountButton)
	case accountManager:
		return nil
	case accountLocked:
		return errors.New("failed to sign in to microsoft office, your account has been locked")
	}

	// Check and skip the dialog at the end of sign in action.
	savePasswordWindow := nodewith.Name("Save password?").Role(role.Window)
	closeSavePasswordWindow := nodewith.Name("Close").Role(role.Button).Ancestor(savePasswordWindow)
	msAccountWebArea := nodewith.Name("Microsoft account").Role(role.RootWebArea)
	staySignInHeading := nodewith.Name("Stay signed in?").Role(role.Heading).Ancestor(msAccountWebArea)
	staySignInYesButton := nodewith.Name("Yes").Role(role.Button).Ancestor(msAccountWebArea).Focusable()
	closeButton := nodewith.Name("Close first run experience").Role(role.Button)

	return uiauto.Combine("sign in and skip dialog",
		goThroughAccountPage,
		// Sometimes it will sign in directly without entering password.
		uiauto.IfSuccessThen(
			app.ui.WaitUntilExists(passwordField),
			// Sometimes entering the password fails on the first try. Retry to ensure correctly enter the password.
			uiauto.Retry(retryTimes, enterPassword)),
		uiauto.IfSuccessThen(
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(closeSavePasswordWindow),
			app.uiHdl.Click(closeSavePasswordWindow)),
		app.skipUpdatingTermsDialog(),
		uiauto.IfSuccessThen(
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(staySignInHeading),
			uiauto.NamedAction("click stay sign in", app.uiHdl.Click(staySignInYesButton))),
		uiauto.IfSuccessThen(
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(closeButton),
			app.uiHdl.Click(closeButton)),
		app.ui.WaitUntilExists(accountManager),
	)(ctx)
}

// reloadPage reloads the website if it down.
// There might be five situations and there is no guarantee that the page will be restored after one click.
// 1. "This page isn’t working" means that the Microsoft website returns an HTTP status code of 500, sometimes with a "Reload" button.
// 2. The heading "Something went wrong" pops up with a "Go to OneDrive" button.
// 3. The image "Something went wrong" pops up with a "Go to my OneDrive" button.
// 4. The heading "This item might not exist or is no longer available" pops up with a "Go to OneDrive" button.
// 5. The link "Microsoft OneDrive" pops up.
func (app *MicrosoftWebOffice) reloadPage(ctx context.Context) error {
	testing.ContextLog(ctx, "Checking if the website needs to be reloaded")

	reloadButton := nodewith.Name("Reload").Role(role.Button).ClassName("blue-button text-button")
	goToOneDrive := nodewith.Name("Go to OneDrive").Role(role.Button).First()
	goToMyOneDrive := nodewith.Name("Go to my OneDrive").Role(role.Button).First()
	oneDriveLink := nodewith.Name("Microsoft OneDrive").Role(role.Link).First()

	// dialogsInfo holds the information of dialogs that will encountered and needs to be handled during testing.
	// The order of slices starts with the most frequent occurrence.
	dialogsInfo := []dialogInfo{
		{
			name: "Reload",
			node: reloadButton,
		},
		{
			name: "Go to OneDrive",
			node: goToOneDrive,
		},
		{
			name: "Go to my OneDrive",
			node: goToMyOneDrive,
		},
		{
			name: "Microsoft OneDrive",
			node: oneDriveLink,
		},
	}

	for _, info := range dialogsInfo {
		name, node := info.name, info.node

		testing.ContextLogf(ctx, "Checking if the %q node exists", name)
		if err := app.ui.WaitUntilExists(node)(ctx); err != nil {
			continue
		}

		return app.ui.Retry(retryTimes, func(ctx context.Context) error {
			if err := app.uiHdl.ClickUntil(node, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilGone(node))(ctx); err != nil {
				return err
			}
			// Sometimes it just disappears for a while and then reappears.
			// Make sure that the node does not appear at all.
			return app.ui.EnsureGoneFor(node, 15*time.Second)(ctx)
		})(ctx)
	}

	return nil
}

// reload reloads the page if the display is different from what we expected.
// If the tab navigates to the "My Files" page after reloading, then we need to re-operate the operation.
// After clicking the "Go to OneDrive" or "Go to My OneDrive" button, it will create another new tab called "My files - OneDrive".
// Therefore, it needs to be closed after re-operation. Otherwise, the number of current tabs will be affected and subsequent operations will fail.
func (app *MicrosoftWebOffice) reload(finder *nodewith.Finder, action action.Action) action.Action {
	return func(ctx context.Context) error {
		if err := app.ui.WithTimeout(longerUIWaitTime).WaitUntilExists(finder)(ctx); err != nil {
			return uiauto.Combine("reload and reoperate the action",
				app.reloadPage,
				uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(myFilesWebArea), action),
				app.maybeCloseOneDriveTab(myFilesTab),
			)(ctx)
		}
		return nil
	}
}

// openOneDrive navigates to OneDrive web page from Microsoft Office Home.
func (app *MicrosoftWebOffice) openOneDrive(ctx context.Context) (*chrome.Conn, error) {
	conn, err := app.br.NewConn(ctx, cuj.Microsoft365URL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open URL: %s", cuj.Microsoft365URL)
	}
	if err := webutil.WaitForQuiescence(ctx, conn, longerUIWaitTime); err != nil {
		return nil, errors.Wrap(err, "failed to wait for microsoft page to finish loading")
	}

	appLauncher := nodewith.Name("App launcher").Role(role.Button).Collapsed()
	appLauncherOpened := nodewith.Name("App launcher opened").Role(role.GenericContainer)
	closeAppLauncher := nodewith.Name("Close the app launcher").Role(role.Button).Ancestor(appLauncherOpened)
	oneDriveItem := nodewith.Name("OneDrive").Ancestor(appLauncherOpened).First()
	goToOffice := nodewith.Name("Go to Office").Role(role.Link)
	securityHeading := nodewith.Name("Is your security info still accurate?").Role(role.Heading)
	looksGoodButton := nodewith.Name("Looks good!").Role(role.Button)
	navigateToOneDrive := uiauto.Retry(retryTimes, func(ctx context.Context) (err error) {
		if err = uiauto.NamedCombine("navigate to OneDrive",
			uiauto.IfSuccessThen(app.ui.Exists(securityHeading), app.uiHdl.Click(looksGoodButton)),
			app.ui.DoDefault(appLauncher),
			app.ui.WaitUntilExists(closeAppLauncher),
			app.ui.DoDefault(oneDriveItem),
			app.skipUpdatingTermsDialog(),
		)(ctx); err == nil {
			return nil
		}
		return uiauto.Combine(`click the "Office" link`,
			app.uiHdl.Click(goToOffice),
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilGone(goToOffice),
		)(ctx)
	})

	myFiles := nodewith.Name("My files").Role(role.Heading).First()
	alertDialog := nodewith.Role(role.AlertDialog).First()
	closeDialog := nodewith.NameStartingWith("Close").Role(role.Button).Ancestor(alertDialog)
	noThanksButton := nodewith.Name("No, thanks").Role(role.Button).Ancestor(alertDialog)
	gotItButton := nodewith.Name("Got it").Role(role.Button)

	if err := uiauto.Combine("check if already signed in and navigate to OneDrive",
		app.signIn,
		navigateToOneDrive,
		app.reload(myFiles, func(ctx context.Context) error { return nil }),
		uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(closeDialog), app.uiHdl.Click(closeDialog)),
		uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(noThanksButton), app.uiHdl.Click(noThanksButton)),
		uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(gotItButton), app.uiHdl.Click(gotItButton)),
	)(ctx); err != nil {
		return nil, err
	}

	return conn, nil
}

// openNewFile opens a new document for the specified service.
func (app *MicrosoftWebOffice) openNewFile(service string) action.Action {
	newItem := nodewith.NameStartingWith("New").Role(role.MenuItem).Ancestor(myFilesWebArea)
	newMenu := nodewith.Name("New").Role(role.Menu).Ancestor(myFilesWebArea)
	serviceItem := nodewith.NameContaining(service).Role(role.MenuItem).Ancestor(newMenu)
	return uiauto.NamedCombine("open a new "+service,
		// Make sure "New" exists before creating a new file. This is especially necessary on low-end DUTs.
		app.ui.WithTimeout(longerUIWaitTime).WaitUntilExists(newItem),
		app.uiHdl.ClickUntil(newItem, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(newMenu)),
		app.uiHdl.ClickUntil(serviceItem, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilGone(myFilesWebArea)),
	)
}

// openBlankDocument opens a blank document with specified service.
// When we try to open a blank document on the corresponding service page, it will jump to the Microsoft Office App to request permission.
// Therefore, we try to open a blank document from OneDrive to avoid this situation.
func (app *MicrosoftWebOffice) openBlankDocument(service string) action.Action {
	return func(ctx context.Context) error {
		noThanksButton := nodewith.Name("No thanks").Role(role.Button).Focusable()
		closeDialogButton := nodewith.Name("Close dialog").Role(role.Button).Focusable()
		if err := uiauto.Combine("close dialogs",
			// Skip an alert dialog "Get the most out of your OneDrive" when it pops up.
			uiauto.IfSuccessThen(
				app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(noThanksButton),
				app.uiHdl.Click(noThanksButton),
			),
			// Skip "Let's get you started" when the dialog pops up.
			uiauto.IfSuccessThen(
				app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(closeDialogButton),
				app.uiHdl.Click(closeDialogButton),
			),
		)(ctx); err != nil {
			return err
		}

		if err := app.openNewFile(service)(ctx); err != nil {
			return err
		}

		paragraph := nodewith.Role(role.Paragraph).Editable()
		title := nodewith.Name("Click to add title").First()

		// element defines the node to specify whether it navigates to the corresponding service page correctly.
		element := map[string]*nodewith.Finder{
			word:       paragraph,
			powerpoint: title,
			excel:      canvas,
		}

		return app.reload(element[service], app.openNewFile(service))(ctx)
	}
}

// clickNavigationItem clicks the specified item in the navigation bar.
func (app *MicrosoftWebOffice) clickNavigationItem(itemName string) action.Action {
	return func(ctx context.Context) error {
		window := nodewith.NameContaining(myFilesTab).Role(role.Window).First()
		maximizeButton := nodewith.Name("Maximize").Role(role.Button).HasClass("FrameCaptionButton").Ancestor(window)
		// Maximize the browser window so that the navigation bar appears on the screen.
		if err := uiauto.IfSuccessThen(app.ui.Exists(maximizeButton), app.uiHdl.Click(maximizeButton))(ctx); err != nil {
			return err
		}

		navigation := nodewith.Role(role.Navigation)
		navigationList := nodewith.Role(role.List).Ancestor(navigation)
		itemLink := nodewith.NameContaining(itemName).Role(role.Link).Ancestor(navigationList)

		if err := app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(itemLink)(ctx); err == nil {
			testing.ContextLogf(ctx, "Directly enter to %q from the navigation list", itemName)
			return app.ui.DoDefault(itemLink)(ctx)
		}

		menu := nodewith.Role(role.Menu).Ancestor(navigation)
		menuItem := nodewith.NameContaining(itemName).Role(role.MenuItem).Ancestor(menu).Visited()

		if err := app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(menuItem)(ctx); err == nil {
			testing.ContextLogf(ctx, "Directly enter to %q from the menu", itemName)
			return app.uiHdl.Click(menuItem)(ctx)
		}

		commandBar := nodewith.NameStartingWith("Command bar").Role(role.MenuBar).Horizontal()
		appMenu := nodewith.NameContaining("App menu").Role(role.MenuItem).Ancestor(commandBar)
		navigationMenu := nodewith.Role(role.Menu).Ancestor(navigation)
		itemLink = nodewith.NameContaining(itemName).Role(role.MenuItem).Ancestor(navigationMenu).Visited()

		return uiauto.NamedCombine(fmt.Sprintf("click on the %q item in the navigation bar", itemName),
			app.ui.DoDefaultUntil(appMenu, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(navigationMenu)),
			app.uiHdl.Click(itemLink),
		)(ctx)
	}
}

// switchToListView switches the view option to list view.
func (app *MicrosoftWebOffice) switchToListView() uiauto.Action {
	viewMenu := nodewith.NameRegex(regexp.MustCompile(`[Vv]iew options`)).Role(role.MenuItem)
	return uiauto.NamedAction("switch the view option to list view",
		app.selectMenuItem(viewMenu, listView),
	)
}

// sortByModified sorts by date modified in descending order.
func (app *MicrosoftWebOffice) sortByModified() uiauto.Action {
	sortMenu := nodewith.NameStartingWith("Sort").Role(role.MenuItem).First()
	return uiauto.NamedCombine("sort by date modified in descending order",
		app.selectMenuItem(sortMenu, modified),
		app.selectMenuItem(sortMenu, descending),
	)
}

// selectMenuItem selects specific item in the menu.
func (app *MicrosoftWebOffice) selectMenuItem(menu *nodewith.Finder, itemName string) uiauto.Action {
	details := nodewith.NameRegex(regexp.MustCompile(`([Dd]etails|Info).*`)).Role(role.MenuItem).First()
	menuExpanded := menu.Expanded()
	checkIfMenuExpanded := app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(menuExpanded)
	checkIfMenuCollapsed := app.ui.WithTimeout(defaultUIWaitTime).WaitUntilGone(menuExpanded)

	expandMenu := uiauto.Combine("expand menu",
		app.ui.WaitUntilExists(details),
		uiauto.IfSuccessThen(checkIfMenuCollapsed, app.ui.DoDefaultUntil(menu, checkIfMenuExpanded)),
	)

	checkMenuItem := func(ctx context.Context) error {
		menuItem := nodewith.Name(itemName).Role(role.MenuItemCheckBox).Ancestor(myFilesWebArea)
		if found, err := app.ui.IsNodeFound(ctx, menuItem); err != nil {
			return errors.Wrapf(err, "failed to check %v", menuItem)
		} else if !found {
			// Normally, the node would be the "menuItemCheckBox" role, but sometimes it would be "menuItemRadio".
			menuItem = menuItem.Role(role.MenuItemRadio)
			testing.ContextLog(ctx, "Changing the role to: ", role.MenuItemRadio)
		}
		menuItemChecked := menuItem.Attribute("checked", checked.True)
		waitMenuItemChecked := app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(menuItemChecked)
		return uiauto.IfFailThen(waitMenuItemChecked, app.ui.DoDefaultUntil(menuItem, checkIfMenuCollapsed))(ctx)
	}

	return uiauto.Combine("select the specific item in the menu",
		// Sometimes "Details" will be displayed later, causing the position of the menu button to change.
		expandMenu,
		checkMenuItem,
	)
}

// searchSampleSheet searches for the existence of the sample spreadsheet.
func (app *MicrosoftWebOffice) searchSampleSheet(ctx context.Context) error {

	// Check if the sample file exists via searching box.
	searchFromBox := func(ctx context.Context) error {
		goToOneDrive := nodewith.Name("Go to OneDrive").Role(role.Button).First()
		searchBox := nodewith.Name("Search box. Suggestions appear as you type.").Role(role.TextFieldWithComboBox)
		suggestedFiles := nodewith.Name("Suggested files").Role(role.Group)
		fileOption := fmt.Sprintf("Excel file result: .xlsx %v.xlsx ,", sheetName)
		fileResult := nodewith.Name(fileOption).Role(role.ListBoxOption).Ancestor(suggestedFiles).First()
		return uiauto.NamedCombine("search through the box",
			uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(goToOneDrive), app.uiHdl.Click(goToOneDrive)),
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(searchBox),
			app.uiHdl.Click(searchBox),
			app.kb.TypeAction(sheetName),
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(fileResult),
			app.uiHdl.ClickUntil(fileResult, app.ui.Gone(fileResult)),
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(canvas),
		)(ctx)
	}

	// Check if the sample file in the list of "My files".
	searchFromMyFiles := func(ctx context.Context) error {
		row := nodewith.NameContaining(sheetName).Role(role.Row).First()
		link := nodewith.NameContaining(sheetName).Role(role.Link).Ancestor(row)
		return uiauto.NamedCombine("search file from my files",
			app.clickNavigationItem(myFiles),
			app.switchToListView(),
			app.sortByModified(),
			app.ui.DoDefaultUntil(link, app.ui.Gone(link)),
			app.ui.WithTimeout(longerUIWaitTime).WaitUntilExists(canvas), // The excel page may takes long time to load.
			app.maybeCloseOneDriveTab(myFilesTab),
		)(ctx)
	}

	// If the spreadsheet doesn't appear in "My files", try searching from the search box.
	if err := searchFromMyFiles(ctx); err != nil {
		return searchFromBox(ctx)
	}

	return nil
}

// openFindAndSelect opens "Find & Select".
func (app *MicrosoftWebOffice) openFindAndSelect(ctx context.Context) error {
	findAndSelectButton := nodewith.Name("Find & Select").Role(role.PopUpButton)
	moreOptionsButton := nodewith.NameContaining("More Options").Role(role.PopUpButton).Ancestor(homeTabPanel)
	moreOptionsMenu := nodewith.NameContaining("More Options").Role(role.Menu).Ancestor(homeTabPanel)
	findAndSelectItem := nodewith.Name("Find & Select").Role(role.MenuItem).Ancestor(moreOptionsMenu)

	// There might be two situations.
	// 1. TabPanel shows "Find & Select" directly.
	// 2. First click on "More Options" or "Editing" then you can find "Find & Select".
	found, err := app.ui.IsNodeFound(ctx, findAndSelectButton)
	if err != nil {
		return err
	}
	if found {
		return uiauto.NamedAction("click Find & Select", app.uiHdl.Click(findAndSelectButton))(ctx)
	}

	return uiauto.NamedCombine("click more options",
		app.uiHdl.Click(moreOptionsButton),
		app.uiHdl.Click(findAndSelectItem),
	)(ctx)
}

// selectRangeWithNameBox selects the range by clicking on the "Name Box".
func (app *MicrosoftWebOffice) selectRangeWithNameBox() action.Action {
	toolBar := nodewith.Role(role.Toolbar).HasClass("ToolbarView")
	reloadButton := nodewith.Name("Reload").Role(role.Button).Ancestor(toolBar)
	// In the clamshell mode, the "Name Box" can be focused with just click.
	nameBox := nodewith.NameContaining("Name Box").Role(role.TextFieldWithComboBox).Editable()
	return uiauto.NamedCombine(`select range by focus on "Name Box"`,
		// On some DUTs' newly created spreadsheets, the "Name Box" would disappear. Reload to make it appear.
		uiauto.IfFailThen(
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(nameBox),
			app.uiHdl.ClickUntil(
				reloadButton,
				app.ui.WaitUntilExists(nameBox)),
		),
		app.ui.DoDefaultUntil(
			nameBox,
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(nameBox.Focused()),
		),
	)
}

// selectRangeWithGoTo selects the range by opening "Go to" box since the tapping response is different with clicking.
func (app *MicrosoftWebOffice) selectRangeWithGoTo(ctx context.Context) error {
	rangeText := nodewith.Name("Range:").Role(role.TextField).Editable()
	rangeTextFocused := rangeText.Focused()
	// Clicking web area before pressing shortcut to ensure not focusing on a text field.
	// After pressing Ctrl+G, two scenarios might happen:
	// 1. If focusing on a text field, the find box appears.
	// 2. Otherwise, the "Go To" box appears.
	openGoToWithShortcut := uiauto.NamedCombine(`open "Go To" with shortcut`,
		app.uiHdl.Click(excelWebArea),
		app.kb.AccelAction("Ctrl+G"),
	)
	// Sometimes key events are typed but the UI does not respond. Retry to ensure the dialog does appear.
	if err := app.ui.WithInterval(time.Second).RetryUntil(
		openGoToWithShortcut,
		app.ui.WithTimeout(3*time.Second).WaitUntilExists(rangeText),
	)(ctx); err != nil {
		testing.ContextLog(ctx, "Opening with panel due to ", err.Error())

		home := nodewith.Name("Home").Role(role.Tab)
		goToMenuItem := nodewith.Name("Go to").Role(role.MenuItem)
		goToDialog := nodewith.Name("Go to").Role(role.Dialog)
		okButton := nodewith.Name("OK").Role(role.Button).Ancestor(goToDialog)
		if err := uiauto.NamedCombine(`open "Go To" with panel`,
			app.uiHdl.ClickUntil(home, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(homeTabPanel)),
			app.openFindAndSelect,
			uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(goToMenuItem), app.uiHdl.Click(goToMenuItem)),
			uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(goToDialog), app.uiHdl.Click(okButton)),
		)(ctx); err != nil {
			return err
		}
	}

	return app.uiHdl.ClickUntil(rangeText, app.ui.Exists(rangeTextFocused))(ctx)
}

// selectBox selects the specified cell using the name box.
func (app *MicrosoftWebOffice) selectBox(box string) action.Action {
	navigateToBox := uiauto.Combine("navigate to box",
		app.kb.AccelAction("Ctrl+A"), // Make sure to clear the content and re-input.
		app.kb.TypeAction(box),
		app.kb.AccelAction("Enter"),
	)

	nameBoxFocused := nodewith.NameContaining("Name Box").Role(role.TextFieldWithComboBox).Focused()
	selectBoxWithNameBox := uiauto.Combine(fmt.Sprintf(`select box %q with "Name Box"`, box),
		app.selectRangeWithNameBox(),
		navigateToBox,
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilGone(nameBoxFocused),
	)

	goToDialog := nodewith.Name("Go to").Role(role.Dialog)
	selectBoxWithGoToBox := uiauto.Combine(fmt.Sprintf(`select box %q with "Go to" box`, box),
		app.selectRangeWithGoTo,
		navigateToBox,
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilGone(goToDialog),
	)
	return uiauto.IfFailThen(selectBoxWithNameBox, selectBoxWithGoToBox)
}

// getBoxValue gets the value of the specified box.
func (app *MicrosoftWebOffice) getBoxValue(ctx context.Context, box string) (clipData string, err error) {
	if err := app.selectBox(box)(ctx); err != nil {
		return "", err
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Due to the unstable network, there might be no data in the clipboard after the copy operation.
		// Therefore, we also need to retry the copy operation.
		if err := app.kb.AccelAction("Ctrl+C")(ctx); err != nil {
			return err
		}
		// Given time to copy data.
		testing.Sleep(ctx, time.Second)
		clipData, err = getClipboardText(ctx, app.tconn)
		if err != nil {
			return err
		}
		if clipData == "Retrieving data. Wait a few seconds and try to cut or copy again." {
			return errors.New("clipboard data is not yet ready")
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Minute}); err != nil {
		return "", err
	}

	testing.ContextLogf(ctx, "Getting box %q value: %s", box, clipData)

	return clipData, nil
}

// editBoxValue edits the cell to the specified value.
func (app *MicrosoftWebOffice) editBoxValue(ctx context.Context, box, value string) error {
	return uiauto.NamedCombine(fmt.Sprintf("write box %q value", box),
		app.selectBox(box),
		app.kb.TypeAction(value),
		app.kb.AccelAction("Enter"),
	)(ctx)
}

// checkFormula checks if the formula is correct.
func (app *MicrosoftWebOffice) checkFormula(box, value string) uiauto.Action {
	formulaBar := nodewith.Name("formula bar").Role(role.TextField).Editable()
	formulaBarText := nodewith.Name(value).Role(role.StaticText).FinalAncestor(formulaBar)
	return uiauto.NamedCombine("check formula",
		app.selectBox(box),
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(formulaBarText),
	)
}

// closeHelpPanel closes the "Help" panel if it exists.
func (app *MicrosoftWebOffice) closeHelpPanel(ctx context.Context) error {
	helpPanel := nodewith.Name("Help").Role(role.TabPanel)
	close := nodewith.Name("Close").Role(role.Button).Ancestor(helpPanel)
	return uiauto.IfSuccessThen(
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(helpPanel),
		app.uiHdl.Click(close),
	)(ctx)
}

// checkDictation checks whether the dictation is turned on by checking the "Start Dictation" button exists.
func (app *MicrosoftWebOffice) checkDictation(ctx context.Context) error {
	dictationToolbar := nodewith.Name("Dictation toolbar").Role(role.Toolbar)
	startDictationButton := nodewith.Name("Start Dictation").Role(role.Button).Ancestor(dictationToolbar)

	if err := app.ui.WaitUntilExists(dictationToolbar)(ctx); err != nil {
		return err
	}

	return uiauto.IfSuccessThen(
		app.ui.WaitUntilExists(startDictationButton),
		app.uiHdl.Click(startDictationButton),
	)(ctx)
}

// turnOnDictationFromMoreOptions turns on the dictation function through "More Options".
func (app *MicrosoftWebOffice) turnOnDictationFromMoreOptions(ctx context.Context) error {
	moreOptions := nodewith.Name("More Options").Role(role.PopUpButton).Ancestor(homeTabPanel).First()
	dictationButton := nodewith.Name("Dictate").Role(role.Button).Ancestor(moreOptions).First()
	dictationCheckBox := nodewith.Name("Dictate").Role(role.MenuItemCheckBox).First()
	dictationToolbar := nodewith.Name("Dictation toolbar").Role(role.Toolbar)
	return uiauto.NamedCombine(`turn on the dictation through "More Options"`,
		app.uiHdl.ClickUntil(moreOptions, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(dictationButton)),
		app.uiHdl.ClickUntil(dictationButton, app.ui.Gone(dictationButton)),
		uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(dictationCheckBox), app.uiHdl.Click(dictationCheckBox)),
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(dictationToolbar),
	)(ctx)
}

// turnOnDictationFromPanel turns on the dictation function via the button in the "Home" panel.
func (app *MicrosoftWebOffice) turnOnDictationFromPanel(ctx context.Context) error {
	dictationToggleButton := nodewith.Name("Dictate").Role(role.ToggleButton).Ancestor(homeTabPanel).First()
	dictationCheckBox := nodewith.Name("Dictate").Role(role.MenuItemCheckBox).First()
	dictationToolbar := nodewith.Name("Dictation toolbar").Role(role.Toolbar)
	return uiauto.NamedCombine("turn on the dictation through the panel",
		app.uiHdl.ClickUntil(dictationToggleButton, uiauto.Combine("check if the dictation works",
			uiauto.IfSuccessThen(app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(dictationCheckBox), app.uiHdl.Click(dictationCheckBox)),
			app.ui.WaitUntilExists(dictationToolbar)),
		),
	)(ctx)
}

// turnOnDictation turns on the dictation function.
func (app *MicrosoftWebOffice) turnOnDictation(ctx context.Context) error {
	testing.ContextLog(ctx, "Turning on dictation")

	openDocument := func(ctx context.Context) error {
		// Since we did not specify the name of the "Word" document, we need to extract the document name from the tab name.
		docTab := nodewith.NameRegex(regexp.MustCompile(".*.docx - Microsoft Word Online")).Role(role.Tab).First()
		node, err := app.ui.Info(ctx, docTab)
		if err != nil {
			return err
		}
		docsName := strings.Replace(node.Name, " - Microsoft Word Online", "", -1)
		testing.ContextLog(ctx, "Getting the document name from the tab: ", docsName)

		row := nodewith.NameContaining(docsName).Role(role.Row).First()
		link := nodewith.NameContaining(docsName).Role(role.Link).Ancestor(row)
		return uiauto.Combine("reopen the document",
			app.clickNavigationItem(myFiles),
			app.uiHdl.Click(link),
		)(ctx)
	}

	reoperate := func(ctx context.Context, turnOnAction action.Action) error {
		testing.ContextLog(ctx, "Reloading the page and reoperating the function to turn on the dictation function")

		reload := nodewith.Name("Reload").ClassName("ReloadButton")
		paragraph := nodewith.Role(role.Paragraph).Editable()
		if err := turnOnAction(ctx); err != nil {
			if err := uiauto.Combine("reload the page",
				app.uiHdl.Click(reload),
				// After reloading the webpage, it might encounter "This page isn’t working".
				app.reload(paragraph, openDocument),
				app.ui.WithTimeout(longerUIWaitTime).WaitUntilGone(homeTabPanel),
			)(ctx); err != nil {
				return err
			}
		}

		dictationToolbar := nodewith.Name("Dictation toolbar").Role(role.Toolbar)
		// Sometimes the "Dictation toolbar" will be displayed directly, so we don't need to click the "Dictation" button again.
		if err := app.ui.WaitUntilExists(dictationToolbar)(ctx); err != nil {
			if nodewith.IsNodeNotFoundErr(err) {
				return turnOnAction(ctx)
			}
			return err
		}

		return nil
	}

	dictateButton := nodewith.Name("Dictate").Role(role.ToggleButton).Ancestor(homeTabPanel).First()
	// If the "Dictation" button is not displayed on the screen, we need to find it through "More Options".
	// Otherwise, we can click it directly on the panel.
	if err := app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(dictateButton)(ctx); err != nil {
		if nodewith.IsNodeNotFoundErr(err) {
			if err := reoperate(ctx, app.turnOnDictationFromMoreOptions); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		if err := reoperate(ctx, app.turnOnDictationFromPanel); err != nil {
			return err
		}
	}

	featureBrokenContainer := nodewith.Name("We couldn't connect to the catalog server for this feature.").Role(role.GenericContainer)
	retryButton := nodewith.Name("RETRY").Role(role.Button).Ancestor(featureBrokenContainer)
	return uiauto.Combine("turn on the dictation",
		uiauto.IfSuccessThen(app.ui.Exists(featureBrokenContainer), app.uiHdl.Click(retryButton)),
		app.checkDictation,
	)(ctx)
}

// closeTab closes the tab with the title of the specified name.
func (app *MicrosoftWebOffice) closeTab(title string) action.Action {
	return func(ctx context.Context) error {
		matcher := func(t *target.Info) bool {
			return strings.Contains(t.Title, title) && t.Type == "page"
		}

		conn, err := app.br.NewConnForTarget(ctx, matcher)
		if err != nil {
			return err
		}
		conn.CloseTarget(ctx)
		conn.Close()

		return nil
	}
}

// renameDocument renames the document with the specified file name.
func (app *MicrosoftWebOffice) renameDocument(fileName string) uiauto.Action {
	renameButton := nodewith.NameContaining("Saved to OneDrive").Role(role.Button)
	fileNameTextField := nodewith.NameContaining("File Name").Role(role.TextField)
	renamedRootWebArea := nodewith.NameContaining(fileName).Role(role.RootWebArea)
	return app.ui.Retry(retryTimes, uiauto.NamedCombine("rename the document: "+fileName,
		uiauto.IfSuccessThen(app.ui.Gone(fileNameTextField),
			app.uiHdl.ClickUntil(renameButton, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(fileNameTextField))),
		app.uiHdl.ClickUntil(fileNameTextField, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(fileNameTextField.Focused())),
		app.kb.AccelAction("Ctrl+A"),
		app.kb.TypeAction(fileName),
		app.kb.AccelAction("Enter"),
		app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(renamedRootWebArea),
	))
}

// removeDocument removes the document with the specified file name.
func (app *MicrosoftWebOffice) removeDocument(fileName string) uiauto.Action {
	row := nodewith.NameContaining(fileName).Role(role.Row).First()
	checkBox := nodewith.Role(role.CheckBox).Ancestor(row)
	complementary := nodewith.Role(role.Complementary).First()
	commandBar := nodewith.NameContaining("Command bar").Role(role.MenuBar).Ancestor(complementary)
	deleteItem := nodewith.Name("Delete").Role(role.MenuItem).Ancestor(commandBar)
	deleteDialog := nodewith.Name("Delete?").Role(role.Dialog)
	deleteButton := nodewith.Name("Delete").Role(role.Button).Ancestor(deleteDialog)
	deleteAlert := nodewith.Role(role.Alert).Ancestor(myFilesWebArea)
	deleteAlertButton := nodewith.Name("Delete").Role(role.Button).Ancestor(deleteAlert)
	return uiauto.NamedCombine("remove the document: "+fileName,
		app.switchToListView(),
		app.ui.DoDefaultUntil(checkBox, app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(commandBar)),
		app.uiHdl.Click(deleteItem),
		uiauto.IfSuccessThen(
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(deleteDialog),
			app.uiHdl.Click(deleteButton),
		),
		// If the file is still open, the website will confirm the deletion again.
		uiauto.IfSuccessThen(
			app.ui.WithTimeout(defaultUIWaitTime).WaitUntilExists(deleteAlertButton),
			app.uiHdl.Click(deleteAlertButton),
		),
	)
}

// NewMicrosoftWebOffice creates MicrosoftWebOffice instance which implements ProductivityApp interface.
func NewMicrosoftWebOffice(tconn *chrome.TestConn, uiHdl cuj.UIActionHandler, kb *input.KeyboardEventWriter, browserType browser.Type, tabletMode bool, username, password string) *MicrosoftWebOffice {
	return &MicrosoftWebOffice{
		tconn:       tconn,
		ui:          uiauto.New(tconn),
		uiHdl:       uiHdl,
		kb:          kb,
		tabletMode:  tabletMode,
		browserType: browserType,
		username:    username,
		password:    password,
	}
}

var _ ProductivityApp = (*MicrosoftWebOffice)(nil)
