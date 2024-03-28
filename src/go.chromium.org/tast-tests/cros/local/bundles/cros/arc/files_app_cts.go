// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

const filesAppCtsTestFilterVarName = "arc.FilesAppCts.testFilter"

var filesAppCtsTestFilterVar = testing.RegisterVarString(
	filesAppCtsTestFilterVarName,
	".*",
	"Regular expression to filter the tests to run.",
)

// These APKs are obtained from
// https://dl.google.com/dl/android/cts/android-cts-13_r7-linux_x86-arm.zip
var filesAppCtsTmApks = []string{
	// For DocumentsClientTest:
	"files_cts/CtsDocumentClient_tm.apk",
	"files_cts/CtsDocumentProvider_tm.apk",
	// For MediaStoreUiTest:
	"files_cts/CtsProviderUiTestCases_tm.apk",
}

// Test cases in T version of DocumentsClientTest that open ChromeOS Files app.
var filesAppCtsDocumentsClientTestTmTestCases = []string{
	"testOpenSimple",
	"testOpenVirtual",
	"testGetContent_rootsShowing",
	"testGetContentWithQuery_matchingFileShowing",
	"testGetContent_returnsResultToCallingActivity",
	"testOpenDocumentAtInitialLocation",
	"testCreateWebLink",
}

// Test cases in T version of MediaStoreUiTest that open ChromeOS Files app.
var filesAppCtsMediaStoreUITestTmTestCases = []string{
	"testGetMediaUriAccess_mediaDocumentsProvider",
	"testOpenFile_onMediaDocumentsProvider_success",
	"testOpenFile_onMediaDocumentsProvider_failsWithoutAccess",
}

type filesAppCtsTest struct {
	name        string
	packageName string
	testCases   []string
}

type filesAppCtsTestParams struct {
	apkNames []string
	tests    []filesAppCtsTest
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FilesAppCts,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run CTS test cases relevant to CrOS Files app",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@google.com", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Fixture:      "arcBootedWithoutUIAutomator",
		Vars:         []string{filesAppCtsTestFilterVarName},
		SoftwareDeps: []string{"chrome"},
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "t",
				ExtraSoftwareDeps: []string{"android_vm_t"},
				ExtraData:         filesAppCtsTmApks,
				Val: filesAppCtsTestParams{
					apkNames: filesAppCtsTmApks,
					tests: []filesAppCtsTest{
						{
							name:        "DocumentsClientTest",
							packageName: "com.android.cts.documentclient",
							testCases:   filesAppCtsDocumentsClientTestTmTestCases,
						},
						{
							name:        "MediaStoreUiTest",
							packageName: "android.providerui.cts",
							testCases:   filesAppCtsMediaStoreUITestTmTestCases,
						},
					},
				},
			},
		},
	})
}

func FilesAppCts(ctx context.Context, s *testing.State) {
	params := s.Param().(filesAppCtsTestParams)

	testFilterRegexp, err := regexp.Compile(filesAppCtsTestFilterVar.Value())
	if err != nil {
		s.Fatalf("Failed to complie regular expression %q: %s", filesAppCtsTestFilterVar.Value(), err)
	}

	a := s.FixtValue().(*arc.PreData).ARC

	for _, apkName := range params.apkNames {
		if err := a.Install(ctx, s.DataPath(apkName)); err != nil {
			s.Fatalf("Failed to install %s: %s", apkName, err)
		}
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// The first test case can be unstable (due to UI timeout) if it's run
	// when the device is still busy.
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Fatal("Failed to wait until CPU become idle: ", err)
	}

	for _, test := range params.tests {
		testing.ContextLog(ctx, "Started "+test.name)
		for _, testCaseName := range test.testCases {
			testFullName := fmt.Sprintf("%s.%s#%s", test.packageName, test.name, testCaseName)
			if !testFilterRegexp.MatchString(testFullName) {
				continue
			}

			testing.ContextLog(ctx, "Running test case "+testCaseName)
			cmd := a.Command(ctx, "am", "instrument", "-w", "-e", "class", testFullName,
				test.packageName+"/androidx.test.runner.AndroidJUnitRunner")
			out, err := cmd.Output(testexec.DumpLogOnError)
			if err != nil {
				s.Fatalf("Failed to run test case %s: %s", testFullName, err)
			}
			outStr := string(out)
			if strings.Contains(outStr, "Error in "+testCaseName) {
				testing.ContextLog(ctx, outStr)
				s.Errorf("Failed test case %s. You can run this test case with the following command on DUT: "+
					"adb shell am instrument -w -e class %s %s/androidx.test.runner.AndroidJUnitRunner",
					testFullName, testFullName, test.packageName)
			}

			// Some test cases leave the file picker window open. Close it.
			if err := keyboard.AccelAction("ctrl+w")(ctx); err != nil {
				testing.ContextLogf(ctx, "Failed to close window after %s#%s", test.name, testCaseName)
			}
		}
		testing.ContextLog(ctx, "Completed "+test.name)
	}
}
