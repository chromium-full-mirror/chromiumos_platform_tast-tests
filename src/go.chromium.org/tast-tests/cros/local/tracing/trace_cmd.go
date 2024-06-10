// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracing

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// All real ftrace instance name will have this prefix.
const tastInstancePrefix = "tast-"

func tastInstanceName(name string) string {
	return tastInstancePrefix + name
}

// TraceInstance represents the sub-buffer of ftrace made by trace-cmd.
type TraceInstance struct {
	// Ftrace instance name, without tastInstancePrefix.
	name string

	// cmd is the process of a running trace-cmd. This can be nil.
	cmd *testexec.Cmd

	// stdout is cmd's stdout.
	stdout *bytes.Buffer

	// stderr is cmd's stderr.
	stderr *bytes.Buffer

	// TraceDataPath is the file path of traced data.
	traceDataPath string
}

// Name returns the instance name
func (t *TraceInstance) Name() string {
	return t.name
}

// TraceDataPath returns the data file's path
func (t *TraceInstance) TraceDataPath() string {
	return t.traceDataPath
}

func getInstanceList(ctx context.Context) ([]string, error) {
	cmd := testexec.CommandContext(ctx, "trace-cmd", "stat")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Get the list of instances.
	// It should be the last part of the output, after "Instances:" line.
	lines := strings.Split(string(output), "\n")
	var instances []string
	instance := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if instance && line != "" {
			instances = append(instances, line)
		}
		if line == "Instances:" {
			instance = true
		}
	}
	return instances, nil
}

func traceInstanceExists(ctx context.Context, name string) bool {
	instances, err := getInstanceList(ctx)
	if err != nil {
		return false
	}
	for _, instance := range instances {
		if instance == tastInstanceName(name) {
			return true
		}
	}
	return false
}

func newTraceCmdWithInstance(ctx context.Context, op, name string, args ...string) *testexec.Cmd {
	args = append([]string{op, "-B", tastInstanceName(name)}, args...)
	return testexec.CommandContext(ctx, "trace-cmd", args...)
}

func createTraceInstance(ctx context.Context, name string) error {
	// `trace-cmd set -B <name>` without any other options will make <name> instance.
	if err := newTraceCmdWithInstance(ctx, "set", name).Run(); err != nil {
		return errors.Wrapf(err, "failed to create instance %s", name)
	}
	return nil
}

func removeTraceInstance(ctx context.Context, name string) error {
	// `trace-cmd reset -B <name>` without any other options will remove <name> instance.
	if err := newTraceCmdWithInstance(ctx, "reset", name).Run(); err != nil {
		return errors.Wrapf(err, "failed to remove instance %s", name)
	}
	return nil
}

func extractTraceInstance(ctx context.Context, name, outfile string) error {
	// `trace-cmd extract -B <name>` dumps data into a file but also remove the buffer
	if err := newTraceCmdWithInstance(ctx, "extract", name, "-k", "-o", outfile).Run(); err != nil {
		return errors.Wrapf(err, "failed to extract instance %s", name)
	}
	return nil
}

func clearBufferTraceInstance(ctx context.Context, name string) error {
	if err := newTraceCmdWithInstance(ctx, "clear", name).Run(); err != nil {
		return errors.Wrapf(err, "failed to clear buffer of %s", name)
	}
	return nil
}

func stopTraceInstance(ctx context.Context, name string) error {
	if err := newTraceCmdWithInstance(ctx, "stop", name).Run(); err != nil {
		return errors.Wrapf(err, "failed to stop tracing of %s", name)
	}
	return nil
}

func restartTraceInstance(ctx context.Context, name string) error {
	if err := newTraceCmdWithInstance(ctx, "restart", name).Run(); err != nil {
		return errors.Wrapf(err, "failed to restart tracing of %s", name)
	}
	return nil
}

// CleanupAllTraceInstances removes all instances which have "tast-" prefix.
// If a test does not know what Instance has been made by the test (e.g.
// test fixture) it can use this method to clean up all instances which should
// be made by the tast tests.
func CleanupAllTraceInstances(ctx context.Context) error {
	instances, err := getInstanceList(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get instance name list")
	}
	for _, instance := range instances {
		if strings.HasPrefix(instance, tastInstancePrefix) {
			name := strings.TrimPrefix(instance, tastInstancePrefix)
			if err := removeTraceInstance(ctx, name); err != nil {
				return err
			}
		}
	}
	return nil
}

type traceCmdConfig struct {
	args          []string
	resetInstance bool
	recordOnDisk  bool
	traceDataPath string
}

type traceCmdOption func(config *traceCmdConfig)

// CreateTraceInstance creates a new instance with new options.
//
// Unless creating with RecordOnDiskMode(), you can disconnect from the instance
// (just by discarding TraceInstance.) To reconnect the created trace instance,
// call ReconnectTraceInstance() with the same name. Note that the last reconnected
// caller must call Finalize() to remove the instance. For example;
//
//		ti, err := tracing.CreateTraceInstance(ctx, "tast", CPUBufferKib(1024), EnableEvents("sched:*"))
//		if err != nil {
//			s.Fatal("Failed to create a trace instance", err)
//		}
//		<logic to be traced, which can disconnect>
//		ti, err := tracing.ReconnectTraceInstance(ctx, "tast")
//		if err != nil {
//			s.Fatal("Failed to reconnect a trace instance", err)
//		}
//	   	defer ti.Finalize()
//	   	if err != ti.Save(ctx, outfile) {
//		    s.Fatal("Failed to save trace data", err)
//		}
//
// If this is called with RecordOnDiskMode(path), the trace-cmd process runs in background and
// continue recording events on the disk. Thus the caller is expected to make sure to call
// TraceInstance.Finalize() even for the error cases. Otherwise, trace-cmd may keep running
// even after the test exits. So, Finalize() is supposed to be called with defer like the
// following example:
//
//	  	ti, err := tracing.CreateTraceInstance(ctx, "tast", RecordOnDiskMode("/path/to/trace.dat"), EnableEvents("syscalls"))
//	  	if err != nil {
//		   	s.Fatal("Failed to start trace-cmd", err)
//	  	}
//	  	defer ti.Finalize() // Make sure calling Finalize() even on failures
//	  	<logic to be traced>
//	  	if err != ti.Collect(ctx) {
//	  		s.Fatal("Failed to collect trace.dat", err)
//	  	}
func CreateTraceInstance(ctx context.Context, name string, opts ...traceCmdOption) (*TraceInstance, error) {
	config := traceCmdConfig{}
	for _, opt := range opts {
		opt(&config)
	}

	if config.resetInstance && traceInstanceExists(ctx, name) {
		if err := removeTraceInstance(ctx, name); err != nil {
			return nil, err
		}
	}

	// Since config.resetInstance might remove the instance if it exists,
	// this needs to recreate the instance.
	if !traceInstanceExists(ctx, name) {
		if err := createTraceInstance(ctx, name); err != nil {
			return nil, err
		}
	}

	if config.recordOnDisk {
		// record on disk mode: TraceInstance must not disconnect before Finalize()
		args := append([]string{"record", "-o", config.traceDataPath}, config.args...)
		traceCmd := newTraceCmdWithInstance(ctx, "record", name, args...)
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}
		traceCmd.Stdout = &stdout
		traceCmd.Stderr = &stderr
		if err := traceCmd.Start(); err != nil {
			removeTraceInstance(ctx, name)
			return nil, errors.Wrap(err, "failed to start trace-cmd")
		}

		return &TraceInstance{name: name, cmd: traceCmd, stdout: &stdout, stderr: &stderr, traceDataPath: config.traceDataPath}, nil
	}

	// Record in kernel ring buffer mode: TraceInstance can be disconnected and reconnected.
	if len(config.args) == 0 {
		return &TraceInstance{name: name}, nil
	}

	// Set some options to the instance.
	if err := newTraceCmdWithInstance(ctx, "set", name, config.args...).Run(); err != nil {
		removeTraceInstance(ctx, name)
		return nil, errors.Wrapf(err, "failed to set options to instance %s", name)
	}

	return &TraceInstance{name: name}, nil
}

// ReconnectTraceInstance re-connects to an existing trace instance.
func ReconnectTraceInstance(ctx context.Context, name string) (*TraceInstance, error) {
	if !traceInstanceExists(ctx, name) {
		return nil, errors.Errorf("failed to reconnect to the instance names %q, no such instance", name)
	}
	return CreateTraceInstance(ctx, name)
}

// Options for CreateTraceInstance()

// EmptyTraceCmdOptions returns an empty array of traceCmdOption.
func EmptyTraceCmdOptions() []traceCmdOption {
	var opts []traceCmdOption
	return opts
}

// RecordOnDiskMode specifies the TraceInstance run trace-cmd running on disk recording.
func RecordOnDiskMode(outPath string) traceCmdOption {
	return func(config *traceCmdConfig) {
		config.recordOnDisk = true
		config.traceDataPath = outPath
	}
}

// WithResetInstance resets instance when creating a new instance if it already exist.
func WithResetInstance() traceCmdOption {
	return func(config *traceCmdConfig) {
		config.resetInstance = true
	}
}

// CPUBufferKiB returns an option to set up the size of per-cpu buffer
func CPUBufferKiB(size int) traceCmdOption {
	return func(config *traceCmdConfig) {
		config.args = append(config.args, "-b", strconv.Itoa(size))
	}
}

// EnableEvents enables a series of events
func EnableEvents(events ...string) traceCmdOption {
	return func(config *traceCmdConfig) {
		for _, e := range events {
			config.args = append(config.args, "-e", e)
		}
	}
}

// EnableEventWith enables an event with specified filter and trigger (e.g. "stacktrace")
// If filter or trigger is empty, it is ignored.
func EnableEventWith(event, filter, trigger string) traceCmdOption {
	return func(config *traceCmdConfig) {
		config.args = append(config.args, "-e", event)
		// Add a filter for this event.
		if filter != "" {
			config.args = append(config.args, "-f", filter)
		}
		// Add a trigger for this event.
		if trigger != "" {
			config.args = append(config.args, "-R", trigger)
		}
	}
}

// PidFilter sets pid filter to the tracers.
func PidFilter(pids ...int) traceCmdOption {
	return func(config *traceCmdConfig) {
		var param string
		l := len(pids) - 1
		for i, pid := range pids {
			if i != l {
				param += strconv.Itoa(pid) + ","
			} else {
				param += strconv.Itoa(pid)
			}
		}
		config.args = append(config.args, "-P", param)
	}
}

// EnableFunctionTracer enables function tracer with filtered functions
func EnableFunctionTracer(functions ...string) traceCmdOption {
	return func(config *traceCmdConfig) {
		config.args = append(config.args, "-p", "function")
		var param string
		l := len(functions) - 1
		if l != -1 {
			for i, f := range functions {
				if i != l {
					param += f + ","
				} else {
					param += f
				}
			}
			config.args = append(config.args, "-l", param)
		}
	}
}

// ClearCPUBuffer only clears the ring buffer, setting is not changed.
func (t *TraceInstance) ClearCPUBuffer(ctx context.Context) error {
	if t.cmd != nil {
		return errors.New("failed to clear CPU buffer for trace-cmd running on disk recording mode")
	}
	return clearBufferTraceInstance(ctx, t.name)
}

// Stop stops tracing on the instance or stop command.
func (t *TraceInstance) Stop(ctx context.Context) error {
	if t.cmd == nil {
		return stopTraceInstance(ctx, t.name)
	}
	if err := t.cmd.Signal(unix.SIGINT); err != nil {
		return errors.Wrap(err, "failed to stop trace-cmd")
	}
	if err := t.cmd.Wait(); err != nil {
		return errors.Wrapf(err, "failed to wait or trace-cmd exited with error. stdout: %q, stderr: %q", t.stdout.String(), t.stderr.String())
	}
	return nil
}

// Restart restarts the stopped tracing. Only if !RecordOnDiskMode().
func (t *TraceInstance) Restart(ctx context.Context) error {
	if t.cmd != nil {
		return errors.New("failed to restart trace-cmd running on disk recording mode")
	}
	return restartTraceInstance(ctx, t.name)
}

// Save writes out the ring buffer contents to outfile.
func (t *TraceInstance) Save(ctx context.Context, outfile string) error {
	if t.cmd != nil {
		if err := t.Collect(ctx); err != nil {
			return err
		}
		if outfile != t.traceDataPath && outfile != "" {
			return os.Rename(t.traceDataPath, outfile)
		}
		return nil
	}
	return extractTraceInstance(ctx, t.name, outfile)
}

// Collect finishes recording on the disk.
// Note: This is only for TraceInstance createed with RecordOnDiskMode().
func (t *TraceInstance) Collect(ctx context.Context) error {
	if t.cmd == nil {
		return errors.New("failed to collect because this is not recorded on the disk")
	}
	if err := t.Stop(ctx); err != nil {
		return err
	}

	if strings.Contains(t.stdout.String(), "events lost") {
		testing.ContextLogf(ctx, "trace-cmd lost some events. Try increasing the buffer size of trace-cmd with -b option. stdout: %q, stderr: %q", t.stdout.String(), t.stderr.String())
	} else if _, err := os.Stat(t.traceDataPath); err != nil {
		return errors.Wrapf(err, "trace-cmd didn't produce data at %s", t.traceDataPath)
	} else {
		testing.ContextLogf(ctx, "trace-cmd record succeeded with %s generated", t.traceDataPath)
	}
	return nil
}

// Finalize removes the instance and settings are gone.
func (t *TraceInstance) Finalize(ctx context.Context) {
	if err := t.Stop(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to stop tracing, but continue to remove instance")
	}
	removeTraceInstance(ctx, t.name)
}
