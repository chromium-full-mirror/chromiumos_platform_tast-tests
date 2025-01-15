// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracing

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"strconv"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/testexec"
	pb "go.chromium.org/tast-tests/cros/services/cros/tracing"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
)

// RemoteSessionToken is a wrapper of PerfettoSessionToken.
type RemoteSessionToken = pb.PerfettoSessionToken

// RemoteSession implements a wrapper of PerfettoTraceService.
type RemoteSession struct {
	service       pb.PerfettoTraceServiceClient
	token         *RemoteSessionToken
	traceDataPath string
}

// Stop stops the remote perfetto.
func (s *RemoteSession) Stop(ctx context.Context) error {
	if _, err := s.service.Stop(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to stop remote session")
	}
	return nil
}

// Finalize finishes the remote perfetto session and remove temporary trace
// data.
func (s *RemoteSession) Finalize(ctx context.Context) error {
	if _, err := s.service.Finalize(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to finalize remote session")
	}
	s.service = nil
	return nil
}

// Token gets a session token from a background perfetto session.
func (s *RemoteSession) Token() *RemoteSessionToken {
	return s.token
}

// TraceDataPath returns the remote file path of trace data.
func (s *RemoteSession) TraceDataPath() string {
	return s.traceDataPath
}

type remoteSessionOption func(config *pb.PerfettoOption)

// WithTraceDataPath configures the session with trace data written to the
// given path.
func WithTraceDataPath(path string) remoteSessionOption {
	return func(config *pb.PerfettoOption) {
		config.TraceDataPath = path
	}
}

// WithCompression configures the session with compressing the trace data
// after the session is finalized.
func WithCompression() remoteSessionOption {
	return func(config *pb.PerfettoOption) {
		config.CompressTraceData = true
	}
}

// InBackground configures the session is running in background (thus it is
// reconnectable.)
func InBackground() remoteSessionOption {
	return func(config *pb.PerfettoOption) {
		config.Background = true
	}
}

// WithConfigPath configures the remote perfetto to run with the specific
// configuration file.
func WithConfigPath(path string) remoteSessionOption {
	return func(config *pb.PerfettoOption) {
		config.Config = &pb.PerfettoOption_FilePath{FilePath: path}
	}
}

// WithConfigTextData configures the remote perfetto to run with the given
// text configuration data.
func WithConfigTextData(data string) remoteSessionOption {
	return func(config *pb.PerfettoOption) {
		config.Config = &pb.PerfettoOption_Data{Data: data}
	}
}

// StartRemoteSession starts a system-wide perfetto tracing with specified
// options in DUT.
// Caller must specify either WithConfigPath or WithConfigTextData option.
// After starting the remote perfetto, run workload, stop tracing, and
// download the trace data file via dutfs. After that, call Finalize to
// remove temporary files.
// If InBackground option is specified, the created RemoteSession is
// reconnectable.
func StartRemoteSession(ctx context.Context, cl *rpc.Client, opts ...remoteSessionOption) (*RemoteSession, error) {
	service := pb.NewPerfettoTraceServiceClient(cl.Conn)
	config := &pb.PerfettoOption{}
	for _, opt := range opts {
		opt(config)
	}
	if config.GetConfig() == nil {
		return nil, errors.New("this requires either config file path or config data")
	}
	res, err := service.Start(ctx, config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start remote session")
	}
	return &RemoteSession{service: service, token: res.GetToken(), traceDataPath: res.TraceDataPath}, nil
}

// ReconnectRemoteSession reconnects to a running perfetto session specified
// by `tok`, which is returned by RemoteSession.Token. This is used for the
// remote test which involves a network disconnection.
// Call RemoteSession.Token() to get a token before running a remote workload
// which causes the network disconnection, and after reconnect to the DUT,
// reconnect to running perfetto session using this ReconnectRemoteSession
// with the token.
func ReconnectRemoteSession(ctx context.Context, cl *rpc.Client, tok *RemoteSessionToken) (*RemoteSession, error) {
	service := pb.NewPerfettoTraceServiceClient(cl.Conn)
	if _, err := service.Reconnect(ctx, tok); err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to a remote session")
	}
	return &RemoteSession{service: service, traceDataPath: tok.TraceDataPath}, nil
}

// SaveRemoteSessionTraceData reconnects to a perfetto session, stops it and
// saves the trace data by saveFunc(). Typically, you can control remote
// background perfetto with StartRemoteSession(), RemoteSession.Token() and
// SaveRemoteSessionTraceData(). E.g.
//
// sess,err := StartRemoteSession(ctx, cl, InBackground(),...)
//
//	if err != nil {
//	    s.Fatal("Failed to start remote perfetto")
//	}
//
// tok := sess.Token()
// runWorkload()	// run a workload which can disconnect service
// cl = redialConnection()  // so redial to the service
//
//	err: = SaveRemoteSessionTraceData(ctx, cl, tok, func(src string) error {
//	  return s.DUT().GetFile(ctx, src, dest)
//	})
//
//	if err != nil {
//	    s.Fatal("Failed to save the trace data")
//	}
//
// s.Logf("trace data is saved to %s", dest)
func SaveRemoteSessionTraceData(ctx context.Context, cl *rpc.Client, tok *RemoteSessionToken, saveFunc func(src string) error) error {
	sess, err := ReconnectRemoteSession(ctx, cl, tok)
	if err != nil {
		return errors.Wrap(err, "failed to reconnect to perfetto session")
	}
	defer sess.Finalize(ctx)
	// Ignore the error because the perfetto command may already stop.
	sess.Stop(ctx)
	return saveFunc(sess.TraceDataPath())
}

// PerfettoQueryValue implements simple query for series of values
type PerfettoQueryValue struct {
	Name      string
	Config    string
	QueryFile string
	token     *RemoteSessionToken
	Values    []float64
}

// NewPerfettoQueryValue setup a PerfettoQueryValue with given name and
// files. Note that the `configFile` must configure perfetto correctly to
// record events (tables) used by `queryFile`. The `name` must specify
// the corresponding column name.
func NewPerfettoQueryValue(name, configFile, queryFile string) (*PerfettoQueryValue, error) {
	config, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	return &PerfettoQueryValue{Name: name, Config: string(config), QueryFile: queryFile}, nil
}

// StartAndDetach starts recording events via perfetto and detach from
// remote machine. Thus, user can record a metric of some actions after
// this function call.
func (q *PerfettoQueryValue) StartAndDetach(ctx context.Context, cl *rpc.Client) error {
	sess, err := StartRemoteSession(ctx, cl, WithConfigTextData(q.Config), InBackground())
	if err != nil {
		return err
	}
	q.token = sess.Token()
	return nil
}

// StopAndQuery stops recording and query a metric from the record.
// The query must returns columns which has PerfettoQueryValue::Name.
// If there are more than one results (lines) exist, this will record
// all values to PerfettoQueryValue::Values.
// Since the PerfettoQueryValue::Values is not cleared automatically,
// if you run PerfettoQueryValue::StartAndDetach() and
// PerfettoQueryValue::StopAndQuery() repeatedly, it appends the results.
func (q *PerfettoQueryValue) StopAndQuery(ctx context.Context, dut *dut.DUT, cl *rpc.Client) error {
	const traceProcessorPath = "/usr/bin/trace_processor_shell"
	if q.token == nil {
		return errors.New("perfetto session is not started")
	}
	tempFile, err := os.CreateTemp("/tmp", "perfetto-trace-*.pb")
	if err != nil {
		return errors.Wrap(err, "failed to create temp file")
	}
	tempFile.Close()
	defer func() {
		os.Remove(tempFile.Name())
	}()

	// Reconnect to remote session and get the result file.
	if err := SaveRemoteSessionTraceData(ctx, cl, q.token,
		func(src string) error {
			return dut.GetFile(ctx, src, tempFile.Name())
		}); err != nil {
		return err
	}
	q.token = nil

	cmd := testexec.CommandContext(ctx, traceProcessorPath, tempFile.Name(), "-q", q.QueryFile)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to run query with trace_processor_shell")
	}
	reader := csv.NewReader(bytes.NewReader(out))
	data, err := reader.ReadAll()
	if err != nil {
		return errors.Wrap(err, "failed to parse query result")
	}
	if len(data) <= 1 {
		return errors.New("failed to decord CSV or there is no data")
	}
	idx := -1
	// Find appropriate column.
	for i, n := range data[0] {
		if q.Name == n {
			idx = i
			break
		}
	}
	if idx == -1 {
		return errors.New("failed to find corresponding column in the query result")
	}
	// Append corresponding results to Values.
	for _, d := range data[1:] {
		fv, err := strconv.ParseFloat(d[idx], 64)
		if err != nil {
			return errors.Wrap(err, "failed to parse query result value")
		}
		q.Values = append(q.Values, fv)
	}
	return nil
}
