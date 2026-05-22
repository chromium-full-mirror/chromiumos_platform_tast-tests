// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"

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

// FindPhysicalKeyboard finds the physical keyboard path.
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

// FindPowerKeyDevice finds the power key device.
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

	// First, look up the shared Chrome instance set by CheckVirtualKeyboardService (or other services).
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
	paramsSlice := []string{"product-id", "vendor-id", "usb-path", "i2c-path"}

	crosCfgRes := fwpb.CrosConfigResponse{}

	for _, v := range paramsSlice {
		value, err := crosconfig.Get(ctx, "/detachable-base", v)
		if err != nil {
			value = ""
		}
		switch v {
		case "product-id":
			crosCfgRes.ProductId = value
		case "vendor-id":
			crosCfgRes.VendorId = value
		case "usb-path":
			crosCfgRes.UsbPath = value
		case "i2c-path":
			crosCfgRes.I2CPath = value
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

func findDevServer(ctx context.Context, devservers []string) (string, error) {
	ch := make(chan devserverStatus, len(devservers))
	cl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			Proxy:               http.ProxyFromEnvironment,
		},
	}

	for _, dsURL := range devservers {
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
	for range devservers {
		s := <-ch
		if s.err != nil {
			testing.ContextLogf(ctx, "Devserver %s not healthy:%v", s.url, s.err)
			continue
		}
		return s.url, nil
	}
	return "", errors.New("no healthy devservers")
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

	devserverURL, err := findDevServer(ctx, req.GetDevserver())
	if err != nil {
		return nil, errors.Wrap(err, "failed to find devserver")
	}

	cl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			Proxy:               http.ProxyFromEnvironment,
		},
	}
	stagingURL := fmt.Sprintf("%s/stage?archive_url=%s&files=chromiumos_test_image.tar.xz", devserverURL, artifactsURL)
	testing.ContextLogf(ctx, "Staging image %q", stagingURL)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", stagingURL, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to stage file at %q", stagingURL)
	}
	res, err := cl.Do(httpReq)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to stage file at %q", stagingURL)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to stage file at %q: %v", stagingURL, res.StatusCode)
	}
	testImageURL := fmt.Sprintf("%s/extract/%s/chromiumos_test_image.tar.xz?file=chromiumos_test_image.bin", devserverURL, strings.TrimPrefix(artifactsURL, "gs://"))
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
		httpReq, err = http.NewRequestWithContext(ctx, "GET", testImageURL, nil)
		if err != nil {
			return err
		}
		res, err = cl.Do(httpReq)
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
	usbRelease, _, err = usb.ValidateUSBImage(ctx, usbDevice, "/media/usbkey", &localRunner{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to verify %s", usbDevice)
	}
	if usbRelease != "" && strings.HasSuffix(artifactsURL, usbRelease) {
		testing.ContextLogf(ctx, "Successfully flashed %q from %q", usbDevice, testImageURL)
		return &empty.Empty{}, nil
	}
	return nil, errors.Errorf("wrong version on %s after flashing, got %s, want %s", usbDevice, usbRelease, artifactsURL)
}

type configData struct {
	ChromeOS struct {
		Configs []struct {
			Firmware struct {
				BuildTargets struct {
					// AP image name, if missing fallback to ImageName
					Coreboot string `yaml:"coreboot"`
					EC       string `yaml:"ec"`
					ZephyrEC string `yaml:"zephyr-ec"`
				} `yaml:"build-targets"`
				ImageName string `yaml:"image-name"`
			} `yaml:"firmware"`
		}
	}
}

// FirmwareBuildTargets returns the names of the firmware targets from the cros config yaml file.
// This code matches the logic in infra/go/src/infra/cros/cmd/provision/cros-fw-provision/service/firmwareservice.go:ReadConfigYaml
// TODO: Figure out how to share the code.
func (us *UtilsService) FirmwareBuildTargets(ctx context.Context, req *fwpb.FirmwareBuildTargetsRequest) (*fwpb.FirmwareBuildTargetsResponse, error) {
	retVal := &fwpb.FirmwareBuildTargetsResponse{}
	out, err := testexec.CommandContext(ctx, "crosid").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run crosid")
	}
	configIndexRe, err := regexp.Compile(`CONFIG_INDEX='([^']*)'`)
	if err != nil {
		return nil, errors.Wrap(err, "config index regex failed")
	}
	m := configIndexRe.FindSubmatch(out)
	if m == nil {
		return nil, errors.Wrapf(err, "regexp match of CONFIG_INDEX failed on %q", string(out))
	}
	configIndex, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return nil, errors.Wrapf(err, "parse of CONFIG_INDEX %q failed", string(m[1]))
	}
	fwManifestKeyRe, err := regexp.Compile(`FIRMWARE_MANIFEST_KEY='([^']*)'`)
	if err != nil {
		return nil, errors.Wrap(err, "config index regex failed")
	}
	m = fwManifestKeyRe.FindSubmatch(out)
	if m == nil {
		return nil, errors.Wrapf(err, "regexp match of FIRMWARE_MANIFEST_KEY failed on %q", string(out))
	}
	retVal.FirmwareManifestKey = string(m[1])

	testing.ContextLogf(ctx, "DUT configIndex = %d firmwareManifestKey = %s", configIndex, retVal.FirmwareManifestKey)
	yamlPath := "/usr/share/chromeos-config/yaml/config.yaml"
	configFile, err := os.Open(yamlPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open config.yaml")
	}
	defer configFile.Close()
	configYaml := configData{}
	parser := yaml.NewDecoder(configFile)
	err = parser.Decode(&configYaml)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse config.yaml")
	}
	config := configYaml.ChromeOS.Configs[configIndex]
	testing.ContextLogf(ctx, "config entry: %+v", config)
	thisAPName := config.Firmware.BuildTargets.Coreboot
	if thisAPName == "" {
		thisAPName = config.Firmware.ImageName
	}
	if thisAPName != "" {
		retVal.CorebootName = thisAPName
	}
	// Bizarrely, firmware branch builders use the coreboot name for the ec.bin file for zephyr binaries.
	if config.Firmware.BuildTargets.ZephyrEC != "" {
		retVal.StandaloneEcName = config.Firmware.BuildTargets.ZephyrEC
		retVal.LegacyEcName = thisAPName
	} else {
		retVal.StandaloneEcName = config.Firmware.BuildTargets.EC
		retVal.LegacyEcName = config.Firmware.BuildTargets.EC
	}
	// Special case for reef boards. See b/398900326
	if retVal.CorebootName != retVal.LegacyEcName && retVal.LegacyEcName == "reef" {
		testing.ContextLogf(ctx, "Overriding EC name to '%q", retVal.CorebootName)
		retVal.LegacyEcName = retVal.CorebootName
	}
	testing.ContextLogf(ctx, "config.yaml image names AP: %s EC(legacy): %s EC(standalone): %s", retVal.CorebootName, retVal.LegacyEcName, retVal.StandaloneEcName)
	return retVal, nil
}

type imageCandidate struct {
	GSURL     string
	Filenames []string
}

// A url like gs://chromeos-image-archive/firmware-brya-14505.B-branch/R100-14505.832.0-1-8730368903603296945/brya/firmware_from_source.tar.bz2
// becomes gs://firmware-image-archive/firmware-brya-14505.B/14505.832.0/omnigul.14505.832.0.tar.bz2
var legacyURLRE = regexp.MustCompile(`^gs://(?:chromeos|firmware)-image-archive/(?:[^/]*/)?(firmware-[^/]*)(?:-branch(?:-firmware)?)?/(?:R\d+-)?(\d+\.\d+\.\d+)[-\d]*/.*`)

// A url like gs://firmware-image-archive/firmware-ec-R135-16209.5.B/16209.5.25/ or gs://chromeos-image-archive/firmware-zephyr-postsubmit/R136-16217.0.0-108800-8720748254242768705/
// with a trailing slash needs the version number extracted so we can append the single target tar file.
var versionedDirRE = regexp.MustCompile(`^(gs://.*)/((?:R\d+-)?(\d+\.\d+\.\d+)[-\d]*)/$`)

// The legacy builder (i.e. firmware-brya-14505.B) creates files using the coreboot name
// gs://firmware-image-archive/firmware-brya-14505.B/14505.846.0/omnigul.14505.846.0.tar.bz2
// gs://firmware-image-archive/firmware-brya-14505.B/14505.846.0/omnigul.EC.14505.846.0.tar.bz2

var titleCaseRe = regexp.MustCompile(`[a-zA-Z]+`)

// titleCase converts string to capitalize every word, using the same weird rules as python's titlecase() function.
// I.e. wonka's_chocolate becomes Wonka'S_Chocolate.
func titleCase(s string) string {
	idxs := titleCaseRe.FindAllStringIndex(s, -1)
	fixed := []rune(s)
	for _, r := range idxs {
		fixed[r[0]] = unicode.ToUpper(fixed[r[0]])
	}
	return string(fixed)
}

// getAPCandidateURLs returns a list of urls and files to extract. Try them in order.
func getAPCandidateURLs(ctx context.Context, gsPath, board, model string, buildTargets *fwpb.FirmwareBuildTargetsResponse) ([]imageCandidate, error) {
	var candidates []imageCandidate
	// If the url matches versionedDirRe, try the single target tarfile, but don't fallback to the other patterns.
	m := versionedDirRE.FindStringSubmatch(gsPath)
	if m != nil {
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("%[1]s/%[2]s/%[4]s.%[3]s.tar.bz2", m[1], m[2], m[3], buildTargets.CorebootName),
			Filenames: []string{fmt.Sprintf("image-%v.bin", buildTargets.CorebootName), "image.bin"},
		})
		capitalCorebootName := titleCase(buildTargets.CorebootName)
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("%[1]s/%[2]s/%[4]s.%[3]s.tbz2", m[1], m[2], m[3], capitalCorebootName),
			Filenames: []string{fmt.Sprintf("image-%v.bin", buildTargets.CorebootName), "image.bin"},
		})
		return candidates, nil
	}
	// If the url matches legacyUrlRe, and we have a coreboot name, try the single target tarfile
	m = legacyURLRE.FindStringSubmatch(gsPath)
	if m != nil && buildTargets.CorebootName != "" {
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("gs://firmware-image-archive/%[4]s/%[1]s/%[2]s/%[3]s.%[2]s.tar.bz2", m[1], m[2], buildTargets.CorebootName, board),
			Filenames: []string{fmt.Sprintf("image-%v.bin", buildTargets.CorebootName), "image.bin"},
		})
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("gs://firmware-image-archive/%[1]s/%[2]s/%[3]s.%[2]s.tar.bz2", m[1], m[2], buildTargets.CorebootName),
			Filenames: []string{fmt.Sprintf("image-%v.bin", buildTargets.CorebootName), "image.bin"},
		})
		capitalCorebootName := titleCase(buildTargets.CorebootName)
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("gs://firmware-image-archive/%[1]s/%[2]s/%[3]s.%[2]s.tbz2", m[1], m[2], capitalCorebootName),
			Filenames: []string{fmt.Sprintf("image-%v.bin", buildTargets.CorebootName), "image.bin"},
		})
	}
	// Then fallback to the giant tarball.
	var filenames []string
	if buildTargets.CorebootName != "" {
		filenames = append(filenames, fmt.Sprintf("image-%v.bin", buildTargets.CorebootName))
	}
	if len(model) > 0 {
		filenames = append(filenames, fmt.Sprintf("image-%v.bin", model))
	}
	if len(board) > 0 {
		filenames = append(filenames, fmt.Sprintf("image-%v.bin", board))
	}
	filenames = append(filenames, "image.bin")
	filenames = append(filenames, "bios.bin")
	candidates = append(candidates, imageCandidate{
		GSURL:     gsPath,
		Filenames: filenames,
	})
	return candidates, nil
}

// getECCandidateURLs returns a list of urls and files to extract. Try them in order.
func getECCandidateURLs(ctx context.Context, gsPath, board, model string, buildTargets *fwpb.FirmwareBuildTargetsResponse) ([]imageCandidate, error) {
	var candidates []imageCandidate
	ecName := buildTargets.LegacyEcName
	// The "standalone" builders that just build zephyr ECs use a different naming scheme.
	if strings.Contains(gsPath, "/firmware-ec-R") || strings.Contains(gsPath, "/firmware-zephyr-") {
		ecName = buildTargets.StandaloneEcName
	}
	// If the url matches versionedDirRE, try the single target tarfile, but don't fallback to the other patterns.
	m := versionedDirRE.FindStringSubmatch(gsPath)
	if m != nil {
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("%[1]s/%[2]s/%[4]s.EC.%[3]s.tar.bz2", m[1], m[2], m[3], ecName),
			Filenames: []string{"ec.bin"},
		})
		capitalECName := titleCase(ecName)
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("%[1]s/%[2]s/%[4]s_EC.%[3]s.tbz2", m[1], m[2], m[3], capitalECName),
			Filenames: []string{"ec.bin"},
		})
		return candidates, nil
	}
	// If the url matches legacyURLRE, and we have a legacy ec name, try the single target tarfile
	m = legacyURLRE.FindStringSubmatch(gsPath)
	if m != nil && buildTargets.LegacyEcName != "" {
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("gs://firmware-image-archive/%[4]s/%[1]s/%[2]s/%[3]s.EC.%[2]s.tar.bz2", m[1], m[2], ecName, board),
			Filenames: []string{"ec.bin"},
		})
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("gs://firmware-image-archive/%[1]s/%[2]s/%[3]s.EC.%[2]s.tar.bz2", m[1], m[2], ecName),
			Filenames: []string{"ec.bin"},
		})
		capitalECName := titleCase(ecName)
		candidates = append(candidates, imageCandidate{
			GSURL:     fmt.Sprintf("gs://firmware-image-archive/%[1]s/%[2]s/%[3]s_EC.%[2]s.tbz2", m[1], m[2], capitalECName),
			Filenames: []string{"ec.bin"},
		})
	}
	// Then fallback to the giant tarball.
	var filenames []string
	if ecName != "" {
		filenames = append(filenames, path.Join(ecName, "ec.bin"))
	}
	if len(model) > 0 {
		filenames = append(filenames, path.Join(model, "ec.bin"))
	}
	if len(board) > 0 {
		filenames = append(filenames, path.Join(board, "ec.bin"))
	}
	filenames = append(filenames, "ec.bin")
	candidates = append(candidates, imageCandidate{
		GSURL:     gsPath,
		Filenames: filenames,
	})
	return candidates, nil
}

// createStageURL returns the URL to stage a gsPath. Pass to curl on the DUT.
func createStageURL(ctx context.Context, gsPath string, cacheServer url.URL) (url.URL, error) {
	gsURL, err := url.Parse(gsPath)
	if err != nil {
		return url.URL{}, err
	}
	stagingURL := cacheServer
	stagingURL.Path = "/stage"
	v := url.Values{}
	v.Set("files", path.Base(gsURL.Path))
	gsURL.Path = path.Dir(gsURL.Path)
	v.Set("archive_url", gsURL.String())
	stagingURL.RawQuery = v.Encode()
	return stagingURL, nil
}

// createExtractURL returns the URL to extract a file from a gsPath. Pass to curl on the DUT.
func createExtractURL(ctx context.Context, gsPath, fileInArchive string, cacheServer url.URL) (url.URL, error) {
	gsPathURL, err := url.Parse(gsPath)
	if err != nil {
		return url.URL{}, errors.Wrapf(err, "failed to parse %q", gsPath)
	}
	extractURL := cacheServer
	extractURL.Path = fmt.Sprintf("/extract/%s%s", gsPathURL.Host, gsPathURL.Path)
	v := url.Values{}
	v.Set("file", fileInArchive)
	extractURL.RawQuery = v.Encode()
	return extractURL, nil
}

var curlErrorRE *regexp.Regexp = regexp.MustCompile(`The requested URL returned error: (\d+)`)

func stageFile(ctx context.Context, gsPath string, devserverURL *url.URL, cl *http.Client) (bool, error) {
	testing.ContextLogf(ctx, "Staging %q", gsPath)
	url, err := createStageURL(ctx, gsPath, *devserverURL)
	if err != nil {
		return false, errors.Wrapf(err, "failed to stage file at %q", url.String())
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
	if err != nil {
		return false, errors.Wrapf(err, "failed to stage file at %q", url.String())
	}
	res, err := cl.Do(httpReq)
	if err != nil {
		return false, errors.Wrapf(err, "failed to stage file at %q", url.String())
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		body, _ := io.ReadAll(res.Body)
		testing.ContextLogf(ctx, "not found at %q: %v %s", url.String(), res.StatusCode, html.UnescapeString(string(body)))
		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return false, errors.Errorf("failed to stage file at %q: %v %s", url.String(), res.StatusCode, html.UnescapeString(string(body)))
	}

	testing.ContextLogf(ctx, "Stage of %q success", gsPath)
	return true, nil
}

// extractFile calls the cache server to extract a file to the DUT, and retries on 5xx http errors.
// Returns false, nil on 404 errors. Returns an error on all other errors.
func extractFile(ctx context.Context, destPath, filename, gsPath string, devserverURL *url.URL, cl *http.Client) (bool, error) {
	testing.ContextLogf(ctx, "Trying %q", filename)
	url, err := createExtractURL(ctx, gsPath, filename, *devserverURL)
	if err != nil {
		return false, errors.Wrapf(err, "no url for %q", filename)
	}
	var lastError error
	lastError = errors.New("this will never happen")
	for i := 0; i < 5; i++ {
		httpReq, err := http.NewRequestWithContext(ctx, "GET", url.String(), nil)
		if err != nil {
			return false, errors.Wrapf(err, "failed to extract file at %q", url.String())
		}
		res, err := cl.Do(httpReq)
		if err != nil {
			return false, errors.Wrapf(err, "failed to extract file at %q", url.String())
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusNotFound {
			body, _ := io.ReadAll(res.Body)
			testing.ContextLogf(ctx, "not found at %q: %v %s", url.String(), res.StatusCode, html.UnescapeString(string(body)))
			return false, nil
		}
		if res.StatusCode > 500 && res.StatusCode < 600 {
			body, _ := io.ReadAll(res.Body)
			lastError = errors.Errorf("failed to extract file at %q: %v %s", url.String(), res.StatusCode, html.UnescapeString(string(body)))
			testing.ContextLog(ctx, "Retryable error: ", lastError)
			continue
		}
		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			return false, errors.Errorf("failed to extract file at %q: %v %s", url.String(), res.StatusCode, html.UnescapeString(string(body)))
		}
		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return false, errors.Wrapf(err, "open %q", destPath)
		}
		defer outFile.Close()

		_, err = io.Copy(outFile, res.Body)
		if err != nil {
			return false, errors.Wrapf(err, "extract %q", filename)
		}

		testing.ContextLog(ctx, "Success: ", destPath)

		return true, nil
	}
	return false, lastError
}

// ExtractAPFirmwareImage downloads and extracts AP firmware images from devservers.
// This code matches the logic in infra/go/src/infra/cros/cmd/provision/cros-fw-provision/service/common.go:PickAndExtractMainImage
// TODO: Figure out how to share the code.
func (us *UtilsService) ExtractAPFirmwareImage(ctx context.Context, req *fwpb.ExtractFirmwareImageRequest) (*fwpb.ExtractFirmwareImageResponse, error) {
	retVal := &fwpb.ExtractFirmwareImageResponse{}
	devserver, err := findDevServer(ctx, req.GetPreferredDevserver())
	if err != nil {
		testing.ContextLog(ctx, "No working preferred devservers: ", err)
		devserver, err = findDevServer(ctx, req.GetBackupDevserver())
		if err != nil {
			return nil, errors.Wrap(err, "failed to find devserver")
		}
	}
	testing.ContextLogf(ctx, "Found a working devserver at %s", devserver)
	devserverURL, err := url.Parse(devserver)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q", devserver)
	}

	// Short circuit if we already downloaded the image
	_, err = os.Stat(req.Dest)
	if err == nil {
		testing.ContextLogf(ctx, "File already downloaded: %s", req.Dest)
		return retVal, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Wrap(err, "failed to check dest")
	}
	candidates, err := getAPCandidateURLs(ctx, req.Url, req.Board, req.Model, req.BuildTargets)
	if err != nil {
		return nil, errors.Wrap(err, "failed to calculate candidates")
	}
	cl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			Proxy:               http.ProxyFromEnvironment,
		},
	}
	for _, candidate := range candidates {
		ok, err := stageFile(ctx, candidate.GSURL, devserverURL, cl)
		if err != nil {
			return nil, errors.Wrap(err, "failed to stage")
		}
		if !ok {
			continue
		}
		for _, filename := range candidate.Filenames {
			ok, err := extractFile(ctx, req.Dest, filename, candidate.GSURL, devserverURL, cl)
			if err != nil {
				return nil, errors.Wrap(err, "failed to extract")
			}
			if ok {
				return retVal, nil
			}
		}
	}
	return nil, errors.Errorf("could not find an AP image in any of: %v", candidates)
}

// ExtractECFirmwareImage downloads and extracts EC firmware images from devservers.
// This code matches the logic in infra/go/src/infra/cros/cmd/provision/cros-fw-provision/service/common.go:PickAndExtractECImage
// TODO: Figure out how to share the code.
func (us *UtilsService) ExtractECFirmwareImage(ctx context.Context, req *fwpb.ExtractFirmwareImageRequest) (*fwpb.ExtractFirmwareImageResponse, error) {
	retVal := &fwpb.ExtractFirmwareImageResponse{}
	devserver, err := findDevServer(ctx, req.GetPreferredDevserver())
	if err != nil {
		testing.ContextLog(ctx, "No working preferred devservers: ", err)
		devserver, err = findDevServer(ctx, req.GetBackupDevserver())
		if err != nil {
			return nil, errors.Wrap(err, "failed to find devserver")
		}
	}
	testing.ContextLogf(ctx, "Found a working devserver at %s", devserver)
	devserverURL, err := url.Parse(devserver)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q", devserver)
	}

	// Short circuit if we already downloaded the image
	_, err = os.Stat(req.Dest)
	if err == nil {
		testing.ContextLogf(ctx, "File already downloaded: %s", req.Dest)
		return retVal, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Wrap(err, "failed to check dest")
	}
	candidates, err := getECCandidateURLs(ctx, req.Url, req.Board, req.Model, req.BuildTargets)
	if err != nil {
		return nil, errors.Wrap(err, "failed to calculate candidates")
	}
	cl := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			Proxy:               http.ProxyFromEnvironment,
		},
	}
	for _, candidate := range candidates {
		ok, err := stageFile(ctx, candidate.GSURL, devserverURL, cl)
		if err != nil {
			return nil, errors.Wrap(err, "failed to stage")
		}
		if !ok {
			continue
		}
		for _, filename := range candidate.Filenames {
			ok, err := extractFile(ctx, req.Dest, filename, candidate.GSURL, devserverURL, cl)
			if err != nil {
				return nil, errors.Wrap(err, "failed to extract ec.bin")
			}
			if !ok {
				continue
			}
			// Try to get ec.config also
			ecConfigFilename := strings.Replace(filename, ".bin", ".config", 1)
			ecConfigDest := strings.Replace(req.Dest, ".bin", ".config", 1)
			ok, err = extractFile(ctx, ecConfigDest, ecConfigFilename, candidate.GSURL, devserverURL, cl)
			if err != nil {
				return nil, errors.Wrap(err, "failed to extract ec.config")
			}
			return retVal, nil
		}
	}
	return nil, errors.Errorf("could not find an EC image in any of: %v", candidates)
}
