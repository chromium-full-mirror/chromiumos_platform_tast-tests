// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fileeventsimpl contains the implementation logic for use in
// both local and remote testing.
package fileeventsimpl

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
	pb "go.chromium.org/tast-tests/cros/services/cros/secagentd"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"

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

// TestName - name of the test. Determines the test case that will be generated.
type TestName string
type fileEventType string

const (
	modifyEvent fileEventType = "MODIFY"
	readEvent   fileEventType = "READ"
)

const (
	tempDir string = "/usr/local/tmp"
	// eventWatchTimeout bounds how long the dbus watcher that DoTest sets up
	// before running the file commands keeps collecting EnqueueRecord calls,
	// i.e. how long it waits for secagentd to report every expected file
	// event. secagentd normally flushes its batches every batchIntervalS
	// seconds, but shortly after a (re)start its main thread can stall for
	// tens of seconds while it hashes large executables (e.g. a >1 GB chrome
	// binary) that other processes exec, which delays every batch. Waiting for
	// the events instead of sleeping for a fixed multiple of the batch interval
	// keeps the test fast in the common case and tolerant of such stalls.
	eventWatchTimeout = 90 * time.Second
)

type restoreFile struct {
	name     string
	copyIsAt string
}
type testDetails struct {
	filesToRestore []*restoreFile
	commandDetails []*commandDetail
	syncText       string
}

type commandDetail struct {
	cmd      *testexec.Cmd
	filePath string // the path of the file in the init mount ns
	cleanup  func(context.Context)
	expected *expectedResult
}

type expectedResult struct {
	command       string // useful for debug
	process       *xdr.Process
	processTimeUs uint64
	parentProcess *xdr.Process
	beforeStat    *syscall.Stat_t // stat taken before the command executes
	afterStat     *syscall.Stat_t // stat taken after the command executes
	filePath      string
	eventType     fileEventType              // read or modify
	eventSubType  *xdr.FileModify_ModifyType // modify, modify and write or write only
	fileType      xdr.SensitiveFileType
}

// CreateForLocalTest - creates a FileEvent object suitable for running
// a local tast tests.
func CreateForLocalTest(s *testing.State) FileEvent {
	var rv FileEvent
	rv.Log = s.Log
	rv.Logf = s.Logf
	rv.Error = s.Error
	rv.Errorf = s.Errorf
	rv.Fatal = s.Fatal
	rv.Fatalf = s.Fatalf
	return rv
}

// FileEvent - contains the business logic of the test. When the test is
// running locally then the below logging functions should point to the logging
// functions provided by the testing utility.
// When the RPC variant of the test should be ran these functions should point
// to the DoLog,DoError... variants.
type FileEvent struct {
	Log    func(args ...interface{})
	Logf   func(format string, args ...interface{})
	Fatal  func(args ...interface{})
	Fatalf func(format string, args ...interface{})
	Error  func(args ...interface{})
	Errorf func(format string, args ...interface{})
}

// DoTest - given a test described by tc execute a test.
// In the case where the context is a remote test execution,
// errorsList will be populated with log messages, error messages
// and fatal messages.
func (f FileEvent) DoTest(ctx context.Context, tc *testDetails) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 15*time.Second)
	defer cancel()
	if len(tc.syncText) == 0 {
		f.Fatal("a non empty string must be provided to wait on")
	}

	// Restart with default parameter.
	defer func(clnupCtx context.Context) {
		f.Log("Restarting secagentd with default parameters")
		secagentdupstart.RestartSecagentd(clnupCtx, false)
	}(cleanupCtx)

	// Clear out old entries from the kernel trace to make an easier failure
	// analysis.
	if err := secagentdcommon.ClearKernelTrace(ctx); err != nil {
		f.Logf("Unable to clear the kernel trace file: %v", err)
	}
	initialOffset, err := secagentdcommon.GetSecagentdLogSize()
	if err != nil {
		f.Fatal("Unable to determine the size of the secagentd log file")
		return
	}
	const batchIntervalS = 5
	f.Logf("Restarting secagentd with BYPASS_POLICY_FOR_TESTING=true,"+
		" BYPASS_ENQ_OK_WAIT_FOR_TESTING=true"+
		" PLUGIN_BATCH_INTERVAL_S_FOR_TESTING=%d", batchIntervalS)
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx, false,
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))

	if err != nil {
		f.Fatal("Failed to restart secagentd: ", err)
	}

	f.Logf("Waiting for the string %q to appear in secagentd.log,"+
		" starting search at offset=%v", tc.syncText, initialOffset)
	if err := secagentdcommon.WaitForStringInLog(ctx, tc.syncText, initialOffset, f.Logf); err != nil {
		f.Fatalf("Failed to find %q in logfile, continuing anyways: %v", tc.syncText, err)
	} else {
		f.Logf("Detected %q in log file, file plugin should be activated..starting test", tc.syncText)
	}

	ew, stopWatcher, err := secagentddbusmonitor.SetupDbusWatcherWithTimeout(ctx, agentPid, eventWatchTimeout, true)
	if err != nil {
		f.Fatal("Failed to setup dbus monitoring: ", err)
		return
	}
	defer stopWatcher()
	// key is the pid of the process triggering a file event.
	// value is the criteria the resulting file event should be evaluated
	// against.
	expectedResults := make(map[uint64]*expectedResult)

	// Set the first exec seen time in secagentd to prevent
	// flaky meta_first_appearance.

	trueCmd, err := exec.LookPath("true")
	if err != nil {
		f.Fatal("Failed to force first exec seen time in secagentd")
	}
	primeCmd := testexec.CommandContext(ctx, trueCmd)
	primeCmd.Start()
	if err = primeCmd.Wait(); err != nil {
		f.Logf("Waiting failed, killing %q:%v", primeCmd, err)
		primeCmd.DumpLog(ctx)
		primeCmd.Kill()
		primeCmd.Wait() // wait again for the killing.
	}

	// GoBigSleepLint first seen is measured in seconds, so wait some time
	// to make sure no test processes are started within the same second interval.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		f.Fatal("Failed to sleep: ", err)
	}

	// Some test cases will overwrite files, before the test case begins copy
	// the contents of these files to a temporary file and then restore them
	// when the test case is finished.
	if len(tc.filesToRestore) != 0 {
		tempDir, err := os.MkdirTemp(tempDir, "fileEventTest")
		if err != nil {
			f.Fatal("Unable to create temporary directory for file restoration")
		}
		f.Logf("Creating %q to store temp files used for sensitive file"+
			" restoration", tempDir)
		for _, fileDetails := range tc.filesToRestore {
			f.Logf("Saving off %q for restoration later", fileDetails.name)
			buff, err := os.ReadFile(fileDetails.name)
			if err != nil {
				f.Fatalf("Error reading from %q:%v", fileDetails.name, err)
				return
			}
			tempFileName := strings.Map(func(r rune) rune {
				if strings.ContainsRune(" -", r) {
					return -1
				}
				return r
			}, filepath.Base(fileDetails.name))

			tempFile, err := os.CreateTemp(tempDir, filepath.Base(tempFileName))
			if err != nil {
				f.Fatalf("Unable to create temp file to save off %q:%v",
					fileDetails.name, err)
				return
			}
			if _, err := tempFile.Write(buff); err != nil {
				f.Fatalf("Error saving %q into %q: %v", fileDetails.name,
					tempFile.Name(), err)
				return
			}
			f.Logf("Successfully saved the contents of %q into %q for"+
				" restoration", fileDetails.name, tempFile.Name())
			fileDetails.copyIsAt = tempFile.Name()
			tempFile.Close()
		}

		// When test is finished, restore the files.
		defer func() {
			for _, fileDetails := range tc.filesToRestore {
				baseFileName := filepath.Base(fileDetails.copyIsAt)
				buff, err := os.ReadFile(fileDetails.copyIsAt)
				if err != nil {
					f.Fatalf("Error while restoring file %q:%v",
						baseFileName, err)
					return
				}
				restoreFile, err := os.OpenFile(fileDetails.name, os.O_RDWR, 0)
				if err != nil {
					f.Fatalf("Error restoring file %q: %v", baseFileName, err)
					return
				}
				if err = restoreFile.Truncate(0); err != nil { // clear the file
					f.Fatalf("Error restoring file %q: %v", baseFileName, err)
					return
				}
				if _, err = restoreFile.Write(buff); err != nil {
					f.Fatalf("Error restoring file %q: %v", baseFileName, err)
					return
				}
				restoreFile.Close()
			}
			os.RemoveAll(tempDir)
		}()
	}
	// Loop through and wait for each file command. This is needed so that
	// an accurate stat can be snap shotted after each anticipated file command.
	for _, detail := range tc.commandDetails {
		if detail.expected != nil {
			f.Logf("Running %q", detail.cmd.String())
			// record the stat before the command executes if the file exists.
			if fileInfo, err := os.Stat(detail.filePath); err == nil {
				detail.expected.beforeStat = fileInfo.Sys().(*syscall.Stat_t)
				f.Logf("before_stat gid=%d uid=%d dev=%d inode=%d mode=%o",
					detail.expected.beforeStat.Gid, detail.expected.beforeStat.Uid,
					detail.expected.beforeStat.Dev, detail.expected.beforeStat.Ino,
					detail.expected.beforeStat.Mode)
			} else {
				f.Logf("Failed to stat %q: %q", detail.filePath, err)
			}
		}
		if err := detail.cmd.Start(); err != nil {
			f.Errorf("Error starting %q: %v ", detail.cmd.String(), err)
		}
		if detail.cleanup != nil {
			defer detail.cleanup(ctx)
		}
		if err = detail.cmd.Wait(); err != nil {
			f.Logf("Waiting failed, killing %q:%v", detail.cmd, err)
			detail.cmd.DumpLog(ctx)
			if err := detail.cmd.Kill(); err != nil {
				f.Fatalf("Failed to kill %q: %v", detail.cmd, err)
				return
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
				f.Fatalf("Failed to stat %q:%v", detail.filePath, err)
				return
			}
			detail.expected.afterStat = fileInfo.Sys().(*syscall.Stat_t)
			f.Logf("after_stat gid=%d uid=%d dev=%d inode=%d mode=%o",
				detail.expected.afterStat.Gid, detail.expected.afterStat.Uid,
				detail.expected.afterStat.Dev, detail.expected.afterStat.Ino,
				detail.expected.afterStat.Mode)
		}
	}

	// Wait until every expected file event has been reported over D-Bus
	// instead of sleeping for a fixed multiple of the batch interval, which
	// expired too early whenever secagentd was stalled (see eventWatchTimeout).
	if err := f.waitForExpectedEvents(ew, expectedResults); err != nil {
		f.Log("Stopped waiting for file events: ", err)
	}

	if len(expectedResults) > 0 {
		for pid, e := range expectedResults {
			f.Logf("command [%d]%v expecting a %v", pid, e.command, e.eventType)
		}
		f.Error("Test failed, not all expectations met")
	}
}

// waitForExpectedEvents consumes the EnqueueRecord calls observed by ew and
// retires the entries of expectedResults that they satisfy. It returns as soon
// as expectedResults is empty, or with an error once the watcher stops, e.g.
// because its timeout elapsed.
func (f FileEvent) waitForExpectedEvents(ew *dbusutil.EventWatcher, expectedResults map[uint64]*expectedResult) error {
	var fileEvents []*xdr.FileEventAtomicVariant
	numRecords := 0
	defer func() {
		f.Logf("secagentd enqueued %d events", numRecords)
		if len(expectedResults) > 0 {
			// Log why the file events seen so far did not satisfy the
			// remaining expectations.
			f.retireMetExpectations(fileEvents, expectedResults, true)
		}
	}()

	for len(expectedResults) > 0 {
		method, ok := <-ew.Events()
		if !ok {
			return errors.Wrap(ew.Err(), "dbus event watcher stopped")
		}
		numRecords++
		events, err := f.handleEnqueueRecord(method, expectedResults)
		if err != nil {
			f.Error("Invalid EnqueueRecord call: ", err)
			continue
		}
		fileEvents = append(fileEvents, events...)
		// The exec events describing the commands and the file events they
		// caused come from different plugins and thus from different
		// batches, so re-examine every file event seen so far whenever a new
		// record arrives.
		f.retireMetExpectations(fileEvents, expectedResults, false)
	}
	return nil
}

// retireMetExpectations deletes from expectedResults every expectation that
// one of fileEvents satisfies. Mismatches are only logged when verbose is
// true because the same unmatched event may be re-examined several times
// while the remaining batches are awaited.
func (f FileEvent) retireMetExpectations(fileEvents []*xdr.FileEventAtomicVariant, expectedResults map[uint64]*expectedResult, verbose bool) {
	logf := func(format string, args ...any) {
		if verbose {
			f.Logf(format, args...)
		}
	}

	for _, fileEvent := range fileEvents {
		modify := fileEvent.GetSensitiveModify()
		read := fileEvent.GetSensitiveRead()
		var process, parentProcess *xdr.Process
		var beforeAttributes, afterImage *xdr.FileImage
		var actualEventType fileEventType
		var sensitiveFileType *xdr.SensitiveFileType

		if modify != nil {
			process = modify.GetProcess()
			parentProcess = modify.GetParentProcess()
			beforeAttributes = modify.FileModify.GetAttributesBefore()
			afterImage = modify.FileModify.GetImageAfter()
			actualEventType = modifyEvent
			sensitiveFileType = modify.FileModify.SensitiveFileType
		} else if read != nil {
			process = read.GetProcess()
			parentProcess = read.GetParentProcess()
			afterImage = read.FileRead.GetImage()
			actualEventType = readEvent
			sensitiveFileType = read.FileRead.SensitiveFileType
		} else {
			continue
		}

		ok := false
		var expectedResult *expectedResult
		pid := process.GetCanonicalPid()
		if expectedResult, ok = expectedResults[pid]; !ok {
			continue
		}
		eInfo := fmt.Sprintf("[%d] cmd %q ", pid, expectedResult.command)
		if expectedResult.eventType != actualEventType {
			logf("MISMATCH[event type] - %v expected %q actual %q", eInfo,
				expectedResult.eventType,
				actualEventType)
			continue
		}
		if !proto.Equal(process, expectedResult.process) {
			logf("MISMATCH - %v process - expected:%q actual: %q", eInfo,
				expectedResult.process, process)
			continue
		}
		if !proto.Equal(parentProcess, expectedResult.parentProcess) {
			logf("MISMATCH - %v parent process - expected: %q actual: %q",
				eInfo, expectedResult.parentProcess, parentProcess)
			continue
		}

		if expectedResult.fileType != *sensitiveFileType {
			logf("MISMATCH - %v sensitiveFileType - expected: %q actual: %q",
				eInfo, expectedResult.fileType, *sensitiveFileType)
			continue
		}

		if err := matchAttributes(afterImage, expectedResult.afterStat); err != nil {
			logf("MISMATCH - [%v] %v after_image attribute matching failed: %v", pid, eInfo, err)
			continue
		}

		if modify != nil { //validation specific to modify
			logf("expectation for pid=%v and event is a modify", pid)
			if modify.GetFileModify().GetModifyType() != *expectedResult.eventSubType {
				logf("MISMATCH - [%v] %v  expected:%q actual:%q",
					pid, eInfo, expectedResult.eventSubType,
					modify.GetFileModify().GetModifyType())
				continue
			}
			// file event indicates attributes have been modified.
			if *modify.GetFileModify().ModifyType == xdr.FileModify_MODIFY_ATTRIBUTE ||
				*modify.GetFileModify().ModifyType == xdr.FileModify_WRITE_AND_MODIFY_ATTRIBUTE {
				if beforeAttributes == nil {
					logf("%v missing beforeAttributes", modify.GetFileModify())
					continue
				}
				if err := matchAttributes(beforeAttributes, expectedResult.beforeStat); err != nil {
					logf("%v before_image attributes mismatch:%v", eInfo, err)
					continue
				}
			}
		}
		// retire the expectation, everything matches.
		delete(expectedResults, pid)
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

// GetFileEventDetails - based on a test case name, generate a test case which can
// be passed into DoTest to run the test.
func GetFileEventDetails(ctx context.Context, testCase pb.TestCase, cr *chrome.Chrome) (*testDetails, error) {
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
	makeWaitString := func(sensitiveFileType xdr.SensitiveFileType) string {
		return fmt.Sprintf("FileEvents: Now monitoring TYPE: %s", sensitiveFileType.String())
	}
	var normalizedUser string
	if testCase != pb.TestCase_ROOT_FS {
		if cr == nil { // cr should only be nil for remote test and the only
			// remote test is the root_fs test.
			return nil, errors.New(testCase.String() +
				" is a local only test and was attempted to be executed as a " +
				"remote test")
		}
		normalizedUser = cr.NormalizedUser()
	}

	switch testCase {
	case pb.TestCase_AUTH_FACTORS:
		hashedUser, _ := cryptohome.UserHash(ctx, cr.NormalizedUser())
		authFactorsDir := "/home/.shadow/" + hashedUser + "/auth_factors"
		rv, err := generateWriteTestCaseForAllFilesUnderDir(ctx, authFactorsDir,
			testCase, &sysCmds,
			xdr.SensitiveFileType_USER_AUTH_FACTORS_FILE)
		rv.syncText = makeWaitString(xdr.SensitiveFileType_USER_AUTH_FACTORS_FILE)
		return rv, err

	case pb.TestCase_USER_CREDENTIAL:
		hashedUser, _ := cryptohome.UserHash(ctx, cr.NormalizedUser())
		secretStashDir := "/home/.shadow/" + hashedUser + "/user_secret_stash"
		rv, err := generateRwTestCaseForAllFilesUnderDir(ctx, secretStashDir,
			testCase, &sysCmds,
			xdr.SensitiveFileType_USER_ENCRYPTED_CREDENTIAL)
		rv.syncText = makeWaitString(xdr.SensitiveFileType_USER_ENCRYPTED_CREDENTIAL)
		return rv, err

	case pb.TestCase_ROOT_FS:
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
		return &testDetails{commandDetails: cmds, syncText: makeWaitString(xdr.SensitiveFileType_ROOT_FS)}, nil

	case pb.TestCase_SYSTEM_PASSWORD:
		fileToRead := "/etc/passwd"
		cmds = appendHexDumpCommand(ctx, cmds, fileToRead, &sysCmds, xdr.SensitiveFileType_SYSTEM_PASSWORDS)
		return &testDetails{commandDetails: cmds, syncText: makeWaitString(xdr.SensitiveFileType_SYSTEM_PASSWORDS)}, nil

	case pb.TestCase_USER_FILES:
		downloadsPath, err := cryptohome.DownloadsPath(ctx, normalizedUser)
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate command list ")

		}
		outputFile := filepath.Join(downloadsPath, "file_events_test")
		cmds = appendRWCommandDetails(ctx, cmds, outputFile, &sysCmds, xdr.SensitiveFileType_USER_FILE)
		return &testDetails{commandDetails: cmds, syncText: makeWaitString(xdr.SensitiveFileType_USER_FILE)}, nil

	case pb.TestCase_COOKIES:
		userPath, _ := cryptohome.UserPath(ctx, normalizedUser)
		rv := testDetails{syncText: makeWaitString(xdr.SensitiveFileType_USER_WEB_COOKIE)}
		cookieNames := []string{"Cookies", "Cookies-journal"}
		for _, fileName := range cookieNames {
			outputFile := filepath.Join(userPath, fileName)
			rv.filesToRestore = append(rv.filesToRestore, &restoreFile{name: outputFile})
			rv.commandDetails = appendRWCommandDetails(ctx, rv.commandDetails, outputFile, &sysCmds, xdr.SensitiveFileType_USER_WEB_COOKIE)
		}
		return &rv, nil

	case pb.TestCase_TPM_KEY:
		rv := testDetails{syncText: makeWaitString(xdr.SensitiveFileType_SYSTEM_TPM_PUBLIC_KEY)}
		secretNames := []string{"cryptohome.key", "cryptohome.ecc.key"}
		for _, fileName := range secretNames {
			outputFile := filepath.Join("/home/.shadow/", fileName)
			rv.filesToRestore = append(rv.filesToRestore, &restoreFile{name: outputFile})
			rv.commandDetails = appendRWCommandDetails(ctx, rv.commandDetails, outputFile, &sysCmds, xdr.SensitiveFileType_SYSTEM_TPM_PUBLIC_KEY)
		}
		return &rv, nil
	}
	return nil, errors.New("could not generate a command detail for " + testCase.String() + ", not supported")
}

func generateRwTestCaseForAllFilesUnderDir(ctx context.Context, dirName string, testCase pb.TestCase, sysCmds *map[string]string, fileType xdr.SensitiveFileType) (*testDetails, error) {
	files, err := recursiveGetFiles(dirName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate %q test vector, error listing files in %q", testCase.String(), dirName)
	}
	var rv testDetails
	for _, file := range files {
		rv.commandDetails = appendRWCommandDetails(ctx, rv.commandDetails, file, sysCmds, fileType)
		rv.filesToRestore = append(rv.filesToRestore, &restoreFile{name: file})
	}
	return &rv, nil
}

func generateWriteTestCaseForAllFilesUnderDir(ctx context.Context, dirName string, testCase pb.TestCase, sysCmds *map[string]string, fileType xdr.SensitiveFileType) (*testDetails, error) {
	files, err := recursiveGetFiles(dirName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate %q test vector, error listing files in %q", testCase.String(), dirName)
	}
	var rv testDetails
	for _, file := range files {
		rv.commandDetails = appendDDCommand(ctx, rv.commandDetails, file, sysCmds, fileType)
		rv.commandDetails = appendModifyAttributeCommand(ctx, rv.commandDetails, file, sysCmds, fileType)
		rv.filesToRestore = append(rv.filesToRestore, &restoreFile{name: file})
	}
	return &rv, nil
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

func recursiveGetFiles(dirName string) ([]string, error) {
	var fileNames []string
	dirEntries, err := os.ReadDir(dirName)
	if err != nil {
		return nil, errors.Wrapf(err, " failed to generate auth factors test vector, unable to read %q directory", dirName)
	}
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			subFileNames, err := recursiveGetFiles(dirEntry.Name())
			if err != nil {
				return nil, err
			}
			fileNames = append(fileNames, subFileNames...)
			continue
		}
		fileNames = append(fileNames, filepath.Join(dirName, dirEntry.Name()))
	}
	return fileNames, nil
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
			fileType:     fileType,
			filePath:     fileName,
			eventSubType: xdr.FileModify_MODIFY_ATTRIBUTE.Enum()},
		filePath: fileName,
		cleanup:  nil})
	return cmdDetails
}

func appendHexDumpCommand(ctx context.Context, cmdDetails []*commandDetail, fileName string, sysCmds *map[string]string, fileType xdr.SensitiveFileType) []*commandDetail {
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

// handleEnqueueRecord parses one EnqueueRecord dbus call made to missived.
// Exec events of the commands in expectedResults are recorded in the
// corresponding expectation, and the batched file events are returned.
func (f FileEvent) handleEnqueueRecord(method *dbusutil.CalledMethod, expectedResults map[uint64]*expectedResult) ([]*xdr.FileEventAtomicVariant, error) {
	if len(method.Arguments) == 0 {
		return nil, nil
	}
	arg, ok := method.Arguments[0].([]byte)
	if !ok {
		return nil, nil
	}
	enq := &rep.EnqueueRecordRequest{}
	if err := proto.Unmarshal(arg, enq); err != nil {
		return nil, errors.Wrap(err,
			"failed to unmarshal an EnqueueRecordRequest")
	}

	f.Logf("Destination is %s", enq.GetRecord().GetDestination())

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
					// Report but keep going so that the other events of the
					// batch still retire their expectations.
					if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
						f.Errorf("CROS_SECURITY_PROCESS process exec of pid %d has an invalid common field: %v", pid, err)
					}
					r.process = processExec.GetSpawnProcess()
					r.processTimeUs = uint64(v.GetCommon().GetCreateTimestampUs())
					// since we captured the exec the metafirst appearance in
					// the file event should always be false.
					*r.process.MetaFirstAppearance = false
					r.parentProcess = processExec.GetProcess()
					// same with the parent.
					*r.parentProcess.MetaFirstAppearance = false
					f.Logf("pid(%d)executed : %q", *r.process.CanonicalPid, *r.process.Commandline)
				}
			}
		}
	}
	// Save off file events.
	var bFileEvents []*xdr.FileEventAtomicVariant
	if enq.GetRecord().GetDestination() == rep.Destination_CROS_SECURITY_FILE {
		fe := &xdr.XdrFileEvent{}
		if err := proto.Unmarshal(enq.GetRecord().GetData(), fe); err != nil {
			return nil, errors.Wrap(err,
				"failed to unmarshal data for a CROS_SECURITY_FILE record")
		}
		for _, v := range fe.GetBatchedEvents() {
			eventType := ""
			if v.GetSensitiveRead() != nil {
				eventType = "Sensitive Read"
			} else if v.GetSensitiveModify() != nil {
				eventType = "Sensitive Modify"
			} else {
				f.Error("encountered an empty file event, no read, no modify found")
				continue
			}
			if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
				f.Errorf("CROS_SECURITY_FILE %v has an invalid common field: %v", eventType, err)
			}
			bFileEvents = append(bFileEvents, v)
		}
	}
	return bFileEvents, nil
}
