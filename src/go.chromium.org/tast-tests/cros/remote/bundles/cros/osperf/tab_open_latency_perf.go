// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package osperf

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabOpenLatencyPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures tab open latency remotely to get stable results quickly",
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Contacts:     []string{"baseos-perf@google.com", "hikalium@google.com"},
		Attr:         []string{"group:mainline", "informational"},
		Data:         []string{"manifest.json", "bench.js", "background.js", "bench.html"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ConnService",
			"tast.cros.ui.TconnService",
		},
	})
}

func TabOpenLatencyPerf(ctx context.Context, s *testing.State) {
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
	for name, path := range dataPaths {
		dataMap[path] = filepath.Join(benchDir, name)
	}
	if bytes, err := linuxssh.PutFiles(ctx, d.Conn(), dataMap, linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy bluebench files via ssh")
	} else if bytes == 0 {
		s.Fatal("zero bytes transferred while copying the bluebench files")
	}

	cr := runBluebench(ctx, s, cl, benchDir, filepath.Dir(dataPaths["manifest.json"]))
	defer cr.Close(ctx, &emptypb.Empty{})
}

// readKeyFromExtensionManifest returns the decoded public key from an
// extension manifest located at path. An error is returned if the manifest
// is missing or malformed. A nil key is returned if the manifest is
// parsable but doesn't contain a key.
//
// Note: This function is brought from src/go.chromium.org/tast-tests/cros/local/chrome/internal/extension/util.go.
func readKeyFromExtensionManifest(path string) ([]byte, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	j := make(map[string]interface{})
	if err = json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	if enc, ok := j["key"].(string); ok {
		return base64.StdEncoding.DecodeString(enc)
	}
	return nil, nil
}

// computeExtensionID computes the 32-character ID that Chrome will use for an unpacked
// extension in dir. The extension's manifest file must contain the "key" field.
// Use the following command to generate a new key:
//
//	openssl genrsa 2048 | openssl rsa -pubout -outform der | openssl base64 -A
//
// Note: This function is brought from src/go.chromium.org/tast-tests/cros/local/chrome/internal/extension/util.go.
func computeExtensionID(dir string) (string, error) {
	key, err := readKeyFromExtensionManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return "", err
	}

	// Chrome computes an extension's ID by creating a SHA-256 digest of the extension's public key
	// and converting its first 16 bytes to 32 hex characters, with the added twist that the
	// characters 'a'-'p' are used rather than '0'-'f'.
	sum := sha256.Sum256(key)
	id := make([]byte, 32)
	for i, b := range sum[:len(id)/2] {
		id[i*2] = b/16 + 'a'
		id[i*2+1] = b%16 + 'a'
	}
	return string(id), nil
}

// queryTestConnTabIDs queries the tab IDs which is used for TconnService.
func queryTestConnTabIDs(ctx context.Context, tconn ui.TconnServiceClient, condition string) ([]int, error) {
	res, err := tconn.Call(ctx, &ui.CallRequest{
		Fn: `async () => {
			let tabs = await tast.promisify(chrome.tabs.query)({` + condition + `});
			return tabs.map((tab) => tab.id);
		  }`,
		Args: []*structpb.Value{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "cannot query tab list")
	}
	ids := res.AsInterface().([]interface{})
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id.(float64))
	}
	return out, nil
}

type tabOpenLatencyTestResult struct {
	TabOpenLatencyMean float64   `json:"tab_open_latency_mean"`
	TabOpenLatencyMax  float64   `json:"tab_open_latency_max"`
	TabOpenLatencyMin  float64   `json:"tab_open_latency_min"`
	TotalDuration      float64   `json:"total_duration"`
	TotalRunCount      float64   `json:"total_run_count"`
	ConvergedMeans     []float64 `json:"converged_means"`
	ResultLog          string    `json:"result_log"`
}

func openBenchPage(ctx context.Context, s *testing.State, benchURL string, conn ui.ConnServiceClient) *ui.NewConnResponse {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resTab, err := conn.NewConn(ctx, &ui.NewConnRequest{Url: benchURL})
	if err != nil {
		s.Fatal("Failed to open new Tab: ", err)
	}
	return resTab
}

func runBluebench(ctx context.Context, s *testing.State, cl *rpc.Client, extDir, hostExtDir string) ui.ChromeServiceClient {

	s.Log("Setting up bluebench extension")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	extID, err := computeExtensionID(hostExtDir)
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
	resTab := openBenchPage(ctx, s, benchURL, conn)

	s.Log("Running the benchmark")
	resRun, err := conn.Call(ctx, &ui.ConnCallRequest{
		Id: resTab.Id,
		Fn: `
		() => {
			// document.getElementById("numConvergedResultsInput").value = "1";
			return window.startBench();
		}
		`,
	})
	if err != nil {
		s.Fatal("startBench failed: ", err)
	}
	s.Log("Completed the benchmark")

	resMap := resRun.AsInterface().(map[string]interface{})
	jsonString, err := json.Marshal(resMap)
	info := tabOpenLatencyTestResult{}
	if err = json.Unmarshal(jsonString, &info); err != nil {
		s.Fatal("Failed to parse tabOpenLatencyTestResult: ", err)
	}
	if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "bluebench_log.txt"),
		[]byte(info.ResultLog), 0644); err != nil {
		s.Error("Failed to write bluebench_log.txt: ", err)
	}
	dumpHardwareInformation(ctx, s)

	// Save the result as results-chart.json.
	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "TabOpenLatencyMean",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, info.TabOpenLatencyMean)
	pv.Set(perf.Metric{
		Name:      "TabOpenLatencyMax",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, info.TabOpenLatencyMax)
	pv.Set(perf.Metric{
		Name:      "TabOpenLatencyMin",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, info.TabOpenLatencyMin)
	pv.Set(perf.Metric{
		Name:      "TotalDuration",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, info.TotalDuration)
	pv.Set(perf.Metric{
		Name:      "TotalRunCount",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, info.TotalRunCount)
	pv.Set(perf.Metric{
		Name:      "ConvergedMeans",
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}, info.ConvergedMeans...)
	pv.Save(s.OutDir())

	return cr
}

func dumpCommandOutput(ctx context.Context, s *testing.State, name, cmd string) {
	output, err := s.DUT().Conn().CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run command to dump info: ", cmd, err)
	}
	if err := ioutil.WriteFile(filepath.Join(s.OutDir(), name),
		[]byte(output), 0644); err != nil {
		s.Errorf("Failed to write %v: %v", name, err)
	}
}

func dumpHardwareInformation(ctx context.Context, s *testing.State) {
	dumpCommandOutput(ctx, s, "crossystem.txt", "crossystem")
	dumpCommandOutput(ctx, s, "vpd.txt", "vpd -l")
	dumpCommandOutput(ctx, s, "uname.txt", "uname -a")
	dumpCommandOutput(ctx, s, "meminfo.txt", "cat /proc/meminfo")
	dumpCommandOutput(ctx, s, "lscpu.txt", "lscpu")
	dumpCommandOutput(ctx, s, "cmdline.txt", "cat /proc/cmdline")
	dumpCommandOutput(ctx, s, "kernel_version.txt", "cat /proc/version")
	dumpCommandOutput(ctx, s, "cpuinfo.txt", "cat /proc/cpuinfo")
	dumpCommandOutput(ctx, s, "ifconfig.txt", "ifconfig")
	dumpCommandOutput(ctx, s, "mount.txt", "mount")
	dumpCommandOutput(ctx, s, "current_kernel_part_hash.txt", "md5sum $(rootdev -s | sed -e 's/5$/4/' -e 's/3$/2/')")
	dumpCommandOutput(ctx, s, "current_kernel_verification.txt", "vbutil_kernel --verify $(rootdev -s | sed -e 's/5$/4/' -e 's/3$/2/')")
	dumpCommandOutput(ctx, s, "rootdev.txt", "rootdev -s")
	dumpCommandOutput(ctx, s, "bootid.txt", "cat /proc/sys/kernel/random/boot_id")
	dumpCommandOutput(ctx, s, "lsb_release.txt", "cat /etc/lsb-release")
	dumpCommandOutput(ctx, s, "messages.txt", "cat /var/log/messages")
}
