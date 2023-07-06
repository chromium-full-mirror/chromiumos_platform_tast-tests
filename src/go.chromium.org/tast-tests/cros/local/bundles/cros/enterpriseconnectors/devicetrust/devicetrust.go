// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package devicetrust

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DefaultUITimeout is the default timeout for UI interactions.
const DefaultUITimeout = 20 * time.Second

// SandboxDMServer is the DMServer used for most tast tests.
const SandboxDMServer = "https://crosman-alpha.sandbox.google.com/devicemanagement/data/api"

// DeviceTrustFeature is the name of the Device Trust feature flag.
const DeviceTrustFeature = "DeviceTrustConnectorEnabled"

// FakeIdPURL is the URL of the fake IdP, which is hosted and maintained by cbe-device-trust-eng@google.com.
// It provides and endpoint for testing the Device Trust attestation flow.
const FakeIdPURL = "https://cbe-integrationtesting-sandbox.uc.r.appspot.com"

// Expected error message for Device Trust attestation flows, where the host is not allowed.
const errorMessageHostNotAllowed = "Missing X-Device-Trust header in the first request"

// StartAttestationFlowWithFakeIdP initiates a Device Trust atteststion flow with the fake IdP.
func StartAttestationFlowWithFakeIdP(ctx context.Context, ui *uiauto.Context) error {
	root := nodewith.Name("Device Trust IdP").Role(role.RootWebArea)

	startButton := nodewith.Name("Start Device Trust Attestation(using VAv2)").Role(role.Link).Ancestor(root).Focusable()
	if err := uiauto.Combine("Click on start button and proceed",
		ui.WaitUntilExists(startButton),
		ui.LeftClick(startButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to start the Device Trust attestation. Fake IdP not loaded correctly")
	}

	return nil
}

// CheckFakeIdPStatus checks if the result of the Device Trust attestation flow is as expected based on the text on the fake IdP.
func CheckFakeIdPStatus(ctx context.Context, cr *chrome.Chrome, ui *uiauto.Context, isInSession, expected bool) error {
	deviceTrustSuccessful, err := wasDeviceTrustAttestationSuccessful(ctx, ui)
	if err != nil {
		return errors.Wrap(err, "failed to check if Device Trust succeeded")
	}

	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix(FakeIdPURL+"/idp/login"))
	if err != nil {
		return errors.Wrap(err, "failed to open existing connection")
	}

	if deviceTrustSuccessful {
		if expected != deviceTrustSuccessful {
			return errors.New("Device Trust succeeded unexpectedly")
		}

		if err := checkSignals(ctx, conn, isInSession); err != nil {
			return errors.Wrap(err, "checking signals failed")
		}
	} else {
		errorMessage, err := getErrorMessage(ctx, conn)
		if err != nil {
			return errors.Wrap(err, "checking error message failed")
		}

		if expected != deviceTrustSuccessful {
			return errors.New("Device trust failed with error: " + errorMessage)
		}

		if errorMessage != errorMessageHostNotAllowed {
			return errors.Errorf("unexpected value for errorMessage: got %q, want %q", errorMessage, errorMessageHostNotAllowed)
		}
	}

	return nil
}

// getErrorMessage returns in case of an unsuccessful Device Trust attestation flow the error message, which should be displayed by the fake IdP.
func getErrorMessage(ctx context.Context, conn *chrome.Conn) (string, error) {
	var errorMessage string
	if err := conn.Call(ctx, &errorMessage, "() => { return document.getElementById('errorMessage').innerText; }"); err != nil {
		return "", err
	}

	errorMessage = strings.ReplaceAll(errorMessage, "\n", "")

	return errorMessage, nil
}

// checkSignals checks the signals for completeness and validity in case of a successful Device Trust attestation flow.
func checkSignals(ctx context.Context, conn *chrome.Conn, isInSession bool) error {
	var serverSignalsString string
	if err := conn.Call(ctx, &serverSignalsString, "() => { return document.getElementById('serverSignals').innerText; }"); err != nil {
		return errors.Wrap(err, "failed reading server signals")
	}
	if serverSignalsString == "" {
		return errors.New("Server signals were empty")
	}

	var clientSignalsString string
	if err := conn.Call(ctx, &clientSignalsString, "() => { return document.getElementById('clientSignals').innerText; }"); err != nil {
		return errors.Wrap(err, "failed reading client signals")
	}
	if clientSignalsString == "" {
		return errors.New("Client signals were empty")
	}

	return Verify([]byte(serverSignalsString), []byte(clientSignalsString), isInSession)
}

// wasDeviceTrustAttestationSuccessful analyzes the current content on the fake IdP site to decide whether the Device Trust attestation flow was successful or not.
func wasDeviceTrustAttestationSuccessful(ctx context.Context, ui *uiauto.Context) (bool, error) {
	root := nodewith.Name("Sample Login page").Role(role.RootWebArea)
	signalText := nodewith.Name("Server Signals:").Role(role.StaticText).Ancestor(root)
	errorMessage := nodewith.Name("Device Trust failed with error:").Role(role.StaticText).Ancestor(root)

	result := false
	err := testing.Poll(ctx, func(ctx context.Context) error {
		err := ui.Exists(signalText)(ctx)
		if err == nil {
			result = true
			return nil
		}
		err = ui.Exists(errorMessage)(ctx)
		if err == nil {
			result = false
			return nil
		}
		return errors.Wrap(err, "found neither the signal list nor the error message")
	}, &testing.PollOptions{Interval: 300 * time.Millisecond,
		Timeout: DefaultUITimeout})

	if err != nil {
		return false, err
	}

	return result, nil
}
