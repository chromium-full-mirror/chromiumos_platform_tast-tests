// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// GoogleMeetConfig defines input params and retry settings for Google meet testing.
type GoogleMeetConfig struct {
	BondCreds     []byte
	RetryTimeout  time.Duration
	RetryInterval time.Duration
}

// GetGoogleMeetConfig returns an object that contains the Google meet configuration.
func GetGoogleMeetConfig(ctx context.Context, s *testing.State, roomType RoomType) (GoogleMeetConfig, error) {
	// If roomType is NoRoom, an empty object is returned.
	if roomType == NoRoom {
		return GoogleMeetConfig{}, nil
	}

	const (
		defaultMeetRetryTimeout  = 40 * time.Minute
		defaultMeetRetryInterval = 2 * time.Minute
	)
	varToDuration := func(name string, defaultValue time.Duration) (time.Duration, error) {
		str, ok := s.Var(name)
		if !ok {
			return defaultValue, nil
		}

		val, err := strconv.Atoi(str)
		if err != nil {
			return 0, errors.Wrapf(err, "failed to parse integer variable %v", name)
		}

		return time.Duration(val) * time.Minute, nil
	}

	meetRetryTimeout, err := varToDuration("ui.meet_retry_timeout", defaultMeetRetryTimeout)
	if err != nil {
		return GoogleMeetConfig{}, errors.Wrapf(err, "failed to parse %q to time duration", defaultMeetRetryTimeout)
	}

	meetRetryInterval, err := varToDuration("ui.meet_retry_interval", defaultMeetRetryInterval)
	if err != nil {
		return GoogleMeetConfig{}, errors.Wrapf(err, "failed to parse %q to time duration", defaultMeetRetryInterval)
	}
	testing.ContextLogf(ctx, "Retry vars: meetRetryTimeout %v, meetRetryInterval %v", meetRetryTimeout, meetRetryInterval)

	bondCreds, ok := s.Var("ui.meet_bond_key")
	if !ok || len(bondCreds) < 1 {
		return GoogleMeetConfig{}, errors.New("ui.meet_bond_key is not set")
	}

	return GoogleMeetConfig{
		BondCreds:     []byte(bondCreds),
		RetryTimeout:  meetRetryTimeout,
		RetryInterval: meetRetryInterval,
	}, nil
}
