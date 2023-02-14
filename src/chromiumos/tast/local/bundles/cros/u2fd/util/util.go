// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	u2f "chromiumos/system_api/u2f_proto"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/lockscreen"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/dbusutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

const (
	jobName       = "u2fd"
	dbusName      = "org.chromium.U2F"
	dbusPath      = "/org/chromium/U2F"
	dbusInterface = "org.chromium.U2F"
)

// U2fDaemon is used to interact with the u2fd process over D-Bus.
type U2fDaemon struct {
	conn *dbus.Conn
	obj  dbus.BusObject
}

// NewU2fDaemon connects to the u2f daemon via D-Bus and returns a U2fDaemon object.
func NewU2fDaemon(ctx context.Context) (*U2fDaemon, error) {
	if err := upstart.EnsureJobRunning(ctx, jobName); err != nil {
		return nil, err
	}

	conn, obj, err := dbusutil.Connect(ctx, dbusName, dbusPath)
	if err != nil {
		return nil, err
	}
	return &U2fDaemon{conn, obj}, nil
}

// callProtoMethod is a thin wrapper of CallProtoMethod for convenience.
func (d *U2fDaemon) callProtoMethod(ctx context.Context, method string, in, out proto.Message) error {
	return dbusutil.CallProtoMethod(ctx, d.obj, dbusInterface+"."+method, in, out)
}

// IsInitialized calls the IsInitialized D-Bus method.
func (d *U2fDaemon) IsInitialized(ctx context.Context) (bool, error) {
	request := &u2f.IsPlatformAuthenticatorInitializedRequest{}
	response := &u2f.IsPlatformAuthenticatorInitializedResponse{}
	err := d.callProtoMethod(ctx, "IsPlatformAuthenticatorInitialized", request, response)
	return response.Initialized, err
}

// WaitUntilInitialized polls the IsInitialized method until it returns true.
func (d *U2fDaemon) WaitUntilInitialized(ctx context.Context) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		initialized, err := d.IsInitialized(ctx)
		if err != nil {
			return err
		}
		if !initialized {
			return errors.New("u2fd isn't initialized yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond})
}

// SetUpUserPIN sets up a test user with a specific PIN.
func SetUpUserPIN(ctx context.Context, cr *chrome.Chrome, keyboard *input.KeyboardEventWriter, PIN, password string, autosubmit bool) (*chrome.TestConn, error) {
	user := cr.NormalizedUser()
	if mounted, err := cryptohome.IsMounted(ctx, user); err != nil {
		return nil, errors.Wrapf(err, "failed to check mounted vault for %q", user)
	} else if !mounted {
		return nil, errors.Wrapf(err, "no mounted vault for %q", user)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting test API connection failed")
	}

	// Set up PIN through a connection to the Settings page.
	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch Settings app")
	}

	if err := settings.EnablePINUnlock(cr, password, PIN, autosubmit)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to enable PIN unlock")
	}

	if err := verifyPINUnlock(ctx, tconn, keyboard, PIN, autosubmit); err != nil {
		return nil, errors.Wrap(err, "PIN unlock doesn't work so IsUvpaa will be false")
	}

	return tconn, nil
}

func verifyPINUnlock(ctx context.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, PIN string, autosubmit bool) error {
	// Lock the screen.
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to lock the screen")
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.Locked && st.ReadyForPassword }, 30*time.Second); err != nil {
		return errors.Wrapf(err, "waiting for screen to be locked failed (last status %+v)", st)
	}

	// Enter and submit the PIN to unlock the DUT.
	if err := lockscreen.EnterPIN(ctx, tconn, keyboard, PIN); err != nil {
		return errors.Wrap(err, "failed to enter PIN")
	}

	if !autosubmit {
		if err := lockscreen.SubmitPINOrPassword(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to submit PIN")
		}
	}

	if st, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return !st.Locked }, 30*time.Second); err != nil {
		return errors.Wrapf(err, "waiting for screen to be unlocked failed (last status %+v)", st)
	}
	return nil
}
