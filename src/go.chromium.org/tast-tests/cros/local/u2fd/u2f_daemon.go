// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package u2fd

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"

	u2f "go.chromium.org/chromiumos/system_api/u2f_proto"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 200 * time.Millisecond})
}
