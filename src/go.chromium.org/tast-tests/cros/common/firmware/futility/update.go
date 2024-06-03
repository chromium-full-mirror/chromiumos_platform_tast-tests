// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// UpdateMode represents mode in which update should proceed.
type UpdateMode string

// Available update modes.
const (
	UpdateModeDefault    UpdateMode = ""
	UpdateModeAutoUpdate UpdateMode = "autoupdate"
	UpdateModeRecovery   UpdateMode = "recovery"
	UpdateModeFactory    UpdateMode = "factory"
)

// WriteProtection configures write protection mode during update.
type WriteProtection int

// Available write protection options.
const (
	WriteProtectionUnchanged WriteProtection = iota
	WriteProtectionEnable
	WriteProtectionDisable
)

type updateOptions struct {
	image           string
	ecImage         string
	tryABUpdate     bool
	fast            bool
	quirks          map[string]string
	mode            UpdateMode
	writeProtection WriteProtection
	hostOnly        bool
	gbbFlags        int
	setGBBFlags     bool
	force           bool
}

// NewUpdateOptions returns new updateOptions with image (BIOS) file path.
func NewUpdateOptions(image string) *updateOptions {
	return &updateOptions{
		image: image,
	}
}

// WithECImage configures EC image path.
func (o *updateOptions) WithECImage(ecImage string) *updateOptions {
	o.ecImage = ecImage
	return o
}

// WithTryAB configures AB update mode.
func (o *updateOptions) WithTryAB(tryAB bool) *updateOptions {
	o.tryABUpdate = tryAB
	return o
}

// WithFast configures fast update mode.
func (o *updateOptions) WithFast(fast bool) *updateOptions {
	o.fast = fast
	return o
}

// WithQuirks configures list of quirks to enable.
// Quirks with empty value default to "1".
func (o *updateOptions) WithQuirks(quirks map[string]string) *updateOptions {
	o.quirks = quirks
	return o
}

// WithMode configures update mode. futility defaults to recovery mode.
func (o *updateOptions) WithMode(mode UpdateMode) *updateOptions {
	o.mode = mode
	return o
}

// WithWriteProtection configures write protection status for futility command.
func (o *updateOptions) WithWriteProtection(wp WriteProtection) *updateOptions {
	o.writeProtection = wp
	return o
}

// WithHostOnly configures host-only mode.
func (o *updateOptions) WithHostOnly(hostOnly bool) *updateOptions {
	o.hostOnly = hostOnly
	return o
}

// WithGBBFlags configures optional GBB flags to set during update.
func (o *updateOptions) WithGBBFlags(flags int) *updateOptions {
	o.gbbFlags = flags
	o.setGBBFlags = true
	return o
}

// WithForce configures the '--force' flag to set during update.
func (o *updateOptions) WithForce(force bool) *updateOptions {
	o.force = force
	return o
}

// Update calls `futility update` with provided options.
//
// Returns program output, and error on failure.
func (i *Instance) Update(ctx context.Context, opts *updateOptions) ([]byte, error) {
	if opts.image == "" {
		return nil, errors.New("no image provided")
	}

	cmdArgs := append(i.futilityCmdArgs(), "update", "--image", opts.image)

	if opts.ecImage != "" {
		cmdArgs = append(cmdArgs, "--ec_image", opts.ecImage)
	}
	if opts.tryABUpdate {
		cmdArgs = append(cmdArgs, "--try")
	}
	if opts.fast {
		cmdArgs = append(cmdArgs, "--fast")
	}
	if len(opts.quirks) > 0 {
		var quirks []string
		for k, v := range opts.quirks {
			val := v
			if v == "" {
				val = "1"
			}
			quirks = append(quirks, fmt.Sprintf("%s=%s", k, val))
		}
		cmdArgs = append(cmdArgs, "--quirks", strings.Join(quirks, ","))
	}
	if opts.mode != UpdateModeDefault {
		cmdArgs = append(cmdArgs, "--mode", string(opts.mode))
	}
	if opts.writeProtection == WriteProtectionEnable {
		cmdArgs = append(cmdArgs, "--wp=1")
	} else if opts.writeProtection == WriteProtectionDisable {
		cmdArgs = append(cmdArgs, "--wp=0")
	}
	if opts.hostOnly {
		cmdArgs = append(cmdArgs, "--host_only")
	}
	if opts.setGBBFlags {
		cmdArgs = append(cmdArgs, "--gbb_flags", fmt.Sprintf("%#x", opts.gbbFlags))
	}
	if opts.force {
		cmdArgs = append(cmdArgs, "--force")
	}

	cmdArgs = i.appendFlashArgs(cmdArgs)

	stdout, stderr, err := i.runCommandLine(ctx, cmdArgs)
	fullOut := joinProgramOutputs(stdout, stderr)
	if err != nil {
		return fullOut, errors.Wrapf(err, "failed to update firmware with arguments %v", cmdArgs)
	}

	return fullOut, nil
}
