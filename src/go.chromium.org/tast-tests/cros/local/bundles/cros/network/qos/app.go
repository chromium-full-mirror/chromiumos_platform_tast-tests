// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package qos

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// APK is the APK used to install test apps.
type APK string

// TestApp represents a test app for QoS tests.
type TestApp struct {
	apk          APK
	a            *arc.ARC
	appInstalled bool
	appStarted   bool
}

const (
	// ProductivityAPK is the apk name for test app of productivity type.
	ProductivityAPK APK = "ArcQosMonitorTest.apk"
	// GameAPK is the apk name for test app of game type.
	GameAPK APK = "ArcQosMonitorTest_game.apk"
	// NewsAPK is the apk name for test app of news type.
	NewsAPK APK = "ArcQosMonitorTest_news.apk"

	pkg = "org.chromium.arc.testapp.qos_monitor"
	cls = "org.chromium.arc.testapp.qos_monitor.QosMonitorActivity"

	// Intents used for setting up sockets and sending messages.
	intentSetupSocket = "org.chromium.arc.testapp.qos_monitor.SETUP_SOCKET"
	intentSendMessage = "org.chromium.arc.testapp.qos_monitor.SEND_SOCKET_MESSAGE"

	// Intents used for start and stop recording.
	intentStartRecording = "org.chromium.arc.testapp.qos_monitor.START_RECORDING"
	intentStopRecording  = "org.chromium.arc.testapp.qos_monitor.STOP_RECORDING"
)

// NewTestApp creates a new test app of given APK. The caller should install
// and start the app before any other operations and should defer a call to
// TearDown() after this function returns.
func NewTestApp(apk APK, a *arc.ARC) (*TestApp, error) {
	if a == nil {
		return nil, errors.New("passed in ARC cannot be nil, get test app failed")
	}
	return &TestApp{apk, a, false, false}, nil
}

// InstallAndStart installs and starts the test app.
func (app *TestApp) InstallAndStart(ctx context.Context) error {
	testing.ContextLog(ctx, "Installing app")
	if err := app.a.Install(ctx, arc.APKPath(string(app.apk))); err != nil {
		return errors.Wrap(err, "failed to install the APK")
	}
	app.appInstalled = true

	testing.ContextLog(ctx, "Starting app")
	if err := app.a.Command(ctx, "am", "start", "-W", pkg+"/"+cls).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to start app")
	}
	app.appStarted = true
	return nil
}

// TearDown uninstalls the test app.
func (app *TestApp) TearDown(ctx context.Context) error {
	if !app.appInstalled {
		return nil
	}
	testing.ContextLog(ctx, "Uninstalling QoS test app")
	if err := app.a.Uninstall(ctx, string(app.apk)); err != nil {
		return errors.Wrap(err, "failed to uninstall QoS test app")
	}
	return nil
}

// SendMessage sends out a message through latest setup socket.
func (app *TestApp) SendMessage(ctx context.Context, msg string) error {
	if !app.appStarted {
		return errors.New("test app has not been installed and started yet, send message failed")
	}
	if _, err := app.a.BroadcastIntent(ctx,
		intentSendMessage,
		"--es", "message", msg); err != nil {
		return errors.Wrap(err, "send message failed")
	}
	return nil
}

// SetupSocket sets up a socket connection which use family for the local
// socket to connect to the remote address:port"
func (app *TestApp) SetupSocket(ctx context.Context, family l4server.Family, address string, port int) error {
	if !app.appStarted {
		return errors.New("test app has not been installed and started yet, setup socket failed")
	}
	if family == l4server.TCP6 || family == l4server.TCP4 || family == l4server.UDP6 || family == l4server.UDP4 {
		if _, err := app.a.BroadcastIntent(ctx,
			intentSetupSocket,
			"--es", "proto", family.String(),
			"--es", "address", address,
			"--ei", "port", strconv.Itoa(port)); err != nil {
			return errors.Wrap(err, "send message failed")
		}
		return nil
	}
	return errors.Errorf("network family not supported: %v", family)
}

// StartRecording starts recording, used for simulating the start of a web
// conference when using productivity app.
func (app *TestApp) StartRecording(ctx context.Context) error {
	if !app.appStarted {
		return errors.New("test app has not been installed and started yet, failed to start recording")
	}
	testing.ContextLog(ctx, "Start recording")
	// Grant RECORD_AUDIO permission through adb command, which is needed for
	// starting recording.
	if err := app.a.Command(ctx, "pm", "grant", pkg, "android.permission.RECORD_AUDIO").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to grant RECORD_AUDIO premission")
	}
	if _, err := app.a.BroadcastIntent(ctx, intentStartRecording); err != nil {
		return errors.Wrap(err, "start recording failed")
	}
	return nil
}

// StopRecording stops recording, used for simulating the end of a web
// conference when using productivity app.
func (app *TestApp) StopRecording(ctx context.Context) error {
	if !app.appStarted {
		return errors.New("test app has not been installed and started yet, failed to stop recording")
	}
	testing.ContextLog(ctx, "Stop recording")
	if _, err := app.a.BroadcastIntent(ctx, intentStopRecording); err != nil {
		return errors.Wrap(err, "start recording failed")
	}
	return nil
}
