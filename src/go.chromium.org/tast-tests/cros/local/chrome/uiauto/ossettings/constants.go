// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ossettings

import (
	"fmt"
	"regexp"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// WindowFinder is the finder for the Settings window.
var WindowFinder *nodewith.Finder = nodewith.MultilingualNameStartingWith("Settings", map[string]string{"de": "Einstellungen"}).
	Role(role.Window).First()

// SearchBoxFinder is the finder for the search box in the settings app.
var SearchBoxFinder = nodewith.MultilingualName("Search settings", map[string]string{"de": "In Einstellungen suchen"}).
	Role(role.SearchBox).Ancestor(WindowFinder)

// Advanced is a button to expand advanced options.
var Advanced = nodewith.Role(role.Button).Ancestor(nodewith.Role(role.Heading).Name("Advanced"))

// getMenuItemFinder returns a finder for a menu item with the `name` in the
// Settings left menu.
func getMenuItemFinder(name string) *nodewith.Finder {
	return nodewith.Name(name).Role(role.Link).Focusable()
}

// Network is a subpage link.
var Network = getMenuItemFinder("Network")

// Bluetooth is a subpage link.
var Bluetooth = getMenuItemFinder("Bluetooth")

// ConnectedDevices is a subpage link.
var ConnectedDevices = getMenuItemFinder("Connected devices")

// Accounts is a subpage link.
var Accounts = getMenuItemFinder("Accounts")

// Device is a subpage link.
var Device = getMenuItemFinder("Device")

// Personalization is a subpage link.
var Personalization = getMenuItemFinder("Personalization")

// SearchAndAssistant is a subpage link.
var SearchAndAssistant = getMenuItemFinder("Search and Assistant")

// SecurityAndPrivacy is a subpage link.
var SecurityAndPrivacy = getMenuItemFinder("Security and Privacy")

// Apps is a subpage link.
var Apps = getMenuItemFinder("Apps")

// DateAndTime is a subpage link.
var DateAndTime = getMenuItemFinder("Date and time")

// LanguagesAndInputs is a subpage link.
var LanguagesAndInputs = getMenuItemFinder("Languages and inputs")

// Elements in "Languages page"
var (
	// AddLanguagesButton is the finder for the add language button UI in the languages page.
	AddLanguagesButton = nodewith.Name("Add languages").Role(role.Button)

	// SearchLanguages is the finder for the search language searchbox UI in the languages page.
	SearchLanguages = nodewith.Name("Search languages").Role(role.SearchBox)
)

// Files is a subpage link.
var Files = getMenuItemFinder("Files")

// PrintersAndScanners is a subpage link.
var PrintersAndScanners = getMenuItemFinder("Printers and scanners")

// Developers is a subpage link.
var Developers = getMenuItemFinder("Developers")

// Accessibility is a subpage link.
var Accessibility = getMenuItemFinder("Accessibility")

// ResetSettings is a subpage link.
var ResetSettings = getMenuItemFinder("Reset settings")

// AboutChromeOS is a subpage link.
var AboutChromeOS = nodewith.MultilingualName("About ChromeOS", map[string]string{"de": "Über ChromeOS"}).
	Role(role.Link).Focusable()

// MenuButton is a button to show the menu on the left side, only exist when the menu does not exist.
var MenuButton = nodewith.Name("Main menu").Role(role.Button).Focusable()

// Elements in "AboutChromeOS"
var (
	VersionInfo       = nodewith.NameStartingWith("Version ").Role(role.StaticText)
	CheckUpdateBtn    = nodewith.Name("Check for updates").Role(role.Button)
	ReportIssue       = nodewith.Name("Send feedback").Role(role.Link)
	AdditionalDetails = nodewith.Name("Additional details").Role(role.Link)
	TermsOfService    = nodewith.Name("Terms of Service").Role(role.Link)

	BrowserOpenSourceSoftware = nodewith.NameContaining("by other open source software").Role(role.Link)
	OSOpenSourceSoftware      = nodewith.Name("open source software").Role(role.Link)

	ChangeChannelBtn = nodewith.Name("Change channel").Role(role.Button)
	BuildDetailsBtn  = nodewith.Name("Build details").Role(role.Button)
)

// BackArrowBtn is the button to return to last page.
var BackArrowBtn = nodewith.HasClass("icon-arrow-back").Role(role.Button)

// NextButton is the finder for a button labelled as Next.
var NextButton = nodewith.NameContaining("Next").Role(role.Button)

// DoneButton is the finder for a button labelled as Done.
var DoneButton = nodewith.NameContaining("Done").Role(role.Button)

// SearchNoResults is the finder when Search does not return a result.
// The input keyword in `SearchBox` is mismatched with any existing option.
var SearchNoResults = nodewith.Name("No search results found").Role(role.StaticText)

// SearchResultFinder is a finder of all possible search results if they exist.
var SearchResultFinder = nodewith.NameRegex(regexp.MustCompile(fmt.Sprintf(`(Search result \d+ of \d+: .*)`))).Onscreen()

// SearchFeedbackButton is a button for sending feedback when a search result doesn't exist.
var SearchFeedbackButton = nodewith.Name("Report this search result").Role(role.Button)

// FeedbackWindowFinder is the finder for the feedback app window.
var FeedbackWindowFinder = nodewith.Name(apps.Feedback.Name).Role(role.Window)

// networkFinder is the finder for the Network page UI in OS setting.
var networkFinder = nodewith.Name("Network").Role(role.Link).Ancestor(WindowFinder)

// SavedDevicesSubpageLink is a subpage link on the Bluetooth Settings subpage.
var SavedDevicesSubpageLink = nodewith.Role(role.Link).NameStartingWith("Devices saved to your account").Focusable()

// Elements in "Saved devices subpage"
var (
	// SavedDeviceRows is the finder for saved device rows in the Saved devices subpage.
	SavedDeviceRows = nodewith.NameRegex(regexp.MustCompile("^Device [0-9] of [0-9],.*")).HasClass("list-item")

	// SavedDeviceMoreActionsBtn is the finder for the more actions buttons on the Saved devices subpage.
	SavedDeviceMoreActionsBtn = nodewith.HasClass("icon-more-vert").Role(role.Button)

	// SavedDeviceRemoveMenuItem is the finder for the Remove menu item on a first saved device in the Saved devices subpage.
	SavedDeviceRemoveMenuItem = nodewith.HasClass("dropdown-item").Role(role.MenuItem)

	// SavedDeviceConfirmRemovalBtn is the finder for the "Confirm" button on the Remove pop-up in the Saved devices subpage.
	SavedDeviceConfirmRemovalBtn = nodewith.HasClass("action-button").Role(role.Button)

	// SavedDevicesNoDevicesText is the finder for the text box displayed on the Saved devices subpage when there are no saved devices.
	SavedDevicesNoDevicesText = nodewith.NameRegex(regexp.MustCompile("^No devices saved to .*")).Role(role.StaticText)
)

// mobileButton is the finder for the Mobile Data page button UI in network page.
var mobileButton = nodewith.Name("Mobile data").Role(role.Button)

// MobileDataToggle is the finder for the mobile data toggle UI in the Mobile data subpage.
var MobileDataToggle = nodewith.NameStartingWith("Mobile data").Role(role.ToggleButton)

// HotspotToggle is the finder for the hotspot toggle UI in both Network page and Hotspot subpage.
var HotspotToggle = nodewith.Name("Hotspot").Role(role.ToggleButton)

// HotspotOnSublabel is the finder for the enabled hotspot status label in both Network page and Hotspot subpage.
var HotspotOnSublabel = nodewith.Name("On").Role(role.StaticText)

// HotspotTurningOnSublabel is the finder for the enabling hotspot status label in both Network page and Hotspot subpage.
var HotspotTurningOnSublabel = nodewith.Name("Turning on…").Role(role.StaticText)

// HotspotOffSublabel is the finder for the disabled hotspot status label in both Network page and Hotspot subpage.
var HotspotOffSublabel = nodewith.Name("Off").Role(role.StaticText)

// HotspotNameTextField is the finder for the hotspot ssid text field in the hotspot configuration dialog
var HotspotNameTextField = nodewith.Name("Hotspot name").Role(role.TextField)

// HotspotConfigureButton is the finder for the button to open up hotspot configuration dialog.
var HotspotConfigureButton = nodewith.Name("Configure").Role(role.Button)

// AddCellularButton is the finder for the Add Cellular button in cellular network list.
var AddCellularButton = nodewith.NameStartingWith("Add Cellular").Role(role.Button)

// UnlockButton is the finder for the Unlock button in cellular network list or settings home page.
var UnlockButton = nodewith.NameContaining("Unlock").Role(role.Button)

// NotActiveCellularRows is the finder for non-active cellular network rows in the Mobile data subpage.
var NotActiveCellularRows = nodewith.NameRegex(regexp.MustCompile("^Network [0-9] of [0-9],.*Signal Strength 0%, .*Connect")).HasClass("horizontal")

// NotActiveCellularBtn is the finder for the button to access the details view of the first non-active cellular network in the Mobile data subpage.
var NotActiveCellularBtn = nodewith.HasClass("subpage-arrow").Role(role.Button).Ancestor(NotActiveCellularRows.First())

// ActiveCellularRows is the finder for the active cellular network's row in the Mobile data subpage. There should only be one.
var ActiveCellularRows = nodewith.NameRegex(regexp.MustCompile("^Network [0-9] of [0-9],.*Details")).HasClass("horizontal")

// ActiveCellularBtn is the finder for the button to access the details view of the active cellular network in the Mobile data subpage.
var ActiveCellularBtn = nodewith.HasClass("subpage-arrow").Role(role.Button).Ancestor(ActiveCellularRows.First()).Focusable()

// APNSubpageButton is the finder for the button on the details view of a cellular network that navigates to the APN details view for the network.
var APNSubpageButton = nodewith.NameContaining("Access point name").Role(role.Link).First()

// Elements in "Cellular detail page"
var (
	// MoreActionsBtn is the finder for the more actions button on an eSIM profile detail view.
	MoreActionsBtn = nodewith.Name("More actions").Role(role.Button)

	// RenameProfileBtn is the finder for the rename profiles button on an eSIM profile detail view.
	RenameProfileBtn = nodewith.Name("Rename Profile").Role(role.MenuItem)

	// RenameProfileDoneButton is the finder for the Done button in the rename eSIM profile nickname dialog.
	RenameProfileDoneButton = nodewith.NameContaining("Rename profile to").Role(role.Button)

	// RemoveProfileOption is the option within the menu opened by `MoreActionsBtn` to remove the eSIM profile.
	RemoveProfileOption = nodewith.NameContaining("Remove Profile").Role(role.StaticText)

	// RemoveProfileButton is the button within the dialog opened by `RemoveProfileOption` to remove the eSIM profile.
	RemoveProfileButton = nodewith.NameContaining("Remove").Role(role.Button)

	// ConnectedStatus is the finder for the connected status text UI in the cellular detail page.
	ConnectedStatus = nodewith.Name("Connected").Role(role.StaticText)

	// LimitedConnectivityStatus is the finder for the limited connected status text UI in the cellular detail page.
	LimitedConnectivityStatus = nodewith.Name("Connected, limited connectivity").Role(role.StaticText)

	// SignInToNetwork is the finder for the sign in to network status text UI in the cellular detail page.
	SignInToNetwork = nodewith.NameContaining("Sign in to network").Role(role.StaticText)

	// DisconnectedStatus is the finder for the disconnected status text UI in the cellular detail page.
	DisconnectedStatus = nodewith.Name("Not Connected").Role(role.StaticText)

	// ConnectingStatus is the finder for the connecting status text UI in the cellular detail page.
	ConnectingStatus = nodewith.NameStartingWith("Connecting").Role(role.StaticText)

	// connectButton is the finder for the connect button UI in the cellular detail page.
	ConnectButton = nodewith.Name("Connect").Role(role.Button)

	// disconnectButton is the finder for the disconnect button UI in the cellular detail page.
	DisconnectButton = nodewith.Name("Disconnect").Role(role.Button)

	// RoamingToggle is the finder for the roaming toggle UI in the cellular detail page.
	RoamingToggle = nodewith.Name("Allow mobile data roaming").Role(role.ToggleButton)

	// AutoconnectToggle is the finder for the autoconnect toggle UI in the cellular detail page.
	AutoconnectToggle = nodewith.Name("Automatically connect to cellular network").Role(role.ToggleButton)

	// ApnSubpageButton is the finder for the APN subpage button in the cellular detail page that opens the APN subpage.
	ApnSubpageButton = nodewith.NameContaining("Access point name").Role(role.Link)

	// CellularAdvanced is the finder for the button that collapses/expands the advanced section of cellular details page.
	CellularAdvanced = nodewith.Name("Show advanced network properties").Role(role.Button)

	// CellularNetwork is the finder for the button that collapses/expands the network section of cellular details page.
	CellularNetwork = nodewith.Name("Show network address settings").Role(role.Button)

	// AccessPointDropdown is the finder for the access point dropdown in the old APN UI item.
	AccessPointDropdown = nodewith.HasClass("md-select").NameContaining("Access Point")

	// AccessPointNameInput is the finder for the access point input in the old APN UI item.
	AccessPointNameInput = nodewith.Role(role.TextField).First()

	// UsernameInput is the finder for the username input in the old APN UI item.
	UsernameInput = nodewith.Role(role.TextField).Nth(1)

	// PasswordInput is the finder for the password input in the old APN UI item.
	PasswordInput = nodewith.Role(role.TextField).Nth(2)

	// SaveButton is the finder for the save button in the old APN UI item.
	SaveButton = nodewith.Role(role.Button).HasClass("action-button").NameContaining("Save")

	// AttachAPNToggle is the finder for the save button in the old APN UI item.
	AttachAPNToggle = nodewith.NameContaining("Attach APN").Role(role.ToggleButton)

	// CellularProxy is the finder for the button that collapses/expands the proxy section of cellular details page.
	CellularProxy = nodewith.Name("Show proxy settings").Role(role.Button)

	// LockSimToggle is the finder for the Lock SIM toggle UI in the cellular details page.
	LockSimToggle = nodewith.NameStartingWith("Lock").Role(role.ToggleButton)

	// EnterButton is the finder for the Enter button in the SIM lock dialog UI.
	EnterButton = nodewith.Role(role.Button).Name("Enter")

	// CancelButton is the finder for the Cancel button in the SIM lock dialog UI.
	CancelButton = nodewith.NameContaining("Cancel").Role(role.Button)

	// VisibilityButton is the finder for the show PIN button in the SIM lock dialog UI.
	VisibilityButton = nodewith.Role(role.Button).HasClass("icon-visibility")

	// NetworkAddedText is the finder for the header to the eSIM cellular setup dialog when an eSIM profile is successfully added.
	NetworkAddedText = nodewith.NameContaining("Network added").Role(role.StaticText)

	ShowTextMessagesToggle = nodewith.NameStartingWith("Show text").Role(role.ToggleButton)
)

// Elements in "Access Point Name subpage"
var (
	// DetailsBtn is the finder for the APN details menu item in more actions menu for an APN.
	DetailsBtn = nodewith.Name("Details").Role(role.MenuItem)

	// DisableBtn is the finder for the disable APN menu item in more actions menu for an APN.
	DisableBtn = nodewith.Name("Disable").Role(role.MenuItem)

	// EnableBtn is the finder for the enable APN menu item in more actions menu for an APN.
	EnableBtn = nodewith.Name("Enable").Role(role.MenuItem)

	// RemoveBtn is the finder for the remove APN menu item in more actions menu for an APN.
	RemoveBtn = nodewith.Name("Remove").Role(role.MenuItem)

	// NameOfAPNInput is the finder for the APN* input in the APN details dialog.
	NameOfAPNInput = nodewith.Name("APN*").Role(role.TextField)

	// UserNameOfAPNInput is the finder for the username input in the APN details dialog.
	UserNameOfAPNInput = nodewith.Name("Username").Role(role.TextField)

	// PasswordOfAPNInput is the finder for the password input in the APN details dialog.
	PasswordOfAPNInput = nodewith.Name("Password").Role(role.TextField)

	// APNAdvancedBtn is the finder for the advanced button in the APN details dialog.
	APNAdvancedBtn = nodewith.Role(role.Button).NameContaining("Advanced settings").Focusable()

	// AuthenticationTypeDropdown is the finder for the authentication dropdown in the APN details dialog.
	AuthenticationTypeDropdown = nodewith.HasClass("select-field").NameContaining("Authentication type")

	// IPTypeDropdown is the finder for the IP type dropdown in the APN details dialog.
	IPTypeDropdown = nodewith.HasClass("select-field").NameContaining("IP type")

	// DefaultAPNCheckbox is the finder for the default checkbox in the APN details dialog.
	DefaultAPNCheckbox = nodewith.Role(role.CheckBox).NameContaining("Default")

	// AttachAPNCheckbox is the finder for the attach checkbox in the APN details dialog.
	AttachAPNCheckbox = nodewith.Role(role.CheckBox).NameContaining("Attach")

	// NewAPNBtn is the finder for the new APN button in the APN details page.
	NewAPNBtn = nodewith.Name("New APN").Role(role.Button)

	// APNLimitTooltip is the finder for the tooltip of the new APN button when number of APN hit the max limit.
	APNLimitTooltip = nodewith.Name("Remove an APN to add a new APN").Role(role.Tooltip)
)

// Elements in "Proxy" section of Network page.
var (
	// ShowProxySettingsTab is the finder for the "show proxy settings" tab.
	ShowProxySettingsTab = nodewith.HasClass("settings-box").Name("Show proxy settings").Role(role.GenericContainer)

	// ShowProxySettingsButton is the finder for the "show proxy settings" button.
	ShowProxySettingsButton = nodewith.Name("Show proxy settings").Role(role.Button)

	// SharedNetworksToggleButton is the finder for the "show shared networks" button.
	SharedNetworksToggleButton = nodewith.Name("Allow proxies for shared networks").Role(role.ToggleButton)

	// ConfirmButton is the finder for the "confirm" button.
	ConfirmButton = nodewith.Name("Confirm").Role(role.Button)

	proxyDropDownNameRegex = regexp.MustCompile(`(C|c)onnection type`)
	// ProxyDropDownMenu is the finder for the proxy drop down menu.
	ProxyDropDownMenu = nodewith.HasClass("md-select").NameRegex(proxyDropDownNameRegex).Role(role.ComboBoxSelect)

	// ManualProxyOption is the finder for the "Manual proxy configuration" option in the proxy drop down menu.
	ManualProxyOption = nodewith.Name("Manual proxy configuration").Role(role.ListBoxOption)

	// HTTPHostTextField is the finder for the "HTTP host" text field.
	HTTPHostTextField = nodewith.Name("HTTP Proxy - Host").Role(role.TextField)

	// HTTPPortTextField is the finder for the "HTTP port" text field.
	HTTPPortTextField = nodewith.Name("HTTP Proxy - Port").Role(role.TextField)

	// HTTPSHostTextField is the finder for the "Secure HTTP host" text field.
	HTTPSHostTextField = nodewith.Name("Secure HTTP Proxy - Host").Role(role.TextField)

	// HTTPSPortTextField is the finder for the "Secure HTTP port" text field.
	HTTPSPortTextField = nodewith.Name("Secure HTTP Proxy - Port").Role(role.TextField)

	// SocksHostTextField is the finder for the "SOCKS host" text field.
	SocksHostTextField = nodewith.Name("SOCKS Host - Host").Role(role.TextField)

	// SocksPortTextField is the finder for the "SOCKS port" text field.
	SocksPortTextField = nodewith.Name("SOCKS Host - Port").Role(role.TextField)

	// SameProxyHostTextField is the finder for the "Proxy host" text field.
	// This field only exists when "Use the same proxy for all protocols" is enabled."
	SameProxyHostTextField = nodewith.Name("Proxy - Host").Role(role.TextField)

	// SameProxyPortTextField is the finder for the "Proxy port" text field.
	// This field only exists when "Use the same proxy for all protocols" is enabled."
	SameProxyPortTextField = nodewith.Name("Proxy - Port").Role(role.TextField)
)

// ManagedEsimProfileName is the name of the managde eSim profile name used in network.CellularPolicyConnection* tests.
const ManagedEsimProfileName = "ManagedProfile"

// UnmanagedEsimProfileName is the name of the unmanagde eSim profile name used in network.CellularPolicyConnection* tests.
const UnmanagedEsimProfileName = "UnmanagedProfile"

// Elements in "Wi-Fi" page (WiFi known networks).
var (
	WifiSubPageArrowButtonNameRegex = regexp.MustCompile(`^(.*), Details`)

	WifiNetworkDetailArrowButton = nodewith.NameRegex(WifiSubPageArrowButtonNameRegex).Role(role.Button).HasClass("subpage-arrow")
)

// Elements in "Known Networks" page (WiFi known networks).
var (
	// KnownNetworksHeading is the finder for the heading of the Known Networks page.
	KnownNetworksHeading = nodewith.NameContaining("Known Networks").Role(role.Heading)

	// MoreActionsButton is the finder for the more actions button of a known network.
	MoreActionsButton = nodewith.NameStartingWith(MoreActionsButtonNamePrefix).Role(role.Button)
)

// MoreActionsButtonNamePrefix is the name prefix of MoreActionsButton.
const MoreActionsButtonNamePrefix = "More actions for "
