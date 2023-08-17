// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package identitycuj provides functions that help with testing of Critical
// User Journeys (CUJs) in CrOS Identity.
package identitycuj

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

const (
	longerUIWaitTime          = time.Minute
	testAccountManagerTimeout = 5 * time.Second
)

// CreateManagedDeviceAccount logs in to a managed account and logs out to create a user pod on the login screen.
func CreateManagedDeviceAccount(ctx context.Context, credsJSON []byte) (*tape.OwnedTestAccount, error) {
	var creds credconfig.Creds

	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	timeout := int32(testAccountManagerTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManager(ctx, credsJSON, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create an account manager and lease an account")
	}
	defer accManager.CleanUp(ctxForCleanup)

	creds = credconfig.Creds{User: acc.Username, Pass: acc.Password}
	cr, err := chrome.New(ctx, chrome.GAIALogin(creds))
	if err != nil {
		return nil, errors.Wrap(err, "chrome login failed")
	}
	cr.Close(ctx)
	return acc, nil
}

// OpenNewURL opens a new Chrome tab with the given url.
func OpenNewURL(ctx context.Context, cr *chrome.Chrome, url string) (*chrome.Conn, error) {
	br := cr.Browser()
	newConn, err := br.NewConn(ctx, url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open URL %v", url)
	}
	if err := webutil.WaitForQuiescence(ctx, newConn, longerUIWaitTime); err != nil {
		return newConn, errors.Wrap(err, "failed to wait for page to finish loading")
	}
	return newConn, nil
}
