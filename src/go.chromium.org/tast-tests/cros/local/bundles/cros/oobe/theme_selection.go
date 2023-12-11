// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ThemeSelection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test Theme Selection screen during OOBE to support light/dark/auto themes",
		Contacts: []string{
			"cros-oobe@google.com",
			"bohdanty@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.LoginTimeout + 3*time.Minute,
	})
}

// convertHexToRGB tries to convert hex string that is returned from JS to a
// string in the "rgb(R,G,B)" format. Hex strings starts with a #.
func convertHexToRGB(colorHex string) (string, error) {
	if len(colorHex) == 0 {
		return "", errors.New("length of the color string should not be 0")
	}
	if colorHex[0] != '#' {
		return "", errors.Errorf("color hex string in a wrong format, '#' expected as a first character, got %s", colorHex)
	}
	// Remove # from the beginning.
	colorHex = colorHex[1:]
	convertedColors, err := hex.DecodeString(colorHex)
	if err != nil {
		return "", errors.Errorf("can't decode string %s", colorHex)
	}
	if len(convertedColors) < 3 {
		return "", errors.Errorf("length of the parsed color hex slice should be at least 3, got %d", len(convertedColors))
	}
	return fmt.Sprintf("rgb(%d,%d,%d)",
		convertedColors[0], convertedColors[1], convertedColors[2]), nil
}

// expectedTextColor executes JS code on the Theme Selection screen page to
// fetch expected color value for a currently enabled color mode.
func expectedTextColor(ctx context.Context, oobeConn *chrome.Conn) (string, error) {
	const colorQuery = "getComputedStyle(document.documentElement).getPropertyValue('--oobe-header-text-color').trim();"
	var colorData string
	if err := oobeConn.Eval(ctx, colorQuery, &colorData); err != nil {
		return "", err
	}
	return convertHexToRGB(colorData)
}

// headerTextColor executes JS code on the Theme Selection screen page to
// fetch current page header text color.
func headerTextColor(ctx context.Context, oobeConn *chrome.Conn) (string, error) {
	var headerTextColor string
	if err := oobeConn.Eval(ctx, queryThemeScreen("getHeaderTextColor()"), &headerTextColor); err != nil {
		return "", err
	}
	return headerTextColor, nil
}

// nameOfSelectedTheme queries OobeApi to fetch selected value of a theme
// selection radio button. This call might return non-string value in case
// there is no selected value. We raise an error if it happens as we expect one
// of the theme modes to be always selected
func nameOfSelectedTheme(ctx context.Context, oobeConn *chrome.Conn) (string, error) {
	var currentThemeModeResponse interface{}
	if err := oobeConn.Eval(ctx, queryThemeScreen("getNameOfSelectedTheme()"), &currentThemeModeResponse); err != nil {
		return "", errors.Wrap(err, "failed to retrieve current theme mode")
	}

	currentThemeMode, ok := currentThemeModeResponse.(string)
	if !ok {
		return "", errors.New("can't convert theme mode call response to string. This could mean that there is no selection on the radio button")
	}
	return currentThemeMode, nil
}

// waitForColorUpdate polls Theme Selection screen until new color schema is
// applied to the UI. It should break polling and return an error if we fail to
// execute JS calls, otherwise it waits until expected colors are present on the
// screen.
func waitForColorUpdate(ctx context.Context, oobeConn *chrome.Conn, previousHeaderTextColor string) error {
	var pollOpts = &testing.PollOptions{Interval: 50 * time.Millisecond, Timeout: 20 * time.Second}
	return testing.Poll(ctx, func(ctx context.Context) error {
		headerTextColor, err := headerTextColor(ctx, oobeConn)
		if err != nil {
			// Break polling if we received any error from the OobeAPI.
			return testing.PollBreak(errors.Wrap(err, "failed to get color of the text header"))
		}

		expectedTextColor, err := expectedTextColor(ctx, oobeConn)
		if err != nil {
			// Break polling if we failed to query JS on the screen.
			return testing.PollBreak(errors.Wrap(err, "failed to get expected color of the text header"))
		}

		// We try to unify color strings in the rgb(x,y,z) format by removing
		// whitespaces from them and then compare given values. JS calls might
		// return fetched properties with a different amount of whitespaces
		// inside them, so we want to mitigate this risk here.
		headerTextColor = strings.ReplaceAll(headerTextColor, " ", "")
		expectedTextColor = strings.ReplaceAll(expectedTextColor, " ", "")
		previousHeaderTextColor = strings.ReplaceAll(previousHeaderTextColor, " ", "")

		// Check that color indeed changed when we switch from one color mode to other.
		// Skip it in case it's the first color mode that we check during the test.
		if len(previousHeaderTextColor) != 0 && previousHeaderTextColor == headerTextColor {
			return errors.Errorf("header color (%s) still has same value as a previous one (%s)", headerTextColor, previousHeaderTextColor)
		}
		if headerTextColor != expectedTextColor {
			return errors.Errorf("header color (%s) doesn't match expected color (%s)", headerTextColor, expectedTextColor)
		}

		return nil
	}, pollOpts)
}

// queryThemeScreen returns correct OobeAPI path for a given function call.
func queryThemeScreen(call string) string {
	return fmt.Sprintf("OobeAPI.screens.ThemeSelectionScreen.%s", call)
}

func ThemeSelection(ctx context.Context, s *testing.State) {
	const (
		lightTheme = "light"
		darkTheme  = "dark"
		autoTheme  = "auto"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.DontSkipOOBEAfterLogin())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('theme-selection')", nil); err != nil {
		s.Fatal("Failed to advance to the theme selection screen: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, queryThemeScreen("isReadyForTesting()")); err != nil {
		s.Fatal("Failed to wait for the theme selection screen: ", err)
	}

	selectedTheme, err := nameOfSelectedTheme(ctx, oobeConn)
	if err != nil {
		s.Fatal("Failed to get name of a selected theme: ", err)
	}
	if selectedTheme != autoTheme {
		s.Fatal("Device should be in the auto theme mode by default")
	}

	previousHeaderTextColor := ""

	// We can combine testing for light and dark modes as they do same steps.
	for _, theme := range []string{lightTheme, darkTheme} {
		selectFuncCall := "selectLightTheme()"
		if theme == darkTheme {
			selectFuncCall = "selectDarkTheme()"
		}

		if err := oobeConn.Eval(ctx, queryThemeScreen(selectFuncCall), nil); err != nil {
			s.Fatal(fmt.Sprintf("Failed to switch to the %s theme mode: ", theme), err)
		}

		selectedTheme, err := nameOfSelectedTheme(ctx, oobeConn)
		if err != nil {
			s.Fatal("Failed to get name of a selected theme: ", err)
		}
		if selectedTheme != theme {
			s.Fatalf("Device should be in the %s theme mode after we select it", theme)
		}

		if err = waitForColorUpdate(ctx, oobeConn, previousHeaderTextColor); err != nil {
			s.Fatal(fmt.Sprintf("Failed to wait until colors update to %s theme: ", theme), err)
		}

		previousHeaderTextColor, err = headerTextColor(ctx, oobeConn)
		if err != nil {
			s.Fatal("Failed to get color of the text header: ", err)
		}
	}
}
