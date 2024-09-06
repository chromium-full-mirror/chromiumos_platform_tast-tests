// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	rep "go.chromium.org/chromiumos/reporting"
	xdr "go.chromium.org/chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdcommon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"
)

type testCase string
type fileEventType string

const (
	userFiles testCase = "USER_FILES"
	rootfs    testCase = "ROOTFS"
	// test hardlink behavior
	hardlink testCase = "HARDLINK"
	// mounts a folder/archive from user directory to /media/archive/test/
	mountedUserArchive testCase = "MOUNTED_USER_ARCHIVE"
	// mounts r/w folder from /tmp to /media/removable/usb_test/
	massStorageUSB  testCase      = "USB_MASS_STORAGE"
	cookies         testCase      = "COOKIES"
	userCredential  testCase      = "ENCRYPTED_USER_CREDENTIAL"
	devicePolicy    testCase      = "DEVICE_POLICY"
	devicePolicyKey testCase      = "DEVICE_POLICY_KEY"
	tpmKey          testCase      = "TPM_KEY"
	authFactors     testCase      = "AUTH_FACTORS"
	modifyEvent     fileEventType = "MODIFY"
	readEvent       fileEventType = "READ"
)

type fileTypeParams struct {
	testType testCase
}

type commandDetails struct {
	cmd      *testexec.Cmd
	filePath string // the path of the file in the init mount ns
	cleanup  func(context.Context)
	expected *expectedResult
}

type expectedResult struct {
	command       string // useful for debug
	process       *xdr.Process
	parentProcess *xdr.Process
	beforeStat    *syscall.Stat_t // stat taken before the command executes
	afterStat     *syscall.Stat_t // stat taken after the command executes
	filePath      string
	eventType     fileEventType              //read or modify
	eventSubType  *xdr.FileModify_ModifyType // modify, modify and write or write only
	fileType      xdr.SensitiveFileType
}

func init() {

	testing.AddTest(&testing.Test{
		Func: FileEvents,
		Desc: "Checks that XDR network events are correctly being reported",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr:         []string{},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"bpf", "chrome", "shipping_kernel"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      fixture.LoggedInWithFileEventsEnabled,
		Params: []testing.Param{{
			Name: "user_fs",
			Val: fileTypeParams{
				testType: userFiles,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "root_fs",
			Val: fileTypeParams{
				testType: rootfs,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}},
	})
}

// FileEvents triggers various file events in different monitored sensitive areas and verifies that the
// correct events are sent over dbus.
func FileEvents(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 15*time.Second)
	defer cancel()

	// On test failure save off the kernel trace.
	defer func() {
		if err := secagentdcommon.OnErrorSaveKernelTrace(cleanupCtx, s.OutDir(), s.HasError); err != nil {
			s.Logf("Unable to export kernel traces for failure analysis:%s", err)
		}
	}()
	// Restart with default parameter.
	defer secagentdupstart.RestartSecagentd(cleanupCtx, false)

	// Clear out old entries from the kernel trace to make an easier failure
	// analysis.
	if err := secagentdcommon.ClearKernelTrace(ctx); err != nil {
		s.Log("Unable to clear the kernel trace file: ", err)
	}

	if err := secagentdcommon.ClearSecagentdLog(); err != nil {
		s.Fatal("Failed to clear secagentd log: ", err)
	}
	const batchIntervalS = 5
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx, false,
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))

	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}
	if err := secagentdcommon.WaitForStringInLog(ctx, "File plugin activated"); err != nil {
		s.Fatal("Failed to verify that fileplugin activated: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	normalizedUser := cr.NormalizedUser()
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	systemPath, err := cryptohome.SystemPath(ctx, normalizedUser)
	userPath, err := cryptohome.UserPath(ctx, normalizedUser)
	mountedVaultPath, err := cryptohome.MountedVaultPath(ctx, normalizedUser)

	s.Log("chrome normalized user:", normalizedUser)
	s.Log("cryptohome systemspath:", systemPath)
	s.Log("cryptohome downloadspath:", downloadsPath)
	s.Log("cryptohome userpath:", userPath)
	s.Log("cryptohome mountedVaultPath:", mountedVaultPath)

	stopDbusMonitoring, err := secagentddbusmonitor.SetupDbusMonitor(ctx, agentPid)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}

	// create a list of expectations

	cmdDetails, err := getFileEventDetails(ctx, s.Param().(fileTypeParams).testType, cr)
	if err != nil {
		s.Fatal("Invalid test case: ", err)
	}

	// key is the pid of the process triggering a file event.
	// value is the criteria the resulting file event should be evaluated against.
	expectedResults := make(map[uint64]*expectedResult)

	// Loop through and wait for each file command. This is needed so that
	// an accurate stat can be snap shotted after each anticipated file command.
	for _, detail := range cmdDetails {
		if detail.expected != nil {
			// record the stat before the command executes if the file exists.
			if fileInfo, err := os.Stat(detail.filePath); err == nil {
				detail.expected.beforeStat = fileInfo.Sys().(*syscall.Stat_t)
			}
		}
		s.Log("Running ", detail.cmd.String())
		if err := detail.cmd.Start(); err != nil {
			s.Errorf("Error starting %q: %v ", detail.cmd.String(), err)
		}

		detail.expected.command = detail.cmd.String()
		detail.expected.filePath = detail.filePath

		pid := uint64(detail.cmd.Process.Pid)
		expectedResults[pid] = detail.expected

		if detail.cleanup != nil {
			defer detail.cleanup(ctx)
		}

		if err = detail.cmd.Wait(); err != nil {
			s.Logf("Waiting failed, killing %q:%v", detail.cmd, err)
			if err := detail.cmd.Kill(); err != nil {
				s.Errorf("Failed to kill %q: %v", detail.cmd, err)
			}
			detail.cmd.Wait() // wait again for the killing.
		}
		fileInfo, err := os.Stat(detail.filePath)
		if err != nil {
			s.Errorf("Failed to stat %q:%v", detail.filePath, err)
		}
		detail.expected.afterStat = fileInfo.Sys().(*syscall.Stat_t)
		s.Logf("gid=%d uid=%d dev=%d inode=%d mode=%d",
			detail.expected.afterStat.Gid, detail.expected.afterStat.Uid,
			detail.expected.afterStat.Dev, detail.expected.afterStat.Ino,
			detail.expected.afterStat.Mode)
	}

	// Wait for the current batch to be flushed.
	// GoBigSleepLint: Using poll here doesn't make sense. There is no particular
	// condition we can poll for. This is simply giving secagentd ample time to
	// process and post events to dbus and is an educated guess.
	// TODO(b/278252387): Convert this to poll when tast's
	// dbusutil.DbusEventMonitor supports it.
	if err := testing.Sleep(ctx, 2*batchIntervalS*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	bFileEvents, err := collectFileDbusMessages(ctx, s, expectedResults, stopDbusMonitoring)

	// Check to make sure we see file events with the proper process ID
	// and then check the actual file event.

	// we go through all file events and when we find a matching event we will retire
	// the entry in the expectation map.
	// A passing condition is that the expectation map is empty.
	for _, fileEvent := range bFileEvents {
		modify := fileEvent.GetSensitiveModify()
		read := fileEvent.GetSensitiveRead()
		var process, parentProcess *xdr.Process
		var beforeAttributes, afterImage *xdr.FileImage
		var actualEventType fileEventType

		if modify != nil {
			process = modify.GetProcess()
			parentProcess = modify.GetParentProcess()
			beforeAttributes = modify.FileModify.GetAttributesBefore()
			afterImage = modify.FileModify.GetImageAfter()
			actualEventType = modifyEvent
		} else if read != nil {
			process = read.GetProcess()
			parentProcess = read.GetParentProcess()
			afterImage = read.FileRead.GetImage()
			actualEventType = readEvent
		} else {
			s.Error("encountered an empty file event, no read, no modify found")
		}

		ok := false
		var expectedResult *expectedResult
		pid := process.GetCanonicalPid()
		if expectedResult, ok = expectedResults[pid]; ok == false {
			continue
		}
		eInfo := fmt.Sprintf("[%d] cmd %q ", pid, expectedResult.command)
		if expectedResult.eventType != actualEventType {
			s.Logf("%v expected event type %q actual %q", eInfo, expectedResult.eventType,
				actualEventType)
			continue
		}

		if !proto.Equal(process, expectedResult.process) {
			s.Logf("%v process mismatch - expected:%q actual: %q", eInfo, expectedResult.process,
				process)
			continue
		}
		if !proto.Equal(parentProcess, expectedResult.parentProcess) {
			s.Logf("%v parent process mismatch - expected: %q actual: %q", eInfo, expectedResult.process,
				process)
			continue
		}

		if err = matchAttributes(afterImage, expectedResult.afterStat); err != nil {
			s.Logf("%v after_image attribute matching failed: %v", eInfo, err)
			continue
		}

		if modify != nil { //validation specific to modify
			if modify.GetFileModify().GetModifyType() != *expectedResult.eventSubType {
				s.Logf("%v modify type mismatch - expected:%q actual:%q", eInfo, expectedResult.eventSubType,
					modify.GetFileModify().GetModifyType())
				continue
			}
			// file event indicates attributes have been modified.
			if *modify.GetFileModify().ModifyType == xdr.FileModify_MODIFY_ATTRIBUTE ||
				*modify.GetFileModify().ModifyType == xdr.FileModify_WRITE_AND_MODIFY_ATTRIBUTE {
				if beforeAttributes == nil {
					s.Logf("%v missing beforeAttributes", modify.GetFileModify())
					continue
				}
				if err = matchAttributes(beforeAttributes, expectedResult.beforeStat); err != nil {
					s.Logf("%v before_image attributes mismatch:%v", eInfo, err)
					continue
				}
			}
		}
		// retire the expectation, everything matches.
		delete(expectedResults, pid)
	}

	if len(expectedResults) > 0 {
		for pid, e := range expectedResults {
			s.Logf("command [%d]%v expecting a %v", pid, e.command, e.eventType)
		}
		s.Error("Test failed, not all expectations met")
	}

}

func matchAttributes(actualImage *xdr.FileImage, expectedStats *syscall.Stat_t) error {
	if actualImage.GetCanonicalGid() == uint64(expectedStats.Gid) &&
		actualImage.GetCanonicalUid() == uint64(expectedStats.Uid) &&
		actualImage.GetMode() == expectedStats.Mode {
		return nil
	}
	return errors.Errorf("expected: gid=%d,uid=%d mode=%o actual:gid=%d,uid=%d mode=%o",
		actualImage.GetCanonicalGid(), actualImage.GetCanonicalUid(), actualImage.GetMode(),
		expectedStats.Gid, expectedStats.Uid, expectedStats.Mode)

}

func getFileEventDetails(ctx context.Context, testCase testCase, cr *chrome.Chrome) ([]*commandDetails, error) {
	sysCmds := map[string]string{
		"dd":      "bad",
		"touch":   "bad",
		"hexdump": "bad",
		"chmod":   "bad",
		"rm":      "bad",
	}
	var err error
	var cmds []*commandDetails
	for key := range sysCmds {
		sysCmds[key], err = exec.LookPath(key)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get command details")
		}
	}

	normalizedUser := cr.NormalizedUser()

	switch testCase {
	case userFiles:
		downloadsPath, err := cryptohome.DownloadsPath(ctx, normalizedUser)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate command list ")

		}
		outputFile := filepath.Join(downloadsPath, "file_events_test")
		cmds = append(cmds, &commandDetails{
			cmd: testexec.CommandContext(ctx, sysCmds["dd"], "if=/dev/zero", "of="+outputFile, "bs=1M", "count=1"),
			expected: &expectedResult{
				eventType:    modifyEvent,
				fileType:     xdr.SensitiveFileType_USER_FILE,
				filePath:     outputFile,
				eventSubType: xdr.FileModify_WRITE.Enum()},
			filePath: outputFile,
			cleanup:  func(ctx context.Context) { os.Remove(outputFile) },
		})
		cmds = append(cmds, &commandDetails{
			cmd:      testexec.CommandContext(ctx, sysCmds["hexdump"], outputFile),
			filePath: outputFile,
			cleanup:  nil,
			expected: &expectedResult{
				eventType: readEvent,
				filePath:  outputFile,
				fileType:  xdr.SensitiveFileType_USER_FILE},
		})
		cmds = append(cmds, &commandDetails{
			cmd: testexec.CommandContext(ctx, sysCmds["chmod"], "777", outputFile),
			expected: &expectedResult{
				eventType:    modifyEvent,
				fileType:     xdr.SensitiveFileType_USER_FILE,
				filePath:     outputFile,
				eventSubType: xdr.FileModify_MODIFY_ATTRIBUTE.Enum()},
			filePath: outputFile,
			cleanup:  func(ctx context.Context) { os.Remove(outputFile) },
		})
		return cmds, nil
	}
	return nil, errors.New("could not generate a command detail for " + string(testCase) + ", not supported")
}

func collectFileDbusMessages(ctx context.Context, s *testing.State,
	expectedResults map[uint64]*expectedResult,
	stopDbusMonitoring func() ([]dbusutil.CalledMethod, error)) ([]*xdr.FileEventAtomicVariant, error) { // Collect the log of EnqueueRecord dbus calls to Missived.
	calledMethods, err := stopDbusMonitoring()
	if err != nil {
		return nil, errors.Wrap(err,
			"failed to capture EnqueueRecord dbus calls to missived")
	}
	s.Logf("secagentd enqueued %d events", len(calledMethods))

	var bFileEvents []*xdr.FileEventAtomicVariant
	for _, method := range calledMethods {
		if len(method.Arguments) == 0 {
			continue
		}
		arg, ok := method.Arguments[0].([]byte)
		if !ok {
			continue
		}
		enq := &rep.EnqueueRecordRequest{}
		if err := proto.Unmarshal(arg, enq); err != nil {
			return nil, errors.Wrap(err,
				"failed to unmarshal an EnqueueRecordRequest")
		}

		s.Logf("Destination is %s", enq.GetRecord().GetDestination())

		// Save off matching ProcessExecs.
		if enq.GetRecord().GetDestination() == rep.Destination_CROS_SECURITY_PROCESS {
			pe := &xdr.XdrProcessEvent{}
			if err := proto.Unmarshal(enq.GetRecord().GetData(), pe); err != nil {
				return nil, errors.Wrap(err,
					"failed to unmarshal data for a CROS_SECURITY_PROCESS record")
			}
			for _, v := range pe.GetBatchedEvents() {
				if v.GetProcessExec() != nil {
					processExec := v.GetProcessExec()
					pid := processExec.GetSpawnProcess().GetCanonicalPid()
					if r, ok := expectedResults[pid]; ok {
						if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
							return nil, errors.Wrap(err,
								"CROS_SECURITY_PROCESS process exec has an invalid common field")
						}
						r.process = processExec.GetSpawnProcess()
						// since we captured the exec the metafirst appearance in
						// the file event should always be false.
						*r.process.MetaFirstAppearance = false
						r.parentProcess = processExec.GetProcess()
						// same with the parent.
						*r.parentProcess.MetaFirstAppearance = false
					}
				}
			}
		}
		// Save off file events.
		eventType := ""
		if enq.GetRecord().GetDestination() == rep.Destination_CROS_SECURITY_FILE {
			fe := &xdr.XdrFileEvent{}
			if err := proto.Unmarshal(enq.GetRecord().GetData(), fe); err != nil {
				return nil, errors.Wrap(err,
					"failed to unmarshal data for a CROS_SECURITY_FILE record")
			}
			for _, v := range fe.GetBatchedEvents() {
				bFileEvents = append(bFileEvents, v)
				if v.GetSensitiveRead() != nil {
					eventType = "Sensitive Read"
				} else if v.GetSensitiveModify() != nil {
					eventType = "Sensitive Modify"
				}
				if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
					return nil, errors.Wrapf(err,
						"CROS_SECURITY_FILE %v has an invalid common field", eventType)
				}
			}
		}
	}
	return bFileEvents, nil
}
