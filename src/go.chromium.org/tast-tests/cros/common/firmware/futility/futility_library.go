// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package futility

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/servo"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Full path to futility binary on DUT.
	dutFutilityPath = "/usr/bin/futility"
)

// Params are common parameters required for all library methods.
type Params struct {
	globalDebug bool
	dut         *dut.DUT
	servoPort   int
	servoProxy  *servo.Proxy
	programmer  string
}

// ContextCommandRunner is interface used by Instance to execute commands.
type ContextCommandRunner interface {
	runCommandLine(ctx context.Context, cmdArgs []string) (stdout, stderr []byte, err error)
}

// remoteRunner implements ContextCommandRunner and executes commands on the servoProxy.
type remoteRunner struct {
	params Params
}

// runCommandLine executes provided command.
func (r *remoteRunner) runCommandLine(ctx context.Context, cmdArgs []string) (stdout, stderr []byte, err error) {
	testing.ContextLog(ctx, "Running command line with arguments: ", cmdArgs)
	outbuf, errbuf, err := r.params.servoProxy.SeparatedOutputCommand(ctx, true, cmdArgs[0], cmdArgs[1:]...)
	if err != nil {
		return outbuf, errbuf, errors.Wrapf(err, "command %q failed", strings.Join(cmdArgs, " "))
	}

	return outbuf, errbuf, nil
}

// localRunner implements ContextCommandRunner and executes commands on the DUT.
type localRunner struct {
	params Params
}

// runCommandLine executes provided command.
func (r *localRunner) runCommandLine(ctx context.Context, cmdArgs []string) (stdout, stderr []byte, err error) {
	conn := r.params.dut.Conn()
	testing.ContextLog(ctx, "Running command line on DUT with arguments: ", cmdArgs)
	cmd := conn.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)

	var outbuf, errbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	if err := cmd.Wait(); err != nil {
		return nil, nil, errors.Wrapf(err, "command %q failed", strings.Join(cmd.Args, " "))
	}

	return outbuf.Bytes(), errbuf.Bytes(), nil
}

// Instance is base struct for futility instance implementations.
type Instance struct {
	params        Params
	futilityPath  string
	commandRunner ContextCommandRunner
}

// runCommandLine executes provided command with arguments.
func (i *Instance) runCommandLine(ctx context.Context, cmdArgs []string) (stdout, stderr []byte, err error) {
	return i.commandRunner.runCommandLine(ctx, cmdArgs)
}

// futilityCmdArgs returns starting list of command arguments.
func (i *Instance) futilityCmdArgs() []string {
	cmdArgs := []string{i.futilityPath}
	if i.params.globalDebug {
		cmdArgs = append(cmdArgs, "--debug")
	}

	return cmdArgs
}

// appendFlashArgs appends flash arguments based on instance parameters.
func (i *Instance) appendFlashArgs(cmdArgs []string) []string {
	if i.params.servoPort > 0 {
		return append(cmdArgs, "--servo", "--servo_port", fmt.Sprintf("%d", i.params.servoPort))
	} else if i.params.programmer != "" {
		return append(cmdArgs, "--programmer", i.params.programmer)
	}
	return cmdArgs
}

// Builder configures futility instance.
type Builder struct {
	params   Params
	isRemote bool
}

// NewLocalBuilder creates new futility instance builder and configures it to execute on DUT.
// testDut must not be a nil.
func NewLocalBuilder(dut *dut.DUT) *Builder {
	return &Builder{
		params: Params{
			dut:       dut,
			servoPort: 0,
		},
		isRemote: false,
	}
}

// NewRemoteBuilder creates new futility instance builder and configures it to execute on the servo host.
func NewRemoteBuilder(servoProxy *servo.Proxy) *Builder {
	return &Builder{
		params: Params{
			servoPort:  servoProxy.GetPort(),
			servoProxy: servoProxy,
		},
		isRemote: true,
	}
}

// Debug enables or disables use of --debug flag for whole futility.
func (b *Builder) Debug(debug bool) *Builder {
	b.params.globalDebug = debug
	return b
}

// isValid verifies builder params during Build().
func (b *Builder) isValid() error {
	if b.params.dut != nil && b.params.servoProxy != nil {
		return errors.New("cannot execute both on DUT and with servo")
	}
	return nil
}

// SetProgrammer sets the servo programmer to use.
func (b *Builder) SetProgrammer(programmer string) error {
	b.params.servoPort = 0
	b.params.programmer = programmer
	return nil
}

// Build returns futility instance, and error on failure.
func (b *Builder) Build() (*Instance, error) {
	if err := b.isValid(); err != nil {
		return nil, err
	}

	if b.isRemote {
		return &Instance{
			params:       b.params,
			futilityPath: "futility",
			commandRunner: &remoteRunner{
				params: b.params,
			},
		}, nil
	}
	return &Instance{
		params:       b.params,
		futilityPath: dutFutilityPath,
		commandRunner: &localRunner{
			params: b.params,
		},
	}, nil
}

// joinProgramOutputs joins stdin and stdout together with new line between them.
func joinProgramOutputs(stdout, stderr []byte) []byte {
	return bytes.Join([][]byte{stdout, stderr}, []byte("\n"))
}
