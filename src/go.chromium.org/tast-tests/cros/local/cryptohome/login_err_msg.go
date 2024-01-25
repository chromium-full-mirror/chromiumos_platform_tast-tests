// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

const (
	authenticateStarted             = "AuthenticateStarted"
	authenticateAuthFactorCompleted = "AuthenticateAuthFactorCompleted"
	mountStarted                    = "MountStarted"
	mountCompleted                  = "MountCompleted"
)

// LoginSignals defines all signals during login.
type LoginSignals struct {
	authStart      *loginSignal
	authCompleted  *loginSignal
	mountStart     *loginSignal
	mountCompleted *loginSignal
}

type loginSignal struct {
	watcher   *dbusutil.SignalWatcher
	found     bool // It indicates whether the signal is found
	errorCode uda.CryptohomeErrorCode
	errorInfo string
}

// NewLoginSignals creates login signals.
func NewLoginSignals(ctx context.Context) (*LoginSignals, error) {
	authStartSignal, err := newLoginSignal(ctx, authenticateStarted)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create %s loginSignal", authenticateStarted)
	}

	authCompletedSignal, err := newLoginSignal(ctx, authenticateAuthFactorCompleted)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create %s loginSignal", authenticateAuthFactorCompleted)
	}

	mountStartSignal, err := newLoginSignal(ctx, mountStarted)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create %s loginSignal", mountStarted)
	}

	mountCompletedSignal, err := newLoginSignal(ctx, mountCompleted)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create %s loginSignal", mountCompleted)
	}

	return &LoginSignals{
		authStart:      authStartSignal,
		authCompleted:  authCompletedSignal,
		mountStart:     mountStartSignal,
		mountCompleted: mountCompletedSignal,
	}, nil
}

// ErrorMessage returns the error message.
func (l *LoginSignals) ErrorMessage(ctx context.Context) string {
	findAuthStart, findAuthComplete, findMountStart, findMountComplete := l.authStart.found, l.authCompleted.found, l.mountStart.found, l.mountCompleted.found

	// Case 1: no signals received
	if !findAuthStart && !findAuthComplete {
		return "chrome side failed before cryptohome started, please check /var/log/message"
	}

	// Case 2: auth start but never complete.
	if findAuthStart && !findAuthComplete {
		return "authentication failed, please check /var/log/message"
	}

	// Case 3: auth complete but we find error.
	if findAuthComplete && l.authCompleted.errorInfo != "" {
		return fmt.Sprintf("authentication failed: error code: %s, error info: %s",
			uda.CryptohomeErrorCode_name[int32(l.authCompleted.errorCode)], l.authCompleted.errorInfo)
	}

	// Case 4: mount start but never complete.
	if findMountStart && !findMountComplete {
		return "mount failed, please check /var/log/message"
	}

	// Case 5: mount complete but we find error
	if findMountComplete && l.mountCompleted.errorInfo != "" {
		return fmt.Sprintf("mount failed: error code: %s, error info %s",
			uda.CryptohomeErrorCode_name[int32(l.mountCompleted.errorCode)], l.authCompleted.errorCode)
	}

	// Case 6: we don't know why it happen, just return a general error.
	return "cryptohome failed, please check /var/log/message"
}

// Close stops all signals in LoginSignals.
func (l *LoginSignals) Close(ctx context.Context) error {
	err1 := l.authStart.close(ctx)
	err2 := l.authCompleted.close(ctx)
	err3 := l.mountStart.close(ctx)
	err4 := l.mountCompleted.close(ctx)

	return errors.Join(err1, err2, err3, err4)
}

func newLoginSignal(ctx context.Context, member string) (*loginSignal, error) {
	watcher, err := dbusutil.NewSignalWatcherForSystemBus(ctx, dbusutil.MatchSpec{
		Type:      "signal",
		Path:      udaDbusPath,
		Interface: udaDbusInterface,
		Member:    member,
	})

	if err != nil {
		return nil, err
	}

	signal := &loginSignal{watcher: watcher}
	signal.found = false

	go func() {
		for {
			select {
			case sig, ok := <-watcher.Signals:
				if !ok {
					return
				}
				if member == authenticateAuthFactorCompleted {
					errorCode, errorInfo, err := parseAuthenticateAuthFactorCompleted(sig)
					if err != nil {
						signal.errorInfo = fmt.Sprintf("Failed to parse AuthenticateAuthFactorCompleted: %s", err)
					}
					signal.errorCode = errorCode
					signal.errorInfo = errorInfo.String()
				} else if member == mountCompleted {
					errorCode, errorInfo, err := parseMountCompleted(sig)
					if err != nil {
						signal.errorInfo = fmt.Sprintf("Failed to parse MountCompleted: %s", err)
					}
					signal.errorCode = errorCode
					signal.errorInfo = errorInfo.String()
				}
				signal.found = true
				return
			}
		}

	}()

	return signal, nil
}

func (s *loginSignal) close(ctx context.Context) error {
	if err := s.watcher.Close(ctx); err != nil {
		return errors.Wrap(err, "failed to close SignalWatcher after another failure")
	}
	return nil
}

func parseAuthenticateAuthFactorCompleted(sig *dbus.Signal) (uda.CryptohomeErrorCode, *uda.CryptohomeErrorInfo, error) {
	out := uda.AuthenticateAuthFactorCompleted{}
	var marshOut []byte

	if err := dbus.Store(sig.Body, &marshOut); err != nil {
		return 0, nil, errors.Wrap(err, "failed reading AuthenticateAuthFactorCompleted signal to bytes")
	}
	if err := proto.Unmarshal(marshOut, &out); err != nil {
		return 0, nil, errors.Wrap(err, "failed unmarshaling AuthenticateAuthFactorCompleted response")
	}
	errorCode, errorInfo := out.Error, out.ErrorInfo
	return errorCode, errorInfo, nil
}

func parseMountCompleted(sig *dbus.Signal) (uda.CryptohomeErrorCode, *uda.CryptohomeErrorInfo, error) {
	out := uda.MountCompleted{}
	var marshOut []byte

	if err := dbus.Store(sig.Body, &marshOut); err != nil {
		return 0, nil, errors.Wrap(err, "failed reading MountCompleted signal to bytes")
	}
	if err := proto.Unmarshal(marshOut, &out); err != nil {
		return 0, nil, errors.Wrap(err, "failed unmarshaling MountCompleted response")
	}
	errorCode, errorInfo := out.Error, out.ErrorInfo
	return errorCode, errorInfo, nil
}
