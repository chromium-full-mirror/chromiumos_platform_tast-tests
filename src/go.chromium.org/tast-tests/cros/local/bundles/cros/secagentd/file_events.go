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
	"strings"
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

type testName string
type fileEventType string

const (
	userFiles testName = "USER_FILES"
	rootfs    testName = "ROOTFS"
	// test hardlink behavior
	hardlink testName = "HARDLINK"
	// mounts a folder/archive from user directory to /media/archive/test/
	mountedUserArchive testName = "MOUNTED_USER_ARCHIVE"
	// mounts r/w folder from /tmp to /media/removable/usb_test/
	massStorageUSB  testName      = "USB_MASS_STORAGE"
	cookies         testName      = "COOKIES"
	userCredential  testName      = "ENCRYPTED_USER_CREDENTIAL"
	devicePolicy    testName      = "DEVICE_POLICY"
	devicePolicyKey testName      = "DEVICE_POLICY_KEY"
	tpmKey          testName      = "TPM_KEY"
	authFactors     testName      = "AUTH_FACTORS"
	modifyEvent     fileEventType = "MODIFY"
	readEvent       fileEventType = "READ"

	tempDir        string = "/usr/local/tmp"
	synchLogString string = "File plugin activated"
)

type restoreFile struct {
	name     string
	copyIsAt string
}
type testCase struct {
	filesToRestore []*restoreFile
	commandDetails []*commandDetail
}

type fileTypeParams struct {
	testType testName
}
type commandDetail struct {
	cmd      *testexec.Cmd
	filePath string // the path of the file in the init mount ns
	cleanup  func(context.Context)
	expected *expectedResult
}

type expectedResult struct {
	command             string // useful for debug
	process             *xdr.Process
	processTimeUs       uint64
	parentProcess       *xdr.Process
	parentProcessTimeUs uint64
	beforeStat          *syscall.Stat_t // stat taken before the command executes
	afterStat           *syscall.Stat_t // stat taken after the command executes
	filePath            string
	eventType           fileEventType              // read or modify
	eventSubType        *xdr.FileModify_ModifyType // modify, modify and write or write only
	fileType            xdr.SensitiveFileType
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
		}, {
			Name: "user_credential",
			Val: fileTypeParams{
				testType: userCredential,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "cookies",
			Val: fileTypeParams{
				testType: cookies,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		}, {
			Name: "tpm_key",
			Val: fileTypeParams{
				testType: tpmKey,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
		},
		},
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
	defer func(clnupCtx context.Context) {
		s.Log("Restarting secagentd with default parameters")
		secagentdupstart.RestartSecagentd(clnupCtx, false)
	}(cleanupCtx)

	// Clear out old entries from the kernel trace to make an easier failure
	// analysis.
	if err := secagentdcommon.ClearKernelTrace(ctx); err != nil {
		s.Log("Unable to clear the kernel trace file: ", err)
	}
	initialOffset, err := secagentdcommon.GetSecagentdLogSize()
	if err != nil {
		s.Fatal("Unable to determine size of the secagentd log file: ", err)
	}
	const batchIntervalS = 5
	s.Logf("Restarting secagentd with BYPASS_POLICY_FOR_TESTING=true,"+
		" BYPASS_ENQ_OK_WAIT_FOR_TESTING=true"+
		" PLUGIN_BATCH_INTERVAL_S_FOR_TESTING=%d", batchIntervalS)
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx, false,
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))

	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}

	s.Logf("Waiting for the string %q to appear in secagentd.log,"+
		" starting search at offset=%v", synchLogString, initialOffset)
	if err := secagentdcommon.WaitForStringInLog(ctx, synchLogString, initialOffset); err != nil {
		s.Log("Failed to verify that fileplugin activated: ", err)
	}
	s.Logf("Detected %q in log file, file plugin should be activated..starting test", synchLogString)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	normalizedUser := cr.NormalizedUser()
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	systemPath, err := cryptohome.SystemPath(ctx, normalizedUser)
	userPath, err := cryptohome.UserPath(ctx, normalizedUser)
	mountedVaultPath, err := cryptohome.MountedVaultPath(ctx, normalizedUser)
	hashedUser, _ := cryptohome.UserHash(ctx, normalizedUser)

	s.Log("chrome normalized user:", normalizedUser)
	s.Log("cryptohome systemspath:", systemPath)
	s.Log("cryptohome downloadspath:", downloadsPath)
	s.Log("cryptohome userpath:", userPath)
	s.Log("cryptohome mountedVaultPath:", mountedVaultPath)
	s.Log("cryptohome hashed user:", hashedUser)

	stopDbusMonitoring, err := secagentddbusmonitor.SetupDbusMonitor(ctx, agentPid)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}

	// create a list of expectations

	testCase, err := getFileEventDetails(ctx, s.Param().(fileTypeParams).testType, cr)
	if err != nil {
		s.Fatal("Invalid test case: ", err)
	}

	// key is the pid of the process triggering a file event.
	// value is the criteria the resulting file event should be evaluated
	// against.
	expectedResults := make(map[uint64]*expectedResult)

	// Set the first exec seen time in secagentd to prevent
	// flaky meta_first_appearance.

	trueCmd, err := exec.LookPath("true")
	if err != nil {
		s.Fatal("Failed to force first exec seen time in secagentd")
	}
	primeCmd := testexec.CommandContext(ctx, trueCmd)
	primeCmd.Start()
	if err = primeCmd.Wait(); err != nil {
		s.Logf("Waiting failed, killing %q:%v", primeCmd, err)
		primeCmd.DumpLog(ctx)
		primeCmd.Kill()
		primeCmd.Wait() // wait again for the killing.
	}

	// GoBigSleepLint first seen is measured in seconds, so wait some time
	// to make sure no test processes are started within the same second interval.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Some test cases will overwrite files, before the test case begins copy
	// the contents of these files to a temporary file and then restore them
	// when the test case is finished.
	if len(testCase.filesToRestore) != 0 {
		tempDir, err := os.MkdirTemp(tempDir, "fileEventTest")
		if err != nil {
			s.Fatal("Unable to create temporary directory for file restoration")
		}
		s.Logf("Creating %q to store temp files used for sensitive file"+
			" restoration", tempDir)
		for _, fileDetails := range testCase.filesToRestore {
			s.Logf("Saving off %q for restoration later", fileDetails.name)
			buff, err := os.ReadFile(fileDetails.name)
			if err != nil {
				s.Fatalf("Error reading from %q:%v", fileDetails.name, err)
			}
			tempFileName := strings.Map(func(r rune) rune {
				if strings.ContainsRune(" -", r) {
					return -1
				}
				return r
			}, filepath.Base(fileDetails.name))

			filepath.Base(fileDetails.name)
			tempFile, err := os.CreateTemp(tempDir, filepath.Base(tempFileName))
			if err != nil {
				s.Fatalf("Unable to create temp file to save off %q:%v",
					fileDetails.name, err)
			}
			if _, err := tempFile.Write(buff); err != nil {
				s.Fatalf("Error saving %q into %q: %v", fileDetails.name,
					tempFile.Name(), err)
			}
			s.Logf("Successfully saved the contents of %q into %q for"+
				" restoration", fileDetails.name, tempFile.Name())
			fileDetails.copyIsAt = tempFile.Name()
			tempFile.Close()
		}

		// When test is finished, restore the files.
		defer func() {
			for _, fileDetails := range testCase.filesToRestore {
				baseFileName := filepath.Base(fileDetails.copyIsAt)
				buff, err := os.ReadFile(fileDetails.copyIsAt)
				if err != nil {
					s.Fatalf("Error while restoring file %q:%v",
						baseFileName, err)
				}
				restoreFile, err := os.OpenFile(fileDetails.name, os.O_RDWR, 0)
				if err != nil {
					s.Fatalf("Error restoring file %q: %v", baseFileName, err)
				}
				if err = restoreFile.Truncate(0); err != nil { // clear the file
					s.Fatalf("Error restoring file %q: %v", baseFileName, err)
				}
				if _, err = restoreFile.Write(buff); err != nil {
					s.Fatalf("Error restoring file %q: %v", baseFileName, err)
				}
				restoreFile.Close()
			}
			os.RemoveAll(tempDir)
		}()
	}
	// Loop through and wait for each file command. This is needed so that
	// an accurate stat can be snap shotted after each anticipated file command.
	for _, detail := range testCase.commandDetails {
		if detail.expected != nil {
			s.Log("Running ", detail.cmd.String())
			// record the stat before the command executes if the file exists.
			if fileInfo, err := os.Stat(detail.filePath); err == nil {
				detail.expected.beforeStat = fileInfo.Sys().(*syscall.Stat_t)
				s.Logf("before_stat gid=%d uid=%d dev=%d inode=%d mode=%o",
					detail.expected.beforeStat.Gid, detail.expected.beforeStat.Uid,
					detail.expected.beforeStat.Dev, detail.expected.beforeStat.Ino,
					detail.expected.beforeStat.Mode)
			} else {
				s.Logf("Failed to stat %q: %q", detail.filePath, err)
			}
		}
		if err := detail.cmd.Start(); err != nil {
			s.Errorf("Error starting %q: %v ", detail.cmd.String(), err)
		}
		if detail.cleanup != nil {
			defer detail.cleanup(ctx)
		}
		if err = detail.cmd.Wait(); err != nil {
			s.Logf("Waiting failed, killing %q:%v", detail.cmd, err)
			detail.cmd.DumpLog(ctx)
			if err := detail.cmd.Kill(); err != nil {
				s.Fatalf("Failed to kill %q: %v", detail.cmd, err)
			}
			detail.cmd.Wait() // wait again for the killing.
		}
		if detail.expected != nil {
			detail.expected.command = detail.cmd.String()
			detail.expected.filePath = detail.filePath

			pid := uint64(detail.cmd.Process.Pid)
			expectedResults[pid] = detail.expected
			fileInfo, err := os.Stat(detail.filePath)
			if err != nil {
				s.Fatalf("Failed to stat %q:%v", detail.filePath, err)
			}
			detail.expected.afterStat = fileInfo.Sys().(*syscall.Stat_t)
			s.Logf("after_stat gid=%d uid=%d dev=%d inode=%d mode=%o",
				detail.expected.afterStat.Gid, detail.expected.afterStat.Uid,
				detail.expected.afterStat.Dev, detail.expected.afterStat.Ino,
				detail.expected.afterStat.Mode)
		}
	}

	// Wait for the current batch to be flushed.
	// GoBigSleepLint: Using poll here doesn't make sense. There is no
	// particular condition we can poll for. This is simply giving secagentd
	// ample time to process and post events to dbus and is an educated guess.
	// TODO(b/278252387): Convert this to poll when tast's
	// dbusutil.DbusEventMonitor supports it.
	if err := testing.Sleep(ctx, 2*batchIntervalS*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	bFileEvents, err := collectFileDbusMessages(ctx, s, expectedResults,
		stopDbusMonitoring)

	// Check to make sure we see file events with the proper process ID
	// and then check the actual file event.

	// we go through all file events and when we find a matching event we will
	// retire the entry in the expectation map. A passing condition is that the
	// expectation map is empty.
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
			s.Logf("%v expected event type %q actual %q", eInfo,
				expectedResult.eventType,
				actualEventType)
			continue
		}

		if !proto.Equal(process, expectedResult.process) {
			s.Logf("%v process mismatch - expected:%q actual: %q", eInfo,
				expectedResult.process, process)
			continue
		}
		if !proto.Equal(parentProcess, expectedResult.parentProcess) {
			s.Logf("%v parent process mismatch - expected: %q actual: %q",
				eInfo, expectedResult.process, process)
			continue
		}

		if err = matchAttributes(afterImage, expectedResult.afterStat); err != nil {
			s.Logf("%v after_image attribute matching failed: %v", eInfo, err)
			continue
		}

		if modify != nil { //validation specific to modify
			if modify.GetFileModify().GetModifyType() != *expectedResult.eventSubType {
				s.Logf("%v modify type mismatch - expected:%q actual:%q",
					eInfo, expectedResult.eventSubType,
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
	if expectedStats == nil {
		return errors.New("no expectation set for attributes")
	}
	if actualImage.GetCanonicalGid() == uint64(expectedStats.Gid) &&
		actualImage.GetCanonicalUid() == uint64(expectedStats.Uid) &&
		actualImage.GetMode() == expectedStats.Mode {
		return nil
	}
	return errors.Errorf("expected: gid=%d,uid=%d mode=%o actual:gid=%d,uid=%d mode=%o",
		actualImage.GetCanonicalGid(), actualImage.GetCanonicalUid(), actualImage.GetMode(),
		expectedStats.Gid, expectedStats.Uid, expectedStats.Mode)

}

func getFileEventDetails(ctx context.Context, testCaseName testName, cr *chrome.Chrome) (*testCase, error) {
	sysCmds := map[string]string{
		"dd":      "bad",
		"touch":   "bad",
		"hexdump": "bad",
		"chmod":   "bad",
		"rm":      "bad",
		"mount":   "bad",
		"cp":      "bad",
	}
	var err error
	var cmds []*commandDetail
	for key := range sysCmds {
		sysCmds[key], err = exec.LookPath(key)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get command details")
		}
	}

	normalizedUser := cr.NormalizedUser()

	switch testCaseName {
	case userCredential:
		hashedUser, _ := cryptohome.UserHash(ctx, cr.NormalizedUser())
		outputFile := "/home/.shadow/" + hashedUser + "/user_secret_stash/uss.0"
		cmds = appendRWCommandDetails(ctx, cmds, outputFile, &sysCmds, xdr.SensitiveFileType_USER_ENCRYPTED_CREDENTIAL)
		return &testCase{filesToRestore: []*restoreFile{{name: outputFile}}, commandDetails: cmds}, nil
	case rootfs:
		outputFile := "/bin/testcase"
		// Test setup, remount rootfs as rw then on exit remount it when test is
		// done.
		cmds = append(cmds, &commandDetail{
			cmd:      testexec.CommandContext(ctx, sysCmds["mount"], "-o", "rw,remount", "/"),
			expected: nil,
			cleanup: func(ctx context.Context) {
				c := testexec.CommandContext(ctx, sysCmds["mount"], "-o", "ro,remount", "/")
				c.Start()
				if err = c.Wait(); err != nil {
					c.Kill()
					c.Wait() // wait again for the killing.
				}
			},
		})
		cmds = appendDDCommand(ctx, cmds, outputFile, &sysCmds, xdr.SensitiveFileType_ROOT_FS)
		cmds = appendModifyAttributeCommand(ctx, cmds, outputFile, &sysCmds, xdr.SensitiveFileType_ROOT_FS)
		return &testCase{commandDetails: cmds}, nil

	case userFiles:
		downloadsPath, err := cryptohome.DownloadsPath(ctx, normalizedUser)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate command list ")

		}
		outputFile := filepath.Join(downloadsPath, "file_events_test")
		cmds = appendRWCommandDetails(ctx, cmds, outputFile, &sysCmds, xdr.SensitiveFileType_USER_FILE)
		return &testCase{commandDetails: cmds}, nil

	case cookies:
		userPath, _ := cryptohome.UserPath(ctx, normalizedUser)
		rv := testCase{}
		cookieNames := []string{"Cookies", "Cookies-journal", "Safe Browsing Cookies", "Safe Browsing Cookies-journal"}
		for _, fileName := range cookieNames {
			outputFile := filepath.Join(userPath, fileName)
			rv.filesToRestore = append(rv.filesToRestore, &restoreFile{name: outputFile})
			rv.commandDetails = appendRWCommandDetails(ctx, rv.commandDetails, outputFile, &sysCmds, xdr.SensitiveFileType_USER_WEB_COOKIE)
		}
		return &rv, nil

	case tpmKey:
		rv := testCase{}
		secretNames := []string{"cryptohome.key", "cryptohome.ecc.key"}
		for _, fileName := range secretNames {
			outputFile := filepath.Join("/home/.shadow/", fileName)
			rv.filesToRestore = append(rv.filesToRestore, &restoreFile{name: outputFile})
			rv.commandDetails = appendRWCommandDetails(ctx, rv.commandDetails, outputFile, &sysCmds, xdr.SensitiveFileType_SYSTEM_TPM_PUBLIC_KEY)
		}
		return &rv, nil
	}
	return nil, errors.New("could not generate a command detail for " + string(testCaseName) + ", not supported")
}

func appendDDCommand(ctx context.Context, cmdDetails []*commandDetail, fileName string, sysCmds *map[string]string, fileType xdr.SensitiveFileType) []*commandDetail {
	cmdDetails = append(cmdDetails, &commandDetail{
		cmd: testexec.CommandContext(ctx, (*sysCmds)["dd"], "if=/dev/zero", "of="+fileName, "bs=1M", "count=1"),
		expected: &expectedResult{
			eventType:    modifyEvent,
			fileType:     fileType,
			filePath:     fileName,
			eventSubType: xdr.FileModify_WRITE.Enum()},
		filePath: fileName,
		cleanup:  nil,
	})
	return cmdDetails
}

func appendModifyAttributeCommand(ctx context.Context, cmdDetails []*commandDetail, fileName string, sysCmds *map[string]string, fileType xdr.SensitiveFileType) []*commandDetail {
	// First command is to set the permissions to a known value to make sure
	// the second chmod actually changes permissions.
	cmdDetails = append(cmdDetails, &commandDetail{
		cmd:      testexec.CommandContext(ctx, (*sysCmds)["chmod"], "666", fileName),
		expected: nil,
		filePath: fileName,
		cleanup:  nil})
	cmdDetails = append(cmdDetails, &commandDetail{
		cmd: testexec.CommandContext(ctx, (*sysCmds)["chmod"], "777", fileName),
		expected: &expectedResult{
			eventType:    modifyEvent,
			fileType:     xdr.SensitiveFileType_SYSTEM_TPM_PUBLIC_KEY,
			filePath:     fileName,
			eventSubType: xdr.FileModify_MODIFY_ATTRIBUTE.Enum()},
		filePath: fileName,
		cleanup:  nil})
	return cmdDetails
}
func appendRWCommandDetails(ctx context.Context, cmdDetails []*commandDetail, fileName string, sysCmds *map[string]string, fileType xdr.SensitiveFileType) []*commandDetail {
	cmdDetails = appendDDCommand(ctx, cmdDetails, fileName, sysCmds, fileType)
	cmdDetails = appendModifyAttributeCommand(ctx, cmdDetails, fileName, sysCmds, fileType)
	cmdDetails = append(cmdDetails, &commandDetail{
		cmd:      testexec.CommandContext(ctx, (*sysCmds)["hexdump"], fileName),
		filePath: fileName,
		cleanup:  nil,
		expected: &expectedResult{
			eventType: readEvent,
			filePath:  fileName,
			fileType:  fileType},
	})
	return cmdDetails
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
						r.processTimeUs = uint64(v.GetCommon().GetCreateTimestampUs())
						// since we captured the exec the metafirst appearance in
						// the file event should always be false.
						*r.process.MetaFirstAppearance = false
						r.parentProcess = processExec.GetProcess()
						// same with the parent.
						*r.parentProcess.MetaFirstAppearance = false
						s.Logf("pid(%d)executed : %q", *r.process.CanonicalPid, *r.process.Commandline)
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
