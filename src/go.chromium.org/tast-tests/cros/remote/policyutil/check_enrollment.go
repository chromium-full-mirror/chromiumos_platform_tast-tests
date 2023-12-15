// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/services/cros/baserpc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
)

const installAttributesPath = "/run/lockbox/install_attributes.pb"
const ownerKeyPath = "/var/lib/devicesettings/owner.key"

func checkEnrollment(ctx context.Context, rpcClient *rpc.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	pc := baserpc.NewFileSystemClient(rpcClient.Conn)

	if res, err := pc.Stat(ctx, &baserpc.StatRequest{
		Name: installAttributesPath,
	}); err != nil {
		return errors.Wrap(err, "failed to get status of install_attributes.pb")
	} else if res.Error != nil {
		return errors.Errorf("install_attributes.pb missing, enrollment likely lost, check if chrome.KeepEnrollment() was passed: %v", res.Error)
	}

	if res, err := pc.Stat(ctx, &baserpc.StatRequest{
		Name: ownerKeyPath,
	}); err != nil {
		return errors.Wrap(err, "failed to get status of owner.key")
	} else if res.Error != nil {
		return errors.Errorf("owner.key missing, enrollment likely lost, check if chrome.KeepEnrollment() was passed: %v", res.Error)
	}

	return nil
}
