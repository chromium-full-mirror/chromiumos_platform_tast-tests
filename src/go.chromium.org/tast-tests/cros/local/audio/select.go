// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// SelectIODevices sets the default input and output devices of CRAS.
//
// UI must be stopped otherwise it may overwrite the settings.
func SelectIODevices(ctx context.Context, cras *Cras, defaultInputDeviceType, defaultOutputDeviceType string) error {
	testing.ContextLog(ctx, "Setting default input device to ", defaultInputDeviceType)
	if err := cras.SetActiveNodeByMatcher(ctx, MatchNodeTypeDirection{
		Type:      defaultInputDeviceType,
		Direction: InputStream,
	}); err != nil {
		return errors.Wrapf(err, "cannot set default input device to %s", defaultInputDeviceType)
	}
	testing.ContextLog(ctx, "Setting default output device to ", defaultOutputDeviceType)
	if err := cras.SetActiveNodeByMatcher(ctx, MatchNodeTypeDirection{
		Type:      defaultOutputDeviceType,
		Direction: OutputStream,
	}); err != nil {
		return errors.Wrapf(err, "cannot set default output device to %s", defaultOutputDeviceType)
	}
	return nil
}
