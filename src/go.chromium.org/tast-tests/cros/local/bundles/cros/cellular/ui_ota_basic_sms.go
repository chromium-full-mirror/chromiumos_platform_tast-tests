// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// This test is only run on the cellular_sim_active group. All boards in that group
// provide the Modem.SimSlots property and have at least one provisioned SIM slot.

func init() {
	testing.AddTest(&testing.Test{
		Func:         UIOtaBasicSms,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that the UI shows a notification for a received MT SMS",
		Contacts:     []string{"chromeos-cellular-team@google.com", "srikanthkumar@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_sms"},
		Fixture:      "cellular",
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
		VarDeps:      []string{"cellular.gaiaAccountPool"},
		Vars:         []string{"autotest_host_info_labels"},
	})
}

// UIOtaBasicSms validates MT SMS, uses google voice to send SMS.
func UIOtaBasicSms(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find mm dbus object with a valid sim: ", err)
	}

	/* a) Check cellular connection and get mobile number on dut
	   b) Create and send SMS on google voice ui from chrome web interface
	   c) Check UI notifications for SMS, verify SMS content
	   d) Clear received SMS notification
	*/

	messageToSend := "Hello " + time.Now().Format(time.UnixDate)

	if err := modem.DeleteAllMessages(ctx); err != nil {
		s.Fatal("Failed to delete all messages: ", err)
	}

	// Device properties from host info store labels.
	labels, err := cellular.GetLabelsAsStringArray(ctx, s.Var, "autotest_host_info_labels")
	if err != nil {
		s.Fatal("Failed to read autotest_host_info_labels: ", err)
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper
	if err := helper.GetHostInfoLabels(ctx, labels); err != nil {
		s.Fatal("Unable to read labels: ", err)
	}

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID: ", err)
	}

	// Read modem property OwnNumber from labels.
	phoneNumber := helper.GetLabelOwnNumber(ctx, iccid)
	if phoneNumber == "" || len(phoneNumber) < 10 {
		s.Fatal("Invalid OwnNumber label value")
	}
	if phoneNumber == "1234567890" || phoneNumber == "1111111111" {
		s.Fatal("SMS test not applicable for this dut")
	}
	s.Logf("Phone number: %s to send message: %s", phoneNumber, messageToSend)

	// Create cleanup context to ensure UI tree dumps correctly.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	gaiaCreds, err := credconfig.PickRandomCreds(s.RequiredVar("cellular.gaiaAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse cellular user creds: ", err)
	}
	uiHelper, err := cellular.NewUIHelper(ctx, gaiaCreds.User, gaiaCreds.Pass)
	if err != nil {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, uiHelper.Tconn)
		s.Fatal("Failed to create cellular.NewUiHelper: ", err)
	}

	s.Log("open google voice web page")

	// Keyboard to input key inputs.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	// Create gv MO sms using google voice configured with owned test accounts.
	gConn, err := uiHelper.GoogleVoiceLogin(ctx)
	if err != nil {
		s.Fatal("Failed to open Google voice website: ", err)
	}
	defer gConn.Close()
	defer gConn.CloseTarget(cleanupCtx)

	if gConn == nil {
		s.Fatal("Could not create new chrome tab")
	}

	s.Log("After finding messages tab")
	err = uiHelper.SendMessage(ctx, phoneNumber, messageToSend)
	if err != nil {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, uiHelper.Tconn)
		s.Fatal("Failed to send message: ", err)
	}

	s.Log("Check for SMS message")
	err = uiHelper.ValidateMessage(ctx, messageToSend)
	if err != nil {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, uiHelper.Tconn)
		s.Fatal("Failed validation of message: ", err)
	}

	alertDialog := nodewith.Role(role.AlertDialog).ClassName("MessagePopupView").Onscreen()
	// Click on alert dialog to close.
	if err := uiauto.Combine("Click on alert dialog",
		uiHelper.UI.WithTimeout(5*time.Second).WaitUntilExists(alertDialog),
		uiHelper.UIHandler.Click(alertDialog),
		kb.AccelAction("Ctrl+X"),
		uiHelper.UI.WaitUntilGone(alertDialog),
	)(ctx); err != nil {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, uiHelper.Tconn)
		s.Fatal("Failed to click on notification  dialog: ", err)
	}

}
