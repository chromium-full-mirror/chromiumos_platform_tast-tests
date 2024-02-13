// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/disk"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// The fio binaries are built from the source (https://github.com/axboe/fio)
	// with the following commands:
	// $ NDK=/usr/lib/android-ndk
	// $ export LIBS="-landroid"
	// $ export UNAME=Android
	//   # For x86_64 boards
	// $ export CC=$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android30-clang
	//   # For ARM boards
	// $ export CC=$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android30-clang
	//   # --extra-clfags is only needed for x86_64 boards.
	// $ ./configure --extra-cflags="-march=goldmont -mno-xsaves"
	// $ make -j16 fio
	fioX86BinaryData = "fio-arc-x86_64"
	fioARMBinaryData = "fio-arc-arm64"
	// fioGuestPath is the guest-side path to place the fio binary.
	fioGuestPath = "/data/local/tmp/fio-arc"
	// fioOutputGuest is the guest-side path to output the fio results.
	fioOutputGuest = "/data/local/tmp/fio-output.json"
	// fioReadTestFileName is the name of the target file for read tests.
	fioReadTestFileName = "fio_read_test_file"
	// fioWriteTestFileName is the name of the target file for write tests.
	fioWriteTestFileName = "fio_write_test_file"
)

// fioTestParams defines the set of parameters for each test case.
type fioTestParams struct {
	// directory is the path to the directory to test the I/O on.
	directory string
	// readTestFilePath is the path to the file to test the read on. The
	// supplied file path must exist. When this is not specified, arc.Fio Tast
	// test creates a file beforehand and use it in the read test.
	readTestFilePath string
	// needsARCSystemServices indicates whether we need to keep ARC system
	// services running during the test. This should be true for testing the
	// primary emulated volume (covered by MediaProvider FUSE).
	needsARCSystemServices bool
	// numRepeat defines how many times the fio job should be repeated.
	// Default is 1.
	numRepeat int
	// readJobs is the set of jobs to be tested that performs read.
	readJobs []string
	// writeJobs is the set of jobs to be tested that performs write.
	writeJobs []string
}

type fioJob struct {
	name string
	// ioType is the type of I/O tested in this job. This can either be "read",
	// "write" or "trim".
	ioType string
}

var defaultFioReadJobs = []string{
	"seq_read_buffered",
	"seq_read_direct",
	"rand_read_buffered",
	"rand_read_direct",
}

var defaultFioWriteJobs = []string{
	"seq_write_buffered",
	"seq_write_direct",
	"seq_write_fsync",
	"rand_write_buffered",
	"rand_write_direct",
	"rand_write_fsync",
}

type jobResult struct {
	// bw is the bandwidth in KiBps.
	bw float64
	// ioKBytes is the bytes read or written in KiB.
	ioKBytes int
}

func fioJobFileNames() []string {
	var fioJobFiles = []string{}
	for _, job := range append(defaultFioReadJobs, defaultFioWriteJobs...) {
		// Job files should be of the form |fio_<jobname>.job|.
		fioJobFiles = append(fioJobFiles, fmt.Sprintf("fio_%s.job", job))
	}
	return fioJobFiles
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Fio,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests running fio inside ARCVM",
		Contacts:     []string{"arc-storage@google.com", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		SoftwareDeps: []string{"chrome", "android_vm"},
		Data:         append(fioJobFileNames(), fioX86BinaryData, fioARMBinaryData),
		Timeout:      20 * time.Minute,
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Params: []testing.Param{{
			Name: "system",
			Val: fioTestParams{
				readTestFilePath: "/system/product/priv-app/PrebuiltGmsCore/PrebuiltGmsCoreArcRelease.apk",
				// /system is read-only, and neither Squashfs or EROFS supports
				// O_DIRECT.
				readJobs:  []string{"seq_read_buffered", "rand_read_buffered"},
				writeJobs: []string{},
				// Because the target file (PrebuiltGmsCoreArcRelease.apk) is
				// not so large (~ 150 MiB), repeat the test 10 times to
				// stabilize the result.
				numRepeat: 10,
			},
		}, {
			Name: "data",
			Val: fioTestParams{
				directory: "/data/local/tmp",
				readJobs:  defaultFioReadJobs,
				writeJobs: defaultFioWriteJobs,
			},
		}, {
			Name: "data_media_download",
			Val: fioTestParams{
				directory: "/data/media/0/Download",
				readJobs:  defaultFioReadJobs,
				writeJobs: defaultFioWriteJobs,
			},
		}, {
			Name: "download",
			Val: fioTestParams{
				directory:              "/storage/emulated/0/Download",
				readJobs:               defaultFioReadJobs,
				writeJobs:              defaultFioWriteJobs,
				needsARCSystemServices: true,
			},
		}, {
			Name: "emulated",
			Val: fioTestParams{
				directory:              "/storage/emulated/0/Documents",
				readJobs:               defaultFioReadJobs,
				writeJobs:              defaultFioWriteJobs,
				needsARCSystemServices: true,
			},
		}},
	})
}

// Fio implements the test logic of arc.Fio.
func Fio(ctx context.Context, s *testing.State) {
	params := s.Param().(fioTestParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Disable multicast to make sure CPU can be stabilized for capturing performance metrics
	multicastCleanup, err := setup.DisableMulticastSetup(ctx)
	if err != nil {
		s.Fatal("Could not disable multicast: ", err)
	}
	defer multicastCleanup(cleanupCtx)

	// Enable adb root on user builds, and remove --serial to disable
	// virtio-console. This needs to be done before starting ARCVM.
	if err := arc.WriteArcvmDevConf(ctx, "--params=androidboot.verifiedbootstate=orange"); err != nil {
		s.Fatal("Failed to set arcvm_dev.conf: ", err)
	}
	defer arc.RestoreArcvmDevConf(cleanupCtx)

	// Use the same options as arcBootedWithDisableExternalStorage fixture.
	cr, err := chrome.New(ctx,
		chrome.ARCEnabled(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
		chrome.ExtraArgs("--disable-features=FirmwareUpdaterApp"),
		chrome.ExtraArgs("--disable-features=ArcExternalStorageAccess"))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	testing.ContextLog(ctx, "Restarting adbd as root")
	if err := a.Root(ctx); err != nil {
		s.Fatal("Failed to start adb root: ", err)
	}

	// Disable CPU frequency scaling and thermal throttling to have more
	// stable results.
	cleanup, err := mediacpu.SetUpBenchmark(ctx)
	if err != nil {
		s.Fatal("Failed to perform setUpBenchmark: ", err)
	}
	defer cleanup(cleanupCtx)

	// Install the fio binary on the guest side.
	fioBinaryData, err := fioBinNameForArch(ctx, a)
	if err != nil {
		s.Fatal("Failed to get fio binary name for architecture: ", err)
	}
	if err := a.PushFile(ctx, s.DataPath(fioBinaryData), fioGuestPath); err != nil {
		s.Fatal("Failed to push fio: ", err)
	}
	if err := a.Command(ctx, "chmod", "u+x", fioGuestPath).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to make fio executable: ", err)
	}

	if !params.needsARCSystemServices {
		// Stop all services that will be stopped by default with
		// "adb shell stop" on T+ (defined in system/core/toolbox/start.cpp).
		// Not using "adb shell stop" here because in R, the command will
		// additionally try to stop iorapd, which fails.
		if err := a.Command(ctx, "stop", "zygote", "netd", "surfaceflinger", "audioserver").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to stop ARC system services: ", err)
		}
	}

	// Set the default value for the |numRepeat| param to be 1.
	if params.numRepeat == 0 {
		params.numRepeat = 1
	}

	// Create the file to read in read tests, rather than letting fio create it.
	// This is because fio only drops the guest-side caches after creating the
	// test file, but we also need to drop the host-side caches.
	if params.readTestFilePath == "" {
		params.readTestFilePath = filepath.Join(params.directory, fioReadTestFileName)

		// Create a 1GiB file.
		if err := a.Command(ctx, fioGuestPath, "--name=create_file", "--create_only=1", "--size=1G", "--filename="+params.readTestFilePath).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to create test file to read: ", err)
		}
	}

	var resultLogs = []string{}
	perfValues := perf.NewValues()

	var jobs = []fioJob{}
	for _, job := range params.readJobs {
		jobs = append(jobs, fioJob{job, "read"})
	}
	for _, job := range params.writeJobs {
		jobs = append(jobs, fioJob{job, "write"})
	}

	for _, job := range jobs {
		jobFileName := fmt.Sprintf("fio_%s.job", job.name)
		jobFilePath := filepath.Join("/data/local/tmp", jobFileName)

		if err := a.PushFile(ctx, s.DataPath(jobFileName), jobFilePath); err != nil {
			s.Fatal("Failed to prepare fio job file: ", err)
		}

		results, err := configureAndRunFioJob(ctx, a, params, job, jobFilePath, s.OutDir())
		if err != nil {
			s.Fatal("Failed to run fio job: ", err)
		}

		for _, result := range results {
			perfValues.Append(perf.Metric{
				Name:      job.name,
				Unit:      "KiBps",
				Direction: perf.BiggerIsBetter,
				Multiple:  true,
			}, result.bw)
		}

		resultSummary := summarizeResults(results)
		testing.ContextLogf(ctx, "Finished %s: %s", job.name, resultSummary)
		resultLogs = append(resultLogs, fmt.Sprintf("Jobname = %s: %s", job.name, resultSummary))
	}

	// Print the results at the end of the test as well.
	for _, resultLog := range resultLogs {
		testing.ContextLog(ctx, resultLog)
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save final perf metrics: ", err)
	}
}

// fioBinNameForArch gets the name of the fio executable to install on the DUT.
func fioBinNameForArch(ctx context.Context, a *arc.ARC) (string, error) {
	out, err := a.GetProp(ctx, "ro.product.cpu.abi")
	if err != nil {
		return "", errors.Wrap(err, "failed to get abi")
	}
	if strings.HasPrefix(string(out), "x86") {
		return fioX86BinaryData, nil
	}
	return fioARMBinaryData, nil
}

// configureAndRunFioJob prepares the fio command line parameters for the given job, runs it for
// the number of times specified by |params.numRepeat|, and returns the result of all runs.
func configureAndRunFioJob(ctx context.Context, a *arc.ARC, params fioTestParams, job fioJob, jobFilePath, outDir string) ([]jobResult, error) {
	cleanupFunc := func(ctx context.Context) error {
		return nil
	}

	fioOpts := []string{
		jobFilePath,
		"--runtime=60", // Run up to 1 minute.
		"--output=" + fioOutputGuest,
		"--output-format=json",
	}
	if job.ioType == "read" {
		fioOpts = append(fioOpts, "--filename="+params.readTestFilePath)
	} else {
		writeTestFilePath := filepath.Join(params.directory, fioWriteTestFileName)

		fioOpts = append(fioOpts, "--filename="+writeTestFilePath)
		fioOpts = append(fioOpts, "--size=1G")

		cleanupFunc = func(ctx context.Context) error {
			// On some devices, the host kernel hangs during sync after a large file
			// created with random write from ARCVM is removed. Apparently this can
			// be mitigated if the large files are overwritten with sequential write
			// before being removed.
			// TODO(b/316307204): Remove this hack after the issue is fixed.
			if strings.HasPrefix(job.name, "rand_write") {
				if err := a.Command(ctx, "dd", "if=/dev/zero", "of="+writeTestFilePath, "bs=4K", "count=262144", "conv=notrunc").Run(testexec.DumpLogOnError); err != nil {
					return errors.Wrap(err, "failed to overwrite the randwrite test file with sequential write")
				}
			}
			return a.RemoveAll(ctx, writeTestFilePath)
		}
	}

	var results = []jobResult{}
	for i := 0; i < params.numRepeat; i++ {
		fioOutputHost := filepath.Join(outDir, fmt.Sprintf("fio-output-%s-%d.json", job.name, i))

		result, err := runFioJobOnce(ctx, a, job, fioOpts, fioOutputHost, cleanupFunc)
		if err != nil {
			return []jobResult{}, err
		}

		results = append(results, result)
	}

	return results, nil
}

// runFioJobOnce performs the preparation for running an fio job, runs the given fio job once,
// returns the parsed result, and performs the cleanup.
func runFioJobOnce(ctx context.Context, a *arc.ARC, job fioJob, fioOpts []string, fioOutputHost string, cleanupFunc func(context.Context) error) (result jobResult, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	if _, err := cpu.WaitUntilStabilized(ctx, cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)); err != nil {
		return jobResult{}, errors.Wrap(err, "failed to wait until CPU is stabilized")
	}

	if err := dropCaches(ctx, a); err != nil {
		return jobResult{}, errors.Wrap(err, "failed to drop caches")
	}

	testing.ContextLog(ctx, "Running fio: "+strings.Join(fioOpts, " "))

	// Make sure to perform the cleanup even when fio failed.
	defer func(ctx context.Context) {
		if err := cleanupFunc(ctx); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to perform cleanup after running fio")
			} else {
				testing.ContextLog(ctx, "Failed to perform cleanup after running fio: ", err)
			}
		}
	}(cleanupCtx)

	if err := a.Command(ctx, fioGuestPath, fioOpts...).Run(testexec.DumpLogOnError); err != nil {
		return jobResult{}, errors.Wrap(err, "failed to run fio")
	}

	if err := a.PullFile(ctx, fioOutputGuest, fioOutputHost); err != nil {
		return jobResult{}, errors.Wrap(err, "failed to pull fio output file")
	}

	result, err := parseResult(fioOutputHost, job.ioType)
	if err != nil {
		return jobResult{}, errors.Wrap(err, "failed to parse the result JSON file")
	}

	return result, nil
}

func dropCaches(ctx context.Context, a *arc.ARC) error {
	// First drop the guest-side caches, then drop the host-side caches.
	testing.ContextLog(ctx, "Dropping guest-side caches")
	if err := a.Command(ctx, "sync").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to sync guest-side")
	}
	if err := testexec.CommandContext(ctx, "adb", "shell", "echo 3 > /proc/sys/vm/drop_caches").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to drop guest-side caches")
	}

	if err := disk.DropCaches(ctx); err != nil {
		return err
	}
	return nil
}

func parseResult(resultFilePath, ioType string) (jobResult, error) {
	// The output json file typically looks like as follows:
	// 	{
	// 	  ...
	// 	  "jobs" : [
	// 	    {
	// 	      "jobname" : "rand_read_buffered",
	// 	      ...
	// 	      "read" : {
	// 	        "io_kbytes" : 149120,
	// 	        "bw" : 38255,
	// 	        ...
	// 	      },
	// 	      "write" : {
	// 	        ...
	// 	      },
	// 	      "trim" : {
	// 	        ...
	// 	      },
	// 	      ...
	// 	    },
	// 	  ],
	// 	  ...
	// 	}

	buf, err := ioutil.ReadFile(resultFilePath)
	if err != nil {
		return jobResult{}, errors.Wrap(err, "failed to read fio result file")
	}

	// Remove log messages from fio (https://github.com/axboe/fio/issues/731)
	fioMsgRegexp := regexp.MustCompile(`\bfio:.*\n`)
	trimmedBuf := fioMsgRegexp.ReplaceAll(buf, []byte{})

	results := make(map[string]interface{})
	if err := json.Unmarshal(trimmedBuf, &results); err != nil {
		return jobResult{}, errors.Wrap(err, "failed to unmarshal fio results")
	}

	var ret jobResult
	for _, i := range results["jobs"].([]interface{}) {
		j := i.(map[string]interface{})
		result := j[ioType].(map[string]interface{})
		ret.bw = result["bw"].(float64)
		ret.ioKBytes = int(result["io_kbytes"].(float64))
	}
	return ret, nil
}

func summarizeResults(results []jobResult) string {
	var bws = []string{}
	bwSum := 0.0
	totalIoBytes := 0

	for _, result := range results {
		bwSum += result.bw
		bws = append(bws, strconv.Itoa(int(result.bw)))
		totalIoBytes += result.ioKBytes
	}

	resultLog := fmt.Sprintf("IoBytes = %d KiB, Bandwidth = %d KiBps",
		totalIoBytes, int(bwSum/float64(len(results))))
	if len(results) > 1 {
		resultLog += fmt.Sprintf(" (avg of [%s])", strings.Join(bws, ","))
	}
	return resultLog
}
