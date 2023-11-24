// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package modemloggerd

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/modemloggerdconst"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

// ModemLogger wraps a Modemloggerd.Modem DBus object.
type ModemLogger struct {
	*dbusutil.DBusObject
}

// NewModemLogger returns a DBusObject to help with logging the first modem exported by modemloggerd.
func NewModemLogger(ctx context.Context) (*ModemLogger, error) {
	modemLoggerPaths, err := GetModemLoggerPaths(ctx)
	if err != nil {
		return nil, err
	}
	if len(modemLoggerPaths) == 0 {
		return nil, errors.New("logging not supported by any modem")
	}
	obj, err := dbusutil.NewDBusObject(ctx, modemloggerdconst.DBusModemloggerdService, modemloggerdconst.DBusModemloggerdModemInterface, modemLoggerPaths[0])
	if err != nil {
		return nil, errors.Wrap(err, "unable to get ModemLogger object")
	}
	return &ModemLogger{obj}, nil
}

// Start triggers logging on the modem.
func (m *ModemLogger) Start(ctx context.Context) error {
	if err := m.DBusObject.Call(ctx, modemloggerdconst.ModemMethodStart).Err; err != nil {
		return errors.Wrap(err, "failed to start logging")
	}
	return nil
}

// Stop ends modem logging.
func (m *ModemLogger) Stop(ctx context.Context) error {
	if err := m.DBusObject.Call(ctx, modemloggerdconst.ModemMethodStop).Err; err != nil {
		return errors.Wrap(err, "failed to start logging")
	}
	return nil
}

// SetOutputDir sets the directory where modem logs are written to.
func (m *ModemLogger) SetOutputDir(ctx context.Context, outputDir string) error {
	if err := m.DBusObject.Call(ctx, modemloggerdconst.ModemMethodSetOutputDir, outputDir).Err; err != nil {
		return errors.Wrap(err, "failed to set output directory")
	}
	return nil
}

// SetEnabled sets up/ tears down modem logging functionality.
func (m *ModemLogger) SetEnabled(ctx context.Context, enable bool) error {
	if err := m.DBusObject.Call(ctx, modemloggerdconst.ModemMethodSetEnabled, enable).Err; err != nil {
		return errors.Wrap(err, "failed to call SetEnabled")
	}
	return nil
}