// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"os"

	ppb "go.chromium.org/chromiumos/system_api/printscanmgr_proto"

	"go.chromium.org/tast-tests/cros/local/printscanmgr"
	"go.chromium.org/tast/core/testing"
)

const (
	ippusbDebugFlagPath = "/run/ippusb/debug/debug-flag"
	cupsDebugFlagPath   = "/run/cups/debug/debug-flag"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintscanDebug,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the printscan_debug D-bus methods",
		Contacts: []string{
			"project-bolton@google.com",
			"masonwilde@google.com",
		},
		// ChromeOS > Platform > baseOS > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:criticalstaging",
			"group:paper-io",
			"paper-io_printing",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "cups", "printscanmgr"},
		Fixture:      "chromeLoggedIn",
	})
}

func PrintscanDebug(ctx context.Context, s *testing.State) {
	p, err := printscanmgr.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to printscanmgr: ", err)
	}

	s.Log("Verifying PrintscanDebugSetCategories setting scanning debugging")
	if result, err := p.PrintscanDebugSetCategories(
		ctx,
		&ppb.PrintscanDebugSetCategoriesRequest{
			Categories:     []ppb.PrintscanDebugSetCategoriesRequest_DebugLogCategory{ppb.PrintscanDebugSetCategoriesRequest_DEBUG_LOG_CATEGORY_SCANNING},
			DisableLogging: false}); err != nil {
		s.Error("Failed to call PrintscanDebugSetCategories: ", err)
	} else if !result.Result {
		s.Error("Could not set categories: ", result.Result)
	}
	_, err = os.Stat(ippusbDebugFlagPath)
	if os.IsNotExist(err) {
		s.Error("Ippusb debug flag not present after setting scanning category")
	}

	s.Log("Verifying PrintscanDebugSetCategories setting printing debugging")
	if result, err := p.PrintscanDebugSetCategories(
		ctx,
		&ppb.PrintscanDebugSetCategoriesRequest{
			Categories:     []ppb.PrintscanDebugSetCategoriesRequest_DebugLogCategory{ppb.PrintscanDebugSetCategoriesRequest_DEBUG_LOG_CATEGORY_PRINTING},
			DisableLogging: false}); err != nil {
		s.Error("Failed to call PrintscanDebugSetCategories: ", err)
	} else if !result.Result {
		s.Error("Could not set categories: ", result.Result)
	}
	_, err = os.Stat(cupsDebugFlagPath)
	if os.IsNotExist(err) {
		s.Error("CUPS debug flag not present after setting printing category")
	}

	s.Log("Verifying PrintscanDebugSetCategories disable logging")
	if result, err := p.PrintscanDebugSetCategories(
		ctx,
		&ppb.PrintscanDebugSetCategoriesRequest{
			Categories:     nil,
			DisableLogging: true}); err != nil {
		s.Error("Failed to call PrintscanDebugSetCategories: ", err)
	} else if !result.Result {
		s.Error("Could not set categories: ", result.Result)
	}

	_, err = os.Stat(ippusbDebugFlagPath)
	if !os.IsNotExist(err) {
		s.Error("Ippusb debug flag still present after disabling logging")
	}
	_, err = os.Stat(cupsDebugFlagPath)
	if !os.IsNotExist(err) {
		s.Error("CUPS debug flag still present after disabling logging")
	}

	s.Log("Verifying PrintscanDebugSetCategories setting printing and scanning category")
	if result, err := p.PrintscanDebugSetCategories(
		ctx,
		&ppb.PrintscanDebugSetCategoriesRequest{
			Categories:     []ppb.PrintscanDebugSetCategoriesRequest_DebugLogCategory{ppb.PrintscanDebugSetCategoriesRequest_DEBUG_LOG_CATEGORY_PRINTING, ppb.PrintscanDebugSetCategoriesRequest_DEBUG_LOG_CATEGORY_SCANNING},
			DisableLogging: false}); err != nil {
		s.Error("Failed to call PrintscanDebugSetCategories: ", err)
	} else if !result.Result {
		s.Error("Could not set categories: ", result.Result)
	}

	_, err = os.Stat(cupsDebugFlagPath)
	if os.IsNotExist(err) {
		s.Error("CUPS debug flag not present after setting printing category")
	}
	_, err = os.Stat(ippusbDebugFlagPath)
	if os.IsNotExist(err) {
		s.Error("Ippusb debug flag not present after setting scanning category")
	}
}
