// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package update contains helpers for update related testing.
package update

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	aupb "go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	"go.chromium.org/tast-tests/cros/services/cros/nebraska"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// TriggerUpdateAndCheckNebraskaLogs triggers an update to a locally started
// nebraska service and checks that the logs contain the expected responses.
func TriggerUpdateAndCheckNebraskaLogs(ctx context.Context, cl *rpc.Client, expectedParameters []string) error {
	// Trigger a request to nebraska.
	nebraskaClient := nebraska.NewServiceClient(cl.Conn)
	updateClient := aupb.NewUpdateServiceClient(cl.Conn)

	startResponse, err := nebraskaClient.Start(ctx, &nebraska.StartRequest{})
	if err != nil {
		return errors.Wrap(err, "failed to start nebraska")
	}
	defer nebraskaClient.Stop(ctx, &empty.Empty{})

	if _, err := updateClient.CheckForUpdate(ctx, &aupb.UpdateRequest{
		OmahaUrl:  fmt.Sprintf("http://127.0.0.1:%d/update", startResponse.Port),
		CheckOnly: true,
	}); err != nil {
		return errors.Wrap(err, "failed to check for udpate")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		logResponse, err := nebraskaClient.ReadLog(ctx, &empty.Empty{})
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read nebraska logs"))
		}

		for _, expected := range expectedParameters {
			if !strings.Contains(string(logResponse.Data), expected) {
				return errors.Errorf("%q not in the nebraska logs", expected)
			}
		}

		return nil
	}, &testing.PollOptions{
		Timeout: 5 * time.Second,
	}); err != nil {
		return err
	}

	return nil
}
