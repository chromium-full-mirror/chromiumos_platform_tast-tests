// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package updateutil

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	aupb "go.chromium.org/tast-tests/cros/services/cros/autoupdate"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// ImageVersion gets the DUT image version from the parsed /etc/lsb-realse file.
func ImageVersion(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) (string, error) {
	return EntryFromLSBRelease(ctx, dut, rpcHint, lsbrelease.Version)
}

// EntryFromLSBRelease is a wrapper for FillFromLSBRelease to get a single entry
// from the /etc/lsb-realse file with a simpler call.
func EntryFromLSBRelease(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, key string) (string, error) {
	lsbContent := map[string]string{
		key: "",
	}
	err := FillFromLSBRelease(ctx, dut, rpcHint, lsbContent)
	if err != nil {
		return "", err
	}

	return lsbContent[key], nil
}

// FillFromLSBRelease fills map[string]string it gets as input with values
// form the /etc/lsb-realse file based on matching keys.
func FillFromLSBRelease(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, req map[string]string) error {
	if req == nil || len(req) == 0 {
		return errors.New("request map should contain at least one key")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	// Connect to DUT.
	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(cleanupCtx)

	updateClient := aupb.NewUpdateServiceClient(cl.Conn)

	response, err := updateClient.LSBReleaseContent(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to read lsb-release")
	}

	var lsb map[string]string
	if err := json.Unmarshal(response.ContentJson, &lsb); err != nil {
		return errors.Wrap(err, "failed to unmarshal lsb-relese content")
	}

	missingKeys := make([]string, 0, len(req))
	for key := range req {
		value, ok := lsb[key]
		if !ok {
			missingKeys = append(missingKeys, key)
			continue
		}
		req[key] = value
	}

	if len(missingKeys) > 0 {
		return errors.Errorf("the following keys were not found in lsb-release %#v", missingKeys)
	}

	return nil
}

// SignBoardName adds an entry to /mnt/stateful_partition/etc/lsb-release with a signed board name
// to enable the DUT to receive updates.
// Returns a callback that restores the original content of /mnt/stateful_partition/etc/lsb-release.
func SignBoardName(ctx context.Context, client aupb.UpdateServiceClient) (func(ctx context.Context) error, error) {
	// Get board name from /etc/lsb-release.
	response, err := client.LSBReleaseContent(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read lsb-release")
	}

	var lsb map[string]string
	if err := json.Unmarshal(response.ContentJson, &lsb); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal lsb-relese content")
	}

	board, ok := lsb[lsbrelease.Board]
	if !ok {
		return nil, errors.New("failed to determine DUT board")
	}
	signedBoardName := board + "-signed-mp-v3keys"

	// Get content of /mnt/stateful_partition/etc/lsb-release.
	response, err = client.StatefulLSBReleaseContent(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read lsb-release on the stateful partition")
	}

	testing.ContextLogf(ctx, "Adding the %q board name to lsb-release in the stateful partition", signedBoardName)
	var statefulLsb map[string]string
	if err := json.Unmarshal(response.ContentJson, &statefulLsb); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal stateful lsb-relese content")
	}
	statefulLsb[lsbrelease.Board] = signedBoardName

	newStatefulLsbJSON, err := json.Marshal(statefulLsb)
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize stateful lsb-release content")
	}

	originalContent := response.ContentJson

	if _, err := client.OverwriteStatefulLSBRelease(ctx, &aupb.LSBRelease{ContentJson: newStatefulLsbJSON}); err != nil {
		return nil, errors.Wrap(err, "failed to overwrite lsb-release in the stateful partition")
	}

	return func(ctx context.Context) error {
		if _, err := client.OverwriteStatefulLSBRelease(ctx, &aupb.LSBRelease{ContentJson: originalContent}); err != nil {
			testing.ContextLog(ctx, "Failed to restore lsb-release in the stateful partition: ", err)
			return err
		}

		return nil
	}, nil
}
