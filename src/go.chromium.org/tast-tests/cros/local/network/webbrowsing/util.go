// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package webbrowsing contains the utils related to testing web browsing in
// Chrome.
package webbrowsing

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/httpserver"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// StartSimpleHTTPServer starts an HTTP server (listening on both IPv4 and IPv6)
// which respond content on every request in virtualnet Env.
func StartSimpleHTTPServer(ctx context.Context, env *virtualnet.Env, content string) error {
	responder := func(rw http.ResponseWriter, req *http.Request) {
		if _, err := rw.Write([]byte(content)); err != nil {
			testing.ContextLog(ctx, "Failed to write response in HTTP server: ", err)
		}
	}
	if err := env.StartServer(ctx, "http4", httpserver.New(httpserver.TCP4, "80", responder, nil)); err != nil {
		return errors.Wrap(err, "failed to start HTTP server on IPv4")
	}
	if err := env.StartServer(ctx, "http6", httpserver.New(httpserver.TCP6, "80", responder, nil)); err != nil {
		return errors.Wrap(err, "failed to start HTTP server on IPv6")
	}
	return nil
}

// VerifyWebPageContains opens url in cr, waits for the page loading finished,
// and checks if the page contains pattern by a substring match.
func VerifyWebPageContains(ctx context.Context, cr *chrome.Chrome, url, content string) error {
	// Have a shorter ctx to limit the execution time of this function.
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		return errors.Wrap(err, "failed to create Chrome connection")
	}
	defer conn.Close()

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "failed to wait for page to load")
	}

	gotContent, err := conn.PageContent(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get page content")
	}

	if !strings.Contains(gotContent, content) {
		// Do not include gotContent in the error msg since it can be very long. In
		// general a screenshot should be available in the failure case which can be
		// used for debugging. It might be helpful to extract the error code from
		// the page and include it in the msg.
		return errors.Wrapf(err, "unexpected page content: want `%s` in the output", content)
	}

	return nil
}
