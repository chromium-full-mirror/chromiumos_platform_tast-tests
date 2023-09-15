// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/health/utils"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
)

type psrInfoTestParams struct {
	// Whether to check platform service record.
	checkPsr bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeSystemInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for system info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"chungsheng@google.com",
			"moises.veleta@intel.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Val: psrInfoTestParams{
				checkPsr: false,
			},
		}, {
			Name: "platform_service_record",
			// TODO(b/277667306): Promote to critical.
			ExtraAttr: []string{"informational"},
			Val: psrInfoTestParams{
				checkPsr: true,
			},
			ExtraSoftwareDeps: []string{"intel_psr"},
		}},
	})
}

func ProbeSystemInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategorySystem}
	testParam := s.Param().(psrInfoTestParams)

	var g systemInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &g); err != nil {
		s.Fatal("Failed to get system info telemetry info: ", err)
	}
	e, err := expectedSystemInfo(ctx)
	if err != nil {
		s.Fatal("Failed to get expected system info: ", err)
	}
	if d := cmp.Diff(e.OSInfo, g.OSInfo); d != "" {
		s.Fatal("Failed to get SystemInfo.OSInfo(-expected + got): ", d)
	}
	if d := cmp.Diff(e.VPDInfo, g.VPDInfo); d != "" {
		s.Fatal("Failed to get SystemInfo.VPDInfo(-expected + got): ", d)
	}
	if d := cmp.Diff(e.DMIInfo, g.DMIInfo); d != "" {
		s.Fatal("Failed to get SystemInfo.DMIInfo(-expected + got): ", d)
	}
	if testParam.checkPsr {
		if g.PSRInfo != nil {
			if *g.PSRInfo.IsSupported {
				psrOut, err := runAndParseIntelPsrTool(ctx)
				if err != nil {
					s.Fatal("Failed to get intel-psrtool output: ", err)
				}
				if err := verifyPSRInfo(psrOut, g.PSRInfo); err != nil {
					s.Fatal("Failed to get expected system info: ", err)
				}
			} else {
				s.Fatal("PSR is not supported on this device")
			}
		} else {
			s.Fatal("Failed to get SystemInfo.PSRInfo")
		}
	}
}

type osVersion struct {
	ReleaseMilestone string `json:"release_milestone"`
	BuildNumber      string `json:"build_number"`
	BranchNumber     string `json:"branch_number"`
	PatchNumber      string `json:"patch_number"`
	ReleaseChannel   string `json:"release_channel"`
}

type osInfo struct {
	CodeName        string    `json:"code_name"`
	MarketingName   *string   `json:"marketing_name"`
	OSVersion       osVersion `json:"os_version"`
	BootMode        string    `json:"boot_mode"`
	OEMName         *string   `json:"oem_name"`
	EfiPlatformSize string    `json:"efi_platform_size"`
}

type vpdInfo struct {
	SerialNumber *string `json:"serial_number"`
	Region       *string `json:"region"`
	MFGDate      *string `json:"mfg_date"`
	ActivateDate *string `json:"activate_date"`
	SKUNumber    *string `json:"sku_number"`
	ModelName    *string `json:"model_name"`
	OEMName      *string `json:"oem_name"`
}

type dmiInfo struct {
	BiosVendor     *string           `json:"bios_vendor"`
	BiosVersion    *string           `json:"bios_version"`
	BoardName      *string           `json:"board_name"`
	BoardVender    *string           `json:"board_vendor"`
	BoardVersion   *string           `json:"board_version"`
	ChassisVendor  *string           `json:"chassis_vendor"`
	ChassisType    *jsontypes.Uint64 `json:"chassis_type"`
	ProductFamily  *string           `json:"product_family"`
	ProductName    *string           `json:"product_name"`
	ProductVersion *string           `json:"product_version"`
	SysVendor      *string           `json:"sys_vendor"`
}

type psrEvent struct {
	Data      jsontypes.Uint32 `json:"data"`
	Time      jsontypes.Uint32 `json:"time"`
	EventType string           `json:"type"`
}

type psrInfo struct {
	Events             []psrEvent        `json:"events"`
	IsSupported        *bool             `json:"is_supported"`
	LogStartDate       *jsontypes.Uint32 `json:"log_start_date"`
	LogState           *string           `json:"log_state"`
	ManufactureCountry *string           `json:"manufacture_country"`
	OEMData            *string           `json:"oem_data"`
	OEMMake            *string           `json:"oem_make"`
	OEMModel           *string           `json:"oem_model"`
	OEMName            *string           `json:"oem_name"`
	S3Counter          *jsontypes.Uint32 `json:"s3_counter"`
	S4Counter          *jsontypes.Uint32 `json:"s4_counter"`
	S5Counter          *jsontypes.Uint32 `json:"s5_counter"`
	UPID               *string           `json:"upid"`
	UptimeSeconds      *jsontypes.Uint32 `json:"uptime_seconds"`
	UUID               *string           `json:"uuid"`
	WarmResetCounter   *jsontypes.Uint32 `json:"warm_reset_counter"`
}

type systemInfo struct {
	OSInfo  osInfo   `json:"os_info"`
	VPDInfo *vpdInfo `json:"vpd_info"`
	DMIInfo *dmiInfo `json:"dmi_info"`
	PSRInfo *psrInfo `json:"psr_info"`
}

func expectedOSVersion(ctx context.Context) (osVersion, error) {
	lsb, err := lsbrelease.Load()
	if err != nil {
		return osVersion{}, errors.Wrap(err, "failed to get lsb-release info")
	}
	return osVersion{
		ReleaseMilestone: lsb[lsbrelease.Milestone],
		BuildNumber:      lsb[lsbrelease.BuildNumber],
		BranchNumber:     lsb[lsbrelease.BranchNumber],
		PatchNumber:      lsb[lsbrelease.PatchNumber],
		ReleaseChannel:   lsb[lsbrelease.ReleaseTrack],
	}, nil
}

func expectedBootMode() (string, error) {
	v, err := utils.ReadStringFile("/proc/cmdline")
	if err != nil {
		return "", err
	}
	modeStr := map[string]bool{
		"cros_secure": true,
		"cros_efi":    true,
		"cros_legacy": true,
	}
	var r []string
	for _, s := range strings.Fields(v) {
		if modeStr[s] {
			r = append(r, s)
			modeStr[s] = false // Only add each type once.
		}
	}
	if len(r) == 0 {
		return "", errors.Errorf("BootMode is not in /proc/cmdline: %v", v)
	}
	if len(r) >= 2 {
		return "", errors.Errorf("too many BootMode in /proc/cmdline, got %v, /proc/cmdline: %v", r, v)
	}
	return r[0], nil
}

func expectedOSInfo(ctx context.Context) (osInfo, error) {
	const (
		cfgCodeName          = "/name"
		cfgMarketingName     = "/branding/marketing-name"
		cfgOEMName           = "/branding/oem-name"
		pathUEFIPlatformSize = "/sys/firmware/efi/fw_platform_size"
	)
	var r osInfo
	var err error
	if r.CodeName, err = utils.GetCrosConfig(ctx, cfgCodeName); err != nil {
		return r, err
	}
	if r.MarketingName, err = utils.GetOptionalCrosConfig(ctx, cfgMarketingName); err != nil {
		return r, err
	}
	if r.OEMName, err = utils.GetOptionalCrosConfig(ctx, cfgOEMName); err != nil {
		return r, err
	}
	if r.OSVersion, err = expectedOSVersion(ctx); err != nil {
		return r, err
	}
	if r.BootMode, err = expectedBootMode(); err != nil {
		return r, err
	}
	if r.BootMode == "cros_efi" {
		if r.EfiPlatformSize, err = utils.ReadStringFile(pathUEFIPlatformSize); err != nil {
			return r, err
		}
	} else {
		r.EfiPlatformSize = "unknown"
	}
	return r, nil
}

// expectedSKUNumber return a string pointer here for expected |SKUNumber|.
// Since healthd uses json package to parse json result, the null field becomes
// nil string in go. We return string pointer to simplify the comparison.
func expectedSKUNumber(ctx context.Context, filePath string) (*string, error) {
	const cfgSKUNumber = "/cros-healthd/cached-vpd/has-sku-number"

	c, err := utils.IsCrosConfigTrue(ctx, cfgSKUNumber)
	if err != nil {
		return nil, err
	}
	if !c {
		return nil, nil
	}
	e, err := utils.ReadStringFile(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "this board must have sku_number, but failed to get")
	}
	return &e, nil
}

func expectedVPDInfo(ctx context.Context) (*vpdInfo, error) {
	const (
		vpd = "/sys/firmware/vpd"
		ro  = "/sys/firmware/vpd/ro/"
		rw  = "/sys/firmware/vpd/rw/"
	)
	if _, err := os.Stat(vpd); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var r vpdInfo
	var err error
	if r.ActivateDate, err = utils.ReadOptionalStringFile(path.Join(rw, "ActivateDate")); err != nil {
		return nil, err
	}
	if r.MFGDate, err = utils.ReadOptionalStringFile(path.Join(ro, "mfg_date")); err != nil {
		return nil, err
	}
	if r.ModelName, err = utils.ReadOptionalStringFile(path.Join(ro, "model_name")); err != nil {
		return nil, err
	}
	if r.OEMName, err = utils.ReadOptionalStringFile(path.Join(ro, "oem_name")); err != nil {
		return nil, err
	}
	if r.Region, err = utils.ReadOptionalStringFile(path.Join(ro, "region")); err != nil {
		return nil, err
	}
	if r.SerialNumber, err = utils.ReadOptionalStringFile(path.Join(ro, "serial_number")); err != nil {
		return nil, err
	}
	if r.SKUNumber, err = expectedSKUNumber(ctx, path.Join(ro, "sku_number")); err != nil {
		return nil, err
	}
	return &r, nil
}

func expectedChassisType(filePath string) (*jsontypes.Uint64, error) {
	v, err := utils.ReadOptionalStringFile(filePath)
	if v == nil {
		return nil, err
	}
	i, err := strconv.Atoi(*v)
	if err != nil {
		return nil, err
	}
	r := jsontypes.Uint64(i)
	return &r, nil
}

func expectedDMIInfo(ctx context.Context) (*dmiInfo, error) {
	const dmi = "/sys/class/dmi/id"

	if _, err := os.Stat(dmi); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var r dmiInfo
	var err error
	if r.BiosVendor, err = utils.ReadOptionalStringFile(path.Join(dmi, "bios_vendor")); err != nil {
		return nil, err
	}
	if r.BiosVersion, err = utils.ReadOptionalStringFile(path.Join(dmi, "bios_version")); err != nil {
		return nil, err
	}
	if r.BoardName, err = utils.ReadOptionalStringFile(path.Join(dmi, "board_name")); err != nil {
		return nil, err
	}
	if r.BoardVender, err = utils.ReadOptionalStringFile(path.Join(dmi, "board_vendor")); err != nil {
		return nil, err
	}
	if r.BoardVersion, err = utils.ReadOptionalStringFile(path.Join(dmi, "board_version")); err != nil {
		return nil, err
	}
	if r.ChassisVendor, err = utils.ReadOptionalStringFile(path.Join(dmi, "chassis_vendor")); err != nil {
		return nil, err
	}
	if r.ChassisType, err = expectedChassisType(path.Join(dmi, "chassis_type")); err != nil {
		return nil, err
	}
	if r.ProductFamily, err = utils.ReadOptionalStringFile(path.Join(dmi, "product_family")); err != nil {
		return nil, err
	}
	if r.ProductName, err = utils.ReadOptionalStringFile(path.Join(dmi, "product_name")); err != nil {
		return nil, err
	}
	if r.ProductVersion, err = utils.ReadOptionalStringFile(path.Join(dmi, "product_version")); err != nil {
		return nil, err
	}
	if r.SysVendor, err = utils.ReadOptionalStringFile(path.Join(dmi, "sys_vendor")); err != nil {
		return nil, err
	}
	return &r, nil
}

func runAndParseIntelPsrTool(ctx context.Context) (map[string]string, error) {
	out, err := testexec.CommandContext(ctx, "intel-psrtool", "-d").Output()
	if err != nil {
		return nil, errors.Errorf("failed to execute 'intel-psrtool -d' command: %s", err)
	}
	strOut := string(out)
	psrToolOutput := make(map[string]string)
	r := regexp.MustCompile(`\t(.*):(.*)`)
	for _, match := range r.FindAllStringSubmatch(strOut, -1) {
		k := strings.TrimSpace(match[1])
		v := strings.TrimSpace(match[2])
		psrToolOutput[k] = v
	}

	return psrToolOutput, nil
}

func verifyPSRInfo(psrMap map[string]string, psr *psrInfo) error {
	logStateCros := strings.ToUpper(*psr.LogState)
	logStateTool := strings.ReplaceAll(psrMap["Log State"], " ", "")
	if logStateCros != logStateTool {
		return errors.Errorf("log_state: %s does not match %s", logStateCros, logStateTool)
	}
	uuidTool := strings.Replace(psrMap["Platform Service Record ID"], "-", "", -1)
	if *psr.UUID != uuidTool {
		return errors.Errorf("uuid: %s does not match %s", *psr.UUID, uuidTool)
	}
	upidTool := strings.Replace(psrMap["Unique Platform ID"], "-", "", -1)
	if *psr.UPID != upidTool {
		return errors.Errorf("upid: %s does not match %s", *psr.UPID, upidTool)
	}
	nameCros := strings.TrimSpace(*psr.OEMName)
	if nameCros != psrMap["OEM Name"] {
		return errors.Errorf("oem_name: %s does not match %s", nameCros, psrMap["OEM Name"])
	}
	makeCros := strings.TrimSpace(*psr.OEMMake)
	if makeCros != psrMap["OEM Make"] {
		return errors.Errorf("oem_make: %s does not match %s", makeCros, psrMap["OEM Make"])
	}
	modelCros := strings.TrimSpace(*psr.OEMModel)
	if modelCros != psrMap["OEM Model"] {
		return errors.Errorf("oem_model: %s does not match %s", modelCros, psrMap["OEM Model"])
	}
	countryCros := strings.TrimSpace(*psr.ManufactureCountry)
	countryTool := psrMap["Country of Manufacturer"]
	if countryCros != countryTool {
		return errors.Errorf("manufacture_country: %s does not match %s", countryCros, countryTool)
	}
	s5Cros := fmt.Sprint(*psr.S5Counter)
	s5Tool := psrMap["Cumulative number of S0->S5"]
	if s5Cros != s5Tool {
		return errors.Errorf("s5_counter: %s does not match %s", s5Cros, s5Tool)
	}
	s4Cros := fmt.Sprint(*psr.S4Counter)
	s4Tool := psrMap["Cumulative number of S0->S4"]
	if s4Cros != s4Tool {
		return errors.Errorf("s4_counter: %s does not match %s", s4Cros, s4Tool)
	}
	s3Cros := fmt.Sprint(*psr.S3Counter)
	s3Tool := psrMap["Cumulative number of S0->S3"]
	if s3Cros != s3Tool {
		return errors.Errorf("s3_counter: %s does not match %s", s3Cros, s3Tool)
	}
	warmResetCros := fmt.Sprint(*psr.WarmResetCounter)
	warmResetTool := psrMap["Cumulative number of warm resets"]
	if warmResetCros != warmResetTool {
		return errors.Errorf("warm_reset_counter: %s does not match %s", warmResetCros, warmResetTool)
	}
	return nil
}

func expectedSystemInfo(ctx context.Context) (systemInfo, error) {
	var r systemInfo
	var err error
	if r.OSInfo, err = expectedOSInfo(ctx); err != nil {
		return r, err
	}
	if r.VPDInfo, err = expectedVPDInfo(ctx); err != nil {
		return r, err
	}
	if r.DMIInfo, err = expectedDMIInfo(ctx); err != nil {
		return r, err
	}
	// Fallback to VPD when OEM name is missing in cros-config.
	if r.OSInfo.OEMName == nil && r.VPDInfo != nil {
		r.OSInfo.OEMName = r.VPDInfo.OEMName
	}
	return r, nil
}
