// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package updateengine provides ways to interact with update_engine daemon and utilities.
package updateengine

import (
	"context"
	"regexp"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/testexec"
	ue "go.chromium.org/tast-tests/cros/common/updateengine"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	aupb "go.chromium.org/tast-tests/cros/services/cros/autoupdate"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Update Engine related constants.
const (
	ClientBin   = "update_engine_client"
	JobName     = "update-engine"
	ServiceName = "org.chromium.UpdateEngine"
)

// StartDaemon will start the daemon, ignoring already running.
func StartDaemon(ctx context.Context) error {
	testing.ContextLog(ctx, "start daemon: ", JobName)
	return upstart.EnsureJobRunning(ctx, JobName)
}

// StopDaemon will stop the daemon.
func StopDaemon(ctx context.Context) error {
	testing.ContextLog(ctx, "stop daemon: ", JobName)
	return upstart.StopJob(ctx, JobName)
}

// RestartDaemon will restart the daemon.
func RestartDaemon(ctx context.Context) error {
	testing.ContextLog(ctx, "restart daemon: ", JobName)
	if err := upstart.RestartJob(ctx, JobName); err != nil {
		return errors.Wrapf(err, "failed to restart %s", JobName)
	}
	if err := WaitForService(ctx); err != nil {
		return errors.Wrapf(err, "failed to wait for service: %s", JobName)
	}
	return nil
}

// WaitForService waits for the update-engine DBus service to be available.
func WaitForService(ctx context.Context) error {
	testing.ContextLog(ctx, "wait for service: ", JobName, " to be available")
	if bus, err := dbusutil.SystemBus(); err != nil {
		return errors.Wrap(err, "failed to connect to the message bus")
	} else if err := dbusutil.WaitForService(ctx, bus, ServiceName); err != nil {
		return errors.Wrapf(err, "failed to wait for D-Bus service %s", ServiceName)
	}
	return nil
}

var reUpdateEngineStatus = regexp.MustCompile(`(?s)CURRENT_OP=(.*?)[\r\n]+.*LAST_CHECKED_TIME=(.*?)[\r\n]+.*PROGRESS=(.*?)[\r\n]+`)

// Status calls the DBus method to fetch update_engine's status.
func Status(ctx context.Context) (*aupb.StatusResult, error) {
	testing.ContextLog(ctx, "status: getting status from ", JobName)

	buf, err := testexec.CommandContext(ctx, ClientBin, "--status").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, err
	}

	match := reUpdateEngineStatus.FindStringSubmatch(string(buf))
	if match == nil {
		return nil, errors.New("status: failed to parse update engine status")
	}

	currentOP := string(match[1])
	LastCheckedTime, err := strconv.ParseInt(string(match[2]), 10, 64)
	if err != nil {
		return nil, errors.New("status: failed to parse last checked time")
	}
	progress, err := strconv.ParseFloat(string(match[3]), 64)
	if err != nil {
		return nil, errors.New("status: failed to parse update progress value")
	}

	return &aupb.StatusResult{
		LastCheckedTime:  LastCheckedTime,
		Progress:         progress,
		CurrentOperation: currentOP,
	}, nil
}

// FeatureEnabled calls the DBus method to see if a feature in update_engine is enabled.
func FeatureEnabled(ctx context.Context, feature ue.Feature) (bool, error) {
	testing.ContextLog(ctx, "is feature enabled: ", feature)

	buf, err := testexec.CommandContext(ctx, ClientBin, "--is_feature_enabled="+string(feature)).Output(testexec.DumpLogOnError)
	if err != nil {
		return false, errors.Wrap(err, "failed to get feature")
	}

	b, err := strconv.ParseBool(string(buf))
	if err != nil {
		return false, errors.Wrap(err, "failed to parse bool")
	}

	return b, nil
}

// ToggleFeature calls the DBus method to toggle a feature in update_engine.
func ToggleFeature(ctx context.Context, feature ue.Feature, enable bool) error {
	testing.ContextLog(ctx, "toggle feature: ", feature, " to ", enable)

	var arg string
	if enable {
		arg = "--enable_feature=" + string(feature)
	} else {
		arg = "--disable_feature=" + string(feature)
	}

	if err := testexec.CommandContext(ctx, ClientBin, arg).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to toggle feature")
	}

	return nil
}
