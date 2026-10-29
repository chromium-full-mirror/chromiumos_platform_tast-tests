// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ms365"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/office"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/onedrive"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	outlookURL = "https://outlook.office.com/"
)

type outlookPWAParam struct {
	outlookAsPWA bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           OdfsOpenFromOutlook,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that a file is opened within the M365 PWA if it's opened from a mail in Outlook",
		BugComponent:   "b:1401215", // ChromeOS > Software > Commercial (Enterprise) > Identity > 3P IdP > Enterprise Clippy
		Timeout:        10 * time.Minute,
		Contacts: []string{
			"cros-commercial-clippy-eng@google.com",
			"lmasopust@google.com",
			"emaxx@chromium.org",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
		},
		Attr: []string{
			"group:hw_agnostic",
			"group:golden_tier_secondary",
		},
		VarDeps: []string{
			"onedrive.managedusernamemicrosoft",
			"onedrive.managedpassword",
		},
		Fixture: "onedriveManaged",
		Params: []testing.Param{{
			Val: outlookPWAParam{
				outlookAsPWA: false,
			},
		}, {
			Name: "pwa",
			Val: outlookPWAParam{
				outlookAsPWA: true,
			},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.WebAppInstallForceList{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOneDriveMount{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.MicrosoftOneDriveAccountRestrictions{}, pci.Served),
			pci.SearchFlag(&policy.MicrosoftOfficeCloudUpload{}, pci.Served),
		},
	})
}

var (
	errAttachmentReopenNeeded = errors.New("M365 sign-in landed on OneDrive home; reopen attachment")
	oneDriveHomeRegex         = regexp.MustCompile(`(?i)^(.*- )?OneDrive($| - .*)`)
)

// completeM365SignIn advances through Microsoft account chooser, credential,
// and post-password prompts until the M365 editor finishes loading fileName.
func completeM365SignIn(ui *uiauto.Context, tconn *chrome.TestConn, ms365App *ms365.Ms365, fileName string) uiauto.Action {
	fileStem := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	m365StemWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining(fileStem).First()
	signInButton := nodewith.Role(role.Button).Name("Sign in").Ancestor(m365StemWindow).First()

	editorWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining(fileName).First()
	editorContainer := nodewith.Attribute("htmlId", "EditorContainer").Ancestor(editorWindow).First()
	documentContents := nodewith.Role(role.GenericContainer).Name("Document Contents").Ancestor(editorWindow).First()
	ribbon := nodewith.Role(role.Toolbar).Name("Ribbon").Ancestor(editorWindow).First()
	headerToolbar := nodewith.Role(role.Toolbar).Name("Header").Ancestor(editorWindow).First()
	wacPageImg := nodewith.Role(role.Image).HasClass("WACPageImg").Ancestor(editorWindow).First()
	oneDriveRoot := nodewith.Role(role.RootWebArea).NameRegex(oneDriveHomeRegex).First()
	oneDriveWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameRegex(oneDriveHomeRegex).First()
	oneDriveCloseButton := nodewith.Role(role.Button).Name("Close").HasClass("FrameCaptionButton").Ancestor(oneDriveWindow)

	msSignInWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(Sign in.*)|(Pick an account)")).First()
	accountTile := nodewith.Ancestor(msSignInWindow).Role(role.Button).NameContaining(ms365App.UserName).First()
	useAnotherAccountButton := nodewith.Ancestor(msSignInWindow).Role(role.Button).Name("Use another account").First()
	usernameInput := nodewith.Ancestor(msSignInWindow).Role(role.TextField).NameRegex(regexp.MustCompile(".*(email|someone@example.com).*"))
	visibleUsernameInput := usernameInput.Visible()

	msPasswordWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(Sign in.*)|(Enter your password)|(Get a code to sign in)|(Other ways to sign in)|(Choose a way to sign in)|(Verify your identity)")).First()
	passwordInput := nodewith.Ancestor(msPasswordWindow).Role(role.TextField).NameRegex(regexp.MustCompile(".*(p|P)assword.*"))
	visiblePasswordInput := passwordInput.Visible()
	otherWaysButton := nodewith.Ancestor(msPasswordWindow).Role(role.Button).NameRegex(regexp.MustCompile("(?i)Other ways to sign in.*")).First()
	useYourPasswordInsteadButton := nodewith.Ancestor(msPasswordWindow).NameRegex(regexp.MustCompile("(?i).*Use your password.*")).First()

	chromeSavePasswordWindow := nodewith.Role(role.Window).Name("Save password?")
	chromeNeverSaveButton := nodewith.Ancestor(chromeSavePasswordWindow).Role(role.Button).Name("Never")

	msUpdateTermsWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i).*updating our terms.*")).First()
	msUpdateTermsNextButton := nodewith.Ancestor(msUpdateTermsWindow).Role(role.Button).Name("Next").First()

	msConfirmSignInWindow := nodewith.Role(role.RootWebArea).Name("Is your security info still accurate?")
	msConfirmSignInButton := nodewith.Ancestor(msConfirmSignInWindow).Name("Looks good!").First()

	msStaySignedInWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(Microsoft account|Sign in to your( Microsoft)? account|" + regexp.QuoteMeta("Stay signed in?") + ")"))
	msStaySignedInYesButton := nodewith.Ancestor(msStaySignedInWindow).Role(role.Button).Name("Yes").First()
	msStaySignedInYesText := nodewith.Ancestor(msStaySignedInWindow).Role(role.StaticText).Name("Yes").First()

	msAcceptPermissionWindow := nodewith.Role(role.RootWebArea).NameContaining("Let this app access your info?")
	msAcceptPermissionButton := nodewith.Ancestor(msAcceptPermissionWindow).Role(role.Button).NameRegex(regexp.MustCompile("^(Accept|Yes)$")).First()

	finders := []*nodewith.Finder{
		signInButton,
		accountTile,
		useAnotherAccountButton,
		visibleUsernameInput,
		visiblePasswordInput,
		otherWaysButton,
		useYourPasswordInsteadButton,
		chromeNeverSaveButton,
		msUpdateTermsNextButton,
		msConfirmSignInButton,
		msStaySignedInYesButton,
		msStaySignedInYesText,
		msAcceptPermissionButton,
		editorContainer,
		documentContents,
		ribbon,
		headerToolbar,
		wacPageImg,
		oneDriveRoot,
	}

	return func(ctx context.Context) error {
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard")
		}
		defer kb.Close(ctx)

		ui60 := ui.WithTimeout(60 * time.Second)
		for step := 0; step < 10; step++ {
			found, err := ui60.FindAnyExists(ctx, finders...)
			if err != nil {
				return errors.Wrap(err, "failed waiting for Microsoft sign-in or M365 editor")
			}

			switch found {
			case editorContainer, documentContents, ribbon, headerToolbar, wacPageImg:
				if ui.Exists(signInButton)(ctx) != nil {
					return nil
				}
				found = signInButton
			case oneDriveRoot:
				testing.ContextLog(ctx, "M365 sign-in landed on OneDrive home; closing window to reopen attachment")
				if err := uiauto.New(tconn).WithTimeout(15*time.Second).LeftClickUntil(oneDriveCloseButton, ui.Gone(oneDriveWindow))(ctx); err != nil {
					return errors.Wrap(err, "failed to close OneDrive home window after sign-in")
				}
				if err := ui.WaitUntilGone(oneDriveRoot)(ctx); err != nil {
					return errors.Wrap(err, "failed waiting for OneDrive home root web area to close")
				}
				return errAttachmentReopenNeeded
			}

			if found == accountTile || (found == useAnotherAccountButton && ui.Exists(accountTile)(ctx) == nil) {
				testing.ContextLogf(ctx, "Selecting existing Microsoft account tile for %s in M365 PWA", ms365App.UserName)
				if err := ui.LeftClickUntil(accountTile, ui.Gone(accountTile))(ctx); err != nil {
					return errors.Wrap(err, "failed to select Microsoft account tile")
				}
				continue
			}

			switch found {
			case signInButton:
				if err := ui.LeftClickUntil(signInButton, ui.Gone(signInButton))(ctx); err != nil {
					return errors.Wrap(err, "failed to click Sign in button in M365 window")
				}
			case useAnotherAccountButton:
				if err := ui.LeftClickUntil(useAnotherAccountButton, ui.Exists(visibleUsernameInput))(ctx); err != nil {
					return errors.Wrap(err, "failed to click Use another account")
				}
			case visibleUsernameInput:
				if err := uiauto.Combine("Enter MS username in M365 PWA",
					ms365App.ChooseSearchEngine(),
					ui.WaitUntilExists(visibleUsernameInput),
					ui.LeftClickUntilFocused(usernameInput),
					kb.TypeAction(ms365App.UserName),
					kb.AccelAction("Enter"),
					ui.WaitUntilGone(visibleUsernameInput),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to enter Microsoft username")
				}
			case otherWaysButton:
				if err := ui.LeftClickUntil(otherWaysButton, ui.Exists(useYourPasswordInsteadButton))(ctx); err != nil {
					return errors.Wrap(err, "failed to click Other ways to sign in")
				}
			case useYourPasswordInsteadButton:
				if err := ui.LeftClickUntil(useYourPasswordInsteadButton, ui.Exists(visiblePasswordInput))(ctx); err != nil {
					return errors.Wrap(err, "failed to click Use your password")
				}
			case visiblePasswordInput:
				if err := uiauto.Combine("Enter MS password in M365 PWA",
					ui.WaitUntilExists(visiblePasswordInput),
					ui.LeftClickUntilFocused(passwordInput),
					kb.TypeAction(ms365App.Password),
					kb.AccelAction("Enter"),
					ui.WaitUntilGone(visiblePasswordInput),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to enter Microsoft password")
				}
			case chromeNeverSaveButton:
				if err := ui.LeftClickUntil(chromeNeverSaveButton, ui.Gone(chromeNeverSaveButton))(ctx); err != nil {
					return errors.Wrap(err, "failed to dismiss Save password prompt")
				}
			case msUpdateTermsNextButton:
				if err := uiauto.Combine("Accept MS updated terms",
					ui.ScrollToVisible(msUpdateTermsNextButton),
					ui.LeftClickUntil(msUpdateTermsNextButton, ui.Gone(msUpdateTermsNextButton)),
					ui.WaitUntilGone(msUpdateTermsWindow),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to accept updated Microsoft terms")
				}
			case msConfirmSignInButton:
				if err := uiauto.Combine("Confirm MS security info",
					ui.ScrollToVisible(msConfirmSignInButton),
					ui.LeftClickUntil(msConfirmSignInButton, ui.Gone(msConfirmSignInButton)),
					ui.WaitUntilGone(msConfirmSignInWindow),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to confirm Microsoft security info")
				}
			case msStaySignedInYesButton, msStaySignedInYesText:
				if err := uiauto.Combine("MS Stay Signed In",
					ui.ScrollToVisible(found),
					ui.LeftClickUntil(found, ui.Gone(found)),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to confirm Stay signed in prompt")
				}
			case msAcceptPermissionButton:
				if err := uiauto.Combine("MS permission screen",
					ui.ScrollToVisible(msAcceptPermissionButton),
					kb.TypeKeyAction(input.KEY_END),
					ui.LeftClickUntil(msAcceptPermissionButton, ui.Gone(msAcceptPermissionButton)),
				)(ctx); err != nil {
					return errors.Wrap(err, "failed to accept Microsoft permission prompt")
				}
			}
		}
		return errors.New("exceeded maximum sign-in steps in M365 window")
	}
}

// signinInsideM365 handles the situation where SharePoint or Office Online asks
// to sign in (for example via SharePoint's AccessDenied.aspx "Sign in" link or
// an Office Online "Sign in" button) before showing the file in the M365 PWA.
func signinInsideM365(ui *uiauto.Context, tconn *chrome.TestConn, ms365App *ms365.Ms365, fileName string) uiauto.Action {
	fileStem := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	m365StemWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining(fileStem).First()
	m365PWAToolbar := nodewith.Role(role.Pane).HasClass("WebAppFrameToolbarView").Ancestor(m365StemWindow)
	signInButton := nodewith.Role(role.Button).Name("Sign in").Ancestor(m365StemWindow).First()
	signInLink := nodewith.Role(role.Link).Name("Sign in").Ancestor(m365StemWindow).First()

	editorWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining(fileName).First()
	editorContainer := nodewith.Attribute("htmlId", "EditorContainer").Ancestor(editorWindow).First()
	documentContents := nodewith.Role(role.GenericContainer).Name("Document Contents").Ancestor(editorWindow).First()
	ribbon := nodewith.Role(role.Toolbar).Name("Ribbon").Ancestor(editorWindow).First()
	headerToolbar := nodewith.Role(role.Toolbar).Name("Header").Ancestor(editorWindow).First()
	wacPageImg := nodewith.Role(role.Image).HasClass("WACPageImg").Ancestor(editorWindow).First()

	msSignInWindow := nodewith.Role(role.RootWebArea).NameRegex(regexp.MustCompile("(?i)(Sign in.*)|(Pick an account)")).First()
	oneDriveRoot := nodewith.Role(role.RootWebArea).NameRegex(oneDriveHomeRegex).First()

	finders := []*nodewith.Finder{
		signInButton,
		signInLink,
		msSignInWindow,
		oneDriveRoot,
		editorContainer,
		documentContents,
		ribbon,
		headerToolbar,
		wacPageImg,
	}

	return func(ctx context.Context) error {
		found, err := ui.WithTimeout(60*time.Second).FindAnyExists(ctx, finders...)
		if err != nil {
			return errors.Wrap(err, "failed to find Sign in prompt or editor in M365 window")
		}

		switch found {
		case editorContainer, documentContents, ribbon, headerToolbar, wacPageImg:
			return nil
		case signInButton, signInLink:
			if err := ui.WaitUntilExists(m365PWAToolbar)(ctx); err != nil {
				return errors.Wrap(err, "failed to find M365 PWA toolbar before signing in")
			}
			if err := ui.LeftClickUntil(found, ui.Gone(found))(ctx); err != nil {
				return errors.Wrap(err, "failed to click Sign in in M365 window")
			}
		}

		return completeM365SignIn(ui, tconn, ms365App, fileName)(ctx)
	}
}

// closeAllOtherTabs closes all other tabs besides the Outlook mail tab and
// ChromeOS system web apps, including Outlook Safe Links interstitial tabs.
func closeAllOtherTabs(ctx context.Context, tconn *chrome.TestConn) error {
	allTabs, err := browser.AllTabs(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get all tabs")
	}

	var tabsToClose []int
	for _, tab := range allTabs {
		lowerURL := strings.ToLower(tab.URL)
		if strings.HasPrefix(lowerURL, "chrome://") {
			continue
		}
		if !strings.Contains(lowerURL, "safelink") &&
			(strings.Contains(lowerURL, "outlook.office.com") ||
				strings.Contains(lowerURL, "outlook.cloud.microsoft") ||
				strings.Contains(lowerURL, "outlook.live.com")) {
			continue
		}
		tabsToClose = append(tabsToClose, tab.ID)
	}
	if len(tabsToClose) == 0 {
		return nil
	}
	return browser.CloseTabsByID(ctx, tconn, tabsToClose)
}

// OdfsOpenFromOutlook verifies that links to Microsoft Office files which are
// opened within Outlook will not be opened within a Chrome tab but within a new
// instance of the M365 PWA. For this test to succeed, the test account needs to
// be manually prepared in a way that the Outlook inbox contains 2 mails. one
// with 3 links to office files (Word, Powerpoint, Excel) and one with
// 3 Office file attachments (Word, Powerpoint, Excel).
func OdfsOpenFromOutlook(ctx context.Context, s *testing.State) {
	data := s.FixtValue().(*onedrive.FixtureData)
	cr := data.Chrome
	tconn := data.TestAPIConn
	fdms := data.FakeDMS()

	var creds credconfig.Creds
	creds.User = s.RequiredVar("onedrive.managedusernamemicrosoft")
	creds.Pass = s.RequiredVar("onedrive.managedpassword")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	param := s.Param().(outlookPWAParam)
	webAppInstallForceListValue := []*policy.WebAppInstallForceListValue{
		{
			Url:                    "https://www.microsoft365.com/?from=Homescreen",
			DefaultLaunchContainer: "window",
			CreateDesktopShortcut:  true,
			CustomName:             "Microsoft 365",
			FallbackAppName:        "Microsoft 365",
			CustomIcon: &policy.WebAppInstallForceListValueCustomIcon{
				Hash: "",
				Url:  "",
			},
		},
	}
	if param.outlookAsPWA {
		webAppInstallForceListValue = append(webAppInstallForceListValue, &policy.WebAppInstallForceListValue{
			Url:                    "https://outlook.office.com/mail/",
			DefaultLaunchContainer: "window",
			CreateDesktopShortcut:  true,
			CustomName:             "",
			FallbackAppName:        "Outlook",
			CustomIcon: &policy.WebAppInstallForceListValueCustomIcon{
				Hash: "",
				Url:  "",
			},
		})
	}

	// Force-install the M365 PWA and set all Clippy policies appropriately.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{
		&policy.WebAppInstallForceList{
			Val: webAppInstallForceListValue,
		},
		&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
		&policy.MicrosoftOneDriveMount{Val: "automated"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"},
	}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}
	// Uninstall the force-installed PWA but keeping the ODFS extension installed by serving Clippy policies without the WebAppInstallForceList policy.
	defer policyutil.ServeAndRefresh(cleanupCtx, fdms, cr, []policy.Policy{&policy.MicrosoftOneDriveAccountRestrictions{Val: []string{"common"}},
		&policy.MicrosoftOneDriveMount{Val: "automated"},
		&policy.MicrosoftOfficeCloudUpload{Val: "allowed"}})

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch Files app: ", err)
	}
	defer files.Close(cleanupCtx)

	// Connect to OneDrive.
	ms365App, err := ms365.AppWithCreds(ctx, tconn, creds)
	if err != nil {
		s.Fatal("Failed to get instance of Ms365: ", err)
	}

	if err := office.ConnectToOneDrive(cr, tconn, files, ms365App)(ctx); err != nil {
		s.Fatal("Failed to connect to OneDrive: ", err)
	}

	if err := files.OpenOneDrive()(ctx); err != nil {
		s.Fatal("Failed to open OneDrive: ", err)
	}

	// Open Outlook.
	if param.outlookAsPWA {
		if err := apps.Launch(ctx, tconn, apps.Outlook.ID); err != nil {
			s.Fatal("Failed to launch Outlook: ", err)
		}
		defer apps.Close(ctx, tconn, apps.Outlook.ID)

	} else {
		conn, err := cr.NewConn(ctx, outlookURL)
		if err != nil {
			s.Fatal("Failed to open office website: ", err)
		}
		defer conn.Close()
		defer conn.CloseTarget(cleanupCtx)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_open_from_outlook")

	// Open the most recent Mail from the Inbox which should contain 3 links to Office files.
	outlookContext := nodewith.Role(role.RootWebArea).NameContaining("Outlook").First()
	messageList := nodewith.Role(role.ListBox).NameContaining("Message").Ancestor(outlookContext)
	linkEmail := nodewith.Role(role.ListBoxOption).NameContaining("Tast Test Outlook Links").Ancestor(messageList)
	linkEmailText := nodewith.Role(role.StaticText).Name("Tast Test Outlook Links:").Ancestor(outlookContext)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	if err := uiauto.Combine("Open the link email in Outlook",
		ui.WaitUntilExists(outlookContext),
		ui.WaitUntilExists(linkEmail),
		ui.DoDefault(linkEmail),
		ui.WaitUntilExists(linkEmailText),
	)(ctx); err != nil {
		s.Fatal("Failed to open the link email in Outlook: ", err)
	}

	// Open the links in the Mail and wait for M365 to show the file.
	fileNames := []string{"Tast_Outlook_Excel.xlsx", "Tast_Outlook_Powerpoint.pptx", "Tast_Outlook_Word.docx"}
	safelinkError := nodewith.Role(role.StaticText).NameContaining("We can't check the safety of this website").First()
	for _, fileName := range fileNames {
		fileStem := strings.TrimSuffix(fileName, filepath.Ext(fileName))
		emailLink := nodewith.Role(role.Link).Name(fileName).Ancestor(outlookContext)
		m365FileWindow := nodewith.Role(role.Window).HasClass("BrowserRootView").NameContaining(fileStem).First()
		m365FilePWAToolbar := nodewith.Role(role.Pane).HasClass("WebAppFrameToolbarView").Ancestor(m365FileWindow)

		interval := 1 * time.Second
		if err := action.Retry(3, func(ctx context.Context) error {
			// Opening the link with a left click to make sure that we test exactly the same behavior, a user would experience.
			if err := uiauto.Combine("Open the link in the mail",
				ui.WaitUntilExists(emailLink),
				ui.LeftClick(emailLink),
			)(ctx); err != nil {
				if closeErr := closeAllOtherTabs(ctx, tconn); closeErr != nil {
					s.Log("Failed to close all other tabs: ", closeErr)
				}
				return errors.Wrap(err, "failed to click the link in the mail")
			}

			found, err := ui.WithTimeout(30*time.Second).FindAnyExists(ctx, m365FilePWAToolbar, safelinkError)
			if err != nil || found == safelinkError {
				if err == nil {
					err = errors.New("Outlook Safe Links returned an error interstitial")
				}
				// Close all other tabs besides Outlook.
				if closeErr := closeAllOtherTabs(ctx, tconn); closeErr != nil {
					s.Log("Failed to close all other tabs: ", closeErr)
				}

				return errors.Wrap(err, "failed to open the link in the mail")
			}

			if err := uiauto.Combine("Sign in if needed and close M365 window",
				signinInsideM365(ui, tconn, ms365App, fileName),
				ms365App.WaitForMicrosoft365EditorWindowAndClose(tconn, fileName),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to close M365 window")
			}

			if !param.outlookAsPWA {
				if closeErr := closeAllOtherTabs(ctx, tconn); closeErr != nil {
					s.Log("Failed to close all other tabs: ", closeErr)
				}
			}

			return nil
		}, interval)(ctx); err != nil {
			s.Fatal("Failed to click the link after 3 retries: ", err)
		}
	}

	// Open another mail from the Inbox which should contain 3 attachments with Office files.
	attachmentEmail := nodewith.Role(role.ListBoxOption).NameContaining("Tast Test Outlook Files").Ancestor(messageList)
	attachmentEmailText := nodewith.Role(role.StaticText).Name("Tast Test Outlook Files:").Ancestor(outlookContext)

	if err := uiauto.Combine("Open the attachment email in Outlooks",
		ui.WaitUntilExists(outlookContext),
		ui.WaitUntilExists(attachmentEmail),
		ui.DoDefault(attachmentEmail),
		ui.WaitUntilExists(attachmentEmailText),
	)(ctx); err != nil {
		s.Fatal("Failed to open theattachment email in Outlook: ", err)
	}

	// Expand the attachments if they exist and are not expanded.
	attachmentExpandButton := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("Show all.*attachments")).State(state.Expanded, false)
	attachmentExpandButtonExpanded := nodewith.Role(role.Button).NameRegex(regexp.MustCompile("Show all.*attachments")).State(state.Expanded, true)

	if err := ui.Exists(attachmentExpandButton)(ctx); err == nil {
		if err := ui.DoDefaultUntil(attachmentExpandButton, ui.Exists(attachmentExpandButtonExpanded))(ctx); err != nil {
			s.Log("Failed to expand attachments: ", err)
		}
	}

	// Open the attachments in the Mail and wait for M365 to show the file.
	for _, fileName := range fileNames {
		attachmentBox := nodewith.Role(role.ListBox).Name("file attachments")
		attachment := nodewith.Role(role.ListBoxOption).NameContaining(fileName).Ancestor(attachmentBox)
		openInApplicationButton := nodewith.Role(role.StaticText).NameRegex(regexp.MustCompile("^Open in (Word|Excel|PowerPoint)$"))

		if err = uiauto.Combine("Open the attachment in the mail",
			ui.WaitUntilExists(attachment),
			ui.DoDefault(attachment),
			ui.WaitUntilExists(openInApplicationButton),
			ui.DoDefault(openInApplicationButton),
		)(ctx); err != nil {
			s.Fatal("Failed to open the attachment in the mail: ", err)
		}

		signInErr := signinInsideM365(ui, tconn, ms365App, fileName)(ctx)
		if errors.Is(signInErr, errAttachmentReopenNeeded) {
			if err := uiauto.Combine("Re-open the attachment after M365 sign-in",
				ui.WaitUntilExists(openInApplicationButton),
				ui.DoDefault(openInApplicationButton),
			)(ctx); err != nil {
				s.Fatal("Failed to re-open the attachment in the mail: ", err)
			}
			signInErr = signinInsideM365(ui, tconn, ms365App, fileName)(ctx)
		}
		if signInErr != nil {
			s.Fatal("Failed to sign in to M365 window: ", signInErr)
		}
		if err := ms365App.WaitForMicrosoft365EditorWindowAndClose(tconn, fileName)(ctx); err != nil {
			s.Fatal("Failed to close M365 window: ", err)
		}

		// Close the office dialog again.
		officeFileDialog := nodewith.Role(role.Dialog).Ancestor(outlookContext).First()
		closeDialogButton := nodewith.Role(role.Button).Name("Close").Focusable().Ancestor(officeFileDialog).First()
		closeDialogMenuItem := nodewith.Role(role.MenuItem).Name("Close").Focusable().Ancestor(outlookContext).First()
		if foundClose, err := ui.WithTimeout(10*time.Second).FindAnyExists(ctx, closeDialogButton, closeDialogMenuItem); err == nil {
			if err := ui.DoDefaultUntil(foundClose, ui.Gone(foundClose))(ctx); err != nil {
				s.Log("Failed to close the office file dialog in Outlook: ", err)
			}
		} else {
			// Don't crash since the dialog maybe doesn't exist anymore and we can try to proceed with the test.
			s.Log("Failed to close the office file dialog in Outlook: ", err)
		}
	}
}
