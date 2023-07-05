// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package netexport

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// This file declares the various internal implementations used by `NetExport`.
// We have different types of controllers each with their own implementation of
// starting and stopping a net export session, based on the different ways to
// control a net export session:
//   1. chrome://net-export -> chromeController
//   2. os://net-export     -> osController

type controller interface {
	Start() error
	Stop(ctx context.Context) error
}

// chromeController starts and stops net exports using chrome://net-export. It
// logs lacros (browser) annotations in lacros mode, and all annotations in ash
// mode.
type chromeController struct {
	ctx context.Context
	cr  *chrome.Chrome
	br  *browser.Browser
}

func (c chromeController) Start() error {
	return annotations.StartLogging(c.ctx, c.cr, c.br, false)
}

func (c chromeController) Stop(ctx context.Context) error {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	// Open the net-export page.
	netConn, err := annotations.NewNetExportConn(ctx, c.br)
	if err != nil {
		return errors.Wrap(err, "failed to load chrome://net-export")
	}
	defer netConn.Close()

	// Click Stop Logging button.
	stopButton := nodewith.Name("Stop Logging").Role(role.Button)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Stop net export session",
		ui.WaitUntilExists(stopButton),
		ui.DoDefault(stopButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to stop net export session")
	}

	return nil
}

// osController starts and stops net exports using os://net-export. It
// logs ash (OS) annotations in lacros mode.
type osController struct {
	ctx context.Context
	cr  *chrome.Chrome
	br  *browser.Browser
	kb  *input.KeyboardEventWriter
}

func (c osController) Start() error {
	return annotations.StartOSLogging(c.ctx, c.cr, c.br, c.kb)
}

func (c osController) Stop(ctx context.Context) error {
	defer c.kb.Close(ctx)

	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	// Open the net-export page.
	netConn, err := annotations.NewOSNetExportConn(ctx, c.br, tconn, c.kb)
	if err != nil {
		return errors.Wrap(err, "failed to load "+annotations.OsNetExportURL)
	}
	defer netConn.Close()

	stopButton := nodewith.Name("Stop Logging").Role(role.Button)
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Stop net export session",
		ui.WaitUntilExists(stopButton),
		ui.DoDefault(stopButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to stop net export session")
	}

	return nil
}
