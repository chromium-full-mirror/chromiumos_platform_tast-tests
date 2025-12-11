// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"math"
	"strings"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/platform/bootperf"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			platform.RegisterBootPerfServiceServer(srv, &BootPerfService{s})
		},
	})
}

// BootPerfService implements tast.cros.platform.BootPerfService
type BootPerfService struct {
	s *testing.ServiceState
}

// EnableBootchart enables bootchart by adding "cros_bootchart" to kernel
// arguments.
func (*BootPerfService) EnableBootchart(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	if err := bootperf.EnableBootchart(ctx); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// DisableBootchart Disables bootchart by removing "cros_bootchart" from kernel
// arguments.
func (*BootPerfService) DisableBootchart(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	if err := bootperf.DisableBootchart(ctx); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// EnsureTlsdatedStopped ensures that tlsdated is stopped.
func (*BootPerfService) EnsureTlsdatedStopped(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	if err := bootperf.EnsureTlsdatedStopped(ctx); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// GetBootPerfMetrics gathers recorded timing and disk usage statistics during
// boot time. The test calculates some or all of the following metrics:
//   - seconds_kernel_to_startup
//   - seconds_kernel_to_startup_done
//   - seconds_kernel_to_chrome_exec
//   - seconds_kernel_to_chrome_main
//   - seconds_kernel_to_signin_start
//   - seconds_kernel_to_signin_wait
//   - seconds_kernel_to_signin_users
//   - seconds_kernel_to_login
//   - seconds_kernel_to_network
//   - seconds_kernel_to_patchpanel_start
//   - seconds_kernel_to_patchpanel_started
//   - seconds_startup_to_chrome_exec
//   - seconds_chrome_exec_to_login
//   - rdbytes_kernel_to_startup
//   - rdbytes_kernel_to_startup_done
//   - rdbytes_kernel_to_chrome_exec
//   - rdbytes_kernel_to_chrome_main
//   - rdbytes_kernel_to_login
//   - rdbytes_startup_to_chrome_exec
//   - rdbytes_chrome_exec_to_login
//   - seconds_power_on_to_kernel
//   - seconds_power_on_to_login
//   - seconds_shutdown_time
//   - seconds_reboot_time
//   - seconds_reboot_error
func (*BootPerfService) GetBootPerfMetrics(ctx context.Context, _ *empty.Empty) (*platform.GetBootPerfMetricsResponse, error) {
	out := &platform.GetBootPerfMetricsResponse{
		Metrics: make(map[string]float64),
	}

	// perform a testing.Poll() to wait for boot perf artifacts to show up.
	testing.ContextLog(ctx, "Wait until boot complete")
	err := bootperf.WaitUntilBootComplete(ctx)
	if err != nil {
		return nil, err
	}

	testing.ContextLog(ctx, "Gather boot time metrics")
	err = bootperf.GatherTimeMetrics(ctx, out)
	if err != nil {
		return nil, err
	}

	testing.ContextLog(ctx, "Gather boot disk read metrics")
	bootperf.GatherDiskMetrics(out)

	testing.ContextLog(ctx, "Gather firmware boot metric")
	err = bootperf.GatherFirmwareBootTime(ctx, out)
	if err != nil {
		return nil, err
	}

	err = bootperf.GatherFirmwareStageTimings(ctx, out)
	if err != nil {
		return nil, err
	}

	testing.ContextLog(ctx, "Calculate diff")
	bootperf.CalculateDiff(out)

	// Round the seconds_* values for nicer presentation.
	for key, value := range out.Metrics {
		if strings.HasPrefix(key, "seconds_") {
			out.Metrics[key] = math.Round(value*1000) / 1000
		}
	}

	return out, nil
}

// GetWiFiBootPerfMetrics gathers recorded WiFi-specific timing statistics
// during boot time. The caller is responsible for ensuring wificell testbed
// dependencies and making sure the targeted WiFi service enters the desired
// states, e.g. associated or ip-configured.
// The test calculates some or all of the following metrics:
//   - seconds_kernel_to_shill_start
//   - seconds_kernel_to_wifi_registered
//   - seconds_kernel_to_wifi_association
//   - seconds_kernel_to_wifi_configuration
//   - seconds_kernel_to_wifi_ready
//   - seconds_kernel_to_patchpanel_start
//   - seconds_kernel_to_patchpanel_started
//   - seconds_kernel_to_network
func (*BootPerfService) GetWiFiBootPerfMetrics(ctx context.Context, _ *empty.Empty) (*platform.GetBootPerfMetricsResponse, error) {
	out := &platform.GetBootPerfMetricsResponse{
		Metrics: make(map[string]float64),
	}

	// Perform a testing.Poll() to wait for boot/wifi perf artifacts to show up.
	testing.ContextLog(ctx, "Wait until boot complete and wifi ready")
	err := bootperf.WaitUntilWiFiReady(ctx)
	if err != nil {
		return nil, err
	}

	testing.ContextLog(ctx, "Gather boot time and wifi metrics")
	err = bootperf.GatherWiFiTimeMetrics(ctx, out)
	if err != nil {
		return nil, err
	}

	// Round the seconds_* values for nicer presentation.
	for key, value := range out.Metrics {
		if strings.HasPrefix(key, "seconds_") {
			out.Metrics[key] = math.Round(value*1000) / 1000
		}
	}

	return out, nil
}

func (*BootPerfService) GetRebootMetrics(ctx context.Context, _ *empty.Empty) (*platform.GetRebootMetricsResponse, error) {
	out := &platform.GetRebootMetricsResponse{
		Metrics: make(map[string]float64),
	}

	testing.ContextLog(ctx, "Gather reboot metrics")
	err := bootperf.GatherRebootMetrics(out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

// GetBootPerfRawData gathers raw data used in calculating boot perf metrics for
// debugging.
func (*BootPerfService) GetBootPerfRawData(ctx context.Context, _ *empty.Empty) (*platform.GetBootPerfRawDataResponse, error) {
	// Passed cached bootstat raw data to the client.
	raw := make(map[string][]byte)

	if err := bootperf.GatherMetricRawDataFiles(raw); err != nil {
		return nil, err
	}
	if err := bootperf.GatherConsoleRamoops(raw); err != nil {
		return nil, err
	}
	bootperf.StoreFirmwareTimestamps(ctx, raw)

	return &platform.GetBootPerfRawDataResponse{
		RawData: raw,
	}, nil
}

// GetRebootRawData gathers raw data used in calculating reboot metrics for
// debugging.
func (*BootPerfService) GetRebootRawData(ctx context.Context, _ *empty.Empty) (*platform.GetRebootRawDataResponse, error) {
	// Passed cached bootstat raw data to the client.
	raw := make(map[string][]byte)

	if err := bootperf.GatherRebootRawDataFiles(raw); err != nil {
		return nil, err
	}

	return &platform.GetRebootRawDataResponse{
		RawData: raw,
	}, nil
}
