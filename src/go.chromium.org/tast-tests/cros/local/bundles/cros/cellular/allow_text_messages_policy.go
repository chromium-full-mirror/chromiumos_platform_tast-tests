// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type allowTextMessagesPolicyTestParam struct {
	globalNetworkConfig           policy.ONCGlobalNetworkConfiguration
	uiNotificationExpected        bool
	textMessagesToggleRestriction restriction.Restriction
	textMessagesToggleEnabled     bool
}

// This test is only run on the cellular_sim_active group. All boards in that group
// provide the Modem.SimSlots property and have at least one provisioned SIM slot.

func init() {
	testing.AddTest(&testing.Test{
		Func:         AllowTextMessagesPolicy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that admins and users can suppress text message notifications",
		Contacts:     []string{"chromeos-cellular-team@google.com", "fahadmansoor@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_sms"},
		Fixture:      "cellularWithFakeDMSEnrolled",
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
		VarDeps:      []string{"cellular.gaiaAccountPool"},
		Vars:         []string{"autotest_host_info_labels"},
		Params: []testing.Param{
			{
				Name: "policy_value_allow",
				Val: allowTextMessagesPolicyTestParam{
					globalNetworkConfig: policy.ONCGlobalNetworkConfiguration{
						AllowTextMessages: "Allow",
					},
					uiNotificationExpected:        true,
					textMessagesToggleRestriction: restriction.Disabled,
					textMessagesToggleEnabled:     true,
				},
			},
			{
				Name: "policy_value_suppress",
				Val: allowTextMessagesPolicyTestParam{
					globalNetworkConfig: policy.ONCGlobalNetworkConfiguration{
						AllowTextMessages: "Suppress",
					},
					uiNotificationExpected:        false,
					textMessagesToggleRestriction: restriction.Disabled,
					textMessagesToggleEnabled:     false,
				},
			},
			{
				Name: "policy_value_unset_user_allow",
				Val: allowTextMessagesPolicyTestParam{
					globalNetworkConfig: policy.ONCGlobalNetworkConfiguration{
						AllowTextMessages: "Unset",
					},
					uiNotificationExpected:        true,
					textMessagesToggleRestriction: restriction.None,
					textMessagesToggleEnabled:     true,
				},
			},
			{
				Name: "policy_value_unset_user_suppress",
				Val: allowTextMessagesPolicyTestParam{
					globalNetworkConfig: policy.ONCGlobalNetworkConfiguration{
						AllowTextMessages: "Unset",
					},
					uiNotificationExpected:        false,
					textMessagesToggleRestriction: restriction.None,
					textMessagesToggleEnabled:     false,
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceOpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// AllowTextMessagesPolicy validates that admins and users can configure devices to allow or suppress text message notifications.
func AllowTextMessagesPolicy(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find modem manager dbus object with a valid sim: ", err)
	}

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

	gaiaCreds, err := credconfig.PickRandomCreds(s.RequiredVar("cellular.gaiaAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse cellular user creds: ", err)
	}

	uiHelper, err := cellular.NewUIHelperWithOpts(ctx,
		chrome.EnableFeatures("SuppressTextMessages"),
		chrome.GAIALogin(chrome.Creds{User: gaiaCreds.User, Pass: gaiaCreds.Pass}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, uiHelper.Tconn)
	if err != nil {
		s.Fatal("Failed to create cellular.NewUiHelper: ", err)
	}

	// Apply the GNC policy.
	testOpts := s.Param().(allowTextMessagesPolicyTestParam)

	deviceNetworkPolicy := &policy.DeviceOpenNetworkConfiguration{
		Val: &policy.ONC{
			GlobalNetworkConfiguration: &testOpts.globalNetworkConfig,
		},
	}
	if err := policyutil.ServeAndRefresh(ctx, fdms, uiHelper.Cr, []policy.Policy{deviceNetworkPolicy}); err != nil {
		s.Fatal("Failed to ServeAndRefresh ONC policy: ", err)
	}

	if _, err := helper.ConnectToDefault(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	networkName, err := helper.GetCurrentNetworkName(ctx)
	if err != nil || networkName == "" {
		s.Fatal("Failed to fetch network name by iccid, err: ", err)
	}

	app, err := ossettings.OpenMobileDataSubpage(ctx, uiHelper.Tconn, uiHelper.Cr)
	if err != nil {
		s.Fatal("Failed to open mobile data sub page: ", err)
	}
	defer app.Close(ctx)

	ui := uiauto.New(uiHelper.Tconn).WithTimeout(60 * time.Second)

	connectedCellularRow := nodewith.NameContaining(networkName + ", Connected").First()
	if err = uiauto.Combine("Open the Internet detail subpage and expand the advanced section",
		ui.LeftClick(connectedCellularRow),
		ui.DoDefault(ossettings.CellularAdvanced),
	)(ctx); err != nil {
		s.Fatal("Failed to click on connected network row and expand the advanced seciton: ", err)
	}

	showTextMessagesToggle := ossettings.ShowTextMessagesToggle

	if err := ui.CheckRestriction(showTextMessagesToggle,
		testOpts.textMessagesToggleRestriction)(ctx); err != nil {
		s.Fatal("Show text messages toggle does not have the correct restriction: ", err)
	}

	toggleInfo, err := ui.Info(ctx, showTextMessagesToggle)
	if err != nil {
		s.Fatal("Failed to find the toggle info: ", err)
	}

	showTextMessagesToggleName := toggleInfo.Name

	settings := ossettings.New(uiHelper.Tconn)
	defer settings.Close(cleanupCtx)

	err = settings.SetToggleOption(uiHelper.Cr, showTextMessagesToggleName, testOpts.textMessagesToggleEnabled)(ctx)
	if err != nil {
		s.Fatal("Failed to set the state for show text messages toggle: ", err)
	}

	// Defer reset to enabled to make sure other tests are unaffected by any failed runs of this test.
	if testOpts.textMessagesToggleRestriction == restriction.None {
		defer settings.SetToggleOption(uiHelper.Cr, showTextMessagesToggleName, true)(cleanupCtx)
	}

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

	// Send the text message.
	err = uiHelper.SendMessage(ctx, phoneNumber, messageToSend)
	if err != nil {
		s.Fatal("Failed to send message: ", err)
	}

	if testOpts.uiNotificationExpected {
		err = uiHelper.ValidateMessage(ctx, messageToSend)
		if err != nil {
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
			s.Fatal("Failed to click on notification  dialog: ", err)
		}
	} else {
		err = uiHelper.ValidateSuppressedMessage(ctx)
		if err != nil {
			s.Fatal("Failed validation of message: ", err)
		}
	}

}
