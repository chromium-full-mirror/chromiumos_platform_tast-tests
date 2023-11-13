// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/firmware/usb"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast-tests/cros/local/firmware"
	"go.chromium.org/tast-tests/cros/local/input"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			fwpb.RegisterUtilsServiceServer(srv, &UtilsService{
				s:            s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
	})
}

// UtilsService implements tast.cros.firmware.UtilsService.
type UtilsService struct {
	s            *testing.ServiceState
	cr           *chrome.Chrome
	sharedObject *common.SharedObjectsForService
}

// BlockingSync syncs the root device and internal device.
func (*UtilsService) BlockingSync(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	// The double calls to sync fakes a blocking call
	// since the first call returns before the flush is complete,
	// but the second will wait for the first to finish.
	for i := 0; i < 2; i++ {
		if err := testexec.CommandContext(ctx, "sync").Run(testexec.DumpLogOnError); err != nil {
			return nil, errors.Wrapf(err, "sending sync command #%d to DUT", i+1)
		}
	}

	// Find the root device.
	rootDevice, err := firmware.RootDevice(ctx)
	if err != nil {
		return nil, err
	}
	devices := []string{rootDevice}

	// If booted from removable media, sync the internal device too.
	isRemovable, err := firmware.BootDeviceRemovable(ctx)
	if err != nil {
		return nil, err
	}
	if isRemovable {
		internalDevice, err := firmware.InternalDevice(ctx)
		if err != nil {
			return nil, err
		}
		devices = append(devices, internalDevice)
	}

	// sync only sends SYNCHRONIZE_CACHE but doesn't check the status.
	// This function will perform a device-specific sync command.
	for _, device := range devices {
		if strings.Contains(device, "mmcblk") {
			// For mmc devices, use `mmc status get` command to send an
			// empty command to wait for the disk to be available again.
			if err := testexec.CommandContext(ctx, "mmc", "status", "get", device).Run(testexec.DumpLogOnError); err != nil {
				return nil, errors.Wrapf(err, "sending mmc command to device %s", device)
			}
		} else if strings.Contains(device, "nvme") {
			// For NVMe devices, use `nvme flush` command to commit data
			// and metadata to non-volatile media.
			// Get a list of NVMe namespaces, and flush them individually.
			// The output is assumed to be in the following format:
			// [ 0]:0x1
			// [ 1]:0x2
			namespaces, err := testexec.CommandContext(ctx, "nvme", "list-ns", device).Output(testexec.DumpLogOnError)
			if err != nil {
				return nil, errors.Wrapf(err, "listing namespaces for device %s", device)
			}
			if len(namespaces) == 0 {
				return nil, errors.Errorf("Listing namespaces for device %s returned no output", device)
			}
			for _, namespace := range strings.Split(strings.TrimSpace(string(namespaces)), "\n") {
				ns := strings.Split(namespace, ":")[1]
				if err := testexec.CommandContext(ctx, "nvme", "flush", device, "-n", ns).Run(testexec.DumpLogOnError); err != nil {
					return nil, errors.Wrapf(err, "flushing namespace %s on device %s", ns, device)
				}
			}
		} else {
			// For other devices, hdparm sends TUR to check if
			// a device is ready for transfer operation.
			if err := testexec.CommandContext(ctx, "hdparm", "-f", device).Run(testexec.DumpLogOnError); err != nil {
				return nil, errors.Wrapf(err, "sending hdparm command to device %s", device)
			}
		}
	}
	return &empty.Empty{}, nil
}

func (us *UtilsService) FindPhysicalKeyboard(ctx context.Context, req *empty.Empty) (*fwpb.InputDevicePath, error) {
	foundKB, path, err := input.FindPhysicalKeyboard(ctx)
	if err != nil {
		return nil, err
	} else if !foundKB {
		return nil, errors.New("no physical keyboard found")
	} else {
		return &fwpb.InputDevicePath{Path: path}, nil
	}
}

func (us *UtilsService) FindPowerKeyDevice(ctx context.Context, req *empty.Empty) (*fwpb.InputDevicePath, error) {
	foundPowerKey, path, err := input.FindPowerKeyDevice(ctx)
	if err != nil {
		return nil, err
	} else if !foundPowerKey {
		return nil, errors.New("no input device for power key found")
	} else {
		return &fwpb.InputDevicePath{Path: path}, nil
	}
}

// NewChrome starts a new Chrome session and logs in as a test user.
func (us *UtilsService) NewChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if us.cr != nil {
		return nil, errors.New("Chrome already available")
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		return nil, err
	}
	us.cr = cr
	return &empty.Empty{}, nil
}

// CloseChrome closes a Chrome session and cleans up the resources obtained by NewChrome.
func (us *UtilsService) CloseChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if us.cr == nil {
		return nil, errors.New("Chrome not available")
	}
	err := us.cr.Close(ctx)
	us.cr = nil
	return &empty.Empty{}, err
}

// ReuseChrome reuses the existing Chrome session if there's already one.
func (us *UtilsService) ReuseChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if us.cr != nil {
		testing.ContextLog(ctx, "Chrome already available")
		return &empty.Empty{}, nil
	}

	// First, look up the shared Chrome instance set by CheckVirtualKeyboarService (or other services).
	// Otherwise, reuse the one created by NewChrome in this service with the same options.
	us.sharedObject.ChromeMutex.Lock()
	defer us.sharedObject.ChromeMutex.Unlock()
	if us.sharedObject.Chrome != nil {
		us.cr = us.sharedObject.Chrome
	} else {
		cr, err := chrome.New(ctx, chrome.TryReuseSession())
		if err != nil {
			return nil, err
		}
		us.cr = cr
	}
	return &empty.Empty{}, nil
}

// EvalTabletMode evaluates tablet mode status.
func (us *UtilsService) EvalTabletMode(ctx context.Context, req *empty.Empty) (*fwpb.EvalTabletModeResponse, error) {
	if us.cr == nil {
		return nil, errors.New("Chrome not available")
	}
	tconn, err := us.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating test API connection failed")
	}
	// Check if tablet mode is enabled on DUT.
	tabletModeEnabled, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tablet mode enabled status")
	}
	return &fwpb.EvalTabletModeResponse{TabletModeEnabled: tabletModeEnabled}, nil
}

// FindSingleNode finds the specific UI node based on the passed in element.
func (us *UtilsService) FindSingleNode(ctx context.Context, req *fwpb.NodeElement) (*empty.Empty, error) {
	if us.cr == nil {
		return nil, errors.New("missing chrome instance")
	}

	tconn, err := us.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	uiauto := uiauto.New(tconn)
	uiNode := nodewith.Name(req.Name).First()

	if err := uiauto.WithTimeout(10 * time.Second).WaitUntilExists(uiNode)(ctx); err != nil {
		if err := saveLogsOnError(ctx, us, func() bool { return true }); err != nil {
			return nil, errors.Wrapf(err, "could not save logs when node %q not found", req.Name)
		}
		return nil, errors.Wrapf(err, "could not find node: %s", req.Name)
	}

	return &empty.Empty{}, nil
}

func saveLogsOnError(ctx context.Context, us *UtilsService, hasError func() bool) error {
	tconn, err := us.cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("could not get the output directory")
	}
	faillog.DumpUITreeOnError(ctx, filepath.Join(outDir, "BaseECUpdate"), hasError, tconn)
	faillog.SaveScreenshotOnError(ctx, us.cr, filepath.Join(outDir, "BaseECUpdate"), hasError)
	return nil
}

// GetDetachableBaseValue retrieves the values of a few detachable-base attributes,
// such as product-id, usb-path, and vendor-id. The values are saved and returned
// in a list.
func (us *UtilsService) GetDetachableBaseValue(ctx context.Context, req *empty.Empty) (*fwpb.CrosConfigResponse, error) {
	paramsSlice := []string{"product-id", "vendor-id", "usb-path"}

	crosCfgRes := fwpb.CrosConfigResponse{}

	for _, v := range paramsSlice {
		value, err := crosconfig.Get(ctx, "/detachable-base", v)
		if err != nil {
			return nil, err
		}
		if value == "" {
			return nil, errors.Errorf("%s is empty", v)
		}
		switch v {
		case "product-id":
			crosCfgRes.ProductId = value
		case "vendor-id":
			crosCfgRes.VendorId = value
		case "usb-path":
			crosCfgRes.UsbPath = value
		}
	}
	return &crosCfgRes, nil
}

// PerformSpeedometerTest runs the speedometer test on one external website, and returns the result value.
func (us *UtilsService) PerformSpeedometerTest(ctx context.Context, req *empty.Empty) (*fwpb.SpeedometerResponse, error) {
	// Verify we have logged in.
	if us.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	// Open the Speedometer website.
	conn, err := us.cr.NewConn(ctx, "https://browserbench.org/Speedometer2.0/")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open Speedometer website")
	}
	defer conn.Close()

	// Connect to Test API to use it with the UI library.
	tconn, err := us.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	// Find and click on the 'Start Test' button.
	uia := uiauto.New(tconn)
	startButton := nodewith.Name("Start Test").Role(role.Button).Onscreen()
	if err := uiauto.Combine("Click Start Test",
		uia.WaitUntilExists(startButton),
		uia.LeftClick(startButton),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find and click the start button")
	}

	// Wait for the result to appear.
	title := nodewith.Name("Runs / Minute").Role(role.Heading).Onscreen()
	if err := uia.WithTimeout(10 * time.Minute).WaitUntilExists(title)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find the title for speedometer test result value")
	}

	// Get the result from speedometer test.
	result := nodewith.NameRegex(regexp.MustCompile(`^[0-9]+[.]?[0-9]*$`)).Role(role.InlineTextBox).First()
	resultInfo, err := uia.Info(ctx, result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the result value")
	}

	return &fwpb.SpeedometerResponse{Result: resultInfo.Name}, err
}

// CheckCrosConfigProperty runs 'cros_config' and checks the value for a given
// hardware property.
func (us *UtilsService) CheckCrosConfigProperty(ctx context.Context, req *fwpb.CheckCrosConfigRequest) (*fwpb.CheckCrosConfigResponse, error) {
	out, err := crosconfig.Get(ctx, req.CrosConfigPath, req.CrosConfigProperty)
	if err != nil && !crosconfig.IsNotFound(err) {
		return nil, err
	}
	return &fwpb.CheckCrosConfigResponse{CrosConfigPropertyValue: out}, nil
}

type devserverStatus struct {
	url string
	err error
}

type localRunner struct{}

func (lr *localRunner) RunCommand(ctx context.Context, asRoot bool, name string, args ...string) error {
	return testexec.CommandContext(ctx, name, args...).Run(testexec.DumpLogOnError)
}

func (lr *localRunner) RunCommandQuiet(ctx context.Context, asRoot bool, name string, args ...string) error {
	return testexec.CommandContext(ctx, name, args...).Run()
}

func (lr *localRunner) OutputCommand(ctx context.Context, asRoot bool, name string, args ...string) ([]byte, error) {
	return testexec.CommandContext(ctx, name, args...).Output(testexec.DumpLogOnError)
}

// FlashUSBDrive flashes a test image on a usb drive.
// - Find the USB device in /dev/sd*, make sure it is removable by reading /sys/block/sdX/removable
// - Find a healthy devserver
// - Stage the OS image
// - Extract the OS image and write it to /dev/sdX
// - sync /dev/sdX
// - blockdev --rereadpt /dev/sdX
func (us *UtilsService) FlashUSBDrive(ctx context.Context, req *fwpb.FlashUSBDriveRequest) (*empty.Empty, error) {
	artifactsURL := strings.TrimSuffix(req.GetGsPath(), "/")

	files, err := os.ReadDir("/dev")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read /dev")
	}

	usbDevice := ""
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "sd") && len(file.Name()) == 3 {
			testing.ContextLogf(ctx, "Found usb drive: /dev/%s", file.Name())
			removeable := fmt.Sprintf("/sys/block/%s/removable", file.Name())
			f, err := os.Open(removeable)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to open %s", removeable)
			}
			buf := make([]byte, 1)
			c, err := f.Read(buf)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to read %s", removeable)
			}
			if c == 1 && buf[0] == '1' {
				usbDevice = fmt.Sprintf("/dev/%s", file.Name())
				break
			}
		}
	}
	usbRelease, _, err := usb.ValidateUSBImage(ctx, usbDevice, "/media/usbkey", &localRunner{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to verify %s", usbDevice)
	}
	if usbRelease != "" && strings.HasSuffix(artifactsURL, usbRelease) {
		testing.ContextLogf(ctx, "USB drive version is already correct %s", usbRelease)
		return &empty.Empty{}, nil
	}

	ch := make(chan devserverStatus, len(req.GetDevserver()))
	cl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			Proxy:               http.ProxyFromEnvironment,
		},
	}

	for _, dsURL := range req.GetDevserver() {
		go func(dsURL string) {
			req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/check_health", dsURL), nil)
			if err != nil {
				ch <- devserverStatus{dsURL, err}
				return
			}
			res, err := cl.Do(req)
			if err != nil {
				ch <- devserverStatus{dsURL, err}
				return
			}
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				ch <- devserverStatus{dsURL, nil}
			}
		}(dsURL)
	}
	for range req.GetDevserver() {
		s := <-ch
		if s.err != nil {
			testing.ContextLogf(ctx, "Devserver %s not healthy:%v", s.url, s.err)
			continue
		}
		stagingURL := fmt.Sprintf("%s/stage?archive_url=%s&files=chromiumos_test_image.tar.xz", s.url, artifactsURL)
		testing.ContextLogf(ctx, "Staging image %q", stagingURL)
		req, err := http.NewRequestWithContext(ctx, "GET", stagingURL, nil)
		if err != nil {
			testing.ContextLogf(ctx, "Failed to stage file at %q: %v", stagingURL, err)
			continue
		}
		res, err := cl.Do(req)
		if err != nil {
			testing.ContextLogf(ctx, "Failed to stage file at %q: %v", stagingURL, err)
			continue
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			testing.ContextLogf(ctx, "Failed to stage file at %q: %v", stagingURL, res.Status)
			continue
		}
		testImageURL := fmt.Sprintf("%s/extract/%s/chromiumos_test_image.tar.xz?file=chromiumos_test_image.bin", s.url, strings.TrimPrefix(artifactsURL, "gs://"))
		if err := func() (retErr error) {
			defer func() {
				testing.ContextLogf(ctx, "Syncing %s", usbDevice)
				if err := testexec.CommandContext(ctx, "sync", usbDevice).Run(testexec.DumpLogOnError); err != nil {
					retErr = errors.Join(retErr, err)
				}
				if err := testexec.CommandContext(ctx, "blockdev", "--rereadpt", usbDevice).Run(testexec.DumpLogOnError); err != nil {
					retErr = errors.Join(retErr, err)
				}
			}()
			testing.ContextLogf(ctx, "Flashing test OS image to USB from %q", testImageURL)
			req, err = http.NewRequestWithContext(ctx, "GET", testImageURL, nil)
			if err != nil {
				return err
			}
			res, err = cl.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				return err
			}
			outF, err := os.OpenFile(usbDevice, os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			defer outF.Close()

			bytes, err := io.Copy(outF, res.Body)
			if err != nil {
				return err
			}
			if res.ContentLength >= 0 && bytes != res.ContentLength {
				return errors.Errorf("failed to write all data, got %d, want %d", bytes, res.ContentLength)
			}
			return nil
		}(); err != nil {
			return nil, errors.Wrapf(err, "failed to download %q", testImageURL)
		}

		// ensure that image was successfully flashed by reading back OS version
		usbRelease, _, err := usb.ValidateUSBImage(ctx, usbDevice, "/media/usbkey", &localRunner{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to verify %s", usbDevice)
		}
		if usbRelease != "" && strings.HasSuffix(artifactsURL, usbRelease) {
			testing.ContextLogf(ctx, "Successfully flashed %q from %q", usbDevice, testImageURL)
			return &empty.Empty{}, nil
		}
		return nil, errors.Errorf("wrong version on %s after flashing, got %s, want %s", usbDevice, usbRelease, artifactsURL)
	}
	return &empty.Empty{}, errors.New("no healthy devservers")
}
