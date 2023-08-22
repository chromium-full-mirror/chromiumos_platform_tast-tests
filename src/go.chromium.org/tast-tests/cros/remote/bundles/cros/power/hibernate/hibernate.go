// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hibernate contains functionality shared by hibernate tests.
package hibernate

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
)

const (
	// CycleMaxDuration is the maximum duration of a hibernate cycle.
	CycleMaxDuration     = 5 * time.Minute
	hibernateCycleIDPath = "/tmp/hibernate_cycle_id"
)

type logger interface {
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}

// Tester provides shared functionality for hibernate tests and mainains
// state between different parts of a test.
type Tester struct {
	dut          *dut.DUT
	cycleID      uint32
	userAccount  *tape.OwnedTestAccount
	grpcClient   *crosserverutil.Client
	urlsForTabs  []string
	tabTargetIDs []string
	isFirstCycle bool
	logger       logger
	rpcHint      *testing.RPCHint
}

// NewTester creates and returns an instance of Tester.
func NewTester(ctx context.Context, s *testing.State, maxTestDuration time.Duration) *Tester {
	testAccount, err := leaseTestAccount(ctx, s.RequiredVar(tape.ServiceAccountVar), maxTestDuration)
	if err != nil {
		s.Fatal("Failed to lease test account: ", err)
	}

	return &Tester{dut: s.DUT(), userAccount: testAccount, isFirstCycle: true, logger: s, rpcHint: s.RPCHint()}
}

// HibernateAndResume performs a full hibernate cycle of hibernating the system
// (with reboot) and resuming it, including various checks.
//
// This function can serve as a template for other hibernate tests that
// only perform a partial cycle (e.g. due to forced errors).
func (t *Tester) HibernateAndResume(ctx context.Context) {
	defer t.CloseGRPCClient(ctx)

	t.PreHibernateSteps(ctx)

	if t.isFirstCycle && t.urlsForTabs != nil {
		t.openChromeTabs(ctx)
	}

	t.hibernateAndReboot(ctx)

	t.resumeFromHibernate(ctx)

	t.postResumeSteps(ctx)

	if t.urlsForTabs != nil {
		// verify previously open tabs still exist
		if err := t.login(ctx, true, true); err != nil {
			t.logger.Fatal("Failed to login: ", err)
		}

		t.verifyOpenChromeTabs(ctx)
	}

	t.isFirstCycle = false
}

// PreHibernateSteps prepares the system for hibernation. This includes checks
// and prework for later checks.
func (t *Tester) PreHibernateSteps(ctx context.Context) {
	// Create a new context for this hibernate cycle.
	ctxCycle, cancel := context.WithTimeout(ctx, CycleMaxDuration)
	defer cancel()

	if t.isFirstCycle {
		// Make sure the system is in a consistent state.
		t.reboot(ctxCycle)
	}

	// Write the id of this hibernate cycle to tmpfs, so we can confirm
	// that we read the same value on resume.
	t.writeCycleID(ctxCycle)

	// Get a new GRPC client after the reboot.
	t.getGRPCClient(ctxCycle)

	if t.isFirstCycle {
		// Log in with the user account that is used for hibernate.
		if err := t.login(ctxCycle, false, false); err != nil {
			t.logger.Fatal("Failed to login: ", err)
		}
	}

	// Check the kernel log for entries that indicate file system corruption.
	t.checkForFileSystemCorruptions(ctxCycle)
}

// Logout logs a signed in user out of the system.
func (t *Tester) Logout(ctx context.Context) {
	cl, err := rpc.Dial(ctx, t.dut, t.rpcHint)
	if err != nil {
		t.logger.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	upstartService := platform.NewUpstartServiceClient(cl.Conn)

	_, err = upstartService.StopJob(ctx, &platform.StopJobRequest{
		JobName: "ui",
	})
	if err != nil {
		t.logger.Fatal("Failed to stop 'ui' job: ", err)
	}
}

// HiberimageExists returns true if the 'hiberimage' logical volume exists,
// otherwise false.
func (t *Tester) HiberimageExists(ctx context.Context) bool {
	out, err := t.dut.Conn().CommandContext(ctx, "/sbin/lvs", "--options=name", "--noheadings").CombinedOutput()
	if err != nil {
		t.logger.Fatal("Failed to get list of logical volumes: ", err)
	}

	lvs := strings.Split(string(out), "\n")
	for i := 0; i < len(lvs); i++ {
		lv := strings.TrimSpace(lvs[i])

		if lv == "hiberimage" {
			return true
		}
	}

	return false
}

// CloseGRPCClient closes the associated GRPC client.
func (t *Tester) CloseGRPCClient(ctx context.Context) {
	if t.grpcClient == nil {
		return
	}

	t.grpcClient.Close(ctx)
}

// SetURLsForTabs allows to specify a list of URLs that should be opened
// in tabs before the system hibernates.
func (t *Tester) SetURLsForTabs(urlsForTabs []string) {
	t.urlsForTabs = urlsForTabs
}

func (t *Tester) hibernateAndReboot(ctx context.Context) {
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	t.logger.Log("Starting hibernation ...")

	out, err := t.dut.Conn().CommandContext(cmdCtx, "/sbin/minijail0", "-v", "/usr/sbin/hiberman", "hibernate", "-r").CombinedOutput()
	t.logger.Logf("hiberman output: %s", out)

	if err != nil {
		if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) ||
			strings.Contains(err.Error(), (&ssh.ExitMissingError{}).Error()) {
			// The command is expected to time out if hibernate was successful.
			return
		}
		t.logger.Fatal("failed to hibernate: ", err)
	}

	// no timeout or error, something went wrong.
	t.logger.Fatal("\"hiberman hibernate -r\" should disconnect the DUT")
}

func (t *Tester) resumeFromHibernate(ctx context.Context) {
	t.waitForDutToBoot(ctx)

	// a new GRPC client is needed after the reboot
	t.getGRPCClient(ctx)

	t.loginToResume(ctx)

	// a new GRPC client is needed after the resume from hibernate
	t.getGRPCClient(ctx)
}

func (t *Tester) postResumeSteps(ctx context.Context) {
	// Verify that the hibernate cycle id that we wrote earlier to
	// tmpfs can be read back.
	t.verifyCycleID(ctx)

	// Verify that the kernel log contains the expected entries for
	// a hibernate/resume cycle
	t.verifyKernelHibernateRestoreLogs(ctx)

	// Check the kernel log for entries that indicate file system corruption.
	t.checkForFileSystemCorruptions(ctx)
}

func (t *Tester) loginToResume(ctx context.Context) {
	loginCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// login() may or may not return an error in case of a successful login
	// that results in resuming the hibernated system.
	err := t.login(loginCtx, false, true)
	if err != nil &&
		!strings.Contains(err.Error(), context.DeadlineExceeded.Error()) &&
		!strings.Contains(err.Error(), "rpcc: the connection is closing") {
		t.logger.Fatal("unexpected login error: ", err)
	}

	// Wait for the hibernated system to resume, otherwise we might reconnect
	// to the DUT when the bootstrap system is still running, which defeats
	// the purpose.
	t.sleepWithContext(ctx, 20*time.Second)

	t.reconnectDUT(ctx)
}

func (t *Tester) login(ctx context.Context, reuseSession, keepState bool) error {
	// Start Chrome with the username and password.
	cs := pb.NewChromeServiceClient(t.grpcClient.Conn)
	if _, err := cs.New(ctx, &pb.NewRequest{
		EnableFeatures: []string{"CrOSSuspendToDisk", "CrOSSuspendToDiskAllowS4"},
		LoginMode:      pb.LoginMode_LOGIN_MODE_GAIA_LOGIN,
		Credentials: &pb.NewRequest_Credentials{
			Username: t.userAccount.Username,
			Password: t.userAccount.Password,
		},
		TryReuseSession: reuseSession,
		KeepState:       keepState,
	}); err != nil {
		return errors.Wrap(err, "failed to log into chrome on DUT")
	}

	return nil
}

func (t *Tester) reconnectDUT(ctx context.Context) {
	waitConnectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := t.dut.WaitConnect(waitConnectCtx); err != nil {
		t.logger.Fatal("Failed to reconnect: ", err)
	}
}

func (t *Tester) checkForFileSystemCorruptions(ctx context.Context) {
	fileCorruptionPatterns := []string{
		"space map common: bitmap check failed:",
		"sm_bitmap validator check failed",
		"metadata operation 'dm_pool_alloc_data_block' failed",
		"aborting current metadata transaction",
		"integrity: Error on flusing disk cache",
		"provision_block: alloc_data_block() failed",
		"Buffer I/O error on device",
		"error count since last fsck",
		"submitting bio failed at sector",
		"bad block bitmap checksum",
		"block bitmap corrupt",
		"block bitmap and bg descriptor inconsistent",
		"ext4_journal_check_start:83: Detected aborted journal",
		"I/O error while writing superblock",
		"blk_update_request: I/O error",
		"Buffer I/O error on device",
		"blk_update_request: I/O error",
	}

	kernelLog := t.getKernelLog(ctx)

	re := regexp.MustCompile(strings.Join(fileCorruptionPatterns, "|"))
	match := re.FindString(kernelLog)
	if match != "" {
		t.logger.Fatalf("the kernel log has entries that indicate system corruptions: '%s'", match)
	}

	t.logger.Log("No file corruption log entries found")
}

func (t *Tester) verifyKernelHibernateRestoreLogs(ctx context.Context) {
	expectedEntries := []string{
		// suspend
		"Freezing user space processes ...",
		"PM: end freeze of devices complete",
		"Disabling non-boot CPUs ...",
		// restore
		"PM: restore of devices complete",
		"Restarting tasks ...",
	}

	kernelLog := t.getKernelLog(ctx)

	for _, entry := range expectedEntries {
		if !strings.Contains(kernelLog, entry) {
			t.logger.Fatalf("Expected entry not found in kernel log: '%s'", entry)
		}
	}
}

func (t *Tester) writeCycleID(ctx context.Context) {
	t.cycleID = rand.Uint32()
	t.logger.Logf("Writing hibernate cycle id %d to %s", t.cycleID, hibernateCycleIDPath)

	command := fmt.Sprintf("echo %d > %s", t.cycleID, hibernateCycleIDPath)
	_, err := t.dut.Conn().CommandContext(ctx, "/bin/sh", "-c", command).Output()
	if err != nil {
		t.logger.Fatal("failed to write to hibernate cycle id: ", err)
	}
}

func (t *Tester) verifyCycleID(ctx context.Context) {
	out, err := t.dut.Conn().CommandContext(ctx, "/bin/cat", hibernateCycleIDPath).Output()
	if err != nil {
		t.logger.Fatal("failed to read hibernate cycle id: resume from hibernate failed")
	}

	str := string(out[:])
	str = strings.ReplaceAll(str, "\n", "")
	result, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		t.logger.Fatal("failed to parse hibernate cycle id, content: ", str)
	}

	cycleID := uint32(result)

	if cycleID != t.cycleID {
		t.logger.Fatalf("hibernate cycle id %d read from %s is incorrect. Expected value: %d",
			cycleID, hibernateCycleIDPath, t.cycleID)
	}
}

func (t *Tester) getGRPCClient(ctx context.Context) {
	if t.grpcClient != nil {
		// Close the existing GRPC client before getting a new one
		t.grpcClient.Close(ctx)
	}

	var err error
	t.grpcClient, err = crosserverutil.GetGRPCClient(ctx, t.dut)
	if err != nil {
		t.logger.Fatal("failed to connect to the GRPC server on the DUT: ", err)
	}
}

func (t *Tester) openChromeTabs(ctx context.Context) {
	t.logger.Log("Opening tabs")

	urls := t.urlsForTabs
	t.tabTargetIDs = make([]string, len(urls))

	svc := pb.NewConnServiceClient(t.grpcClient.Conn)
	for i := 0; i < len(urls); i++ {
		res, err := svc.NewConn(ctx, &pb.NewConnRequest{Url: urls[i]})
		if err != nil {
			t.logger.Fatalf("failed to open page %v: %o", urls[i], err)
		}

		t.tabTargetIDs[i] = res.TargetId
	}
}

func (t *Tester) verifyOpenChromeTabs(ctx context.Context) {
	t.logger.Log("Verifying open tabs")

	svc := pb.NewConnServiceClient(t.grpcClient.Conn)
	for _, targetID := range t.tabTargetIDs {
		t.logger.Log("Connecting to target: ", targetID)

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		conn, err := svc.NewConnForTarget(ctx, &pb.NewConnForTargetRequest{TargetId: targetID})
		if err != nil {
			t.logger.Fatalf("failed to get connection for target %v failed: %o", targetID, err)
		}

		t.logger.Log("Connected to target: ", targetID)

		req := &pb.ActivateTargetRequest{
			Id: conn.Id,
		}
		_, err = svc.ActivateTarget(ctx, req)
		if err != nil {
			t.logger.Fatalf("Failed to activate target %v: %o", targetID, err)
		}
	}
}

func (t *Tester) reboot(ctx context.Context) {
	t.logger.Log("Starting reboot")

	if err := t.dut.Reboot(ctx); err != nil {
		t.logger.Fatal("Failed to reboot DUT: ", err)
	}

	t.logger.Log("Reboot completed")
}

func (t *Tester) waitForDutToBoot(ctx context.Context) {
	t.logger.Log("Waiting for DUT to boot")

	waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := t.dut.WaitConnect(waitConnectCtx); err != nil {
		t.logger.Fatal("Failed to reconnect: ", err)
	}
}

func (t *Tester) getKernelLog(ctx context.Context) string {
	out, err := t.dut.Conn().CommandContext(ctx, "dmesg", "--since", "1 minute ago").Output()
	if err != nil {
		t.logger.Fatal("Failed to get kernel log: ", err)
	}

	return string(out[:])
}

func (t *Tester) sleepWithContext(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
		t.logger.Fatal(ctx.Err())
	case <-time.After(d):
		return
	}
}

func leaseTestAccount(ctx context.Context, accountID string, duration time.Duration) (*tape.OwnedTestAccount, error) {
	tapeClient, err := tape.NewClient(ctx, []byte(accountID))
	if err != nil {
		return nil, errors.Errorf("failed to create tape client: %s", err)
	}

	timeout := int32(duration.Seconds())
	accountManager, account, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		return nil, errors.Errorf("failed to create an account manager and lease an account: %s", err)
	}
	defer accountManager.CleanUp(ctx)

	return account, nil
}
