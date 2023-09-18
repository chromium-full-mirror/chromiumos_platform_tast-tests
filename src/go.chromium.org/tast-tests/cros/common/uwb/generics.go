// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

import (
	"go.chromium.org/tast/core/errors"
)

type responseInterface interface {
	GetStatus() Status
}

// CallAndCheckOK is a generic for calling the UWB D-Bus methods in both the local client and the remote gRPC. It accepts the
// return values, checks for errors and then checks that the Status received is  Status_OK
func CallAndCheckOK[T responseInterface](response T, err error) error {
	if err != nil {
		return err
	}

	if response.GetStatus() != Status_OK {
		return errors.Errorf("received status %v instead of Status_OK in DBus call", response.GetStatus())
	}
	return nil
}
