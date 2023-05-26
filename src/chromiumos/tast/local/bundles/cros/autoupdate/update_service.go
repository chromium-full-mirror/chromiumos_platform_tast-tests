// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/local/updateengine"
	"go.chromium.org/tast-tests/cros/common/testexec"
	ue "go.chromium.org/tast-tests/cros/common/updateengine"
	aupb "go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			aupb.RegisterUpdateServiceServer(srv, &UpdateService{s: s})
		},
	})
}

// UpdateService implements tast.cros.autoupdate.UpdateService.
type UpdateService struct { // NOLINT
	s *testing.ServiceState
}

const statefulPath = "/mnt/stateful_partition/etc/lsb-release"

// CheckForUpdate starts update_engine_client to update the OS.
func (u *UpdateService) CheckForUpdate(ctx context.Context, req *aupb.UpdateRequest) (*empty.Empty, error) {
	// Collect the arguments.
	var args []string
	if req.CheckOnly {
		args = append(args, "--check_for_update")
	} else {
		args = append(args, "--update")
	}

	if req.OmahaUrl != "" {
		testing.ContextLog(ctx, "Adding Omaha URL to arguments")
		args = append(args, fmt.Sprintf("--omaha_url=%s", req.OmahaUrl))
	}

	if req.AppVersion != "" {
		testing.ContextLog(ctx, "Adding app version to arguments")
		args = append(args, fmt.Sprintf("--app_version=%s", req.AppVersion))
	}

	cmd := testexec.CommandContext(ctx, "update_engine_client", args...)

	testing.ContextLog(ctx, "Starting the update")
	_, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to start update engine client")
	}

	return &empty.Empty{}, nil
}

// PeriodicCheckForUpdate triggers periodic update check to update the OS.
func (u *UpdateService) PeriodicCheckForUpdate(ctx context.Context, e *empty.Empty) (*aupb.StatusResult, error) {
	status := &aupb.StatusResult{}

	// Mark being OOBE complete, so update-engine won't block update checks.
	if err := updateengine.MarkOobeCompletion(ctx); err != nil {
		return status, err
	}

	// Temporarily stop update-engine to override background update check interval.
	if err := updateengine.StopDaemon(ctx); err != nil {
		return status, err
	}

	// No delay in performing background update check.
	if err := updateengine.SetPref(ctx, updateengine.TestUpdateCheckIntervalTimeout, "0"); err != nil {
		return status, err
	}
	defer updateengine.ForceClearPrefs(ctx)

	if err := updateengine.StartDaemon(ctx); err != nil {
		return status, err
	}
	if err := updateengine.WaitForService(ctx); err != nil {
		return status, err
	}

	// Monitor the update_engine status and wait for the update to complete.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var checkTime int64 = 0
		var err error
		status, err = updateengine.Status(ctx)
		if err != nil {
			return testing.PollBreak(err)
		} else if status.LastCheckedTime == 0 {
			return errors.New("update check was not performed when initiated a periodic update check")
		} else if checkTime > 0 && status.LastCheckedTime > checkTime {
			return testing.PollBreak(errors.New("another update check happened while waiting for update to complete"))
		} else {
			checkTime = status.LastCheckedTime

			switch status.CurrentOperation {
			case string(ue.UpdateStatusIdle):
				return testing.PollBreak(errors.New("update status changed to IDLE unexpectedly"))
			case string(ue.UpdateStatusReportingErrorEvent):
				return testing.PollBreak(errors.New("error happened during update"))
			case string(ue.UpdateStatusUpdatedNeedReboot), string(ue.UpdateStatusUpdatedButDeferred):
				return nil
			default:
				return errors.Errorf("update in progress, current operation: %s, progress: %f", status.CurrentOperation, status.Progress)
			}
		}
	}, &testing.PollOptions{Timeout: time.Minute * 15, Interval: time.Second * 5}); err != nil {
		return status, err
	}

	return status, nil
}

// ResetUpdateEngine stops update engine, clears the prefs and restarts it to
// ensure the DUT is left in a clean state after an update attempt.
func (u *UpdateService) ResetUpdateEngine(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	testing.ContextLog(ctx, "Resetting update engine status")
	if err := updateengine.ClearPrefs(ctx); err != nil {
		return &empty.Empty{}, errors.Wrap(err, "failed to reset udpate engine status")
	}
	return &empty.Empty{}, nil
}

// EnsureUpdateEngineReady checks that update engine is running and idle.
func (u *UpdateService) EnsureUpdateEngineReady(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	testing.ContextLog(ctx, "Ensuring update engine is ready")
	latestStatus := &aupb.StatusResult{}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		latestStatus, err = updateengine.Status(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get update engine status, latest polled status was %q", latestStatus.CurrentOperation)
		}

		if latestStatus.CurrentOperation != string(ue.UpdateStatusIdle) {
			return errors.Wrapf(err, "update engine is not ready yet, current status is %q", latestStatus.CurrentOperation)
		}

		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return &empty.Empty{}, err
	}

	return &empty.Empty{}, nil
}

// LSBReleaseContent gets the content of /etc/lsb-release.
func (u *UpdateService) LSBReleaseContent(ctx context.Context, req *empty.Empty) (*aupb.LSBRelease, error) {
	content, err := lsbrelease.Load()
	if err != nil {
		return &aupb.LSBRelease{}, errors.Wrap(err, "failed to load lsbrelease information from /etc/lsb-release")
	}

	contentJSON, err := json.Marshal(content)
	if err != nil {
		return &aupb.LSBRelease{}, errors.Wrap(err, "failed to serialize the content of /etc/lsb-release")
	}

	return &aupb.LSBRelease{ContentJson: contentJSON}, nil
}

// StatefulLSBReleaseContent gets the content of /mnt/stateful_partition/etc/lsb-release.
// The values in this file overwrite the effect of the ones in /etc/lsb-release.
func (u *UpdateService) StatefulLSBReleaseContent(ctx context.Context, req *empty.Empty) (*aupb.LSBRelease, error) {
	content, err := lsbrelease.LoadFrom(statefulPath)
	if err != nil {
		return &aupb.LSBRelease{}, errors.Wrapf(err, "failed to retreive lsbrelease information from %s", statefulPath)
	}

	contentJSON, err := json.Marshal(content)
	if err != nil {
		return &aupb.LSBRelease{}, errors.Wrapf(err, "failed to serialize the content of %s", statefulPath)
	}

	return &aupb.LSBRelease{ContentJson: contentJSON}, nil
}

// OverwriteStatefulLSBRelease overwrites the content of /mnt/stateful_partition/etc/lsb-release.
func (u *UpdateService) OverwriteStatefulLSBRelease(ctx context.Context, req *aupb.LSBRelease) (*empty.Empty, error) {
	var content map[string]string
	if err := json.Unmarshal(req.ContentJson, &content); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal the request content")
	}

	output := new(bytes.Buffer)
	for key, value := range content {
		fmt.Fprintf(output, "%s=%s\n", key, value)
	}

	err := ioutil.WriteFile(statefulPath, output.Bytes(), 0644)
	if err != nil {
		return &empty.Empty{}, errors.Wrapf(err, "failed to write the new content to %s", statefulPath)
	}

	return &empty.Empty{}, nil
}
