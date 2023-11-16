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

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	// CycleMaxDuration is the maximum duration of a hibernate cycle.
	CycleMaxDuration         = 8 * time.Minute
	hibernateCycleIDPath     = "/tmp/hibernate_cycle_id"
	rebootAfterHibernatePath = "/run/power_manager/root/reboot_after_hibernate"
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
	dut             *dut.DUT
	cycleID         uint32
	overrideCycleID bool
	userAccount     *tape.OwnedTestAccount
	accountManager  *tape.OwnedTestAccountManager
	grpcClient      *crosserverutil.Client
	urlsForTabs     []string
	tabTargetIDs    []string
	isFirstCycle    bool
	logger          logger
	rpcHint         *testing.RPCHint
	memPressureMB   uint32
}

// NewTester creates and returns an instance of Tester.
func NewTester(ctx context.Context, s *testing.State, account *tape.OwnedTestAccount, maxTestDuration time.Duration) (*Tester, error) {
	var accountManager *tape.OwnedTestAccountManager
	var err error
	testAccount := account

	if testAccount == nil {
		testAccount, accountManager, err = leaseTestAccount(ctx, s.RequiredVar(tape.ServiceAccountVar), maxTestDuration)
		if err != nil {
			return nil, errors.Wrap(err, "Unable to lease test account")
		}
	}

	return &Tester{dut: s.DUT(), userAccount: testAccount, accountManager: accountManager, isFirstCycle: true, logger: s, rpcHint: s.RPCHint()}, nil
}

// CleanUp should be called when the test is about to exit to clean up any resources that
// might be left behind otherwise.
func (t *Tester) CleanUp(ctx context.Context) error {
	if t.accountManager != nil {
		t.logger.Log("Cleaning up owned test accounts")
		return t.accountManager.CleanUp(ctx)
	}

	return nil
}

// OverrideCycleID overrides the hibernate cycle id.
func (t *Tester) OverrideCycleID(cycleID uint32) {
	t.cycleID = cycleID
	t.overrideCycleID = true
}

// SetSimulateMemoryPressure can be used to set the amount of simulated memory pressure in MB.
func (t *Tester) SetSimulateMemoryPressure(memMB uint32) {
	t.memPressureMB = memMB
}

// HibernateAndResume performs a full hibernate cycle of hibernating the system
// (with reboot) and resuming it, including various checks.
//
// This function can serve as a template for other hibernate tests that
// only perform a partial cycle (e.g. due to forced errors).
func (t *Tester) HibernateAndResume(ctx context.Context) error {
	defer t.CloseGRPCClient(ctx)

	if err := t.PreHibernateSteps(ctx, false); err != nil {
		return errors.Wrap(err, "pre-hibernate steps failed")
	}

	if t.isFirstCycle && t.urlsForTabs != nil {
		if err := t.openChromeTabs(ctx); err != nil {
			return errors.Wrap(err, "failed to open chrome tabs")
		}
	}

	err := t.dut.Conn().CommandContext(ctx, "/usr/bin/touch", rebootAfterHibernatePath).Run()
	if err != nil {
		return errors.Wrapf(err, "failed to create %s", rebootAfterHibernatePath)
	}
	defer func() {
	      _ = t.dut.Conn().CommandContext(ctx, "/bin/rm", rebootAfterHibernatePath).Run()
	}()

	if err := t.hibernate(ctx, true); err != nil {
		return errors.Wrap(err, "failed to hibernate and reboot")
	}

	if err := t.Resume(ctx); err != nil {
		return err
	}

	t.isFirstCycle = false
	return nil
}

// HibernateToShutdown performs a hibernate to system shutdown.
//
// This mode can be useful for creating VM images which can be replayed via
// resume multiple times.
func (t *Tester) HibernateToShutdown(ctx context.Context) error {
	defer t.CloseGRPCClient(ctx)

	if err := t.PreHibernateSteps(ctx, true); err != nil {
		return errors.Wrap(err, "pre-hibernate steps failed")
	}

	if t.urlsForTabs != nil {
		if err := t.openChromeTabs(ctx); err != nil {
			return errors.Wrap(err, "failed to open chrome tabs")
		}
	}

	if err := t.hibernate(ctx, false); err != nil {
		return errors.Wrap(err, "failed to hibernate and shutdown")
	}

	return nil
}

// Resume performs a system login and resume
//
// This is useful when replaying a system vm image
func (t *Tester) Resume(ctx context.Context) error {
	if err := t.resumeFromHibernate(ctx); err != nil {
		return errors.Wrap(err, "resume from hibernate failed")
	}

	if err := t.postResumeSteps(ctx); err != nil {
		return errors.Wrap(err, "post resume steps failed")
	}

	// verify previously open tabs still exist
	if t.urlsForTabs != nil {
		if err := t.login(ctx, true, true); err != nil {
			return errors.Wrap(err, "user login failed on resume")
		}

		if err := t.verifyOpenChromeTabs(ctx); err != nil {
			return errors.Wrap(err, "verification of open tabs failed")
		}
	}

	return nil
}

// PreHibernateSteps prepares the system for hibernation. This includes checks
// and prework for later checks.
func (t *Tester) PreHibernateSteps(ctx context.Context, skipReboot bool) error {
	// Create a new context for this hibernate cycle.
	ctxCycle, cancel := context.WithTimeout(ctx, CycleMaxDuration)
	defer cancel()

	if t.isFirstCycle && !skipReboot {
		// Make sure the system is in a consistent state.
		if err := t.reboot(ctxCycle); err != nil {
			return err
		}
	}

	// Write the id of this hibernate cycle to tmpfs, so we can confirm
	// that we read the same value on resume.
	if err := t.writeCycleID(ctxCycle); err != nil {
		return err
	}

	// Get a new GRPC client after the reboot.
	if err := t.getGRPCClient(ctxCycle); err != nil {
		return err
	}

	if t.isFirstCycle {
		// Log in with the user account that is used for hibernate.
		if err := t.login(ctxCycle, false, false); err != nil {
			return err
		}

		// Wait for 'hiberman resume' to complete.
		if err := t.waitHibermanResumeDone(ctxCycle); err != nil {
			return err
		}
	}

	l, err := t.getKernelLog(ctx)
	if err != nil {
		return err
	}

	if err := t.checkForFileSystemCorruptions(ctx, l); err != nil {
		return err
	}
	return nil
}

// Logout logs a signed in user out of the system.
func (t *Tester) Logout(ctx context.Context) error {
	cl, err := rpc.Dial(ctx, t.dut, t.rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}

	upstartService := platform.NewUpstartServiceClient(cl.Conn)
	_, err = upstartService.StopJob(ctx, &platform.StopJobRequest{
		JobName: "ui",
	})
	if err != nil {
		return errors.Wrap(err, "failed to stop 'ui' job")
	}

	return nil
}

// HiberimageExists returns true if the 'hiberimage' logical volume exists,
// otherwise false.
func (t *Tester) HiberimageExists(ctx context.Context) (bool, error) {
	out, err := t.dut.Conn().CommandContext(ctx, "/sbin/lvs", "--options=name", "--noheadings").CombinedOutput()
	if err != nil {
		return false, err
	}

	lvs := strings.Split(string(out), "\n")
	for i := 0; i < len(lvs); i++ {
		lv := strings.TrimSpace(lvs[i])

		if lv == "hiberimage" {
			return true, nil
		}
	}

	return false, nil
}

// CloseGRPCClient closes the associated GRPC client.
func (t *Tester) CloseGRPCClient(ctx context.Context) error {
	defer func() { t.grpcClient = nil }()
	if t.grpcClient == nil {
		return nil
	}

	return t.grpcClient.Close(ctx)
}

// SetURLsForTabs allows to specify a list of URLs that should be opened
// in tabs before the system hibernates.
func (t *Tester) SetURLsForTabs(urlsForTabs []string) {
	t.urlsForTabs = urlsForTabs
}

func (t *Tester) disableConsoleSuspend(ctx context.Context) error {
	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	t.logger.Log("Disabling console_suspend")
	if out, err := t.dut.Conn().CommandContext(cmdCtx, "/bin/sh", "-c", "echo N | sudo tee /sys/module/printk/parameters/console_suspend").CombinedOutput(); err != nil {
		return errors.Wrapf(err, "Disabling console_suspend failed: %s", out)
	}

	return nil
}

func (t *Tester) forceMemPressure(ctx context.Context, sizeMB uint32) error {
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	t.logger.Logf("Allocating %dMB of memory...", sizeMB)
	// We use ramfs rather than tmpfs for a few reasons. The primary reason is that
	// ramfs is not evictable so we don't have to worry about it being compressible or
	// not because it will never be swapped out anyway. Additionally, it's not size
	// restricted to the size of the mount as it would be with tmpfs.
	//
	// We also use this as an opportunity to check for any corruption by storing
	// the sha256 of this large allocation along with it which can be verified on
	// resume.
	allocCmd := `
	mountpoint /run/mem_pressure || \
	mkdir /run/mem_pressure 2>/dev/null ; \
	mount -t ramfs ramfs /run/mem_pressure ; \
	dd if=/dev/urandom of=/run/mem_pressure/alloc bs=1M count=%d && \
	sha256sum /run/mem_pressure/alloc | cut -f1 -d' ' | tee /run/mem_pressure/alloc_sha256
	`
	allocCmdToRun := fmt.Sprintf(allocCmd, sizeMB)
	out, err := t.dut.Conn().CommandContext(cmdCtx, "/bin/sh", "-c", allocCmdToRun).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "allocating memory failed: %s", out)
	}

	return nil
}

func (t *Tester) verifyMemPressureHashOnResume(ctx context.Context) error {
	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	t.logger.Log("Verifiying mem pressure hash after resume...")
	verifyCmd := `
	set -e ; \
	[ -f /run/mem_pressure/alloc ] && \
	COMPUTED_HASH=$(sha256sum /run/mem_pressure/alloc | cut -f1 -d' '); \
	INMEM_HASH=$(cat /run/mem_pressure/alloc_sha256); \
	if [ "$COMPUTED_HASH" = "$INMEM_HASH" ]; then \
		echo -n "OK"
		umount /run/mem_pressure
		exit 0; \
	fi; \
	echo -n "FAILED HASH VERIFICATION, GOT:$COMPUTED_HASH WANTED:$INMEM_HASH"; \
	exit 1;
	`
	out, err := t.dut.Conn().CommandContext(cmdCtx, "/bin/sh", "-c", verifyCmd).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to verify mem pressure: %s", string(out))
	}

	if strings.Contains(string(out), "FAILED HASH VERIFICATION") {
		msg := fmt.Sprintf("memory pressure contents didn't match: %s", string(out))
		return errors.New(msg)
	}

	t.logger.Logf("Mem pressure hash Verification result: %s", out)
	return nil
}

func (t *Tester) waitForDutUnreachable(ctx context.Context) error {
	defer t.dut.Close(ctx)
	if err := t.dut.WaitUnreachable(ctx); err != nil {
		t.logger.Logf("Error waiting for dut to become unreachable: %v", err)
		return err
	}
	t.logger.Logf("DUT has become unreachable")
	return nil
}

func (t *Tester) hibernate(ctx context.Context, reboot bool) error {
	if err := t.disableConsoleSuspend(ctx); err != nil {
		return err
	}

	if t.memPressureMB > 0 {
		if err := t.forceMemPressure(ctx, t.memPressureMB); err != nil {
			return err
		}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	t.logger.Log("Starting hibernation ...")

	// We will wait for the dut to become unreachable which is expected. By doing this
	// we can close the connection earlier allowing the next stages of the test to kick off.
	go t.waitForDutUnreachable(cmdCtx)

	err := t.dut.Conn().CommandContext(cmdCtx, "powerd_dbus_suspend", "--flavor=2").Run()

	if err != nil {
		if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) ||
			strings.Contains(err.Error(), (&ssh.ExitMissingError{}).Error()) ||
			strings.Contains(err.Error(), context.Canceled.Error()) {
			// The command is expected to time out or the connection to drop if hibernate was successful.
			return nil
		}

		return errors.Wrap(err, "failed to hibernate")
	}

	return errors.New("'hiberman hibernate' did not cause a shutdown/reboot")
}

func (t *Tester) resumeFromHibernate(ctx context.Context) error {
	if err := t.waitForDutToBoot(ctx); err != nil {
		return err
	}

	// a new GRPC client is needed after the reboot
	if err := t.getGRPCClient(ctx); err != nil {
		return err
	}

	// Console suspend is re-enabled on reboot, disable it again.
	if err := t.disableConsoleSuspend(ctx); err != nil {
		return err
	}

	if err := t.loginToResume(ctx); err != nil {
		return err
	}

	// a new GRPC client is needed after the resume from hibernate
	if err := t.getGRPCClient(ctx); err != nil {
		return nil
	}

	return nil
}

func (t *Tester) postResumeSteps(ctx context.Context) error {
	// Check the kernel log for entries that indicate file system corruption. Additionally,
	// we will check for integrity errors. We do these checks first because we want to
	// ensure that the original cause is found early rather than failing later
	// due to a mismatched cycle id.
	kernelLog, err := t.getKernelLog(ctx)
	if err != nil {
		return err
	}

	if err := t.checkForFileSystemCorruptions(ctx, kernelLog); err != nil {
		return err
	}

	if err := t.checkforHibernateImageErrors(ctx, kernelLog); err != nil {
		return err
	}

	// Verify that the hibernate cycle id that we wrote earlier to
	// tmpfs can be read back.
	if err := t.verifyCycleID(ctx); err != nil {
		return err
	}

	// Verify that the kernel log contains the expected entries for
	// a hibernate/resume cycle
	if err := t.verifyKernelHibernateRestoreLogs(ctx, kernelLog); err != nil {
		return err
	}

	if t.memPressureMB > 0 {
		if err := t.verifyMemPressureHashOnResume(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (t *Tester) loginToResume(ctx context.Context) error {
	loginCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// login() may or may not return an error in case of a successful login
	// that results in resuming the hibernated system.
	err := t.login(loginCtx, false, true)

	if err != nil && strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		// Before we assume that a DeadlineExceeded is valid, let's check if the connection was terminated
		if t.dut.Health(ctx) != nil {
			// Eat the error
			err = nil
		}
	}

	if err != nil &&
		!strings.Contains(err.Error(), "rpcc: the connection is closing") {
		return errors.Wrap(err, "unexpected error type from login")
	}

	// Wait for the hibernated system to resume, otherwise we might reconnect
	// to the DUT when the bootstrap system is still running, which defeats
	// the purpose.
	t.sleepWithContext(ctx, 20*time.Second)

	if err := t.reconnectDUT(ctx); err != nil {
		return errors.Wrap(err, "failed to reconnect to the DUT")
	}

	return nil
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

func (t *Tester) reconnectDUT(ctx context.Context) error {
	if err := t.dut.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "reconnecting to the DUT failed")
	}

	return nil
}

func (t *Tester) checkForFileSystemCorruptions(ctx context.Context, kernelLog string) error {
	fileCorruptionPatterns := []string{
		"space map common: bitmap check failed:",
		"failed to insert inode",
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
		"EXT4-fs error",
		"I/O error while writing superblock",
		"blk_update_request: I/O error",
		"Buffer I/O error on device",
		"blk_update_request: I/O error",
	}

	re := regexp.MustCompile(strings.Join(fileCorruptionPatterns, "|"))
	match := re.FindString(kernelLog)
	if match != "" {
		return errors.Errorf("the kernel log has entries that indicate system corruptions: %q", match)
	}

	t.logger.Log("No file corruption log entries found")
	return nil
}

func (t *Tester) checkforHibernateImageErrors(ctx context.Context, kernelLog string) error {
	patterns := []string{
		"Unrecognized hibernate image header format!",
		"hibernation: Image mismatch: architecture specific data",
		"dm-[0-9]+: INTEGRITY AEAD ERROR, sector [0-9]+",
	}

	re := regexp.MustCompile(strings.Join(patterns, "|"))
	match := re.FindString(kernelLog)
	if match != "" {
		return errors.Errorf("the kernel log has entries that indicate an error: %q", match)
	}

	t.logger.Log("No unexpected kernel error entries were found")
	return nil
}

func (t *Tester) verifyKernelHibernateRestoreLogs(ctx context.Context, kernelLog string) error {
	expectedEntries := []string{
		// suspend
		"Freezing user space processes ...",
		"PM: end freeze of devices complete",
		"Disabling non-boot CPUs ...",
		// restore
		"PM: restore of devices complete",
		"Restarting tasks ...",
	}

	for _, entry := range expectedEntries {
		if !strings.Contains(kernelLog, entry) {
			return errors.Errorf("expected entry not found in kernel log: %q", entry)
		}
	}

	return nil
}

func (t *Tester) writeCycleID(ctx context.Context) error {
	if !t.overrideCycleID {
		t.cycleID = rand.Uint32()
	}
	t.logger.Logf("Writing hibernate cycle id %d to %s", t.cycleID, hibernateCycleIDPath)

	command := fmt.Sprintf("echo %d > %s", t.cycleID, hibernateCycleIDPath)
	if _, err := t.dut.Conn().CommandContext(ctx, "/bin/sh", "-c", command).Output(); err != nil {
		return errors.Wrap(err, "failed to write to hibernate cycle id")
	}

	return nil
}

func (t *Tester) verifyCycleID(ctx context.Context) error {
	out, err := t.dut.Conn().CommandContext(ctx, "/bin/cat", hibernateCycleIDPath).Output()
	if err != nil {
		return errors.Wrapf(err, "Unable to read cycle ID from %s", hibernateCycleIDPath)
	}

	str := string(out[:])
	str = strings.ReplaceAll(str, "\n", "")
	result, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		return errors.Wrapf(err, "Unable to parse cycle id %q", str)
	}

	cycleID := uint32(result)
	if cycleID != t.cycleID {
		return errors.Errorf("hibernate cycle id %d read from %s is incorrect. Expected value: %d", cycleID, hibernateCycleIDPath, t.cycleID)
	}

	t.logger.Logf("Read correct hibernate cycle id %d from %s", cycleID, hibernateCycleIDPath)
	return nil
}

func (t *Tester) getGRPCClient(ctx context.Context) error {
	if t.grpcClient != nil {
		// Close the existing GRPC client before getting a new one
		if err := t.grpcClient.Close(ctx); err != nil {
			return errors.Wrap(err, "Unable to close existing grpc client")
		}
	}

	var err error
	t.grpcClient, err = crosserverutil.GetGRPCClient(ctx, t.dut)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the GRPC server on the DUT")
	}

	return nil
}

func (t *Tester) openChromeTabs(ctx context.Context) error {
	t.logger.Log("Opening tabs")

	urls := t.urlsForTabs
	t.tabTargetIDs = make([]string, len(urls))

	svc := pb.NewConnServiceClient(t.grpcClient.Conn)
	for i := 0; i < len(urls); i++ {
		res, err := svc.NewConn(ctx, &pb.NewConnRequest{Url: urls[i]})
		if err != nil {
			return errors.Wrapf(err, "failed to open page %v", urls[i])
		}

		t.tabTargetIDs[i] = res.TargetId
	}

	return nil
}

func (t *Tester) verifyOpenChromeTabs(ctx context.Context) error {
	t.logger.Log("Verifying open tabs")

	svc := pb.NewConnServiceClient(t.grpcClient.Conn)
	for _, targetID := range t.tabTargetIDs {
		t.logger.Log("Connecting to target: ", targetID)

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		conn, err := svc.NewConnForTarget(ctx, &pb.NewConnForTargetRequest{TargetId: targetID})
		if err != nil {
			return errors.Wrapf(err, "failed to get connection for target %v", targetID)
		}

		t.logger.Log("Connected to target: ", targetID)

		req := &pb.ActivateTargetRequest{
			Id: conn.Id,
		}
		_, err = svc.ActivateTarget(ctx, req)
		if err != nil {
			return errors.Wrapf(err, "failed to activate target %v", targetID)
		}
	}

	return nil
}

func (t *Tester) reboot(ctx context.Context) error {
	t.logger.Log("Starting reboot")

	if err := t.dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	t.logger.Log("Reboot completed")
	return nil
}

func (t *Tester) waitForDutToBoot(ctx context.Context) error {
	t.logger.Log("Waiting for DUT to boot")

	ctxReconnect, cancelReconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelReconnect()

	for {
		ctxConnect, cancelConnect := context.WithTimeout(ctxReconnect, 3*time.Second)
		defer cancelConnect()

		if err := t.dut.WaitConnect(ctxConnect); err == nil {
			break
		}

		select {
		case <-time.After(time.Second * 1):
			break
		case <-ctxReconnect.Done():
			return ctxReconnect.Err()
		}
	}

	return nil
}

func (t *Tester) waitHibermanResumeDone(ctx context.Context) error {
	err := testing.Poll(ctx, func(ctx context.Context) error {
		err := t.dut.Conn().CommandContext(ctx, "pidof", "hiberman").Run()
		if err == nil {
			return errors.New("hiberman resume is still running")
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second})

	return err
}

func (t *Tester) getKernelLog(ctx context.Context) (string, error) {
	out, err := t.dut.Conn().CommandContext(ctx, "dmesg", "-k").Output()
	if err != nil {
		return "", err
	}

	return string(out[:]), nil
}

func (t *Tester) sleepWithContext(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
		t.logger.Fatal(ctx.Err())
	case <-time.After(d):
		return
	}
}

func leaseTestAccount(ctx context.Context, accountID string, duration time.Duration) (*tape.OwnedTestAccount, *tape.OwnedTestAccountManager, error) {
	tapeClient, err := tape.NewClient(ctx, []byte(accountID))
	if err != nil {
		return nil, nil, errors.Errorf("failed to create tape client: %s", err)
	}

	timeout := int32(duration.Seconds())
	accountManager, account, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		return nil, nil, errors.Errorf("failed to create an account manager and lease an account: %s", err)
	}

	return account, accountManager, nil
}
