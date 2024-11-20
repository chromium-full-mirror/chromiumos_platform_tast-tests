// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package osperf

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/chrome/extension"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/osperf"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabOpenLatencyPerfRaw,
		Desc:         "Measures tab open latency remotely to get stable results quickly",
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Contacts:     []string{"baseos-perf@google.com", "mtomoya@google.com"},
		Attr:         []string{"group:mainline", "informational"},
		Data: []string{
			"tab_open_latency_perf_raw/manifest.json",
			"tab_open_latency_perf_raw/bench.js",
			"tab_open_latency_perf_raw/background.js",
			"tab_open_latency_perf_raw/bench.html",
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ConnService",
			"tast.cros.ui.TconnService",
		},
	})
}

func TabOpenLatencyPerfRaw(ctx context.Context, s *testing.State) {
	d := s.DUT()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to dial to DUT for remote file system: ", err)
	}
	defer cl.Close(ctx)

	const benchDir = "/tmp/bluebench" // Using /tmp dir to make it runnable with read-only rootfs.
	fs := dutfs.NewClient(cl.Conn)
	// (Re)create benchDir.
	if dirExists, err := fs.Exists(ctx, benchDir); err != nil {
		s.Fatal("Failed to check the existence of the USB device's original mount point: ", err)
	} else if dirExists {
		if err := fs.RemoveAll(ctx, benchDir); err != nil {
			s.Fatalf("Failed to mkdir %s: %v", benchDir, err)
		}
	}
	if err := fs.MkDir(ctx, benchDir, os.FileMode(0750)); err != nil {
		s.Fatalf("Failed to mkdir %s: %v", benchDir, err)
	}
	dataPaths := s.DataPaths()
	dataMap := make(map[string]string, len(dataPaths))

	const dataDir = "tab_open_latency_perf_raw/"
	for name, path := range dataPaths {
		if !strings.HasPrefix(name, dataDir) {
			s.Fatalf("The data path should start with %s but got %s", dataDir, name)
		}
		dataMap[path] = filepath.Join(benchDir, strings.TrimPrefix(name, dataDir))
	}
	if bytes, err := linuxssh.PutFiles(ctx, d.Conn(), dataMap, linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy bluebench files via ssh")
	} else if bytes == 0 {
		s.Fatal("zero bytes transferred while copying the bluebench files")
	}

	cr := run(ctx, s, cl, benchDir, filepath.Dir(dataPaths[dataDir+"manifest.json"]))
	defer cr.Close(ctx, &emptypb.Empty{})
}

type tabOpenLatencyTestRawResult struct {
	TabOpenLatencies []float64 `json:"tab_open_latencies"`
	TotalDuration    float64   `json:"total_duration"`
	NumIterations    int64     `json:"num_iterations"`
	ResultLog        string    `json:"result_log"`
}

func run(ctx context.Context, s *testing.State, cl *rpc.Client, extDir, hostExtDir string) ui.ChromeServiceClient {
	s.Log("Setting up bluebench extension")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	extID, err := extension.ComputeExtensionID(hostExtDir)
	if err != nil {
		s.Fatalf("Failed to compute extension ID for %v: %v", extDir, err)
	}

	cr := ui.NewChromeServiceClient(cl.Conn)
	req := ui.NewRequest{UnpackedExtensions: []string{extDir}}
	if _, err := cr.New(ctx, &req); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	conn := ui.NewConnServiceClient(cl.Conn)

	benchURL := "chrome-extension://" + extID + "/bench.html"
	resTab, err := osperf.OpenBenchPage(ctx, benchURL, conn)
	if err != nil {
		s.Fatal("Failed to open new Tab: ", err)
	}
	s.Log("Running the benchmark")
	resRun, err := conn.Call(ctx, &ui.ConnCallRequest{
		Id: resTab.Id,
		Fn: `
			() => {
				return window.startBench();
			}
			`,
	})
	if err != nil {
		s.Fatal("startBench failed: ", err)
	}
	s.Log("Completed the benchmark")

	resMap := resRun.AsInterface().(map[string]interface{})
	jsonString, _ := json.Marshal(resMap)
	info := tabOpenLatencyTestRawResult{}

	if err = json.Unmarshal(jsonString, &info); err != nil {
		s.Fatal("Failed to parse tabOpenLatencyTestRawResult: ", err)
	}

	if err := os.WriteFile(filepath.Join(s.OutDir(), "bluebench_log.txt"),

		[]byte(info.ResultLog), 0644); err != nil {
		s.Error("Failed to write bluebench_log.txt: ", err)
	}

	dumpHardwareInformationWithVulnerabilityStatus(ctx, s)

	// Save the result as results-chart.json.
	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "TabOpenLatencies",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, info.TabOpenLatencies...)
	pv.Set(perf.Metric{
		Name:      "TotalDuration",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, info.TotalDuration)
	pv.Set(perf.Metric{
		Name:      "NumIterations",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, float64(info.NumIterations))
	pv.Save(s.OutDir())

	return cr
}

func dumpHardwareInformationWithVulnerabilityStatus(ctx context.Context, s *testing.State) {
	dumpCommandMap := map[string]string{
		"crossystem.txt":                  "crossystem",
		"vpd.txt":                         "vpd -l",
		"uname.txt":                       "uname -a",
		"meminfo.txt":                     "cat /proc/meminfo",
		"lscpu.txt":                       "lscpu",
		"cmdline.txt":                     "cat /proc/cmdline",
		"kernel_version.txt":              "cat /proc/version",
		"cpuinfo.txt":                     "cat /proc/cpuinfo",
		"ifconfig.txt":                    "ifconfig",
		"mount.txt":                       "mount",
		"current_kernel_part_hash.txt":    "md5sum $(rootdev -s | sed -e 's/5$/4/' -e 's/3$/2/')",
		"current_kernel_verification.txt": "vbutil_kernel --verify $(rootdev -s | sed -e 's/5$/4/' -e 's/3$/2/')",
		"rootdev.txt":                     "rootdev -s",
		"bootid.txt":                      "cat /proc/sys/kernel/random/boot_id",
		"lsb_release.txt":                 "cat /etc/lsb-release",
		"messages.txt":                    "cat /var/log/messages",
		"hw_vuln.txt":                     "find /sys/devices/system/cpu/vulnerabilities/ -type f -print -exec cat {} \\;",
	}
	dut := s.DUT()
	for name, command := range dumpCommandMap {
		if err := osperf.DumpCommandOutput(ctx, dut, filepath.Join(s.OutDir(), name), command); err != nil {
			s.Fatal("Failed to dump info: ", err)
		}
	}
}
