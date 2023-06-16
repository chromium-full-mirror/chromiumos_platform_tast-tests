// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package printer

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParam struct {
	descriptorPath string
	browserType    browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HardwarePrintCombinations,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that settings can be set in the print dialog",
		Contacts:     []string{"project-bolton@google.com"},
		// ChromeOS > Platform > Services > Printing
		BugComponent: "b:167231",
		Attr: []string{
			"group:paper-io",
			"paper-io_printing",
		},
		Timeout:      4 * time.Minute,
		SoftwareDeps: []string{"chrome", "cros_internal", "cups"},
		Params: []testing.Param{
			// Format for test cases is as follows:
			// Name: manufacturer_model
			// Val: printer descriptor file
			// ExtraData: printer descriptor file
			{
				//MFP in test lab
				Name: "sharp_mx_b467f",
				Val: &testParam{
					descriptorPath: "sharp_mx_b467f_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"sharp_mx_b467f_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in test lab
				Name:              "lacros_sharp_mx_b467f",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "sharp_mx_b467f_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"sharp_mx_b467f_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in test lab
				Name: "hp_laserjet_pro_m478f",
				Val: &testParam{
					descriptorPath: "hp_laserjet_pro_m478f_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"hp_laserjet_pro_m478f_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in test lab
				Name:              "lacros_hp_laserjet_pro_m478f",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "hp_laserjet_pro_m478f_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"hp_laserjet_pro_m478f_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in test lab
				Name: "brother_dcp_l2550dw_series",
				Val: &testParam{
					descriptorPath: "brother_dcp_l2550dw_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"brother_dcp_l2550dw_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in test lab
				Name:              "lacros_brother_dcp_l2550dw_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "brother_dcp_l2550dw_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"brother_dcp_l2550dw_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in test lab
				Name: "brother_hl_l8360cdw_series",
				Val: &testParam{
					descriptorPath: "brother_hl_l8360cdw_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"brother_hl_l8360cdw_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in test lab
				Name:              "lacros_brother_hl_l8360cdw_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "brother_hl_l8360cdw_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"brother_hl_l8360cdw_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in test lab
				Name: "hp_laserjet_mfp_m234dw",
				Val: &testParam{
					descriptorPath: "hp_laserjet_mfp_m234dw_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"hp_laserjet_mfp_m234dw_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in test lab
				Name:              "lacros_hp_laserjet_mfp_m234dw",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "hp_laserjet_mfp_m234dw_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"hp_laserjet_mfp_m234dw_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in test lab
				Name: "lexmark_mc3426adw",
				Val: &testParam{
					descriptorPath: "lexmark_mc3426adw_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"lexmark_mc3426adw_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in test lab
				Name:              "lacros_lexmark_mc3426adw",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "lexmark_mc3426adw_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"lexmark_mc3426adw_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in test lab
				Name: "usb_canon_tr4700_series",
				Val: &testParam{
					descriptorPath: "usb_canon_tr4700_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData:         []string{"usb_canon_tr4700_series_descriptor.json"},
				ExtraAttr:         []string{"paper-io_mfp_printscan"},
				Fixture:           "chromeLoggedIn",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("treeya")),
			}, {
				//MFP in test lab
				Name:              "lacros_usb_canon_tr4700_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "usb_canon_tr4700_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData:         []string{"usb_canon_tr4700_series_descriptor.json"},
				ExtraAttr:         []string{"paper-io_mfp_printscan"},
				Fixture:           "lacros",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("treeya")),
			}, {
				//MFP in test lab
				Name: "usb_hp_deskjet_2700_series",
				Val: &testParam{
					descriptorPath: "usb_hp_deskjet_2700_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData:         []string{"usb_hp_deskjet_2700_series_descriptor.json"},
				ExtraAttr:         []string{"paper-io_mfp_printscan"},
				Fixture:           "chromeLoggedIn",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("lazor")),
			}, {
				//MFP in test lab
				Name:              "lacros_usb_hp_deskjet_2700_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: &testParam{
					descriptorPath: "usb_hp_deskjet_2700_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData:         []string{"usb_hp_deskjet_2700_series_descriptor.json"},
				ExtraAttr:         []string{"paper-io_mfp_printscan"},
				Fixture:           "lacros",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("lazor")),
			},
			// printers in BLD lab: un-comment to test them
			/*{
				//MFP in BLD lab
				Name:      "brother_hl_l2395dw_series",
				Val:       &testParam{
					descriptorPath: "brother_hl_l2395dw_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"brother_hl_l2395dw_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_brother_hl_l2395dw_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "brother_hl_l2395dw_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"brother_hl_l2395dw_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in BLD lab
				Name:      "epson_xp_7100_series",
				Val:       &testParam{
					descriptorPath: "epson_xp_7100_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"epson_xp_7100_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_epson_xp_7100_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "epson_xp_7100_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"epson_xp_7100_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in BLD lab
				Name:      "epson_wf_2540_series",
				Val:       &testParam{
					descriptorPath: "epson_wf_2540_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"epson_wf_2540_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_epson_wf_2540_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "epson_wf_2540_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"epson_wf_2540_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in BLD lab
				Name:      "epson_artisan_837",
				Val:       &testParam{
					descriptorPath: "epson_artisan_837_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"epson_artisan_837_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_epson_artisan_837",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "epson_artisan_837_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"epson_artisan_837_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in BLD lab
				Name:      "hp_envy_photo_7100_series",
				Val:       &testParam{
					descriptorPath: "hp_envy_photo_7100_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"hp_envy_photo_7100_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_hp_envy_photo_7100_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "hp_envy_photo_7100_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"hp_envy_photo_7100_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in BLD lab
				Name:      "hp_envy_inspire_7900_series",
				Val:       &testParam{
					descriptorPath: "hp_envy_inspire_7900_series_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"hp_envy_inspire_7900_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_hp_envy_inspire_7900_series",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "hp_envy_inspire_7900_series_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"hp_envy_inspire_7900_series_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in BLD lab
				Name:      "lexmark_mb2236adwe",
				Val:       &testParam{
					descriptorPath: "lexmark_mb2236adwe_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"lexmark_mb2236adwe_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_lexmark_mb2236adwe",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "lexmark_mb2236adwe_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"lexmark_mb2236adwe_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			}, {
				//MFP in BLD lab
				Name:      "xerox_workcentre_6515",
				Val:       &testParam{
					descriptorPath: "xerox_workcentre_6515_descriptor.json",
					browserType:    browser.TypeAsh,
				},
				ExtraData: []string{"xerox_workcentre_6515_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "chromeLoggedIn",
			}, {
				//MFP in BLD lab
				Name:      "lacros_xerox_workcentre_6515",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:       &testParam{
					descriptorPath: "xerox_workcentre_6515_descriptor.json",
					browserType:    browser.TypeLacros,
				},
				ExtraData: []string{"xerox_workcentre_6515_descriptor.json"},
				ExtraAttr: []string{"paper-io_mfp_printscan"},
				Fixture:   "lacros",
			},*/
		},
	})
}

func HardwarePrintCombinations(ctx context.Context, s *testing.State) {
	param := s.Param().(*testParam)

	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	br, closeBrowser, err := browserfixt.SetUp(ctx, s.FixtValue().(*chrome.Chrome),
		param.browserType)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Open a new Chrome tab so that there's something to print.
	ui := uiauto.New(tconn)
	if _, err := br.NewConn(ctx, chrome.NewTabURL); err != nil {
		s.Fatal("Failed to open new Chrome tab: ", err)
	}

	// Read and parse the JSON file describing the printer's available settings.
	fileContents, err := ioutil.ReadFile(s.DataPath(param.descriptorPath))
	if err != nil {
		s.Fatal("Unable to read printer descriptor file: ", err)
	}
	var desc printerDescription
	if err := json.Unmarshal(fileContents, &desc); err != nil {
		s.Fatal("Failed to parse printer's JSON description: ", err)
	}
	printerName := desc.Name

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := uiauto.Combine("open Print Preview with shortcut Ctrl+P",
		kb.AccelAction("Ctrl+P"),
	)(ctx); err != nil {
		s.Fatal("Failed to open Print Preview: ", err)
	}

	// Select printer.
	s.Log("Selecting printer: ", printerName)
	if err := printpreview.SelectPrinter(ctx, tconn, printerName); err != nil {
		s.Fatal("Failed to select printer: ", err)
	}

	if err := printpreview.ExpandMoreSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to expand 'more settings': ", err)
	}

	printButton := nodewith.Name("Print").Role(role.Button)
	printButtonFocusable := uiauto.Combine("ensure Print button is focusable",
		ui.EnsureFocused(printButton),
	)

	// Test each value for each dropdown setting.
	for name, info := range desc.RegularSettings.Dropdowns {
		if err := setDependencies(ctx, tconn, info.Dependencies); err != nil {
			s.Fatal("Failed to set dependencies: ", err)
		}
		for _, value := range info.Values {
			s.Log("Setting dropdown '", name, "' to '", value, "'")
			if err := printpreview.SetDropdown(ctx, tconn, name, value); err != nil {
				s.Fatal("Failed to set value of dropdown '", name, "' to '", value, "': ", err)
			}
			if err := printButtonFocusable(ctx); err != nil {
				s.Fatal("Failed to focus on print button: ", err)
			}
		}
	}

	// Test both values (on and off) for each checkbox setting.
	for name, info := range desc.RegularSettings.Checkboxes {
		if err := setDependencies(ctx, tconn, info.Dependencies); err != nil {
			s.Fatal("Failed to set dependencies: ", err)
		}
		for _, value := range []bool{true, false} {
			s.Log("Setting checkbox '", name, "' to '", value, "'")
			if err := printpreview.SetCheckboxState(ctx, tconn, name, value); err != nil {
				s.Fatal("Failed to set value of checkbox '", name, "' to '", value, "': ", err)
			}
			if err := printButtonFocusable(ctx); err != nil {
				s.Fatal("Failed to focus on print button: ", err)
			}
		}
	}

	// Test each value for each advanced setting.
	for name, values := range desc.AdvancedSettings {
		for _, value := range values.([]interface{}) {
			s.Log("Set advanced setting '", name, "' to '", value, "'")
			if err := printpreview.OpenAdvancedSettings(ctx, tconn); err != nil {
				s.Fatal("Failed to open advanced settings dialog: ", err)
			}
			if err := printpreview.SetAdvancedSetting(ctx, tconn, name, value.(string)); err != nil {
				s.Fatal("Failed to set advanced setting: ", err)
			}
			if err := printpreview.CloseAdvancedSettings(ctx, tconn); err != nil {
				s.Fatal("Failed to close advanced settings dialog: ", err)
			}
			if err := printButtonFocusable(ctx); err != nil {
				s.Fatal("Failed to focus on print button: ", err)
			}
		}
	}
}

type printerDescription struct {
	Name             string
	RegularSettings  regularSettings
	AdvancedSettings map[string]interface{}
}

type regularSettings struct {
	Dropdowns  map[string]dropdownSetting
	Checkboxes map[string]checkboxSetting
}

type dropdownSetting struct {
	Type         string
	Values       []string
	Dependencies map[string]interface{}
}

type checkboxSetting struct {
	Dependencies map[string]interface{}
}

func setDependencies(ctx context.Context, tconn *chrome.TestConn, dependencies map[string]interface{}) error {
	for name, value := range dependencies {
		var err error
		switch value.(type) {
		case bool:
			err = printpreview.SetCheckboxState(ctx, tconn, name, value.(bool))
		case string:
			err = printpreview.SetDropdown(ctx, tconn, name, value.(string))
		default:
			err = errors.Errorf("unknown dependency type %T", value)
		}
		if err != nil {
			return err
		}
	}

	return nil
}
