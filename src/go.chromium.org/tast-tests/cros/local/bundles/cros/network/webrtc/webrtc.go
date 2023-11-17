// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package webrtc contains the code for interacting with the webpage located in
// ../data/webrtc.
package webrtc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

const (
	htmlFile = "qos/webrtc.html"
	jsFile   = "qos/webrtc.js"
)

// TestFiles returns the files required running the test.
func TestFiles() []string {
	return []string{
		htmlFile,
		jsFile,
	}
}

type env struct {
	cr     *chrome.Chrome
	server *httptest.Server
	conn   *chrome.Conn
}

// NewEnv creates a new test env for WebRTC networking tests. The caller should
// defer a call to TearDown() after this function returns.
func NewEnv() *env {
	return &env{}
}

// SetUp starts (or restarts) Chrome with the required args for the WebRTC
// tests, and a HTTP server hosted with httpServerRoot, which should contains
// TestFiles().
func (e *env) SetUp(ctx context.Context, httpServerRoot http.FileSystem) error {
	cr, err := chrome.New(
		ctx,
		// Avoid the need to grant camera/microphone permissions.
		chrome.ExtraArgs("--auto-accept-camera-and-microphone-capture"),
	)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}

	e.cr = cr
	e.server = httptest.NewServer(http.FileServer(httpServerRoot))
	return nil
}

// StartConn opens a web page which contains two RTCPeerConnection, where the
// first connection ("local") will stream video and audio to the second
// connection ("remote"). We simulate a NAT environment, so the local peer will
// talk to remoteIPAfterNAT and the remote peer will talk to localIPAfterNAT.
// The caller should guarantee that the routing setup is done before calling
// this function to make them reachable.
func (e *env) StartConn(ctx context.Context, localIPAfterNAT, remoteIPAfterNAT string) error {
	url := e.server.URL + "/" + htmlFile
	conn, err := e.cr.NewConn(ctx, url)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", url)
	}

	e.conn = conn

	// Shorten the ctx for establishing the WebRTC connection.
	d := time.Now().Add(10 * time.Second)
	ctx, cancel := context.WithDeadline(ctx, d)
	defer cancel()

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "failed to wait for page to load")
	}

	evalStr := fmt.Sprintf("start('%s', '%s')", localIPAfterNAT, remoteIPAfterNAT)
	if err := conn.Eval(ctx, evalStr, nil); err != nil {
		return errors.Wrap(err, "failed to start the WebRTC connection")
	}

	// Check if the peer connections are connected.
	if err := conn.WaitForExpr(ctx, "localPeerConnection.connectionState === 'connected'"); err != nil {
		return errors.Wrap(err, "failed to wait for local peer connection connected")
	}
	if err := conn.WaitForExpr(ctx, "remotePeerConnection.connectionState === 'connected'"); err != nil {
		return errors.Wrap(err, "failed to wait for remote peer connection connected")
	}

	return nil
}

// TearDown cleans up the resources.
func (e *env) TearDown(ctx context.Context) {
	if e.conn != nil {
		e.conn.CloseTarget(ctx)
		e.conn.Close()
	}

	if e.server != nil {
		e.server.Close()
	}
}
