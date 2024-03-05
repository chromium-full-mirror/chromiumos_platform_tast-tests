// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracing

import (
	"bytes"
	"context"
	"encoding/csv"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"android.googlesource.com/platform/external/perfetto/protos/perfetto/metrics/github.com/google/perfetto/perfetto_proto"
	"github.com/golang/protobuf/proto"
	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const traceProcessorPath = "/usr/local/bin/trace_processor_shell"

// Session stores the cmd and the result file of the trace.
// Remember to call Session.Finalize to finalize the tracing session.
type Session struct {
	// cmd is used for running the perfetto process for non-background session.
	cmd *testexec.Cmd
	// proc is used for attaching perfetto process for background session.
	// Both cmd and proc can be nil if user recoonected to a perfetto session
	// which has already finished tracing.
	proc              *process.Process
	useTempFile       bool
	compressTraceData bool
	traceDataPath     string
}

// SessionToken stores the current session state so that ReconnectBackgroundSession
// can reconnect to the session using it. External program should not change it.
type SessionToken struct {
	pid               int
	useTempFile       bool
	compressTraceData bool
	traceDataPath     string
}

func createTempFileForTrace() (*os.File, error) {
	return ioutil.TempFile("", "perfetto-trace-*.pb")
}

type option struct {
	traceDataPath string
	compression   bool
	background    bool
}

type traceSessionOption func(*option)

// WithTraceDataPath configures the session with trace data written to the given path.
func WithTraceDataPath(path string) traceSessionOption {
	return func(opt *option) {
		opt.traceDataPath = path
	}
}

// WithCompression configures the session with compressing the trace data after the session is finalized.
// Note: The compressed data file name is traceDataPath + ".gz".
func WithCompression() traceSessionOption {
	return func(opt *option) {
		opt.compression = true
	}
}

// InBackground configures the session is running in background.
// The background session allows user to reconnect it by a token returned from
// `Token()` method. Thus the remote service which is expected to be
// disconnected (e.g. suspend & resume) can use that background session.
// On the other hand, non-background session cannot be disconnected. If it is
// disconnected, the perfetto session will be aborted.
// Note that the caller needs to call `Stop()` to stop the session and call
// `Finalize()` method to remove a temporary trace data file (if you do not
// specify a trace data file by `WithTraceDataPath()`.)
// To prevent runaway background tracing sessions, set `duration_ms`
// in the perfetto configuration, which forcibly terminates the tracing
// after specified time elapsed.
func InBackground() traceSessionOption {
	return func(opt *option) {
		opt.background = true
	}
}

// TraceDataPath returns the path of the trace data file.
func (sess *Session) TraceDataPath() string {
	return sess.traceDataPath
}

// Stop stops the system-wide trace, which should be created by StartSession.
// If the session is already detached, this returns an error.
// Note that the session shouldn't be stopped too early (like in 500
// milliseconds), so that perfetto_cmd has time to register the signal handler
// to handle SIGTERM properly.
func (sess *Session) Stop(ctx context.Context) error {
	var err error
	if sess.cmd != nil {
		err = sess.cmd.Signal(unix.SIGTERM)
	} else if sess.proc != nil {
		err = sess.proc.SendSignal(unix.SIGTERM)
	} else {
		return errors.New("this session is already detached from traced")
	}

	if err != nil {
		return errors.Wrap(err, "failed to terminate the tracing session")
	}

	return sess.Wait(ctx)
}

const (
	defaultWaitTimeout = 5 * time.Second
)

// Wait waits until the tracing session is done, which should be created
// by StartSession.
func (sess *Session) Wait(ctx context.Context) error {
	if sess.cmd != nil {
		// TODO(b/330329525): this requires a timeout check.
		return sess.cmd.Wait()
	}
	if sess.proc != nil {
		// sess.proc.Wait() doesn't work here because proc is not a child process.
		return procutil.WaitForTerminated(ctx, sess.proc, defaultWaitTimeout)
	}

	// The perfetto process already exits.
	return nil
}

// Token returns a token for reconnect.
func (sess *Session) Token() (*SessionToken, error) {
	if sess.cmd != nil {
		return nil, errors.New("failed to get token because it is not running in background")
	}
	return &SessionToken{pid: int(sess.proc.Pid), useTempFile: sess.useTempFile, compressTraceData: sess.compressTraceData, traceDataPath: sess.traceDataPath}, nil
}

// RunMetrics collects the result with trace_processor_shell.
func (sess *Session) RunMetrics(ctx context.Context, metrics []string) (*perfetto_proto.TraceMetrics, error) {
	metric := strings.Join(metrics, ",")
	cmd := testexec.CommandContext(ctx, traceProcessorPath, sess.traceDataPath, "--run-metrics", metric)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run metrics with trace_processor_shell")
	}

	tbm := &perfetto_proto.TraceMetrics{}
	if err := proto.UnmarshalText(string(out), tbm); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal metrics result")
	}

	return tbm, nil
}

// RunQueryString processes the trace data with a SQL query string and returns the query csv result as [][]string.
func (sess *Session) RunQueryString(ctx context.Context, query string) ([][]string, error) {
	// Save some time for cleaning up the temp file created for the query string.
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	// trace_processor_shell accepts the SQL query as a file. Create a temp query file.
	queryFile, err := ioutil.TempFile("", "trace_processor_query_*.sql")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a temp file for SQL query")
	}
	defer func(ctx context.Context) {
		if err := os.Remove(queryFile.Name()); err != nil {
			testing.ContextLog(ctx, "Failed to remove the temp file for SQL query: ", err)
		}
		queryFile.Close()
	}(ctxForCleanup)

	if _, err := queryFile.WriteString(query); err != nil {
		return nil, errors.Wrap(err, "failed to populate the temp file for SQL query")
	}

	return sess.RunQuery(ctx, queryFile.Name())
}

// RunQuery processes the trace data with a SQL query and returns the query csv result as [][]string.
func (sess *Session) RunQuery(ctx context.Context, queryPath string) ([][]string, error) {
	cmd := testexec.CommandContext(ctx, traceProcessorPath, sess.traceDataPath, "-q", queryPath)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run metrics with trace_processor_shell")
	}

	// trace_procesor_shell query output is in csv.
	csv := csv.NewReader(bytes.NewReader(out))
	// Return the query csv result as [][]string.
	return csv.ReadAll()
}

// traceDataFileExists checks if the trace data file exists.
func (sess *Session) traceDataFileExists() bool {
	if _, err := os.Stat(sess.traceDataPath); err != nil {
		return false
	}
	return true
}

// removeTempTraceDataFile removes the temp trace data file owned by this session.
func (sess *Session) removeTempTraceDataFile() error {
	// Remove the trace data file only if the file is owned by the session (useTempFile == true).
	if !sess.useTempFile || !sess.traceDataFileExists() {
		return nil
	}

	if err := os.Remove(sess.traceDataPath); err != nil {
		return errors.Wrap(err, "failed to remove the temp file for trace data")
	}

	return nil
}

// Finalize performs the final actions for the tracing session:
// Remove the trace data file if the session uses a temporary file for trace data.
// Compress the trace data if the session is started with the compression option.
func (sess *Session) Finalize(ctx context.Context) error {
	if !sess.traceDataFileExists() {
		return nil
	}

	// Auto remove the trace data file if the session outputs to a temp file.
	if sess.useTempFile {
		return sess.removeTempTraceDataFile()
	}

	// Compress the trace data if the session is configured with the compression option.
	if sess.compressTraceData {
		cmd := testexec.CommandContext(ctx, "/bin/gzip", sess.traceDataPath)
		err := cmd.Run(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to compress the trace data file")
		}
	}

	return nil
}

// CheckTracingServices checks the status of job traced and traced_probes.
// Returns the traced and traced_probes process IDs on success or an error if
// either job is not in the running state, or either jobs has crashed (and
// remains in the zombie process status.
func CheckTracingServices(ctx context.Context) (tracedPID, tracedProbesPID int, err error) {
	// Ensure traced is not in the zombie process state.
	if err = upstart.CheckJob(ctx, TracedJobName); err != nil {
		return 0, 0, err
	}
	// Get the PID of traced.
	if _, _, tracedPID, err = upstart.JobStatus(ctx, TracedJobName); err != nil {
		return 0, 0, err
	}

	if err = upstart.CheckJob(ctx, TracedProbesJobName); err != nil {
		return 0, 0, err
	}
	// Get the PID of traced_probes.
	if _, _, tracedProbesPID, err = upstart.JobStatus(ctx, TracedProbesJobName); err != nil {
		return 0, 0, err
	}

	return tracedPID, tracedProbesPID, nil
}

// StartSession starts a system-wide trace using the perfetto command line tool in the background.
// Returns a Session instance on success or error on failure.
// The caller is *not* blocked when the tracing session is running. It's the caller's responsibility to call
// Wait() to ensure that the session is done before.
// The caller may optionally call Stop() to interrupt and terminate the tracing session before the session terminates automatically
// after the duration_ms in the trace config elapses.
// The caller should call Finalize() to perform the final actions with the tracing session whether the test is successful or not.
// The trace session can be configured using WithTraceDataPath(path) or WithCompression() options.
func StartSession(ctx context.Context, configFile string, opts ...traceSessionOption) (*Session, error) {
	option := &option{}
	for _, opt := range opts {
		opt(option)
	}

	useTempFile := true
	var traceDataFile *os.File = nil
	var err error = nil

	if option.traceDataPath != "" {
		useTempFile = false
		traceDataFile, err = os.OpenFile(option.traceDataPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the trace data file")
		}
	} else {
		traceDataFile, err = createTempFileForTrace()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create a temp file for trace data")
		}
	}
	defer traceDataFile.Close()
	traceDataPath := traceDataFile.Name()

	// This runs a perfetto trace session with the options:
	//   -c traceConfigPath --txt: configure the trace session as defined in the text proto |traceConfigPath|
	//   -o traceOutputPath      : save the trace data (binary proto) to |traceOutputPath|
	//   -D                      : run traced in background (optional)
	args := []string{"-c", configFile, "--txt", "-o", traceDataPath}
	if option.background {
		args = append(args, "-D")
	}
	cmd := testexec.CommandContext(ctx, "perfetto", args...)

	var proc *process.Process = nil
	if !option.background {
		err = cmd.Start()
	} else {
		var out []byte
		out, err = cmd.Output()
		if err == nil {
			proc, err = func(out string) (*process.Process, error) {
				// Regular expression to find one or more digits (PID)
				re := regexp.MustCompile(`\d+`)
				match := re.FindString(out)
				if match == "" {
					return nil, errors.New("failed to find the tracing session pid")
				}
				pid, err := strconv.Atoi(match)
				if err != nil || match == "" {
					return nil, errors.Wrap(err, "failed to convert the tracing session pid")
				}
				return process.NewProcess(int32(pid))
			}(string(out))
		}
		cmd = nil
	}

	if err != nil {
		if e := os.Remove(traceDataFile.Name()); e != nil {
			// Cleanup the temp file is non-fatal. Just log the error.
			testing.ContextLog(ctx, "Failed to remove the trace data file: ", e)
		}
		return nil, errors.Wrap(err, "failed to start the tracing session")
	}

	return &Session{cmd: cmd, proc: proc, useTempFile: useTempFile, compressTraceData: option.compression, traceDataPath: traceDataPath}, nil
}

// StartSessionAndWaitUntilDone collects a system-wide trace using the perfetto command line tool.
// Returns a Session instance on success or error on failure.
// The caller should call Finalize() to perform the final actions with the tracing session whether the test is successful or not.
// The trace session can be configured using WithTraceDataPath(path) or WithCompression() options.
func StartSessionAndWaitUntilDone(ctx context.Context, configFile string, opts ...traceSessionOption) (*Session, error) {
	sess, err := StartSession(ctx, configFile, opts...)
	if err != nil {
		return nil, err
	}

	if err := sess.Wait(ctx); err != nil {
		// Session is already started. We need to remove the temp file.
		if errRemove := sess.removeTempTraceDataFile(); err != nil {
			testing.ContextLog(ctx, "Failed to remove the temp trace data file", errRemove)
		}
		return nil, errors.Wrap(err, "failed to stop the tracing session")
	}

	return sess, nil
}

// ReconnectBackgroundSession reconnects to an existing system-wide background perfetto trace.
// Returns a Session instance on success or error on failure.
// The caller should call Finalize() to perform the final actions with the
// tracing session whether the test is successful or not.
// Or, the caller can Disconnect() to perform detaching from current trace.
func ReconnectBackgroundSession(ctx context.Context, tok *SessionToken) (*Session, error) {
	if tok.pid <= 0 {
		return nil, errors.Errorf("wrong pid (%d) is specified", tok.pid)
	}
	if _, err := os.Stat(tok.traceDataPath); err != nil {
		return nil, errors.Wrapf(err, "trace data file (%s) does not exist", tok.traceDataPath)
	}
	// The perfetto trace daemon can have exited already (e.g. after "duration_ms" elapsed).
	// So ignore error.
	proc, _ := process.NewProcess(int32(tok.pid))
	return &Session{cmd: nil, proc: proc, useTempFile: tok.useTempFile, compressTraceData: tok.compressTraceData, traceDataPath: tok.traceDataPath}, nil
}
