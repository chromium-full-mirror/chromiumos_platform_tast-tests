// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apps provides general ChromeOS app utilities.
package apps

import (
	"context"
	"net/url"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosinfo"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// App is used to represent a ChromeOS app.
type App struct {
	// ID is the Chrome extension ID of the app.
	ID string
	// Name is the name of the app.
	Name string
}

// App IDs can be found at chrome://app-service-internals.

// Borealis App represents the installer/launcher for the borealis.
var Borealis = App{
	ID:   "dkecggknbdokeipkgnhifhiokailichf",
	Name: "Borealis",
}

// Chat App has details about the Google Chat app.
var Chat = App{
	ID:   "mhihbbhgcjldimhaopinoigbbglkihll",
	Name: "Google Chat",
}

// Chrome has details about the Chrome app.
var Chrome = App{
	ID:   "mgndgikekgjfcpckkfioiadnlibdjbkf",
	Name: "Chrome",
}

// Chromium has details about the Chromium app.
// It replaces Chrome on amd64-generic builds.
var Chromium = App{
	ID:   "mgndgikekgjfcpckkfioiadnlibdjbkf",
	Name: "Chromium",
}

// Camera has details about the Camera app.
var Camera = App{
	ID:   "njfbnohfdkmbmnjapinfcopialeghnmh",
	Name: "Camera",
}

// Canvas has details about the Chrome Canvas app.
var Canvas = App{
	ID:   "ieailfmhaghpphfffooibmlghaeopach",
	Name: "Chrome Canvas",
}

// Crosh has details about Crosh SWA.
var Crosh = App{
	ID:   "cgfnfgkafmcdkdgilmojlnaadileaach",
	Name: "Crosh",
}

// Cursive has details about the Cursive app.
var Cursive = App{
	ID:   "apignacaigpffemhdbhmnajajaccbckh",
	Name: "Cursive",
}

// ConnectivityDiagnostics has details about the Chrome Connectivity Diagnostics
// app.
var ConnectivityDiagnostics = App{
	ID:   "pinjbkpghjkgmlmfidajjdjocdpegjkg",
	Name: "Connectivity Diagnostics",
}

// Diagnostics has details about Diagnostics SWA.
var Diagnostics = App{
	ID:   "keejpcfcpecjhmepmpcfgjemkmlicpam",
	Name: "Diagnostics",
}

// Docs has details about the Google Docs app.
var Docs = App{
	ID:   "aohghmighlieiainnegkcijnfilokake",
	Name: "Docs",
}

// Drive has details about the Google Drive Web app.
var Drive = App{
	ID:   "aghbiahbpaijignceidepookljebhfak",
	Name: "Google Drive",
}

// Duo has details about the Duo app.
var Duo = App{
	ID:   "djkcbcmkefiiphjkonbeknmcgiheajce",
	Name: "Duo",
}

// Element has details about the Element app.
var Element = App{
	ID:   "ldkimphifgfeohfgodpbhdlcgegcllbp",
	Name: "Element",
}

// FamilyLink has details about the Family Link app.
var FamilyLink = App{
	ID:   "mljomdcpdfpfdplmgghfeoofmbbianlf",
	Name: "Family Link",
}

// Feedback has details about the Feedback app.
var Feedback = App{
	ID:   "iffgohomcomlpmkfikfffagkkoojjffm",
	Name: "Feedback",
}

// Files has details about the Files Chrome app.
var Files = App{
	ID:   "hhaomjibdihmijegdhdafkllkbggdgoj",
	Name: "Files",
}

// FilesSWA has details about the Files System Web App.
var FilesSWA = App{
	ID:   "fkiggjmkendpmbegkagpmagjepfkpmeb",
	Name: "Files",
}

// FirmwareUpdate has details about the FirmwareUpdate SWA.
var FirmwareUpdate = App{
	ID:   "nedcdcceagjbkiaecmdbpafcmlhkiifa",
	Name: "Firmware Updates",
}

// Gallery (aka Backlight) has details about the Gallery app.
var Gallery = App{
	ID:   "jhdjimmaggjajfjphpljagpgkidjilnj",
	Name: "Gallery",
}

// Gmail has details about the gmail app.
var Gmail = App{
	ID:   "hhkfkjpmacfncmbapfohfocpjpdnobjg",
	Name: "Gmail",
}

// Help (aka Explore) has details about the Help app.
var Help = App{
	ID:   "nbljnnecbjbmifnoehiemkgefbnpoeak",
	Name: "Explore",
}

// Lacros has details about the Lacros browser app.
var Lacros = App{
	ID:   "jaimifaeiicidiikhmjedcgdimealfbh",
	Name: "Chrome",
}

// Maps has details about Arc Maps app.
var Maps = App{
	ID:   "gmhipfhgnoelkiiofcnimehjnpaejiel",
	Name: "Maps",
}

// Meet has details about Google Meet PWA.
var Meet = App{
	ID:   "kjgfgldnnfoeklkmfkjfagphfepbbdan",
	Name: "Meet",
}

// Photos has details about the Photos app.
var Photos = App{
	ID:   "fdbkkojdbojonckghlanfaopfakedeca",
	Name: "Photos",
}

// PlayBooks has details about the Play Books app.
var PlayBooks = App{
	ID:   "cafegjnmmjpfibnlddppihpnkbkgicbg",
	Name: "Play Books",
}

// PlayGames has details about the Play Games app.
var PlayGames = App{
	ID:   "nplnnjkbeijcggmpdcecpabgbjgeiedc",
	Name: "Play Games",
}

// GoogleTV has details about the Google TV app.
var GoogleTV = App{
	ID:   "kadljooblnjdohjelobhphgeimdbcpbo",
	Name: "Google TV",
}

// PlayStore has details about the Play Store app.
var PlayStore = App{
	ID:   "cnbgggchhmkkdmeppjobngjoejnihlei",
	Name: "Play Store",
}

// Calculator has details about the Calculator app.
var Calculator = App{
	ID:   "oabkinaljpjeilageghcdlnekhphhphl",
	Name: "Calculator",
}

// Clock has details about the Clock app.
var Clock = App{
	ID:   "ddmmnabaeomoacfpfjgghfpocfolhjlg",
	Name: "Clock",
}

// Contacts has details about the Contacts app.
var Contacts = App{
	ID:   "kipfkokfekalckplgaikemhghlbkgpfl",
	Name: "Contacts",
}

// PrintManagement has details about the Print Management app.
var PrintManagement = App{
	ID:   "fglkccnmnaankjodgccmiodmlkpaiodc",
	Name: "Print jobs",
}

// Scan has details about the Scan SWA.
var Scan = App{
	ID:   "cdkahakpgkdaoffdmfgnhgomkelkocfo",
	Name: "Scan",
}

// AndroidSettings has details about ARC settings app.
var AndroidSettings = App{
	ID:   "mconboelelhjpkbdhhiijkgcimoangdj",
	Name: "Android Settings",
}

// Settings has details about the Settings app.
var Settings = App{
	ID:   "odknhmnlageboeamepcngndbggdpaobj",
	Name: "Settings",
}

// ShimlessRMA has details about the Shimless RMA app.
var ShimlessRMA = App{
	ID:   "ijolhdommgkkhpenofmpkkhlepahelcm",
	Name: "Shimless RMA",
}

// ShortcutCustomization has details about the Shortcut Customization app.
var ShortcutCustomization = App{
	ID:   "ihgeegogifolehadhdgelgcnbnmemikp",
	Name: "Key Shortcuts",
}

// TaskManager has details about the Task Manager app.
var TaskManager = App{
	ID:   "ijaigheoohcacdnplfbdimmcfldnnhdi",
	Name: "Task Manager",
}

// Translate has details about the Translate app.
var Translate = App{
	ID:   "pacmnfddiadhhfmngijgjdbnodjkmojl",
	Name: "Translate",
}

// TelemetryExtension has details about the TelemetryExtension app.
var TelemetryExtension = App{
	ID:   "lhoocnmbcmmbjgdeaallonfplogkcneb",
	Name: "Telemetry Extension",
}

// Terminal has details about the Crostini Terminal app.
var Terminal = App{
	ID:   "fhicihalidkgcimdmhpohldehjmcabcf",
	Name: "Terminal",
}

// WallpaperPicker has details about the Wallpaper Picker app.
var WallpaperPicker = App{
	ID:   "obklkkbkpaoaejdabbfldmcfplpdgolj",
	Name: "Wallpaper Picker",
}

// WebStore has details about the WebStore app.
var WebStore = App{
	ID:   "ahfgeienlihckogmohjhadlkjgocpleb",
	Name: "Web Store",
}

// Youtube has details about the Youtube app.
var Youtube = App{
	ID:   "aniolghapcdkoolpkffememnhpphmjkl",
	Name: "Youtube",
}

// YouTubeCWS has details about the YouTube app from Chrome Web Store.
var YouTubeCWS = App{
	ID:   "blpcfgokakmgnkcojhhkbfbldkacnbeo",
	Name: "YouTube",
}

// Parallels has details about the Parallels app.
var Parallels = App{
	ID:   "lgjpclljbbmphhnalkeplcmnjpfmmaek",
	Name: "Parallels Desktop",
}

// Citrix has details about Citrix Workspace app.
var Citrix = App{
	ID:   "haiffjcadagjlijoggckpgfnoeiflnem",
	Name: "Citrix Workspace",
}

// VMWare has details about VMware Horizon app.
var VMWare = App{
	ID:   "ppkfnjlimknmjoaemnpidmdlfchhehel",
	Name: "VMware Horizon",
}

// Projector has details about the Projector app.
// This id corresponds to the app hosted under
// chrome://projector main frame.
var Projector = App{
	ID:   "nblbgfbmjfjaeonhjnbbkabkdploocij",
	Name: "Screencast",
}

// ProjectorV2 has details about the Projector app
// hosted under the chrome-untrusted://projector
// main frame.
var ProjectorV2 = App{
	ID:   "hohmppfoilmflgicnofelkdablfahbnl",
	Name: "Screencast",
}

// KeyboardSV has details about the Keyboard Shortcut Viewer app.
var KeyboardSV = App{
	ID:   "bhbpmkoclkgbgaefijcdgkfjghcmiijm",
	Name: "Keyboard Shortcut Viewer",
}

// Viridi has details about the Viridi Steam game app.
var Viridi = App{
	ID:   "bipdjohdnebjjgpeankgpbmbcmjpfgpg",
	Name: "Viridi",
}

// Zoom has details about the Zoom meeting app.
var Zoom = App{
	ID:   "gbmplfifepjenigdepeahbecfkcalfhg",
	Name: "Zoom",
}

// Text has details about the Text app.
var Text = App{
	ID:   "mmfbcljfglbokpmkimbfghdkjmjhdgbg",
	Name: "Text",
}

// Asphalt8 has details about the Asphalt8 Game app.
var Asphalt8 = App{
	ID:   "jabjbdmildinpcomplomlhlocemeidbl",
	Name: "Asphalt 8",
}

// SuperTuxKart has details about the SuperTuxKart Game app.
var SuperTuxKart = App{
	ID:   "pcngdpbcmobdidkoebcgkdfnfiigcpmg",
	Name: "SuperTuxKart",
}

// Microsoft365 has details about the Office PWA app.
var Microsoft365 = App{
	ID:   "onhfoihkhodaeblmangmjjgfpfehnlkm",
	Name: "Microsoft 365",
}

// Launch launches an app specified by appID.
func Launch(ctx context.Context, tconn *chrome.TestConn, appID string) error {
	_, err := getInstalledAppID(ctx, tconn, func(app *ash.ChromeApp) bool { return app.AppID == appID }, nil)
	if err != nil {
		return err
	}
	return tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.launchApp)`, appID)
}

func getInstalledAppID(ctx context.Context, tconn *chrome.TestConn, predicate func(*ash.ChromeApp) bool, pollOpts *testing.PollOptions) (string, error) {
	appID := ""
	err := testing.Poll(ctx, func(ctx context.Context) error {
		capps, err := ash.ChromeApps(ctx, tconn)
		if err != nil {
			return testing.PollBreak(err)
		}
		for _, capp := range capps {
			if predicate(capp) {
				appID = capp.AppID
				return nil
			}
		}
		return errors.New("App not yet found in available Chrome apps - have you added --enable-features=<app> to chrome options?")
	}, pollOpts)
	return appID, err
}

// FindSystemWebAppByOrigin returns an `ash.ChromeApp` that is a system web app and matches `origin`.
// This won't match Terminal app, which is managed by Crostini and tested separately.
// Returns `nil` if the app isn't found (i.e. installed).
func FindSystemWebAppByOrigin(ctx context.Context, tconn *chrome.TestConn, origin string) (*ash.ChromeApp, error) {
	installedApps, err := ash.ChromeApps(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get installed apps")
	}

	swaURL, err := url.Parse(origin)
	if err != nil {
		return nil, errors.Wrapf(err, "system web app origin is invalid, got %s", origin)
	}

	for _, app := range installedApps {
		if app.InstallSource == "System" && app.Type == "Web" {
			// SWA's `publisher_id` is their start_url, match it against the provided `origin`.
			appURL, _ := url.Parse(app.PublisherID)
			if appURL != nil && appURL.Scheme == swaURL.Scheme && appURL.Host == swaURL.Host {
				return app, nil
			}
		}
	}
	return nil, nil
}

// LaunchSystemWebApp launches a system web app specifide by its name and URL.
func LaunchSystemWebApp(ctx context.Context, tconn *chrome.TestConn, appName, url string) error {
	return tconn.Call(ctx, nil, `async (appName, url) => {
		await tast.promisify(chrome.autotestPrivate.waitForSystemWebAppsInstall)();
		await tast.promisify(chrome.autotestPrivate.launchSystemWebApp)(appName, url);
	}`, appName, url)
}

// SystemWebApp corresponds to `SystemWebApp` defined in autotest_private.idl
type SystemWebApp struct {
	InternalName string `json:"internalName"`
	URL          string `json:"url"`
	Name         string `json:"name"`
	StartURL     string `json:"startUrl"`
}

// ListRegisteredSystemWebApps returns all registered system web apps.
func ListRegisteredSystemWebApps(ctx context.Context, tconn *chrome.TestConn) ([]*SystemWebApp, error) {
	var s []*SystemWebApp
	if err := tconn.Call(ctx, &s, "tast.promisify(chrome.autotestPrivate.getRegisteredSystemWebApps)"); err != nil {
		return nil, errors.Wrap(err, "failed to call getRegisteredSystemWebApps")
	}
	return s, nil
}

// LaunchOSSettings launches the OS Settings app to its subpage URL, and returns
// a connection to it. When this method returns, OS Settings page has finished
// loading.
//
// This method is necessary because OS Settings now uses System Web App link
// capturing, which doesn't work with DevTools protocol CreateTarget.
//
// Note, `url` needs to exactly match the page OS Settings ends up navigating to.
// For example, chrome://os-settings/.
func LaunchOSSettings(ctx context.Context, cr *chrome.Chrome, url string) (*chrome.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect Test API")
	}

	if LaunchSystemWebApp(ctx, tconn, "OSSettings", url); err != nil {
		return nil, errors.Wrap(err, "failed to launch OS Settings")
	}

	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(url))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get connection to OS Settings")
	}

	if conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return nil, errors.Wrap(err, "failed to wait for document load")
	}

	return conn, nil
}

// Close closes an app specified by appID.
func Close(ctx context.Context, tconn *chrome.TestConn, appID string) error {
	return tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.closeApp)`, appID)
}

// ChromeOrChromium returns the correct browser for the current build.
// Chromium is returned on non branded builds (e.g amd64-generic).
func ChromeOrChromium(ctx context.Context, tconn *chrome.TestConn) (App, error) {
	capps, err := ash.ChromeApps(ctx, tconn)
	if err != nil {
		return App{}, errors.Wrap(err, "failed to get list of installed apps")
	}
	for _, app := range capps {
		if app.AppID == Chrome.ID {
			if app.Name == Chrome.Name {
				return Chrome, nil
			}
			return Chromium, nil
		}
	}
	return App{}, errors.New("Neither Chrome nor Chromium were found in available apps")
}

// PrimaryBrowser returns the primary browser for the current system configuration.
// If Lacros is enabled, it behaves the same as the Lacros function above.
// Otherwise it returns 'Chrome' or 'Chromium' depending on branding.
// The given TestConn must be a connection to Ash.
func PrimaryBrowser(ctx context.Context, tconn *chrome.TestConn) (App, error) {
	lacrosInfo, err := lacrosinfo.Snapshot(ctx, tconn)
	if err != nil {
		return App{}, errors.Wrap(err, "failed to get lacros info")
	}
	switch lacrosInfo.Mode {
	case lacrosinfo.LacrosModeDisabled:
		return ChromeOrChromium(ctx, tconn)
	case lacrosinfo.LacrosModeOnly:
		return Lacros, nil
	}
	return App{}, errors.Wrapf(err, "unexpected LacrosMode: %v", lacrosInfo.Mode)
}

// InstallPWAForURL navigates to a PWA and attempts to install it.
// The given TestConn must be a connection to Ash.
func InstallPWAForURL(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser, pwaURL string, timeout time.Duration) error {
	sctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	conn, err := br.NewConn(sctx, pwaURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open URL %q", pwaURL)
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		return errors.Wrapf(err, "failed to wait for %q to be loaded and achieve quiescence", pwaURL)
	}

	ui := uiauto.New(tconn).WithInterval(2 * time.Second)
	statusBubble := nodewith.Role(role.Window).ClassName("StatusBubble").First()
	installIcon := nodewith.ClassName("PwaInstallView").Role(role.Button)
	installAppDialog := nodewith.NameStartingWith("Install app").Role(role.AlertDialog).HasClass("Widget")
	installButton := nodewith.Name("Install").Role(role.Button).Ancestor(installAppDialog)

	installPWA := uiauto.NamedCombine("install PWA through omnibox",
		// The status bubble indicates the page is still loading.
		// Wait for the status bubble to disappear before installing.
		ui.WithTimeout(time.Minute).WaitUntilGone(statusBubble),
		// The installability checks occur asynchronously for PWAs.
		// Wait for the Install button to appear in the Chrome omnibox before installing.
		ui.WithTimeout(timeout).WaitUntilExists(installIcon),
		// Low-end DUTs may take longer to wait for the dialog to pop up.
		ui.WithTimeout(2*time.Minute).LeftClickUntil(installIcon, ui.WaitUntilExists(installAppDialog)),
		ui.LeftClick(installButton),
	)

	// The page might be stuck at loading, and the install icon will not pop up.
	// Retry installing PWA be reloading the page to enhance the stability.
	const retryTimes = 3
	var lastErr error
	return uiauto.Retry(retryTimes, func(ctx context.Context) error {
		if lastErr != nil {
			if err := br.ReloadActiveTab(ctx); err != nil {
				return errors.Wrap(err, "failed to reload the tab")
			}
			// The page might be usable even if it failed to quiesce.
			// Log the error and try to continue the installation.
			if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
				testing.ContextLogf(ctx, "Failed to wait for %q to be loaded and quiesce: %v", pwaURL, err)
			}
		}
		lastErr = installPWA(ctx)
		return lastErr
	})(ctx)
}

// LaunchChromeByShortcut launches a new Chrome window in either normal user mode by shortcut `Ctl+N`
// or incognito mode by shortcut `Ctl+Shift+N`.
func LaunchChromeByShortcut(tconn *chrome.TestConn, incognitoMode bool) action.Action {
	return func(ctx context.Context) error {
		return tconn.Call(ctx, nil, `async (incognito) => {
			let accelerator = {keyCode: 'n', shift: incognito, control: true, alt: false, search: false, pressed: true};
			await tast.promisify(chrome.autotestPrivate.activateAccelerator)(accelerator);
			accelerator.pressed = false;
			await tast.promisify(chrome.autotestPrivate.activateAccelerator)(accelerator);
		}`, incognitoMode)
	}
}
