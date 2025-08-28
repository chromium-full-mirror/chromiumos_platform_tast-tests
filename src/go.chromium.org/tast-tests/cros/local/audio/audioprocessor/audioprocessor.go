// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package audioprocessor interacts with src/third_party/adhd/audio_processor plugins.
package audioprocessor

import (
	"context"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// PluginInstaller is an interface that provides an Install method
// that returns a *Plugin or an error.
type PluginInstaller interface {
	Install(ctx context.Context) (*Plugin, error)
}

// Plugin is an audio processor plugin.
type Plugin struct {
	// The path of the plugin.
	Path string

	// The name of the processor_create function.
	// See also plugin_processor.h.
	Constructor string
}

var _ PluginInstaller = &Plugin{}

// Install returns the Plugin itself.
func (p *Plugin) Install(ctx context.Context) (*Plugin, error) {
	return p, nil
}

// DLCPlugin is an audio processor plugin from DLC.
type DLCPlugin struct {
	// The ID of the DLC.
	// Also known as the `--id` option of `dlcservice_util`.
	DLCID ConstOrVar

	// The path within the DLC root.
	Path ConstOrVar

	// The name of the processor_create function.
	// See also plugin_processor.h.
	Constructor string
}

var _ PluginInstaller = &DLCPlugin{}

// ConstOrVar is a constant string or a cras_processor_vars defined in board.ini.
type ConstOrVar struct {
	constant string
	variable string
}

// Const returns a constant string.
func Const(val string) ConstOrVar {
	return ConstOrVar{constant: val}
}

// Var returns a cras_processor_vars variable defined in board.ini.
func Var(val string) ConstOrVar {
	return ConstOrVar{variable: val}
}

func (cov ConstOrVar) resolve(ctx context.Context, crasProcessorVars map[string]string) (string, error) {
	if cov.constant != "" {
		return cov.constant, nil
	}
	value, ok := crasProcessorVars[cov.variable]
	if !ok {
		return "", errors.Errorf("%q not found in cras_processor_vars", cov.variable)
	}
	testing.ContextLogf(ctx, "$%v = %q", cov.variable, value)
	return value, nil
}

// Install the DLCPlugin and return the absolute path of the plugin.
func (p *DLCPlugin) Install(ctx context.Context) (*Plugin, error) {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to CRAS")
	}

	vars, err := cras.GetCrasProcessorVars(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "cras.GetCrasProcessorVars()")
	}

	dlcID, err := p.DLCID.resolve(ctx, vars)
	if err != nil {
		return nil, err
	}
	dlcPath, err := p.Path.resolve(ctx, vars)
	if err != nil {
		return nil, err
	}

	if err := dlc.Install(ctx, dlcID, ""); err != nil {
		return nil, errors.Wrapf(err, "failed to install DLC %q", dlcID)
	}

	dlcState, err := dlc.GetDlcState(ctx, dlcID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get DLC %q state", dlcID)
	}

	return &Plugin{
		Path:        filepath.Join(dlcState.RootPath, dlcPath),
		Constructor: p.Constructor,
	}, nil
}
