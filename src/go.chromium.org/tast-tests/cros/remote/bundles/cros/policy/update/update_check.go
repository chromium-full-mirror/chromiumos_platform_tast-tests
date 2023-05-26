// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package update contains helpers for update related testing.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/updateutil"
	aupb "go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	"go.chromium.org/tast-tests/cros/services/cros/nebraska"

	"go.chromium.org/tast/core/ctxutil"
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

// PerformUpdateAndCheckImage requests an update from Omaha and applies it.
// It then checks the installed image's content. Finally it reverts the update
// to not leave the DUT in a broken state.
func PerformUpdateAndCheckImage(ctx context.Context, cl *rpc.Client, expectedLSBReleaseRegex map[string]string) (retErr error) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	client := aupb.NewUpdateServiceClient(cl.Conn)

	// Enable the DUT to receive updates.
	cleanup, err := updateutil.SignBoardName(ctx, client)
	if err != nil {
		return errors.Wrap(err, "failed to prepare for update")
	}
	defer cleanup(cleanupCtx)

	// Make sure to revert any installed update. Also reset update_engine.
	defer client.ResetUpdateEngine(cleanupCtx, &empty.Empty{})
	defer func() {
		if _, err := client.RevertUpdate(cleanupCtx, &empty.Empty{}); err != nil {
			testing.ContextLog(ctx, "Failed to revert update: ", err)

			if retErr != nil {
				testing.ContextLog(ctx, "Overwriting existing error: ", retErr)
			}

			retErr = errors.Wrap(err, "failed to revert update")
		}
	}()

	if _, err := client.ResetUpdateEngine(cleanupCtx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to reset update-engine")
	}

	if _, err := client.CheckForUpdate(ctx, &aupb.UpdateRequest{
		OmahaUrl: "https://tools.google.com/service/update2",
	}); err != nil {
		return errors.Wrap(err, "failed to check for udpate")
	}

	response, err := client.InstalledLSBReleaseContent(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to read lsb-release on the newly installed partition")
	}

	var lsbRelease map[string]string
	if err := json.Unmarshal(response.ContentJson, &lsbRelease); err != nil {
		return errors.Wrap(err, "failed to unmarshal lsb-release content")
	}

	for key, expected := range expectedLSBReleaseRegex {
		actual, ok := lsbRelease[key]

		if !ok {
			return errors.Errorf("Key %q missing from lsb-release", key)
		}

		compiled, err := regexp.Compile(expected)
		if err != nil {
			return errors.Wrapf(err, "failed to compile %q", expected)
		}

		if !compiled.MatchString(actual) {
			return errors.Errorf("/etc/lsb-release[%q]=%q, want %q", key, actual, expected)
		}
	}

	return nil
}
