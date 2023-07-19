// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This test is similar to MonitorUnuploadedCrashEvent. If you make any changes,
// please also apply the same change to MonitorUnuploadedCrashEvent if it's
// applicable.
//
// /var/log/messages will contain the following error message:
//
//     ERR crash_sender[6350]: dryrun:ERROR crash_sender:
//     [crash_sender_base.cc(152)] Error writing out crash-sender-done file:
//     /run/crash_reporter/crash-sender-done: Read-only file system (30)
//
// This is because cros_healthd does not mount /run/crash_reporter and thus
// crash_sender won't be able to write to that directory. This is not a problem
// for production because crash_sender only writes to /run/crash_reporter in the
// mock mode to indicate its state.

package health

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// JSON fields of uploaded crash events.
type uploadedCrashInfo struct {
	CaptureTime float64 `json:"capture_time"`
	CrashType   int     `json:"crash_type"`
	LocalID     string  `json:"local_id"`
	UploadInfo  struct {
		CrashReportID string  `json:"crash_report_id"`
		CreationTime  float64 `json:"creation_time"`
		Offset        int     `json:"offset"`
	} `json:"upload_info"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         MonitorUploadedCrashEvent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Monitors uploaded crash events detected properly or not",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		// TODO(b/266018436): Promote to critical.
		Attr:         []string{"group:criticalstaging", "group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
		// crash_sender needs more time to run because crash_sender
		// would hold off for 30 seconds if the crash meta file is too
		// new. See the comments above
		// |SenderBase::Options::hold_off_time| in
		// platform2/crash-reporter/crash_sender_base.h.
		Timeout: 4 * time.Minute,
	})
}

// checkUploadedOutput checks the output. Returns true if OK.
func checkUploadedOutput(ctx context.Context, s *testing.State, stdout []byte) bool {
	// Sample output:
	// Subscribe to crash events successfully.
	// Crash event received: {"capture_time":1686094638,"crash_type":0,"local_id":"de95be1e8e9fa3f23a3b79743806a255"}
	// Crash event received: {"capture_time":1686097875,"crash_type":0,"local_id":"e678e596590f52a86ac76dea8b0849f4"}
	// Crash event received: {"capture_time":1686094545,"crash_type":0,"local_id":"9bd36ec364a908d135b83dc150306766"}
	// Crash event received: {"capture_time":1686094545,"crash_type":0,"local_id":"9bd36ec364a908d135b83dc150306766","upload_info":{"crash_report_id":"bf510c0d4136e24f","creation_time":1686097916.592052,"offset":0}}
	// Crash event received: {"capture_time":1686094638,"crash_type":0,"local_id":"de95be1e8e9fa3f23a3b79743806a255","upload_info":{"crash_report_id":"0596fc4bcf9f05ca","creation_time":1686097916.592052,"offset":1}}
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := scanner.Bytes()
		// Skip the "Crash event received:" prefix and jump to the first
		// curly brace if it exists.
		_, line, found := bytes.Cut(line, []byte{'{'})
		if !found {
			s.Log("Not able to find { in ", scanner.Text(), " Move onto the next line")
			continue
		}
		line = append([]byte{'{'}, line...)

		// Parse the rest of the line as json.
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		var crash uploadedCrashInfo
		if err := decoder.Decode(&crash); err != nil {
			s.Log("Not able to parse ", scanner.Text(), ": ", err, " Move onto the next line")
			continue
		}
		// Check that the crash struct have been filled with some info.
		// Specifically, check crash.UploadInfo so that unuploaded
		// crashes are excluded.
		if crash.CaptureTime == 0 || len(crash.UploadInfo.CrashReportID) == 0 {
			s.Log("Not able to extract crash info from ", scanner.Text(), " Move onto the next line")
			continue
		}
		s.Log("Extracted uploaded crash event from ", scanner.Text())
		return true
	}
	return false
}

func MonitorUploadedCrashEvent(ctx context.Context, s *testing.State) {
	// Reserve 5 seconds for cleaning up crash tests.
	ctxForCleanUpCrashSetup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// This will ensure no pre-existing crashes are in /var/spool/crash etc.
	// It will ensure that no other instance of crash_sender is running. It
	// will also help set up consent.
	//
	// It will also kill existing crash_sender processes. This isn't a
	// problem for the test because there is no subscriber at the beginning
	// of this test and healthD doesn't invoke crash_sender --dry_run in
	// this case.
	crash.SetUpCrashTest(ctx, crash.WithMockConsent())
	defer crash.TearDownCrashTest(ctxForCleanUpCrashSetup)
	// Don't actually upload crashes.
	crash.EnableMockSending(true)

	// Trigger unuploaded crash event: Run the sleep command and crash it.
	sleepCmd := testexec.CommandContext(ctx, "sleep", "100")
	if err := sleepCmd.Start(); err != nil {
		s.Fatal("Failed to start the sleep command: ", err)
	}
	if err := sleepCmd.Signal(unix.SIGSEGV); err != nil {
		s.Fatal("Failed to crash the sleep command: ", err)
	}
	err := sleepCmd.Wait()
	waitStatus, ok := testexec.GetWaitStatus(err)
	if !ok {
		s.Fatal("Failed to get sleep's wait status: ", err)
	}
	if !waitStatus.Signaled() || waitStatus.Signal() != unix.SIGSEGV {
		s.Fatal("Failed to crash sleep: ", err)
	}
	// When a crash occurs, crash_reporter is responsible for writing crash
	// meta files. However, occasionally `cros-health-tool` already has
	// started subscribing while the *.meta file hasn't been created yet.
	// Wait for crash files to be ready.
	if crashFiles, err := crash.WaitForCrashFiles(ctx, crash.GetAllCrashDirs(ctx), []string{`coreutils\.[\d\.]+\.meta`}); err != nil {
		s.Fatal("Failed to wait for crash files: ", err)
	} else {
		for _, files := range crashFiles {
			for _, file := range files {
				s.Log("Crash meta file used to create the test uploaded crash: ", file)
			}
		}
	}

	// Convert the unuploaded crash event to uploaded crash event.
	s.Log("Starting crash_sender")
	if _, err := crash.RunSender(ctx); err != nil {
		s.Fatal("Failed to run crash_sender: ", err)
	}

	// Run monitor command in background.
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", "--category=crash",
		"--length_seconds=10")
	stdout, stderr, err := monitorCmd.SeparatedOutput()
	if err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}

	if len(stderr) > 0 {
		s.Fatal("Failed to detect uploaded crash event, stderr: ", string(stderr), ", stdout: ", string(stdout))
	}

	if !checkUploadedOutput(ctx, s, stdout) {
		s.Fatal("Failed to detect uploaded event, event output: ", string(stdout))
	}
}
