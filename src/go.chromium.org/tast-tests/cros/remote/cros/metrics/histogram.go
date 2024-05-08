// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package metrics remotely queries metrics collected by Chrome.
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// GetHistogram gets a histogram from DUT.
func GetHistogram(ctx context.Context, tconn ui.TconnServiceClient, name string, fromLacros bool) (*histogram.Histogram, error) {
	var h = histogram.Histogram{Name: name}
	res, err := tconn.Call(ctx, &ui.CallRequest{
		Fn:           `tast.promisify(chrome.metricsPrivate.getHistogram)`,
		Args:         []*structpb.Value{structpb.NewStringValue(name)},
		CallOnLacros: fromLacros,
	})
	if err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("Histogram %s not found", name)) {
			return &h, nil
		}
		return nil, errors.Wrap(err, "failed to call chrome.metricsPrivate.getHistogram")
	}
	j, err := res.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal response")
	}
	if err := json.Unmarshal(j, &h); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response")
	}
	return &h, nil
}

// WaitForHistogramUpdate is a convenience function that calls GetHistogram until the
// requested histogram contains at least one sample not present in old, an earlier
// snapshot of the same histogram.
// A histogram containing the new samples is returned; see Histogram.Diff for details.
func WaitForHistogramUpdate(ctx context.Context, tconn ui.TconnServiceClient, name string, fromLacros bool, old *histogram.Histogram, timeout time.Duration) (*histogram.Histogram, error) {
	var h *histogram.Histogram
	err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		if h, err = GetHistogram(ctx, tconn, name, fromLacros); err != nil {
			return err
		}
		if reflect.DeepEqual(h, old) {
			return errors.New("histogram unchanged")
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout})

	if err != nil {
		return nil, err
	}
	return h.Diff(old)
}
