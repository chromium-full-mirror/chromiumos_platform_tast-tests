// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
)

const (
	// FootPedalAppName is the name of the Foot Pedal application.
	FootPedalAppName      AppName = "Philips"
	footPedalDeviceName   string  = "Foot Control 2330"
	footPedalMarkIcon     string  = "citrix/foot_pedal_mark.png"
	footPedalMaximizeIcon string  = "citrix/foot_pedal_maximize.png"
)

// FootPedalButton is the name of the foot pedal button.
type FootPedalButton string

const (
	// FootPedalButtonCenter is the center button.
	FootPedalButtonCenter FootPedalButton = "Play"
	// FootPedalButtonLeft is the left button.
	FootPedalButtonLeft FootPedalButton = "Forward"
	// FootPedalButtonRight is the right button.
	FootPedalButtonRight FootPedalButton = "Rewind"
	// FootPedalButtonTop is the top button.
	FootPedalButtonTop FootPedalButton = "EOL"
)

// VerifyFootPedalButtonPressed verifies that foot pedal button is pressed.
func VerifyFootPedalButtonPressed(ud *uidetection.Context, button FootPedalButton) action.Action {
	pressedButtonText := fmt.Sprintf("%s Pressed", button)
	pressedButtonFinder := uidetection.TextBlockFromSentence(pressedButtonText)
	return uiauto.NamedAction("verify foot pedal button pressed: "+pressedButtonText,
		ud.WaitUntilExists(pressedButtonFinder),
	)
}

// SetupFootPedalTest sets up foot pedal test.
func SetupFootPedalTest(kb *input.KeyboardEventWriter, ud *uidetection.Context, dataPath func(string) string) action.Action {
	mark := uidetection.CustomIcon(dataPath(footPedalMarkIcon))
	maximizeButton := uidetection.CustomIcon(dataPath(footPedalMaximizeIcon)).Above(mark)
	footPedalDeviceText := uidetection.TextBlockFromSentence(footPedalDeviceName)
	lastEventText := uidetection.TextBlockFromSentence("Last event")
	selectFootControl := uiauto.Combine("select foot control",
		kb.AccelAction("Tab"),
		kb.AccelAction("Tab"),
		uiauto.Retry(3, uiauto.Combine("press down to select foot control",
			kb.AccelAction("Down"),
			ud.WithTimeout(10*time.Second).WaitUntilExists(footPedalDeviceText),
		)),
	)
	return uiauto.NamedCombine("set up foot pedal test",
		ud.LeftClick(maximizeButton),
		uiauto.NamedCombine("switch to device tab",
			uiauto.IfFailThen(
				ud.WithTimeout(10*time.Second).WaitUntilExists(footPedalDeviceText),
				selectFootControl,
			),
			kb.AccelAction("Alt+1"),
			// Wait for the text "Last event" to verify that it has switched
			// to the device tab.
			ud.WaitUntilExists(lastEventText),
		))
}
