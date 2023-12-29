// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/systemlogs"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filepicker"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// UIHelper creates chrome target and testapi connectoin.
type UIHelper struct {
	UIHandler cuj.UIActionHandler
	UI        *uiauto.Context
	Cr        *chrome.Chrome
	Tconn     *chrome.TestConn
}

const (
	gvoiceMessagesURL = "https://voice.google.com/u/0/messages"
	// testIPv6DotCom    = "https://test-ipv6.com"
	simLockPortalURL = "https://partnerdash.google.com/apps/nexussimunlockportal/imeiupload?a=1054655288&t=1772550927529650125"
)

// NewUIHelperWithOpts creates a Helper object with a new Chrome instance, and ensures that a UI is loaded.
func NewUIHelperWithOpts(ctx context.Context, opts ...chrome.Option) (*UIHelper, error) {

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "chrome login failed")
	}

	creds := cr.Creds()
	testing.ContextLogf(ctx, "test api conn: %s", creds)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect test api")
	}

	ui := uiauto.New(tconn)

	// uiHandler will be assigned with different instances for clamshell and tablet mode.
	uiHandler, err := cuj.NewClamshellActionHandler(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create clamshell action handler")
	}

	helper := UIHelper{UIHandler: uiHandler, UI: ui, Cr: cr, Tconn: tconn}
	return &helper, nil
}

// NewUIHelper creates a Helper object logged into a GAIA user, and ensures that a UI is loaded.
func NewUIHelper(ctx context.Context, username, password string) (*UIHelper, error) {
	return NewUIHelperWithOpts(ctx, chrome.GAIALogin(chrome.Creds{User: username, Pass: password}),
		chrome.ProdPolicy())
}

// LaunchChromeWithCarrierLock launches chrome with carrier lock service enabled.
func (h *UIHelper) LaunchChromeWithCarrierLock(ctx context.Context, username, password string) error {
	_, err := chrome.New(ctx,
		chrome.GAIALogin(chrome.Creds{User: username, Pass: password}),
		chrome.EnableFeatures("CellularCarrierLock:LastConfigDateDelta/-1"),
		chrome.ProdPolicy(),
	)
	if err != nil {
		return errors.Wrap(err, "failed to launch chrome with carrier lock enabled")
	}

	return nil
}

// GoogleVoiceLogin attempts to login on google voice message page.
func (h *UIHelper) GoogleVoiceLogin(ctx context.Context) (*chrome.Conn, error) {

	testing.ContextLog(ctx, "open googlevoice web url: ", gvoiceMessagesURL)
	// Open google voice tab, set new window to true to be first tab
	driverconn, err := h.UIHandler.NewChromeTab(ctx, h.Cr.Browser(), gvoiceMessagesURL, true)
	if err != nil {
		return driverconn, errors.Wrap(err, "failed to open voice web page")
	}

	if err := webutil.WaitForRender(ctx, driverconn, 2*time.Minute); err != nil {
		return driverconn, errors.Wrap(err, "failed to wait for render to finish")
	}

	if err := webutil.WaitForQuiescence(ctx, driverconn, 2*time.Minute); err != nil {
		return driverconn, errors.Wrap(err, "failed to wait for voice page to finish loading")
	}

	// Google Voice sometimes pops up a prompt to notice about notifications,
	// asking to allow/dismiss notifications.
	allowPrompt := nodewith.Name("Allow").Role(role.Button).Onscreen()

	if err := uiauto.Combine("Click on allow button",
		h.UI.WithTimeout(10*time.Second).WaitUntilExists(allowPrompt),
		h.UI.FocusAndWait(allowPrompt),
		h.UI.LeftClick(allowPrompt),
	)(ctx); err != nil {
		return driverconn, errors.Wrap(err, "failed to click on allow button")
	}

	return driverconn, nil
}

// SendMessage - sends sms to dut mobile number using google voice with ota.
func (h *UIHelper) SendMessage(ctx context.Context, number, message string) error {

	// Keyboard to input key inputs.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	// Click on + Send new message.
	sendButton := nodewith.NameContaining("Send new message").Role(role.Button)
	if err := uiauto.Combine("Click on send new message button",
		h.UI.WithTimeout(10*time.Second).WaitUntilExists(sendButton),
		h.UI.LeftClick(sendButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on + button")
	}

	testing.ContextLog(ctx, "selected send new message button")
	// Enter number on 'To'.
	textArea := nodewith.Name("To").Role(role.InlineTextBox).Onscreen()
	textAreaFocused := textArea.Focused()
	h.UIHandler.ClickUntil(textArea, h.UI.Exists(textAreaFocused))
	if err := uiauto.Combine("Focus number text field",
		h.UI.WithTimeout(10*time.Second).WaitUntilExists(textArea),
		kb.AccelAction("Ctrl+A"),
		kb.AccelAction("Backspace"),
		kb.TypeAction(number),
		kb.AccelAction("Down"),
		kb.AccelAction("Enter"),
		kb.AccelAction("Esc"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to focus the phone number text field")
	}

	testing.ContextLog(ctx, "enter text in type a message textfield")
	smsTextField := nodewith.Name("Type a message").Role(role.TextField).State(state.Editable, true).Onscreen()
	smsTextFieldFocused := smsTextField.Focused()
	info, err := h.UI.Info(ctx, smsTextField)
	if err != nil {
		return errors.Wrap(err, "error reading smstextfield")
	}
	if info.State[state.Invisible] {
		testing.ContextLog(ctx, "sms text field invisible")
		if err := kb.Accel(ctx, "Shift+Tab+Enter"); err != nil {
			return errors.Wrap(err, "failed to press shift+tab+enter to close consent banner")
		}
	}

	// Get text box and enter mobile number.
	h.UIHandler.ClickUntil(smsTextField, h.UI.Exists(smsTextFieldFocused))
	if err := uiauto.Combine("Focus message text field",
		h.UI.WithTimeout(10*time.Second).WaitUntilExists(smsTextField.Visible()),
		h.UI.LeftClick(smsTextField),
		kb.TypeAction(message),
		kb.AccelAction("Enter"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to focus the message text area")
	}

	testing.ContextLog(ctx, "click on send button")
	// Do click enter or send 'Enter' key.
	sendButton = nodewith.Name("Send message").Role(role.Button)
	if err := uiauto.Combine("Click on send button",
		h.UI.WithTimeout(10*time.Second).WaitUntilExists(sendButton),
		h.UI.LeftClick(sendButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on send button")
	}

	return nil
}

// ValidateMessage - validates sms received.
func (h *UIHelper) ValidateMessage(ctx context.Context, messageSent string) error {

	// check message content.
	notification := nodewith.Role(role.Window).ClassName("ash/message_center/MessagePopup")

	// check notification window.
	if err := h.UI.WithTimeout(1 * time.Minute).WaitUntilExists(notification)(ctx); err != nil {
		return errors.Wrap(err, "failed to see sms notification dialog")
	}

	testing.ContextLog(ctx, "notification details:", notification)
	alertDialog := nodewith.Role(role.AlertDialog).ClassName("MessagePopupView").Onscreen()

	// Read number and sms message and compare and close dialog.
	smsDetails, err := h.UI.Info(ctx, alertDialog)
	if err != nil {
		return errors.Wrap(err, "failed to see sms notification dialog content")
	}

	testing.ContextLog(ctx, "alert dialog data: ", smsDetails)
	strPattern := regexp.MustCompile(`\s+`)
	smsReceived := strPattern.ReplaceAllString(smsDetails.Name, " ")
	smsSent := strPattern.ReplaceAllString(messageSent, " ")

	testing.ContextLog(ctx, "smsReceived: ", smsReceived)
	testing.ContextLog(ctx, "smsSent: ", smsSent)
	if strings.Contains(smsReceived, smsSent) {
		testing.ContextLog(ctx, "success message received")
		return nil
	}

	return errors.Wrap(err, "notification does not contain sent sms")
}

// ValidateSuppressedMessage validates that the network log reports a suppressed text message.
func (h *UIHelper) ValidateSuppressedMessage(ctx context.Context) error {
	const networkSection = "network_event_log"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		logs, err := systemlogs.GetSystemLogs(ctx, h.Tconn, networkSection)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read network_event_log from system logs"))
		}

		const errorKey = "Suppressing text message"
		lines := strings.Split(logs, "\n")
		for _, l := range lines {
			if strings.Contains(l, errorKey) {
				return nil
			}
		}

		return errors.Wrap(err, "supress message log not found in network logs")
	}, &testing.PollOptions{
		Timeout: 3 * time.Minute,
	}); err != nil {
		return err
	}

	return nil
}

// UploadCsvSimLockPortal logs in to the prod SimLock portal and uploads the CSV file to lock/unlock
// device.
func (h *UIHelper) UploadCsvSimLockPortal(ctx context.Context, simlockConfigCsvFilePath string) error {
	testing.ContextLog(ctx, "open SimLockPortal web url: ", simLockPortalURL)
	// Open sim lock portal, set new window to true to be first tab
	driverconn, err := h.UIHandler.NewChromeTab(ctx, h.Cr.Browser(), simLockPortalURL, true)
	if err != nil {
		return errors.Wrap(err, "failed to open simLockPortal web page")
	}

	defer driverconn.Close()
	defer driverconn.CloseTarget(ctx)

	if err := webutil.WaitForRender(ctx, driverconn, 2*time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for render to finish")
	}

	if err := webutil.WaitForQuiescence(ctx, driverconn, 2*time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for simLockPortal to finish loading")
	}

	testing.ContextLog(ctx, "click on uplaod button")
	uploadButton := nodewith.Name(" UPLOAD").Role(role.Button)
	if err := uiauto.Combine("Click on upload button",
		h.UI.WithTimeout(30*time.Second).WaitUntilExists(uploadButton),
		h.UI.LeftClick(uploadButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click on upload button")
	}

	fp, err := filepicker.Find(ctx, h.Tconn)
	if err != nil {
		return errors.Wrap(err, "failed to find file picker")
	}
	fp.OpenFile(simlockConfigCsvFilePath)(ctx)
	// GoBigSleepLint: wait for processing to complete
	testing.Sleep(ctx, 30*time.Second)

	return nil
}
