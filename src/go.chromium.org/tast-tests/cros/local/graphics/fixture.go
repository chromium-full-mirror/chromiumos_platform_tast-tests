// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	graphics_common "go.chromium.org/tast-tests/cros/common/graphics"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/debugd"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

var (
	disableFirmwareUpdater = chrome.ExtraArgs("--disable-features=FirmwareUpdaterApp")
	enablePassthrough      = chrome.ExtraArgs("--enable-features=DefaultPassthroughCommandDecoder")
	enableVulkan           = chrome.ExtraArgs("--enable-features=Vulkan")
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "gpuWatchHangsEnrolled",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Desc:            "Check if there any GPU related hangs during a test in an enrolled device",
		Contacts:        []string{"chromeos-gfx@google.com"},
		Impl:            &gpuWatchHangsFixture{},
		SetUpTimeout:    1 * time.Minute,
		TearDownTimeout: 1 * time.Minute,
		PreTestTimeout:  2 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		Parent:          fixture.Enrolled,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "gpuWatchHangs",
		Desc:            "Check if there any GPU related hangs during a test",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Contacts:        []string{"chromeos-gfx@google.com"},
		Impl:            &gpuWatchHangsFixture{},
		SetUpTimeout:    1 * time.Minute,
		TearDownTimeout: 1 * time.Minute,
		PreTestTimeout:  2 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		Parent:          fixture.GpuRemoteWatcher,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "gpuWatchDog",
		Desc:            "Check if there any GPU related problems(hangs+crashes) observed during a test",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Contacts:        []string{"chromeos-gfx@google.com"},
		Parent:          "gpuWatchHangs",
		Impl:            &gpuWatchDogFixture{},
		PreTestTimeout:  5 * time.Second,
		PostTestTimeout: 5 * time.Second,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeGraphics",
		Desc:         "Logged into a user session for graphics testing",
		BugComponent: "b:885255", // ChromeOS > Platform > Graphics
		Contacts:     []string{"chromeos-gfx@google.com"},
		Parent:       "gpuWatchDog",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{disableFirmwareUpdater}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeGraphicsHwOverlays",
		Desc:         "Logged into a user session for graphics testing for HwOverlays test",
		BugComponent: "b:885255", // ChromeOS > Platform > Graphics
		Contacts:     []string{"chromeos-gfx@chromium.org", "syedfaaiz@google.com"},
		Parent:       "gpuWatchDog",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs("--ash-no-nudges"),
			}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeGraphicsHDR",
		Desc:         "Logged into a user session for graphics testing for HDR buffers",
		BugComponent: "b:885255", // ChromeOS > Platform > Graphics
		Contacts:     []string{"chromeos-gfx-compositor@google.com"},
		Parent:       "gpuWatchDog",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.EnableFeatures("UseHDRTransferFunction"),
			}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// TODO(b/233238923): Remove when passthrough is enabled by default.
	testing.AddFixture(&testing.Fixture{
		Name:         "chromeGraphicsPassthrough",
		Desc:         "Logged into a user session for graphics testing with the passthrough command decoder feature enabled",
		BugComponent: "b:885255", // ChromeOS > Platform > Graphics
		Contacts:     []string{"chromeos-gfx@google.com"},
		Parent:       "gpuWatchDog",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{disableFirmwareUpdater, enablePassthrough}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "chromeGraphicsVulkan",
		Desc:         "Logged into a user session for graphics testing with Vulkan raster and composite",
		BugComponent: "b:885255", // ChromeOS > Platform > Graphics
		Contacts:     []string{"chromeos-gfx@google.com"},
		Parent:       "gpuWatchDog",
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				disableFirmwareUpdater,
				enableVulkan,
				chrome.ExtraArgs("--ash-no-nudges"),
			}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "graphicsNoChrome",
		Desc:            "Stop UI before tests, start UI after",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Contacts:        []string{"chromeos-gfx@google.com"},
		Impl:            &graphicsNoChromeFixture{},
		Parent:          "gpuWatchHangs",
		SetUpTimeout:    upstart.UIRestartTimeout,
		TearDownTimeout: upstart.UIRestartTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "chromeGraphicsIgt",
		Desc:            "Stop and later restart services for IGT",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Contacts:        []string{"markyacoub@google.com", "chromeos-gfx-display@google.com"},
		Parent:          "graphicsNoChrome",
		Impl:            &graphicsIgtFixture{},
		SetUpTimeout:    upstart.UIRestartTimeout,
		TearDownTimeout: upstart.UIRestartTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "chromeGraphicsIdle",
		Desc:            "Logged into a user session for graphics Idle testing. This fixture starts a chrome dedicated for graphics.Idle tests",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Contacts:        []string{"chromeos-gfx@google.com"},
		Parent:          "gpuWatchDog",
		Impl:            &graphicsIdleFixture{fOpt: []chrome.Option{disableFirmwareUpdater}},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "chromeGraphicsIdleArc",
		Desc:            "Logged into a user session for graphics Idle testing. This fixture starts an arc enabled chrome dedicated for graphics.Idle.*arc tests",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Contacts:        []string{"chromeos-gfx@google.com"},
		Parent:          "gpuWatchDog",
		Impl:            &graphicsIdleFixture{fOpt: []chrome.Option{chrome.ARCEnabled(), disableFirmwareUpdater}},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "chromeGraphicsWebContent",
		Desc:            "Logs into Chrome and launches a browser window with specific content, like playing WebGL Aquarium for instance",
		BugComponent:    "b:885255", // ChromeOS > Platform > Graphics
		Contacts:        []string{"chromeos-gfx@google.com"},
		Parent:          "gpuWatchDog",
		Impl:            &graphicsWebContentFixture{fOpt: []chrome.Option{chrome.ARCEnabled(), disableFirmwareUpdater}},
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Data:            []string{"webgl_aquarium_static_20221212.tar.zst"},
		Params: []testing.FixtureParam{{
			Name: "webglaquarium",
			Val: graphicsWebContentVal{
				data:       "webgl_aquarium_static_20221212.tar.zst",
				serverName: "webgl_aquarium_static",
				url:        "aquarium.html",
			},
		}},
	})
}

// graphicsWebContentFixture starts Chrome, opens specific web content in the background.
type graphicsWebContentFixture struct {
	cr   *chrome.Chrome
	fOpt []chrome.Option // Function to generate Chrome Options

	server  *httptest.Server // Server instance to serve the local data file.
	conn    *chrome.Conn     // chrome connection to the opened web content.
	tempDir string
}

type graphicsWebContentVal struct {
	data       string // data is the zst file that contains the content.
	serverName string // serverName is the uncompressed folder name that we should start httpServer on.
	url        string // url is the url to access the main content.
}

func (f *graphicsWebContentFixture) Reset(ctx context.Context) error {
	if crErr := f.cr.Responded(ctx); crErr != nil {
		testing.ContextLog(ctx, "Reconnect to browser connection")
		if err := f.cr.Reconnect(ctx); err != nil {
			return errors.Wrap(err, "failed to reconnect to Chrome")
		}
		conn, err := f.cr.NewConnForTarget(ctx, func(t *chrome.Target) bool {
			return strings.HasSuffix(t.URL, "aquarium.html")
		})
		if err != nil {
			return errors.Wrap(err, "failed to establish browser connection")
		}
		f.conn = conn
	}
	if err := f.conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "page failed to load")
	}
	return nil
}

func (f *graphicsWebContentFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsWebContentFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsWebContentFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx, f.fOpt...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	chrome.Lock()
	f.cr = cr

	val := s.Param().(graphicsWebContentVal)
	if val.data != "" {
		tempDir, err := os.MkdirTemp("/tmp", "")
		if err != nil {
			s.Fatal("Failed to created temp dir: ", err)
		}
		f.tempDir = tempDir
		// Assuming it is zst compression.
		srcFile := s.DataPath(val.data)
		if err := testexec.CommandContext(ctx, "tar", "-xf", srcFile, "-C", f.tempDir).Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("Failed to extract %s: %s", val.data, err)
		}
		server := httptest.NewServer(http.FileServer(http.Dir(f.tempDir + "/" + val.serverName)))
		f.server = server
		s.Logf("Extracted %s", val.data)
	} else {
		s.Fatal("Directly opening an url is not supported yet")
	}

	url := filepath.Join(f.server.URL, val.url)
	conn, err := f.cr.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	f.conn = conn

	if err = f.conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Page failed to load: ", err)
	}
	return cr
}

func (f *graphicsWebContentFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	os.RemoveAll(f.tempDir)
	f.tempDir = ""

	f.server.Close()
	f.server = nil

	f.conn.Close()
	f.conn = nil

	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}

type graphicsNoChromeFixture struct {
}

func (f *graphicsNoChromeFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *graphicsNoChromeFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsNoChromeFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsNoChromeFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	s.Log("Setup: Stop Chrome UI")
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui job: ", err)
	}
	return nil
}

func (f *graphicsNoChromeFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	s.Log("Setup: Start Chrome UI")
	upstart.EnsureJobRunning(ctx, "ui")
}

type gpuWatchHangsFixture struct {
	postFunc     []func(ctx context.Context) error
	tearDownFunc []func(ctx context.Context) error
}

func modifyHangCheckTimer(ctx context.Context) (func(context.Context) error, error) {
	hangCheckTimer, err := GetHangCheckTimer(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get hangcheck timer")
	}
	testing.ContextLog(ctx, "Hangcheck timer: ", hangCheckTimer)
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Set hangcheck timer back to ", hangCheckTimer)
		if err := SetHangCheckTimer(ctx, hangCheckTimer); err != nil {
			return errors.Wrapf(err, "failed to set hangcheck timer back to %v", hangCheckTimer)
		}
		return nil
	}, nil
}

func modifyDrmTraceVerbosity(ctx context.Context) (func(context.Context) error, error) {
	d, err := debugd.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to debugd service")
	}
	const testCategories debugd.DRMTraceCategories = debugd.DRMTraceCategoryCore | debugd.DRMTraceCategoryKMS
	if err := d.DRMTraceSetCategories(ctx, testCategories); err != nil {
		return nil, errors.Wrap(err, "failed to set to default categories")
	}
	// Set for debug.
	if err := d.DRMTraceSetSize(ctx, debugd.DRMTraceSizeDebug); err != nil {
		return nil, errors.Wrap(err, "failed to set to default size")
	}

	return func(ctx context.Context) error {
		d, err := debugd.New(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to debugd service")
		}
		if err := d.DRMTraceSetCategories(ctx, 0); err != nil {
			return errors.Wrap(err, "failed to set to default categories")
		}
		if err := d.DRMTraceSetSize(ctx, debugd.DRMTraceSizeDefault); err != nil {
			return errors.Wrap(err, "failed to set to default size")
		}
		return nil
	}, nil
}

func (f *gpuWatchHangsFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	hangCheckCleanup, err := modifyHangCheckTimer(ctx)
	if err != nil {
		s.Log("Failed to set hangcheck timer, which is normal for kernels that doesn't support hangcheck timer configuration: ", err)
	} else {
		f.tearDownFunc = append(f.tearDownFunc, hangCheckCleanup)
	}

	drmTraceCleanup, err := modifyDrmTraceVerbosity(ctx)
	if err != nil {
		s.Log("Failed to set verbose logging in drm-trace")
	} else {
		f.tearDownFunc = append(f.tearDownFunc, drmTraceCleanup)
	}
	return nil
}

func (f *gpuWatchHangsFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	for i := len(f.tearDownFunc) - 1; i >= 0; i-- {
		if err := f.tearDownFunc[i](ctx); err != nil {
			s.Error("TearDown failed: ", err)
		}
	}
}

func (f *gpuWatchHangsFixture) Reset(ctx context.Context) error {
	return nil
}

func parseDrmTrace(ctx context.Context, startMsg, stopMsg string) (string, error) {
	matches, err := filepath.Glob("/var/log/display_debug/drm_trace_verbose.*")
	if err != nil {
		return "", errors.Wrap(err, "failed to glob drm_trace snapshot directory")
	} else if len(matches) != 1 {
		return "", errors.Errorf("unexpected number of snapshot found: got %v, want 1", len(matches))
	}
	bytes, err := os.ReadFile(matches[0])
	if err != nil {
		return "", errors.Wrap(err, "failed to read snapshot")
	}
	const replacementCharacter = '\uFFFD'
	lines := strings.Split(strings.TrimSpace(strings.ToValidUTF8(string(bytes), string(replacementCharacter))), "\n")
	startIdx := -1
	stopIdx := -1
	for idx, line := range lines {
		if strings.Contains(line, startMsg) {
			startIdx = idx
		}
		if strings.Contains(line, stopMsg) {
			stopIdx = idx
		}
	}
	if startIdx < 0 || stopIdx < 0 || stopIdx < startIdx {
		testing.ContextLog(ctx, "Failed to identify the start/stop test marker in drm-trace logs, outputing the entire logs instead")
		return strings.Join(lines, "\n"), nil
	}
	return strings.Join(lines[startIdx:stopIdx+1], "\n"), nil
}

func drmTracePreTest(ctx context.Context, testName string) (func(context.Context) error, error) {
	d, err := debugd.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to debugd service")
	}
	startTracingMsg := fmt.Sprintf("Test %v start tracing", testName)
	stopTracingMsg := fmt.Sprintf("Test %v stop tracing", testName)
	if err := d.DRMTraceAnnotateLog(ctx, startTracingMsg); err != nil {
		return nil, errors.Wrap(err, "failed to annotate the start of tracing")
	}
	return func(ctx context.Context) error {
		d, err := debugd.New(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to debugd service")
		}
		if err := d.DRMTraceAnnotateLog(ctx, stopTracingMsg); err != nil {
			return errors.Wrap(err, "failed to annotate the end of tracing")
		}
		// Remove existing snapshots.
		snapshotDir := "/var/log/display_debug/*"
		files, err := filepath.Glob(snapshotDir)
		if err != nil {
			return errors.Wrap(err, "failed to glob drm_trace snapshot directory")
		}
		for _, file := range files {
			if err = os.RemoveAll(file); err != nil {
				return errors.Wrapf(err, "failed to remove %v", file)
			}
		}
		if err := d.DRMTraceSnapshot(ctx, debugd.DRMTraceSnapshotTypeTrace); err != nil {
			return errors.Wrap(err, "failed to dump DRMTraceSnapshot")
		}
		log, err := parseDrmTrace(ctx, startTracingMsg, stopTracingMsg)
		if err != nil {
			return errors.Wrap(err, "failed to parse drm-trace")
		}
		out, ok := testing.ContextOutDir(ctx)
		if !ok {
			return errors.New("failed to get test output directory")
		}
		if err := os.WriteFile(filepath.Join(out, "drm_trace"), []byte(log), 0644); err != nil {
			return errors.Wrap(err, "failed to write drm-trace to tast output folder")
		}
		return nil
	}, nil
}

func (f *gpuWatchHangsFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.postFunc = nil
	// Attempt flushing system logs every second instead of every 10 minutes.
	dirtyWritebackDuration, err := GetDirtyWritebackDuration()
	if err != nil {
		s.Log("Failed to get initial dirty writeback duration: ", err)
	} else {
		// Performing a full sync makes it less likely that there are pending writes that might defer logging from being written immediately later.
		if err := testexec.CommandContext(ctx, "sync").Run(testexec.DumpLogOnError); err != nil {
			s.Log("Failed to sync the file system, proceed without failing the tests: ", err)
		}
		SetDirtyWritebackDuration(ctx, 1*time.Second)
		// Set dirty writeback duration to initial value even if we fails to set to 1 second.
		f.postFunc = append(f.postFunc, func(ctx context.Context) error {
			return SetDirtyWritebackDuration(ctx, dirtyWritebackDuration)
		})
	}

	// syslog.NewReader reports syslog message written after it is started for GPU hang detection.
	sysLogReader, err := syslog.NewReader(ctx, syslog.Severities(syslog.Info, syslog.Warning, syslog.Err))
	if err != nil {
		s.Log("Failed to get syslog reader: ", err)
	} else {
		f.postFunc = append(f.postFunc, func(ctx context.Context) error {
			defer sysLogReader.Close()
			if err := CheckSysLog(ctx, s.TestName(), sysLogReader); err != nil {
				if sErr := DumpGraphicsDebugFiles(ctx, s.OutDir()); sErr != nil {
					return errors.Wrap(err, sErr.Error())
				}
				return err
			}
			return nil
		})
	}

	drmTracePostTest, err := drmTracePreTest(ctx, s.TestName())
	if err != nil {
		s.Log("Failed to setup drm_trace settings, it is normal for kernel < 5.4: ", err)
	} else {
		f.postFunc = append(f.postFunc, drmTracePostTest)
	}
}

func (f *gpuWatchHangsFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	var postErr error
	const ignoredSyslogError = "ieee80211_rx_list"
	for i := len(f.postFunc) - 1; i >= 0; i-- {
		if err := f.postFunc[i](ctx); err != nil {
			if strings.Contains(err.Error(), ignoredSyslogError) {
				s.Log("Ignoring syslog error: ", err)
				continue
			}
			postErr = errors.Wrap(postErr, err.Error())
		}
	}
	if postErr != nil {
		s.Error("PostTest failed: ", postErr)
		// If postTest fails, it often means there's a hang and following tests are likely to suffer from a possibly failed recovery.
		// Mark the DUT for reboot by the remote server instance.
		os.Create(graphics_common.GraphicsRemoteWatcherRebootFile)
	}
}

type gpuWatchDogFixture struct {
	postFunc []func(ctx context.Context) error
}

func (f *gpuWatchDogFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	return nil
}

func (f *gpuWatchDogFixture) TearDown(ctx context.Context, s *testing.FixtState) {}

func (f *gpuWatchDogFixture) Reset(ctx context.Context) error {
	return nil
}

// isChromeGPUCrash returns true iff filename corresponds to a Chrome GPU crash dump.
func isChromeGPUCrash(filename string) (bool, error) {
	if !strings.HasSuffix(filename, crash.MinidumpExt) {
		// We only care about minidump crash files.
		return false, nil
	}
	// Inspect the corresponding crash metadata file.
	metaFilename := strings.TrimSuffix(filename, crash.MinidumpExt) + crash.MetadataExt
	f, err := os.Open(metaFilename)
	if err != nil {
		return false, errors.Wrapf(err, "could not open %v", metaFilename)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if sc.Text() == "upload_var_ptype=gpu-process" {
			return true, nil
		}
	}
	if err := sc.Err(); err != nil {
		return false, errors.Wrapf(err, "could not read %v", metaFilename)
	}
	return false, nil
}

// getGPUCrash returns gpu related crash files found in system.
func (f *gpuWatchDogFixture) getGPUCrash(ctx context.Context) ([]string, error) {
	crashFiles, err := crash.GetCrashes(crash.DefaultDirs()...)
	if err != nil {
		return nil, err
	}

	isGPUStateEmpty := func(filename string) bool {
		output, err := testexec.CommandContext(ctx, "sh", "-c", "xz --robot --list "+filename).Output()
		if err != nil {
			// Failed to determine if it is empty, assume it is not.
			return false
		}
		// Example output
		// name    chrome.20240304.101635.28891.4909.i915_error_state.log.xz
		// file    1       0       32      0       ---     CRC64   0
		// totals  1       0       32      0       ---     CRC64   0       1
		re := regexp.MustCompile(`totals\s+\d+\s+\d+\s+\d+\s+(\d+)`)
		for _, line := range strings.Split(string(output), "\n") {
			if match := re.FindStringSubmatch(line); match != nil && match[1] == "0" {
				return true
			}
		}
		return false
	}

	// Filter the gpu related crash.
	var crashes []string
	for _, file := range crashFiles {
		isGPUCrash := false
		// Sometimes we see crash reporter generated an empty i915_error_state file as part of chrome crash. Ignore it if it is empty.
		if strings.HasSuffix(file, crash.GPUStateExt) && !isGPUStateEmpty(file) {
			isGPUCrash = true
		} else if isGPUCrash, err = isChromeGPUCrash(file); err != nil {
			return nil, err
		}

		if isGPUCrash {
			crashes = append(crashes, file)
		}
	}
	return crashes, nil
}

// checkNewCrashes checks the difference between the oldCrashes and the current crashes. It will try to save the new crash to outDir and return error if fails to retrieve current crashes or the list is mismatched.
func (f *gpuWatchDogFixture) checkNewCrashes(ctx context.Context, oldCrashes []string, outDir string) error {
	crashes, err := f.getGPUCrash(ctx)
	if err != nil {
		return err
	}

	// Check if there're new crash files got generated during the test.
	var newCrashes []string
	for _, crash := range crashes {
		found := false
		for _, preTestCrash := range oldCrashes {
			if preTestCrash == crash {
				found = true
				break
			}
		}
		if !found {
			newCrashes = append(newCrashes, crash)
		}
	}

	if len(newCrashes) > 0 {
		sort.Strings(newCrashes)
		resultErr := errors.Errorf("found gpu crash file: %v", newCrashes)
		for _, crash := range newCrashes {
			destPath := filepath.Join(outDir, filepath.Base(crash))
			if err := fsutil.CopyFile(crash, destPath); err != nil {
				resultErr = errors.Wrapf(resultErr, "failed to copy crash file %v: %v", crash, err.Error())
			}
		}
		return resultErr
	}
	return nil
}

func (f *gpuWatchDogFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.postFunc = nil
	// Record PreTest crashes.
	crashes, err := f.getGPUCrash(ctx)
	if err != nil {
		s.Log("Failed to get gpu crashes: ", err)
	} else {
		f.postFunc = append(f.postFunc, func(ctx context.Context) error {
			return f.checkNewCrashes(ctx, crashes, s.OutDir())
		})
	}
}

func (f *gpuWatchDogFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	var postErr error
	for i := len(f.postFunc) - 1; i >= 0; i-- {
		if err := f.postFunc[i](ctx); err != nil {
			postErr = errors.Wrap(postErr, err.Error())
		}
	}
	if postErr != nil {
		s.Error("PostTest failed: ", postErr)
	}
}

type graphicsIgtFixture struct {
}

func (f *graphicsIgtFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *graphicsIgtFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsIgtFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsIgtFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Make sure we start in VT1, otherwise tests will fail
	if err := OpenVT1(ctx); err != nil {
		s.Fatal("Failed to open VT1: ", err)
	}

	// Tests such as kms_flip requires Suspend and Wake-up which are achieved using the RTC wake-up alarm.
	// tlsdated is holding /dev/rtc so IGT fails to take the lock and set a wake up alarm. Hence, it
	// is required to stop the tlsdated before running the IGT test.
	s.Log("SetUp: Stop tlsdated")
	if err := upstart.StopJob(ctx, "tlsdated"); err != nil {
		s.Fatal("Failed to stop tlsdated job: ", err)
	}

	return nil
}

func (f *graphicsIgtFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	s.Log("TearDown: Start tlsdated")
	upstart.EnsureJobRunning(ctx, "tlsdated")
}

// graphicsIdleFixture starts chrome, waits till the machine is cooled down then proceed.
type graphicsIdleFixture struct {
	cr   *chrome.Chrome
	fOpt []chrome.Option // Function to generate Chrome Options
}

func (f *graphicsIdleFixture) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	return nil
}

func (f *graphicsIdleFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsIdleFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *graphicsIdleFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx, f.fOpt...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	if _, err := cpu.WaitUntilStabilized(ctx, cpu.CoolDownConfig{
		PollTimeout:              2 * time.Minute,
		PollInterval:             2 * time.Second,
		TemperatureThresholdMode: cpu.TemperatureThresholdPerModel,
		TemperatureThreshold:     55000,
		CoolDownMode:             cpu.CoolDownPreserveUI,
	}); err != nil {
		s.Log("Failed to get stable CPU before running tests: ", err)
	}
	chrome.Lock()
	f.cr = cr
	return cr
}

func (f *graphicsIdleFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}
