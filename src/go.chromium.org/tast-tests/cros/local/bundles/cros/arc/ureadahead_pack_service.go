// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/ureadahead"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/upstart"
	arcpb "go.chromium.org/tast-tests/cros/services/cros/arc"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Defines custom error that indicate required flag file is not set to "1".
type flagIsNotSetError struct {
	reason string
}

func (e *flagIsNotSetError) Error() string {
	return e.reason
}

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			arcpb.RegisterUreadaheadPackServiceServer(srv, &UreadaheadPackService{s: s})
		},
	})
}

// UreadaheadPackService implements tast.cros.arc.UreadaheadPackService.
type UreadaheadPackService struct {
	s *testing.ServiceState
}

// Generate generates ureadahead pack for requested Chrome login mode for VM or container.
func (c *UreadaheadPackService) Generate(ctx context.Context, request *arcpb.UreadaheadPackRequest) (*arcpb.UreadaheadPackResponse, error) {
	const (
		ureadaheadDataDir = "/var/lib/ureadahead"

		containerPackName = "opt.google.containers.android.rootfs.root.pack"
		containerRoot     = "/opt/google/containers/android/rootfs/root"

		tracingRoot = "/sys/kernel/tracing"

		logName   = "ureadahead.log"
		vmLogName = "vm_ureadahead.log"

		ureadaheadTimeout = 30 * time.Second

		// These are the same limits as arc.UreadaheadValidation test performed after
		// PFQ / Uprev and ChromeOS build image (BSS) are completed. This is to catch
		// issues before building full ChromeOS image but post-build testing is still
		// important to catch any unexpected build infra issues.
		minAcceptableHostPackSizeKB  = 300 * 1024
		minAcceptableGuestPackSizeKB = 100 * 1024
	)

	// Create arguments for running ureadahead.
	args := []string{
		"--verbose",
		"--force-trace",
		fmt.Sprintf("--path-prefix=%s", containerRoot),
		containerRoot,
	}

	// Stop UI to make sure we don't have any pending holds and race condition restarting Chrome.
	testing.ContextLog(ctx, "Stopping UI to release all possible locks")
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		return nil, errors.Wrap(err, "failed to stop ui")
	}
	defer upstart.EnsureJobRunning(ctx, "ui")

	// Check for whether ARCVM is present.
	vmEnabled, err := arc.VMEnabled()
	if err != nil {
		return nil, errors.Wrap(err, "failed to check whether ARCVM is enabled")
	}

	var packPath string
	var logPath string
	if vmEnabled {
		// Pass kernel params to ARCVM dev config
		if err := arc.AppendToArcvmDevConf(ctx, "--params=androidboot.arcvm_mount_debugfs=1"); err != nil {
			return nil, errors.Wrap(err, "failed to write mount debugfs boot flag to arcvm dev config")
		}
		if request.UseDevCaches {
			if err := arc.AppendToArcvmDevConf(ctx, "--params=androidboot.use_dev_caches=true"); err != nil {
				return nil, errors.Wrap(err, "failed to write use dev caches boot flag to arcvm dev config")
			}
		}
		defer arc.RestoreArcvmDevConf(ctx)
	} else {
		packPath = filepath.Join(ureadaheadDataDir, containerPackName)

		out, err := testexec.CommandContext(ctx, "lsof", "+D", containerRoot).CombinedOutput()
		if err != nil {
			// In case nobody holds file, lsof returns 1.
			if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != 1 {
				return nil, errors.Wrap(err, "failed to verify android root is not locked")
			}
		}
		outStr := string(out)
		if outStr != "" {
			return nil, errors.Errorf("found locks for %q: %q", containerRoot, outStr)
		}

		if err := os.Remove(packPath); err != nil && !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "failed to clean up existing pack")
		}
	}

	testing.ContextLog(ctx, "Login Chrome")
	// Switch to ureadahead generation mode in order to bind all services properly.
	chromeArgs := append(arc.DisableSyncFlags(), "--arc-force-show-optin-ui")
	if vmEnabled {
		// If VM, only generate guest OS pack file.
		chromeArgs = append(chromeArgs, "--arcvm-ureadahead-mode=generate")
	} else {
		chromeArgs = append(chromeArgs, "--arc-host-ureadahead-generation")
	}

	opts := []chrome.Option{
		chrome.ARCSupported(), // This does not start ARC automatically
		chrome.GAIALoginPool(request.Creds),
		chrome.ExtraArgs(chromeArgs...)}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Chrome")
	}

	// Shorten the total context by 5 seconds to allow for cleanup.
	cleanCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer cr.Close(cleanCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get name of the output directory")
	}

	// Drop caches before starting ureadahead generation.
	if err := disk.DropCaches(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to drop caches")
	}

	var vmPackPath string
	var vmLogPath string
	if vmEnabled {
		// Opt in.
		testing.ContextLog(ctx, "Waiting for ARC opt-in flow to complete")
		if err := optin.Perform(ctx, cr, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to perform opt-in")
		}

		vmLogPath = filepath.Join(ureadaheadDataDir, vmLogName)

		// Pull and obtain ARCVM pack from guest OS and dump pack file content to log.
		vmPackPath, err = getGuestPack(ctx, outDir, vmLogPath, request.UseDevCaches)
		if err != nil {
			return nil, errors.Wrap(err, "failed to obtain ureadahead pack from ARCVM guest OS")
		}

		// Verify the guest pack file dump for VM.
		if err := ureadahead.CheckPackFileDump(ctx, vmLogPath, minAcceptableGuestPackSizeKB); err != nil {
			return nil, errors.Wrapf(err, "failed to verify guest ureadahead pack file dump, please check %q", vmLogName)
		}
	} else {
		// Generate host OS pack file.
		testing.ContextLog(ctx, "Start ureadahead tracing")

		flags := []string{
			filepath.Join(tracingRoot, "tracing_on"),
			filepath.Join(tracingRoot, "events/fs/do_sys_open/enable"),
		}

		// Define callback to handle flag.
		type flagHandler func(string) error

		// Helper that processes all tracked tracing flags.
		processFlags := func(fn flagHandler) error {
			for _, flag := range flags {
				if err := fn(flag); err != nil {
					return err
				}
			}
			return nil
		}

		// Make sure ureadahead flips these flags to confirm it is started.
		resetFlag := func(flag string) error {
			return ioutil.WriteFile(flag, []byte("0"), 0644)
		}

		if err := processFlags(resetFlag); err != nil {
			return nil, errors.Wrap(err, "failed to reset ureadahead flag")
		}

		if err := ioutil.WriteFile(filepath.Join(tracingRoot, "trace"), []byte(""), 0644); err != nil {
			return nil, errors.Wrap(err, "failed to reset tracing buffer")
		}

		logPath = filepath.Join(ureadaheadDataDir, logName)
		log, err := os.Create(logPath)

		if err != nil {
			return nil, errors.Wrap(err, "failed to create log file")
		}
		defer log.Close()

		cmd := testexec.CommandContext(ctx, "ureadahead", args...)
		cmd.Stdout = log
		cmd.Stderr = log

		if err := cmd.Start(); err != nil {
			return nil, errors.Wrap(err, "failed to start ureadahead tracing")
		}

		// Make sure that content of the flag is set to "1".
		enusureFlagSet := func(flag string) error {
			content, err := ioutil.ReadFile(flag)
			if err != nil {
				return err
			}
			contentStr := strings.TrimSpace(string(content))
			// 1 means flag is enabled.
			if contentStr != "1" {
				return &flagIsNotSetError{
					reason: fmt.Sprintf("flag %q is not enabled; got: %v, want: 1", flag, contentStr),
				}
			}
			return nil
		}

		// Wait ureadahead actually started. All tracked flags must be flipped to "1".
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := processFlags(enusureFlagSet); err != nil {
				if _, ok := err.(*flagIsNotSetError); ok {
					return err
				}
				return testing.PollBreak(errors.Wrap(err, "failed to read flag"))
			}
			return nil
		}, &testing.PollOptions{Timeout: ureadaheadTimeout}); err != nil {
			return nil, errors.Wrap(err, "failed to ensure ureadahead started")
		}

		defer func() {
			if err := stopUreadaheadTracing(cleanCtx, cmd); err != nil {
				testing.ContextLog(cleanCtx, "Failed to stop ureadahead tracing")
			}
		}()

		// Opt in.
		testing.ContextLog(ctx, "Waiting for ARC opt-in flow to complete")
		if err := optin.Perform(ctx, cr, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to perform opt-in")
		}

		// Make sure tracing was not stopped in between. This verifies that all tracked flags
		// are still set to 1. If it not, that indicates that other component altered it while
		// ureadahead tracing session was running.
		if err := processFlags(enusureFlagSet); err != nil {
			return nil, errors.Wrap(err, "failed to ensure flag is set")
		}

		if err := stopUreadaheadTracing(ctx, cmd); err != nil {
			return nil, err
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			_, err := os.Stat(packPath)
			return err
		}, &testing.PollOptions{Timeout: ureadaheadTimeout}); err != nil {
			return nil, errors.Wrap(err, "failed to ensure pack file exists")
		}

		testing.ContextLog(ctx, "Ureadahead pack was generated")

		if err = ureadahead.DumpHostPack(ctx, packPath, logPath); err != nil {
			return nil, errors.Wrap(err, "failed to dump host ureadahead pack")
		}

		// Verify the host pack file dump for Container.
		if err = ureadahead.CheckPackFileDump(ctx, logPath, minAcceptableHostPackSizeKB); err != nil {
			return nil, errors.Wrapf(err, "failed to verify host ureadahead pack file dump, please check %q", logName)
		}

		a, err := arc.New(ctx, outDir)
		if err != nil {
			return nil, errors.Wrap(err, "could not connect to ARC Container")
		}

		if err := verifyDevCachesInstalled(ctx, a, request.UseDevCaches); err != nil {
			return nil, errors.Wrap(err, "failed to verify dev caches were installed during tracing")
		}
	}

	response := arcpb.UreadaheadPackResponse{
		PackPath:   packPath,
		VmPackPath: vmPackPath,
		LogPath:    logPath,
		VmLogPath:  vmLogPath,
	}
	return &response, nil
}

// stopUreadaheadTracing stops ureadahead tracing by sending interrupt request and waits until it
// stops. If ureadahead tracing is already stopped this returns no error.
func stopUreadaheadTracing(ctx context.Context, cmd *testexec.Cmd) error {
	if cmd.ProcessState != nil {
		// Already stopped. Do nothing.
		return nil
	}

	testing.ContextLog(ctx, "Sending interrupt signal to ureadahead tracing process")
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return errors.Wrap(err, "failed to send interrupt signal to ureadahead tracing")
	}

	if err := cmd.Wait(); err != nil {
		return errors.Wrap(err, "failed to wait ureadahead tracing done")
	}

	return nil
}

func (c *UreadaheadPackService) CheckMinMemory(ctx context.Context, request *empty.Empty) (*arcpb.CheckMinMemoryResponse, error) {
	const (
		ureadaheadVMMinMemoryKB = 7500000
	)

	// Check for whether ARCVM is present.
	vmEnabled, err := arc.VMEnabled()
	if err != nil {
		return nil, errors.Wrap(err, "failed to check whether ARCVM is enabled")
	}

	result := true
	if vmEnabled {
		testing.ContextLog(ctx, "Checking minimum memory requirement for ARCVM")
		memInfo, err := ioutil.ReadFile("/proc/meminfo")
		if err != nil {
			return nil, errors.Wrap(err, "failed to read /proc/meminfo")
		}
		memTotal := regexp.MustCompile(`(\n|^)MemTotal:\s+(\d+)\s+kB(\n|$)`).FindSubmatch(memInfo)
		if memTotal == nil {
			return nil, errors.Wrapf(err, "failed to find required MemTotal string: %q", memInfo)
		}
		memTotalInt, err := strconv.Atoi(string(memTotal[2]))
		if err != nil || memTotalInt <= 0 {
			return nil, errors.Wrapf(err, "failed to parse %q into valid value", memTotal[2])
		}
		if memTotalInt < ureadaheadVMMinMemoryKB {
			testing.ContextLogf(ctx, "Total memory %dkB < %dkB required, will not generate ureadahead pack", memTotalInt, ureadaheadVMMinMemoryKB)
			result = false
		}
	}

	response := arcpb.CheckMinMemoryResponse{
		Result: result,
	}
	return &response, nil
}

// verifyTracedServicesStopped will confirm traced services are not running.
func verifyTracedServicesStopped(ctx context.Context, a *arc.ARC) error {
	expected := "0"
	if value, err := a.GetProp(ctx, "persist.traced.enable"); err != nil || value != expected {
		return errors.Wrapf(err, "failed to verify traced is disabled (%v), got %v instead", expected, value)
	}
	expected = "stopped"
	if value, err := a.GetProp(ctx, "init.svc.traced_probes"); err != nil || value != expected {
		return errors.Wrapf(err, "failed to verify traced_probes is %v, got %v instead", expected, value)
	}
	if value, err := a.GetProp(ctx, "init.svc.traced_perf"); err != nil || value != expected {
		return errors.Wrapf(err, "failed to verify traced_perf is %v, got %v instead", expected, value)
	}
	if value, err := a.GetProp(ctx, "init.svc.traced"); err != nil || value != expected {
		return errors.Wrapf(err, "failed to verify traced is %v, got %v instead", expected, value)
	}
	return nil
}

// getGuestPack pulls ureadahead initial pack for requested Chrome login mode from guest OS.
func getGuestPack(ctx context.Context, outDir, logPath string, useDevCaches bool) (string, error) {
	const (
		ureadaheadDataDir = "/var/lib/ureadahead"

		arcvmPackName = "arcvm.var.lib.ureadahead.pack"

		ureadaheadStopTimeout      = 50 * time.Second
		ureadaheadStopInterval     = 5 * time.Second
		ureadaheadFileStatTimeout  = 90 * time.Second
		ureadaheadFileStatInterval = 15 * time.Second
	)

	packPath := filepath.Join(ureadaheadDataDir, arcvmPackName)

	if err := os.Remove(packPath); err != nil && !os.IsNotExist(err) {
		return "", errors.Wrapf(err, "failed to clean up %s on the host", packPath)
	}

	// Connect to ARCVM instance.
	a, err := arc.New(ctx, outDir)
	if err != nil {
		return "", errors.Wrap(err, "could not connect to ARCVM")
	}
	defer a.Close(ctx)

	// Verify traced services are stopped as they can interfere with pack generation.
	if err := verifyTracedServicesStopped(ctx, a); err != nil {
		return "", errors.Wrap(err, "failed to verify traced services are stopped")
	}

	// Confirm ureadahead_generate service has stopped.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if value, err := a.GetProp(ctx, "init.svc.ureadahead_generate"); err != nil {
			return testing.PollBreak(err)
		} else if value != "stopped" {
			return errors.New("ureadahead is not yet stopped")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  ureadaheadStopTimeout,
		Interval: ureadaheadStopInterval,
	}); err != nil {
		return "", errors.Wrap(err, "failed to wait for ureadahead to stop")
	}

	// Verify ureadahead exited which is triggered by opt-in completion.
	if value, err := a.GetProp(ctx, "dev.arc.ureadahead.exit"); err != nil || value != "1" {
		return "", errors.Wrap(err, "failed to verify ureadahead exited")
	}

	if err := verifyDevCachesInstalled(ctx, a, useDevCaches); err != nil {
		return "", errors.Wrap(err, "failed to verify dev caches were installed during tracing")
	}

	// Check for existence of newly generated pack file on guest side and get size.
	var sizeKB int64
	srcPath := filepath.Join(ureadaheadDataDir, "pack")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		sizeBytes, err := a.FileSize(ctx, srcPath)
		if err == nil {
			sizeKB = int64(sizeBytes / 1024)
		}
		return err
	}, &testing.PollOptions{
		Timeout:  ureadaheadFileStatTimeout,
		Interval: ureadaheadFileStatInterval,
	}); err != nil {
		return "", errors.Wrap(err, "failed to ensure pack file exists")
	}
	testing.ContextLogf(ctx, "Found ureadahead pack file with size %vkB in ARCVM", sizeKB)

	if err := a.PullFile(ctx, srcPath, packPath); err != nil {
		return "", errors.Wrapf(err, "failed to pull %s from ARCVM", srcPath)
	}

	// Dump guest ureadahead pack for validation.
	if err := ureadahead.DumpGuestPack(ctx, a, logPath); err != nil {
		return "", errors.Wrap(err, "failed to dump guest ureadahead pack")
	}

	return packPath, nil
}

// verifyDevCachesInstalled verifies dev caches were installed properly by arccachesetup service.
func verifyDevCachesInstalled(ctx context.Context, a *arc.ARC, useDevCaches bool) error {
	if useDevCaches {
		// Value of 3 corresponds to GMS Core, Packages and File Hashes caches.
		if value, err := a.GetProp(ctx, "dev.arc.caches_installed"); err != nil || value != "3" {
			return errors.Wrap(err, "failed to verify dev caches installed correctly")
		}
	}
	return nil
}
