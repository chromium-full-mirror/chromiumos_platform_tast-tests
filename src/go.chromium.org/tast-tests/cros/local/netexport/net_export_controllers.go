// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package netexport

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// This file contains a controller interface and implementation used by
// `NetExport`. It allows for starting and stopping a net export session.

type controller interface {
	Start() error
	Stop(ctx context.Context) error
}

// chromeController starts and stops net exports using chrome://net-export.
type chromeController struct {
	ctx context.Context
	cr  *chrome.Chrome
}

func (c chromeController) Start() error {
	return annotations.StartLogging(c.ctx, c.cr, false)
}

func (c chromeController) Stop(ctx context.Context) error {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	// Open the net-export page.
	netConn, err := annotations.NewNetExportConn(ctx, c.cr)
	if err != nil {
		return errors.Wrap(err, "failed to load chrome://net-export")
	}
	defer netConn.Close()

	// Click Stop Logging button.
	stopButton := nodewith.Name("Stop Logging").Role(role.Button).First()
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Stop net export session",
		ui.WaitUntilExists(stopButton),
		ui.DoDefault(stopButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to stop net export session")
	}

	return nil
}
