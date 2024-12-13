// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perfetto

import (
	"context"
	"io"
	"os"
	"strings"

	"android.googlesource.com/platform/external/perfetto/protos/perfetto/metrics/github.com/google/perfetto/perfetto_proto"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/errors"
)

// RunPerfetto uses gRPC to run perfetto cmdline with
// |traceConfigFile| in the DUT.
func RunPerfetto(ctx context.Context, pc platform.PerfettoTraceBasedMetricsServiceClient, traceConfigPath string) (ret string, retErr error) {
	config, err := os.ReadFile(traceConfigPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to read config file")
	}

	stream, err := pc.GeneratePerfettoTrace(ctx, &platform.GeneratePerfettoTraceRequest{Config: string(config)})
	if err != nil {
		return "", errors.Wrap(err, "failed to call gRPC GeneratePerfettoTrace")
	}

	tempFile, err := os.CreateTemp("/tmp", "perfetto-trace-*.pb")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp file")
	}

	defer func() {
		if err := tempFile.Close(); err != nil && retErr == nil {
			ret = ""
			retErr = err
		}
		if retErr != nil {
			os.Remove(tempFile.Name())
		}
	}()

	for {
		res, err := stream.Recv()
		if err == io.EOF {
			return tempFile.Name(), nil
		}
		if err != nil {
			return "", errors.Wrap(err, "failed to receive from the stream")
		}
		if _, err := tempFile.Write(res.Result); err != nil {
			return "", errors.Wrap(err, "failed to write to temp file")
		}
	}
}

// RunMetrics collects the result with trace_processor_shell.
func RunMetrics(ctx context.Context, outputPath string, metrics []string) (*perfetto_proto.TraceMetrics, error) {
	const traceProcessorPath = "/usr/bin/trace_processor_shell"
	metric := strings.Join(metrics, ",")
	cmd := testexec.CommandContext(ctx, traceProcessorPath, outputPath, "--run-metrics", metric)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run metrics with trace_processor_shell")
	}

	tbm := &perfetto_proto.TraceMetrics{}
	if err := prototext.Unmarshal(out, tbm); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal metrics result")
	}

	return tbm, nil
}
