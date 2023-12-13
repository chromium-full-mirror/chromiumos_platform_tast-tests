// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util contains RMAResources which manges Shimless RMA resources.
package util

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/shimlessrmaapp"
)

// RMAResources holds the resources required for Shimless RMA test.
type RMAResources struct {
	cr  *chrome.Chrome
	app *shimlessrmaapp.RMAApp
}

// InitResource initializes RMA resources.
func InitResource(ctx context.Context, manifestKey, state string) (*RMAResources, *shimlessrmaapp.RMAApp, error) {
	// Make sure rmad is not currently running.
	testexec.CommandContext(ctx, "stop", "rmad").Run()
	if state == "" {
		// Create a valid empty rmad state file.
		if err := shimlessrmaapp.CreateEmptyStateFile(); err != nil {
			return nil, nil, err
		}
	} else {
		if err := shimlessrmaapp.CreateStateFile(state); err != nil {
			return nil, nil, err
		}
	}

	// Open Chrome with Shimless RMA enabled.
	cr, err := chrome.New(ctx, chrome.EnableFeatures("ShimlessRMAFlow"),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(manifestKey),
		chrome.ExtraArgs("--launch-rma"))

	if err != nil {
		return nil, nil, err
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Create Shimless RMA app helper.
	app, err := shimlessrmaapp.App(ctx, tconn)
	if err != nil {
		return nil, nil, err
	}
	r := &RMAResources{cr, app}
	return r, app, nil
}

// DisposeResource dispose RMA resources.
func (r *RMAResources) DisposeResource(ctx context.Context) {
	shimlessrmaapp.RemoveStateFile()
	testexec.CommandContext(ctx, "stop", "rmad").Run()
	r.app.Close(ctx)
	r.cr.Close(ctx)
}
