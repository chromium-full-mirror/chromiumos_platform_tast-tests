// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package uitools contains strings and different nodewith.Finder objects from
// the UI that can be used by multiple tests.
package uitools

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// List of strings that are used in the UI.
const (
	AddPrinterManuallyName = "Add a printer manually"
	// TODO(b/297499031): This UI string is deprecated and AddPrinterName is
	// used instead. Delete this string when the current UI is stabilised. Also
	// simplify the OpenAddPrinterDialog as the other available printers button
	// will always exist.
	AddPrinterNameDeprecated   = "Add printer"
	AddPrinterName             = "Add printer manually"
	AddName                    = "Add"
	AddressName                = "Address"
	AdvancedConfigName         = "Advanced printer configuration"
	AppSocketName              = "AppSocket"
	EditName                   = "Edit"
	EditPrinterName            = "Edit printer"
	EulaRegex                  = "End User Licen[cs]e Agreement: "
	ManufacturerName           = "Manufacturer"
	ModelName                  = "Model"
	NameName                   = "Name"
	OtherAvailablePrintersName = "Other available printers"
	PpdStartTextName           = "PPD-Adobe:"
	PpdRetrieveErrorName       = "Unable to retrieve PPD"
	PrintersName               = "Printers"
	ProtocolName               = "Protocol"
	SearchPrintersName         = "Search printers"
	SettingsPageName           = "osPrinting"
	ViewPpdName                = "View printer PPD"
)

// nodewith.Finder objects for items in the UI.
var (
	AddPrinterManuallyFinder     *nodewith.Finder = nodewith.Role(role.Dialog).Name(AddPrinterManuallyName)
	AddPrinterFinder             *nodewith.Finder = nodewith.Role(role.Button).NameRegex(regexp.MustCompile(AddPrinterName + `|` + AddPrinterNameDeprecated))
	AddFinder                    *nodewith.Finder = nodewith.Role(role.Button).Name(AddName)
	AddressFinder                *nodewith.Finder = nodewith.Role(role.TextField).Name(AddressName)
	AdvancedConfigFinder         *nodewith.Finder = nodewith.Role(role.Dialog).Name(AdvancedConfigName)
	AppSocketFinder              *nodewith.Finder = nodewith.Role(role.ListBoxOption).NameContaining(AppSocketName)
	EditFinder                   *nodewith.Finder = nodewith.Role(role.StaticText).Name(EditName)
	EditPrinterFinder            *nodewith.Finder = nodewith.Role(role.Dialog).Name(EditPrinterName)
	ManufacturerFinder           *nodewith.Finder = nodewith.Role(role.TextField).Name(ManufacturerName)
	ModelFinder                  *nodewith.Finder = nodewith.Role(role.TextField).Name(ModelName)
	NameFinder                   *nodewith.Finder = nodewith.Role(role.TextField).Name(NameName)
	OtherAvailablePrintersFinder *nodewith.Finder = nodewith.Role(role.Button).Name(OtherAvailablePrintersName)
	PpdStartTextFinder           *nodewith.Finder = nodewith.Role(role.StaticText).NameContaining(PpdStartTextName)
	PpdRetrieveErrorFinder       *nodewith.Finder = nodewith.Role(role.StaticText).NameContaining(PpdRetrieveErrorName)
	PrintersFinder               *nodewith.Finder = nodewith.Role(role.Link).Name(PrintersName)
	ProtocolFinder               *nodewith.Finder = nodewith.Role(role.ComboBoxSelect).Name(ProtocolName)
	SearchPrintersFinder         *nodewith.Finder = nodewith.Role(role.SearchBox).Name(SearchPrintersName)
	ViewPpdFinder                *nodewith.Finder = nodewith.Role(role.Button).Name(ViewPpdName)
)

// GetPpdWindowFinder will create the finder for the given printer name.
func GetPpdWindowFinder(printerName string) *nodewith.Finder {
	return nodewith.Role(role.Window).HasClass("Widget").NameStartingWith(printerName + ".ppd").First()
}

// GetEulaFinder will create the finder for the given eulaLink string.
func GetEulaFinder(eulaLink string) *nodewith.Finder {
	return nodewith.Role(role.StaticText).NameRegex(regexp.MustCompile(EulaRegex + regexp.QuoteMeta(eulaLink)))
}

// NavigateToPrintersSettingsPage opens OS settings and navigates to the
// printers page.
func NavigateToPrintersSettingsPage(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, ui *uiauto.Context) error {
	printerButton := PrintersFinder.Ancestor(ossettings.WindowFinder)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, SettingsPageName, ui.Exists(printerButton)); err != nil {
		return err
	}
	return nil
}

// OpenAddPrinterDialog opens the "Add printer" dialog. This function assumes
// that the printers settings page is open.
func OpenAddPrinterDialog(ctx context.Context, ui *uiauto.Context) error {
	// A network connection is required in order for the Add Printer button to
	// be present/active.
	if err := shill.WaitForOnlineAfterResume(ctx); err != nil {
		return errors.Wrap(err, "network connection is required for Add Printer dialog")
	}

	// Open the printers settings page and wait for it to load.
	printersPageButton := PrintersFinder.Ancestor(ossettings.WindowFinder)
	searchPrintersBar := SearchPrintersFinder.Ancestor(ossettings.WindowFinder)
	if err := uiauto.Combine("open the printers settings page",
		ui.WithTimeout(10*time.Second).WaitUntilExists(printersPageButton),
		ui.DoDefault(printersPageButton),
		ui.WithTimeout(10*time.Second).WaitUntilExists(searchPrintersBar))(ctx); err != nil {
		return errors.Wrap(err, "failed to open the printers settings page")
	}

	// Search for the other available printers button. This button is not
	// presented in the old UI versions.
	otherAvailablePrintersButton := OtherAvailablePrintersFinder.Ancestor(ossettings.WindowFinder)

	if err := ui.WithTimeout(1 * time.Second).WaitUntilExists(otherAvailablePrintersButton)(ctx); err != nil {
		if strings.Contains(err.Error(), nodewith.ErrNotFound) {
			// We are on an older build that does not have this button.  Just continue.
			testing.ContextLog(ctx, OtherAvailablePrintersName+" button does not exist")
		} else {
			return errors.Wrap(err, "failed looking for "+OtherAvailablePrintersName+" button")
		}
	} else {
		testing.ContextLog(ctx, "Found "+OtherAvailablePrintersName+" button")
		// Expand the other available printers dialog if it's collapsed.
		nodeInfo, err := ui.Info(ctx, otherAvailablePrintersButton)
		if err != nil {
			return errors.Wrap(err, "failed to get info of "+OtherAvailablePrintersName+" button")
		}
		if nodeInfo.State[state.Collapsed] {
			testing.ContextLog(ctx, "Expanding "+OtherAvailablePrintersName)
			if err := ui.DoDefault(otherAvailablePrintersButton)(ctx); err != nil {
				return errors.Wrap(err, "error while expanding the "+OtherAvailablePrintersName+" button")
			}
		}
	}

	// Click add printer button.
	addPrinterButton := AddPrinterFinder.Ancestor(ossettings.WindowFinder)
	if err := uiauto.Combine("click add printer button",
		ui.WithTimeout(10*time.Second).WaitUntilEnabled(addPrinterButton),
		ui.DoDefault(addPrinterButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on the add printer button")
	}

	return nil
}
