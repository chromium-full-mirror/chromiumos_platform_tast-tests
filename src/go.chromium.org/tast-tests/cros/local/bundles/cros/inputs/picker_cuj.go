// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package inputs will have tast tests for input-related features on Chromebooks.
package inputs

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/pre"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PickerCuj,
		Desc:         "Checks the CUJs of Picker",
		Contacts:     []string{"essential-inputs-gardener-oncall@google.com", "essential-inputs-team@google.com"},
		BugComponent: "b:95887",
		Attr: []string{
			"group:input-tools",
			"group:input-tools-upstream",
			"group:hw_agnostic",
			// Disabled by TORA.  See:b/341332532.
			// "group:mainline",
			// "informational"
		},
		SoftwareDeps: []string{
			"inputs_deps",
			"chrome",
			"chrome_internal",
			// Disabled by TORA.  See:b/341332532.
			// "gaia",
			"drivefs"},
		HardwareDeps: hwdep.D(hwdep.Model(pre.StableModels...)),
		SearchFlags:  util.IMESearchFlags([]ime.InputMethod{ime.DefaultInputMethod}),
		Timeout:      5 * time.Minute,
		Data:         []string{"capybara.jpg"},
		Params: []testing.Param{
			{
				Fixture: fixture.ClamshellNonVKWithPicker,
			},
		},
	})
}

var pickerFeatureTourContinueButtonFinder = nodewith.Name("Get started").Role(role.Button).Visible().Onscreen()
var pickerWindowFinder = nodewith.HasClass("Picker").Visible().Onscreen()
var pickerEmojiResultsFinder = nodewith.HasClass("PickerEmojiBarView").Visible().Onscreen()
var pickerMainResultsFinder = nodewith.HasClass("PickerSearchResultsView").Visible().Onscreen()
var pickerZeroStateResultsFinder = nodewith.HasClass("PickerZeroStateView").Visible().Onscreen()
var pickerSubmenuResultsFinder = nodewith.HasClass("PickerSubmenu").Visible().Onscreen()
var emojiPickerFinder = nodewith.HasClass("EmojiBubbleDialogView").Role(role.Window).Visible().Onscreen()

func pickerEmojiResultFinder(emoji, description string) *nodewith.Finder {
	// Emoji results are displayed as a button with a label as the only child.
	// The label name is the emoji itself.
	// The button name is the textual description of the emoji.
	return nodewith.Ancestor(nodewith.Name(description).Role(role.Button).Ancestor(pickerEmojiResultsFinder)).Name(emoji).Role(role.StaticText).Visible().First()
}

func pickerMainResultFinder(text string) *nodewith.Finder {
	return nodewith.Ancestor(pickerMainResultsFinder).NameContaining(text).Role(role.Button).Visible().First()
}

func pickerMainResultFinderRegexp(r *regexp.Regexp) *nodewith.Finder {
	return nodewith.Ancestor(pickerMainResultsFinder).NameRegex(r).Role(role.Button).Visible().First()
}

func pickerZeroStateResultFinder(text string) *nodewith.Finder {
	return nodewith.Ancestor(pickerZeroStateResultsFinder).NameContaining(text).Role(role.Button).Visible().First()
}

func pickerZeroStateResultWithSubmenuFinder(text string) *nodewith.Finder {
	return nodewith.Ancestor(pickerZeroStateResultsFinder).NameContaining(text).Role(role.PopUpButton).Visible().First()
}

func pickerSubmenuResultFinder(text string) *nodewith.Finder {
	return nodewith.Ancestor(pickerSubmenuResultsFinder).NameContaining(text).Role(role.Button).Visible().First()
}

func orcaButtonFinder(text string) *nodewith.Finder {
	return nodewith.Name(text).Role(role.Button).Ancestor(nodewith.Role(role.Window).ClassNameRegex(regexp.MustCompile("MakoConsentView|MakoRewriteView")))
}

func emojiPickerClearRecentsForCategory(ui *uiauto.Context, category string) uiauto.Action {
	return uiauto.Combine("click category, open 3 dot menu, clear recents, and wait for recents to disappear",
		ui.LeftClick(nodewith.Name(category).Role(role.ToggleButton).Ancestor(emojiPickerFinder)),
		ui.LeftClick(nodewith.Name("More options").Role(role.Button).Ancestor(emojiPickerFinder).Visible().Onscreen()),
		ui.LeftClick(nodewith.NameContaining("Clear recently used").Role(role.Button).Ancestor(emojiPickerFinder).Visible().Onscreen()),
		ui.WaitUntilGone(nodewith.Name("Recently used").Role(role.StaticText).Ancestor(emojiPickerFinder).Visible().Onscreen()),
	)
}

func scrollToThenClick(ui *uiauto.Context, finder *nodewith.Finder) uiauto.Action {
	return uiauto.Combine("scroll to then click",
		ui.ScrollToVisible(finder),
		ui.LeftClick(finder),
	)
}

// setUpHistoryData puts `url` into the browsing history.
func setUpHistoryData(browserUI *browser.Browser, url string) uiauto.Action {
	return func(ctx context.Context) error {
		// Visit a website to leave browsing history.
		if err := browserUI.Navigate(ctx, url); err != nil {
			return errors.Wrapf(err, "failed to navigate to %q", url)
		}
		// Navigate away so that this URL only appears as a browsing history result and not an open tab result.
		if err := browserUI.Navigate(ctx, "about:blank"); err != nil {
			return errors.Wrap(err, "failed to navigate to blank page")
		}
		return nil
	}
}

// setUpBookmarksData adds `url` as a bookmark.
func setUpBookmarksData(tconn *chrome.TestConn, cr *chrome.Chrome, url string) uiauto.Action {
	return func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 1*time.Second)
		defer cancel()

		// Navigate to the url in a new tab.
		browserUI, err := browser.Launch(ctx, tconn, cr, url)
		if err != nil {
			return errors.Wrap(err, "failed to launch browser")
		}
		defer browserUI.Close(cleanupCtx)

		ui := uiauto.New(tconn)

		// Bookmark the tab with the default title.
		if err := ui.LeftClick(nodewith.Role(role.Button).Name("Bookmark this tab"))(ctx); err != nil {
			return errors.Wrapf(err, "failed to add %q to bookmark", url)
		}

		// Clear the browser history so that this URL only appears as a bookmarks result and not history result.
		if err := tconn.Eval(ctx, `tast.promisify(chrome.browsingData.removeHistory({"since": 0}))`, nil); err != nil {
			return errors.Wrap(err, "failed to clear browsing history")
		}

		return nil
	}
}

// setUpOpenTabs opens `url` in a separate tab.
// It also clears all browsing history.
func setUpOpenTabs(browserUI *browser.Browser, tconn *chrome.TestConn, url string) uiauto.Action {
	return func(ctx context.Context) error {
		// Open a tab
		if err := browserUI.Navigate(ctx, url); err != nil {
			return errors.Wrapf(err, "failed to navigate to %q", url)
		}

		// Clear the browser history so that this URL only appears as an open tabs result and not history result.
		if err := tconn.Eval(ctx, `tast.promisify(chrome.browsingData.removeHistory({"since": 0}))`, nil); err != nil {
			return errors.Wrap(err, "failed to clear browsing history")
		}

		return nil
	}
}

// setUpClipboard sets the current clipboard text to `text`.
func setUpClipboard(tconn *chrome.TestConn, text string) uiauto.Action {
	return func(ctx context.Context) error {
		return ash.SetClipboard(ctx, tconn, text)
	}
}

// setUpDownloads adds a test file to the downloads folder.
func setUpDownloads(cr *chrome.Chrome, path string) uiauto.Action {
	return func(ctx context.Context) error {
		cryptohomeUserPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
		if err != nil {
			return errors.Wrapf(err, "failed to get the cryptohome user path for %s", cr.NormalizedUser())
		}

		expected, err := ioutil.ReadFile(path)
		if err != nil {
			return errors.Wrap(err, "could not read test file")
		}

		crosPath := filepath.Join(cryptohomeUserPath, "MyFiles", "Downloads", filepath.Base(path))
		if err = ioutil.WriteFile(crosPath, expected, 0666); err != nil {
			return errors.Wrap(err, "could not write test file")
		}
		return nil
	}
}

func getCurrentDateFromTrayAndThen(ui *uiauto.Context, fn func(time time.Time) uiauto.Action) uiauto.Action {
	return func(ctx context.Context) error {
		dateNode := nodewith.Role(role.Time).Ancestor(nodewith.ClassName("DateTray").Role(role.Button).Ancestor(nodewith.ClassName("StatusAreaWidget")))
		dateNodeInfo, err := ui.Info(ctx, dateNode)
		if err != nil {
			return errors.Wrap(err, "could not find date node in tray")
		}

		_, dateStr, _ := strings.Cut(dateNodeInfo.Name, ", ")
		date, err := time.Parse("Monday, January 2, 2006", dateStr)
		if err != nil {
			return errors.Wrapf(err, "could parse the date in tray: %s", dateStr)
		}

		if err := fn(date)(ctx); err != nil {
			return err
		}
		return nil
	}
}

// setUpOrca sets up Orca with a fake response.
func setUpOrca(tconn *chrome.TestConn, fakeResponse string) uiauto.Action {
	return func(ctx context.Context) error {
		var result bool
		if err := tconn.Call(ctx, &result, `tast.promisify(chrome.autotestPrivate.overrideOrcaResponseForTesting)`, struct {
			Responses []string `json:"responses"`
		}{Responses: []string{fakeResponse}}); err != nil {
			return errors.Wrap(err, "failed to override Orca responses")
		}
		return nil
	}
}

// setUpDoc creates a new Google doc with the specified title, closes it, and clears the browsing history.
// Returns the URL of the doc.
func setUpDoc(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, title string) (string, error) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	conn, err := cr.NewConn(ctx, "https://docs.new")
	if err != nil {
		return "", errors.Wrap(err, "failed to open browser")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	if googledocs.RenameDoc(tconn, keyboard, title)(ctx) != nil {
		return "", errors.Wrap(err, "failed to rename doc")
	}

	var docsHref string
	if err := conn.Eval(ctx, "window.location.href", &docsHref); err != nil {
		return "", errors.Wrap(err, "failed to get Docs URL")
	}

	// Launch Files App to wait for the file to appear in DriveFS.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return "", errors.Wrap(err, "failed launching Files app")
	}
	defer filesApp.Close(cleanupCtx)

	if err := uiauto.Combine("open files app and wait for file to appear",
		filesApp.OpenDrive(),
		filesApp.WithTimeout(30*time.Second).WaitForFile(title+".gdoc"),
	)(ctx); err != nil {
		return "", errors.Wrapf(err, "failed waiting for the test file %q to appear in Drive", title)
	}

	// Clear the browser history so that this URL only appears as a Drive result and not history result.
	if err := tconn.Eval(ctx, `tast.promisify(chrome.browsingData.removeHistory({"since": 0}))`, nil); err != nil {
		return docsHref, errors.Wrap(err, "failed to clear browsing history")
	}

	return docsHref, nil
}

func PickerCuj(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to access keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)

	// Create a browser for setting up browser data for the test, such as history and bookmarks.
	// This is kept in a separate tab from the main test server tab.
	browserUI, err := browser.Launch(ctx, tconn, cr, "about:blank")
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer browserUI.Close(cleanupCtx)

	// Create a new Google Doc so it can be searched later.
	var docURL string
	defer func(ctx context.Context) {
		if docURL != "" {
			if err := googledocs.DeleteDocWithURL(tconn, cr, docURL)(ctx); err != nil {
				s.Fatal(ctx, "Failed to delete doc: ", err)
			}
		}
	}(cleanupCtx)
	var docTitle = fmt.Sprintf("PickerCUJ %s", time.Now().Format(time.RFC822Z))
	docURL, err = setUpDoc(ctx, cr, tconn, keyboard, docTitle)
	if err != nil {
		s.Fatal("Failed to create Google Doc: ", err)
	}

	its, err := testserver.LaunchBrowser(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	defer its.CloseAll(cleanupCtx)

	ui := uiauto.New(tconn)

	plainTextField := testserver.TextAreaInputField
	richTextField := testserver.ContentEditableInputField

	togglePicker := keyboard.AccelAction("Search+F")

	subtests := []struct {
		name     string
		scenario string
		action   uiauto.Action
	}{
		{
			name:     "First-use feature tour dialog",
			scenario: "verify a feature tour dialog appears on first-use",
			action: uiauto.Combine("show picker by finishing feature tour",
				togglePicker,
				ui.LeftClick(pickerFeatureTourContinueButtonFinder),
				ui.WaitUntilExists(pickerWindowFinder),
				// Close the window by toggling Picker again
				togglePicker,
			),
		},
		{
			name:     "Search emoji and insert",
			scenario: "verify emoji search and insert CUJ",
			action: uiauto.Combine("search emoji and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("thumbs up"),
				scrollToThenClick(ui, pickerEmojiResultFinder("👍", "thumbs up emoji")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "👍"),
			),
		},
		{
			name:     "Search symbol and insert",
			scenario: "verify symbol search and insert CUJ",
			action: uiauto.Combine("search symbol and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("superset of"),
				scrollToThenClick(ui, pickerEmojiResultFinder("⊃", "superset of")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "⊃"),
			),
		},
		{
			name:     "Search emoticon and insert",
			scenario: "verify emoticon search and insert CUJ",
			action: uiauto.Combine("search emoticon and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("denko of disapproval"),
				scrollToThenClick(ui, pickerEmojiResultFinder("ಠωಠ", "denko of disapproval emoticon")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "ಠωಠ"),
			),
		},
		{
			name:     "Recently used emojis",
			scenario: "verify recently used emojis CUJ",
			action: uiauto.Combine("check recently used emojis appears on zero-state and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				ui.WaitUntilExists(pickerEmojiResultFinder("👍", "thumbs up emoji")),
				ui.WaitUntilExists(pickerEmojiResultFinder("⊃", "superset of")),
				ui.WaitUntilExists(pickerEmojiResultFinder("ಠωಠ", "denko of disapproval emoticon")),
				scrollToThenClick(ui, pickerEmojiResultFinder("👍", "thumbs up emoji")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "👍"),
			),
		},
		{
			name:     "Clear recently used emojis",
			scenario: "verify clearing recently used emojis CUJ",
			action: uiauto.Combine("check recently used emojis are cleared",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				keyboard.AccelAction("Search+Shift+Space"),
				ui.WaitUntilExists(emojiPickerFinder),
				emojiPickerClearRecentsForCategory(ui, "Emoji category"),
				emojiPickerClearRecentsForCategory(ui, "Symbol category"),
				emojiPickerClearRecentsForCategory(ui, "Emoticon category"),
				keyboard.AccelAction("Esc"),
				ui.WaitUntilGone(emojiPickerFinder),
				togglePicker,
				ui.WaitUntilExists(pickerEmojiResultsFinder),
				// The smiley emoji will appear as a default emoji when there's no recently used emoji.
				ui.WaitUntilExists(pickerEmojiResultFinder("🙂", "slightly smiling face emoji")),
				ui.Gone(pickerEmojiResultFinder("👍", "thumbs up emoji")),
				ui.Gone(pickerEmojiResultFinder("⊃", "superset of")),
				ui.Gone(pickerEmojiResultFinder("ಠωಠ", "denko of disapproval emoticon")),
				togglePicker,
				ui.WaitUntilGone(pickerWindowFinder),
			),
		},
		{
			name:     "Search browsing history and insert",
			scenario: "verify browsing history search and insert CUJ",
			action: uiauto.Combine("search browsing history and insert",
				setUpHistoryData(browserUI, "https://news.google.com/"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("Google News"),
				scrollToThenClick(ui, pickerMainResultFinder("Insert Google News")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "https://news.google.com/"),
			),
		},
		{
			name:     "Search bookmarks and insert",
			scenario: "verify bookmarks search and insert CUJ",
			action: uiauto.Combine("search bookmarks and insert",
				setUpBookmarksData(tconn, cr, "https://www.google.com/finance"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("finance"),
				scrollToThenClick(ui, pickerMainResultFinder("Insert Google Finance")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "https://www.google.com/finance/"),
			),
		},
		{
			name:     "Search open tabs and insert",
			scenario: "verify open tabs search and insert CUJ",
			action: uiauto.Combine("search open tabs and insert",
				setUpOpenTabs(browserUI, tconn, "https://books.google.com/"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("Google Books"),
				scrollToThenClick(ui, pickerMainResultFinder("Insert Google Books")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "https://books.google.com/"),
			),
		},
		{
			name:     "Select browsing history category and insert",
			scenario: "verify browsing history category suggestions, search, and insert CUJ",
			action: uiauto.Combine("select browsing history category and insert",
				setUpHistoryData(browserUI, "https://news.google.com/"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				scrollToThenClick(ui, pickerZeroStateResultFinder("Browsing history")),
				ui.WaitUntilExists(pickerMainResultFinder("Insert Google News")),
				keyboard.TypeAction("n"),
				scrollToThenClick(ui, pickerMainResultFinder("Insert Google News")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "https://news.google.com/"),
			),
		},
		{
			name:     "Search local images and insert",
			scenario: "verify local image search and insert CUJ",
			action: uiauto.Combine("create local image in downloads, search and insert",
				setUpDownloads(cr, s.DataPath("capybara.jpg")),
				its.ClearThenClickFieldAndWaitForActive(richTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("capybara"),
				scrollToThenClick(ui, pickerMainResultFinder("capybara.jpg")),
				ui.WaitUntilGone(pickerWindowFinder),
				ui.WaitUntilExists(nodewith.Ancestor(richTextField.Finder()).Role(role.Image)),
			),
		},
		{
			name:     "Select local images category and insert",
			scenario: "verify local image category suggestions, search, and insert CUJ",
			action: uiauto.Combine("create local image in downloads, select category and insert",
				setUpDownloads(cr, s.DataPath("capybara.jpg")),
				its.ClearThenClickFieldAndWaitForActive(richTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				scrollToThenClick(ui, pickerZeroStateResultFinder("Files")),
				ui.WaitUntilExists(pickerMainResultFinder("capybara.jpg")),
				keyboard.TypeAction("c"),
				scrollToThenClick(ui, pickerMainResultFinder("capybara.jpg")),
				ui.WaitUntilGone(pickerWindowFinder),
				ui.WaitUntilExists(nodewith.Ancestor(richTextField.Finder()).Role(role.Image)),
			),
		},
		{
			name:     "Search drive file and insert",
			scenario: "verify drive search and insert CUJ",
			action: uiauto.Combine("create new Drive file, search and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("pickercuj"),
				scrollToThenClick(ui, pickerMainResultFinder("PickerCUJ")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToSatisfy(tconn, plainTextField.Finder(), "contains correct doc ID", func(text string) bool {
					docURLParts := strings.Split(docURL, "/")
					docID := docURLParts[5]
					return strings.Contains(text, docID)
				}),
			),
		},
		{
			name:     "Search drive category and insert",
			scenario: "verify drive category suggestions, search and insert CUJ",
			action: uiauto.Combine("create new Drive file, select category and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				scrollToThenClick(ui, pickerZeroStateResultFinder("Google Drive")),
				ui.WaitUntilExists(pickerMainResultFinder("PickerCUJ")),
				keyboard.TypeAction("picker"),
				scrollToThenClick(ui, pickerMainResultFinder("PickerCUJ")),
				util.WaitForFieldTextToSatisfy(tconn, plainTextField.Finder(), "contains correct doc ID", func(text string) bool {
					docURLParts := strings.Split(docURL, "/")
					docID := docURLParts[5]
					return strings.Contains(text, docID)
				}),
			),
		},
		{
			name:     "Search clipboard and insert",
			scenario: "verify clipboard search and insert CUJ",
			action: uiauto.Combine("type a calculation and insert",
				setUpClipboard(tconn, "hello world"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("h"),
				scrollToThenClick(ui, pickerMainResultFinder("Insert hello world")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "hello world"),
			),
		},
		{
			name:     "Select clipboard category and insert",
			scenario: "verify clipboard category suggestions, search, and insert CUJ",
			action: uiauto.Combine("copy text, select category and insert",
				setUpClipboard(tconn, "hello world"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				scrollToThenClick(ui, pickerZeroStateResultFinder("Clipboard")),
				ui.WaitUntilExists(pickerMainResultFinder("hello world")),
				keyboard.TypeAction("h"),
				scrollToThenClick(ui, pickerMainResultFinder("hello world")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "hello world"),
			),
		},
		{
			name:     "Search maths and insert",
			scenario: "verify maths calculator and insert CUJ",
			action: uiauto.Combine("type a calculation and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("8/2*(2+2)"),
				scrollToThenClick(ui, pickerMainResultFinder("Insert 16")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "16"),
			),
		},
		{
			name:     "Search unit conversion and insert",
			scenario: "verify unit conversion and insert CUJ",
			action: uiauto.Combine("type a calculation and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("1 l to ml"),
				scrollToThenClick(ui, pickerMainResultFinder("Insert 1000 ml")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "1000 ml"),
			),
		},
		{
			name:     "Search date and insert",
			scenario: "verify date and insert CUJ",
			action: uiauto.Combine("type a date expression and insert",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("today"),
				getCurrentDateFromTrayAndThen(ui, func(time time.Time) uiauto.Action {
					// The date might've change if the test ran past midnight, so check both the current date and also the date of the next day.
					todayDate := time.Format("Jan 2")
					tomorrowDate := time.AddDate(0, 0, 1).Format("Jan 2")
					dateRegex := regexp.MustCompile(todayDate + "|" + tomorrowDate)
					return uiauto.Combine("select date result and verify",
						scrollToThenClick(ui, pickerMainResultFinderRegexp(dateRegex)),
						ui.WaitUntilGone(pickerWindowFinder),
						util.WaitForFieldTextToSatisfy(tconn, plainTextField.Finder(), "matches date", func(text string) bool {
							return dateRegex.MatchString(text)
						}),
					)
				}),
			),
		},
		{
			name:     "Insert zero-state suggestions",
			scenario: "verify zero-state suggestions and insert CUJ",
			action: uiauto.Combine("check zero-state suggestions appear and insert one of them",
				setUpHistoryData(browserUI, "https://scholar.google.com/"),
				setUpDownloads(cr, s.DataPath("capybara.jpg")),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				ui.WaitUntilExists(pickerZeroStateResultFinder("Insert Google Scholar")),
				ui.WaitUntilExists(pickerZeroStateResultFinder("Insert capybara.jpg")),
				scrollToThenClick(ui, pickerZeroStateResultFinder("Insert Google Scholar")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "https://scholar.google.com/"),
			),
		},
		{
			name:     "Use Orca for freeform write",
			scenario: "verify Orca freeform write and insert CUJ",
			action: uiauto.Combine("show Picker, type a sentence, trigger Orca, and insert",
				setUpOrca(tconn, "test poem"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("a poem about tests"),
				scrollToThenClick(ui, pickerMainResultFinder("Help me write")),
				ui.WaitUntilGone(pickerWindowFinder),
				ui.LeftClick(orcaButtonFinder("Got it")),
				ui.LeftClick(orcaButtonFinder("Insert")),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "test poem"),
			),
		},
		{
			name:     "Use Orca for freeform rewrite",
			scenario: "verify Orca freeform rewrite and insert CUJ",
			action: uiauto.Combine("select some text, show Picker, type a sentence, trigger Orca, and insert",
				setUpOrca(tconn, "hello world poem"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				keyboard.TypeAction("hello world"),
				ui.SelectText(nodewith.Role(role.InlineTextBox).Ancestor(plainTextField.Finder()), 0, 11),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				keyboard.TypeAction("make it a poem"),
				scrollToThenClick(ui, pickerMainResultFinder("Rewrite")),
				ui.WaitUntilGone(pickerWindowFinder),
				ui.LeftClick(orcaButtonFinder("Replace")),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "hello world poem"),
			),
		},
		{
			name:     "Use Orca for preset rewrite",
			scenario: "verify Orca preset rewrite and insert CUJ",
			action: uiauto.Combine("select some text, show Picker, select a Orca preset, and insert",
				setUpOrca(tconn, "hello world emoji"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				keyboard.TypeAction("hello world"),
				ui.SelectText(nodewith.Role(role.InlineTextBox).Ancestor(plainTextField.Finder()), 0, 11),
				togglePicker,
				ui.WaitUntilExists(pickerWindowFinder),
				scrollToThenClick(ui, pickerZeroStateResultWithSubmenuFinder("Change tone")),
				ui.WaitUntilExists(pickerSubmenuResultsFinder),
				scrollToThenClick(ui, pickerSubmenuResultFinder("Emojify")),
				ui.WaitUntilGone(pickerWindowFinder),
				ui.LeftClick(orcaButtonFinder("Replace")),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "hello world emoji"),
			),
		},
		{
			name:     "Lowercase transformation",
			scenario: "verify transforming to lowercase CUJ",
			action: uiauto.Combine("select the caps lock option",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				keyboard.TypeAction("hElLo WoRlD"),
				ui.SelectText(nodewith.Role(role.InlineTextBox).Ancestor(plainTextField.Finder()), 0, 11),
				togglePicker,
				scrollToThenClick(ui, pickerZeroStateResultWithSubmenuFinder("Change capitalization")),
				ui.WaitUntilExists(pickerSubmenuResultsFinder),
				ui.WaitUntilExists(pickerSubmenuResultFinder("lowercase")),
				ui.DoDefault(pickerSubmenuResultFinder("lowercase")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "hello world"),
			),
		},
		{
			name:     "Uppercase transformation",
			scenario: "verify transforming to uppercase CUJ",
			action: uiauto.Combine("select the caps lock option",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				keyboard.TypeAction("hElLo WoRlD"),
				ui.SelectText(nodewith.Role(role.InlineTextBox).Ancestor(plainTextField.Finder()), 0, 11),
				togglePicker,
				scrollToThenClick(ui, pickerZeroStateResultWithSubmenuFinder("Change capitalization")),
				ui.WaitUntilExists(pickerSubmenuResultsFinder),
				ui.WaitUntilExists(pickerSubmenuResultFinder("UPPERCASE")),
				ui.DoDefault(pickerSubmenuResultFinder("UPPERCASE")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "HELLO WORLD"),
			),
		},
		{
			name:     "Title Case transformation",
			scenario: "verify transforming to title case CUJ",
			action: uiauto.Combine("select the caps lock option",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				keyboard.TypeAction("hElLo WoRlD"),
				ui.SelectText(nodewith.Role(role.InlineTextBox).Ancestor(plainTextField.Finder()), 0, 11),
				togglePicker,
				scrollToThenClick(ui, pickerZeroStateResultWithSubmenuFinder("Change capitalization")),
				ui.WaitUntilExists(pickerSubmenuResultsFinder),
				ui.WaitUntilExists(pickerSubmenuResultFinder("Title Case")),
				ui.DoDefault(pickerSubmenuResultFinder("Title Case")),
				ui.WaitUntilGone(pickerWindowFinder),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "Hello World"),
			),
		},
		{
			name:     "Toggle caps lock",
			scenario: "verify toggling caps lock",
			action: uiauto.Combine("select the caps lock option",
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				scrollToThenClick(ui, pickerZeroStateResultFinder("Turn on Caps Lock")),
				// Typing 'a' should be in uppercase.
				keyboard.TypeAction("a"),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "A"),
				its.ClearThenClickFieldAndWaitForActive(plainTextField),
				togglePicker,
				// The caps lock result should also be searchable.
				keyboard.TypeAction("caps"),
				scrollToThenClick(ui, pickerMainResultFinder("Turn off Caps Lock")),
				// Typing 'a' should be in lowercase.
				keyboard.TypeAction("a"),
				util.WaitForFieldTextToBe(tconn, plainTextField.Finder(), "a"),
			),
		},
		{
			name:     "Open browsing history",
			scenario: "verify opening browsing history CUJ",
			action: uiauto.Combine("search and open browsing history",
				setUpHistoryData(browserUI, "https://maps.google.com/"),
				// Click a button on the test page to lose focus.
				its.ClickButton(testserver.MakeTextButton),
				togglePicker,
				keyboard.TypeAction("Google Maps"),
				scrollToThenClick(ui, pickerMainResultFinder("Open Google Maps")),
				ui.WaitUntilGone(pickerWindowFinder),
				ui.WaitUntilExists(nodewith.Name("Chrome - Google Maps").HasClass("BrowserFrame")),
				// Close the newly created tab.
				keyboard.AccelAction("Ctrl+w"),
			),
		},
		{
			name:     "Open new Google sheet",
			scenario: "verify opening new Google sheet CUJ",
			action: uiauto.Combine("open new Google sheet",
				// Click a button on the test page to lose focus.
				its.ClickButton(testserver.MakeTextButton),
				togglePicker,
				scrollToThenClick(ui, pickerZeroStateResultWithSubmenuFinder("New")),
				ui.WaitUntilExists(pickerSubmenuResultFinder("Google Sheet")),
				ui.DoDefault(pickerSubmenuResultFinder("Google Sheet")),
				ui.WaitUntilExists(nodewith.NameContaining("Chrome - Untitled spreadsheet").HasClass("BrowserFrame")),
				// Close the newly created tab.
				keyboard.AccelAction("Ctrl+w"),
			),
		},
	}

	for _, subtest := range subtests {
		s.Run(ctx, subtest.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+string(subtest.name))

			if err := uiauto.UserAction("Picker CUJ",
				subtest.action,
				uc, &useractions.UserActionCfg{
					Attributes: map[string]string{
						useractions.AttributeTestScenario: subtest.scenario,
					},
				})(ctx); err != nil {
				s.Fatal("Subtest failed: ", err)
			}
		})
	}
}
