// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package actionlogger contains implementations to collect UI action logs.
package actionlogger

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime/debug"
	"strconv"

	alcommon "go.chromium.org/tast-tests/cros/common/actionlogger"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/screenshot/cliscreenshot"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ActionName is the name of the support UI actions.
type ActionName string

// The list of supported actions.
const (
	KeyAccel             ActionName = "key_accel"
	KeyPress                        = "key_press"
	KeyRelease                      = "key_release"
	KeyCodeType                     = "key_code_type"
	MouseClick                      = "mouse_click"
	MouseDoubleClick                = "mouse_double_click"
	TouchscreenTap                  = "touchscreen_tap"
	TouchscreenLongPress            = "touchscreen_tap"
	TextEntry                       = "text_entry"
	WaitUIExist                     = "wait_ui_exist"
	WaitUIGone                      = "wait_ui_gone"
	WaitUIEnabled                   = "wait_ui_enabled"
	WaitUIFocused                   = "wait_ui_Focused"
)

// ClickInput keeps the info needed for a mouse click.
type ClickInput struct {
	X      int          `json:"x"`
	Y      int          `json:"y"`
	Button mouse.Button `json:"button"`
}

// BoundingBox is the bounding box of UI element with top-left and botton-right points.
type BoundingBox struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

// ActionItem is the info to collect for each UI action.
type ActionItem struct {
	Screenshot   string       `json:"screenshot"`
	UITree       string       `json:"ui_tree"`
	Action       ActionName   `json:"action"`
	UITarget     string       `json:"ui_target"`
	ClickInput   *ClickInput  `json:"click_input,omitempty"`
	BoundingBox  *BoundingBox `json:"bounding_box,omitempty"`
	TestFilename string       `json:"test_filename"`
	LineNumber   string       `json:"line_number"`
}

// ActionLogger is the logger to collect UI action logs.
type ActionLogger struct {
	ShouldRun       bool            // If the logger should run.
	Path            string          // Test path on the host.
	TestName        string          // Test name.
	TestDescription string          // Test description.
	ActionItems     []ActionItem    // A sequence of action records.
	TestCodePaths   map[string]bool // A set of test code paths.
	OutDir          string          // Output dir of the logs.
}

var logger *ActionLogger

const tempLoggerFile = "/tmp/action_logger.gob"

func newActionLogger() *ActionLogger {
	return &ActionLogger{
		ShouldRun:     alcommon.ShouldRun.Value() == "true",
		ActionItems:   []ActionItem{},
		TestCodePaths: map[string]bool{},
	}
}

// recordAction is to be called at the beginning of each UI action.
func (logger *ActionLogger) recordAction(ctx context.Context, tconn *chrome.TestConn, actionName ActionName, uiTarget string, boundingBox *BoundingBox, clickInput *ClickInput) {
	outDir := path.Join(logger.OutDir, "action_logs")
	screenshotsDir := path.Join(outDir, "screenshots")
	uiTreesDir := path.Join(outDir, "ui_trees")

	// Create directories if they don't exist
	if err := os.MkdirAll(outDir, 0755); err != nil {
		testing.ContextLog(ctx, "Failed to create action_logs dir: ", err)
		return
	}
	if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
		testing.ContextLog(ctx, "Failed to create screenshots dir: ", err)
		return
	}
	if err := os.MkdirAll(uiTreesDir, 0755); err != nil {
		testing.ContextLog(ctx, "Failed to create ui_trees dir: ", err)
		return
	}

	screenshotFilename := fmt.Sprintf("step-%d-screenshot.png", len(logger.ActionItems)+1)

	if err := cliscreenshot.Capture(ctx, path.Join(screenshotsDir, screenshotFilename)); err != nil {
		testing.ContextLog(ctx, "Failed to take screenshot: ", err)
	}

	path, lineNumber, err := findCallingTest(ctx)
	if err != nil {
		return
	}

	testFilename, err := extractFilename(path)
	if err != nil {
		testing.ContextLog(ctx, "Failed to extract filename: ", err)
		return
	}

	uiTreeFilename := ""
	if tconn != nil {
		uiTree := ""
		if err := tconn.Eval(ctx, "tast.promisify(chrome.automation.getDesktop)().then(root => root+'')", &uiTree); err != nil {
			testing.ContextLog(ctx, "Failed to extract ui tree: ", err)
		}
		uiTreeFilename = fmt.Sprintf("step-%d-ui_tree.txt", len(logger.ActionItems)+1)

		// Save UI trees.
		saveUITree(ctx, uiTree, uiTreesDir, uiTreeFilename)
	}
	actionIteam := ActionItem{
		Screenshot:   screenshotFilename,
		UITree:       uiTreeFilename,
		Action:       actionName,
		UITarget:     uiTarget,
		BoundingBox:  boundingBox,
		ClickInput:   clickInput,
		TestFilename: testFilename,
		LineNumber:   lineNumber,
	}
	logger.ActionItems = append(logger.ActionItems, actionIteam)
	logger.TestCodePaths[path] = true
}

func (logger *ActionLogger) testCodePathsList() []string {
	codePaths := make([]string, 0, len(logger.TestCodePaths))
	for key := range logger.TestCodePaths {
		codePaths = append(codePaths, key)
	}
	return codePaths
}

// save saves all log info to the output dir, it is to be called at the end of each test.
func (logger *ActionLogger) save(ctx context.Context) {
	outDir := path.Join(logger.OutDir, "action_logs")
	testCodeDir := path.Join(outDir, "test_code")
	pathListFilepath := path.Join(testCodeDir, "path_list.txt")
	actionFilepath := path.Join(outDir, "action_log.json")

	// Create directories if they don't exist.
	if err := os.MkdirAll(outDir, 0755); err != nil {
		testing.ContextLog(ctx, "Failed to create action_logs dir: ", err)
		return
	}
	if err := os.MkdirAll(testCodeDir, 0755); err != nil {
		testing.ContextLog(ctx, "Failed to create test_code dir: ", err)
		return
	}

	// Write a placeholder actionFile with a list of test code paths.
	pathListFile, err := os.Create(pathListFilepath)
	if err != nil {
		testing.ContextLog(ctx, "Failed to create log file: ", err)
		return
	}
	defer pathListFile.Close()

	for _, item := range logger.testCodePathsList() {
		if _, err := pathListFile.WriteString(item + "\n"); err != nil {
			testing.ContextLog(ctx, "Failed to write test code paths: ", err)
			return
		}
	}

	actions, err := json.MarshalIndent(logger.ActionItems, "", " ")
	if err != nil {
		testing.ContextLog(ctx, "Failed to convert actions to json format: ", err)
		return
	}
	if err := os.WriteFile(actionFilepath, actions, 0644); err != nil {
		testing.ContextLog(ctx, "Failed to save actions file: ", err)
	}
}

func saveLogger(ctx context.Context) error {
	f, err := os.Create(tempLoggerFile)
	if err != nil {
		return errors.Wrap(err, "error creating ActionLogger file")
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	err = enc.Encode(logger)
	if err != nil {
		return errors.Wrap(err, "error encoding ActionLogger")
	}
	return nil
}

func readLogger(ctx context.Context) error {
	f, err := os.Open(tempLoggerFile)
	if err != nil {
		return errors.Wrap(err, "error opening ActionLogger file")
	}
	defer f.Close()

	dec := gob.NewDecoder(f)
	logger = newActionLogger()
	err = dec.Decode(logger)
	if err != nil {
		return errors.Wrap(err, "error decoding ActionLogger")
	}
	return nil
}

func deleteLogger(ctx context.Context) error {
	err := os.Remove(tempLoggerFile)
	if err != nil {
		return errors.Wrap(err, "error deleting ActionLogger file")
	}
	return nil
}

// findCallingTest finds the path to the origin test or fixture file that called
// the action, and the line number where the call was made.
func findCallingTest(ctx context.Context) (path, lineNumber string, err error) {
	stack := string(debug.Stack())
	// Match to the calling test, e.g. bundles/cros/inputs/physical_keyboard_grammar_check.go:135
	// Doesn't work for helper functions.
	r := regexp.MustCompile(`(?m)^\s*(.*src\/platform\/tast-tests\/src\/go.chromium.org\/tast-tests\/cros\/local\/bundles\/[^\/]+\/[^\/]+\/[^\/]+\.go):(\d+)`)
	result := r.FindStringSubmatch(stack)
	if result == nil {
		return "", "", errors.New("found no matches searching the call stack for the calling test, this is expected if calling from a fixture")

	}
	expectedGroups := 3
	if len(result) != expectedGroups {
		return "", "", errors.Errorf("found an incorrect number of matches searching the call stack for the calling test. Got %d, want %d", len(result), expectedGroups)
	}

	path = result[1]
	lineNumber = result[2]

	// Convert to int for validation. We won't use the int.
	if _, err = strconv.Atoi(result[2]); err != nil {
		return "", "", errors.Errorf("failed to convert line number %q to integer: %v", err, lineNumber)
	}
	return path, lineNumber, nil
}

// Reset clears the action item list.
func Reset(ctx context.Context) {
	logger = newActionLogger()
	if !logger.ShouldRun {
		return
	}
	if err := saveLogger(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save logger file: ", err)
	}
}

// Save saves the action logs.
func Save(ctx context.Context) error {
	if !logger.ShouldRun {
		return nil
	}
	if err := readLogger(ctx); err != nil {
		return errors.Wrap(err, "cannot read logger, maybe it hasn't been initialized")
	}
	logger.save(ctx)
	if err := deleteLogger(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to delete logger file: ", err)
	}
	logger = nil
	return nil

}

// RecordClickAction is for mouse click or touchscreen tap actions.
// Assume the bounding box and point are in DPI.
func RecordClickAction(ctx context.Context, tconn *chrome.TestConn, actionName ActionName, uiTarget string, boundingBox coords.Rect, point coords.Point, button mouse.Button) {

	if alcommon.ShouldRun.Value() != "true" {
		return
	}
	dsf := displayScaleFactor(ctx, tconn)
	boundingBoxPX := coords.ConvertBoundsFromDPToPX(boundingBox, dsf)
	pointPX := coords.ConvertPointFromDPToPX(point, dsf)
	recordAction(
		ctx,
		tconn,
		actionName,
		uiTarget,
		&BoundingBox{
			X1: boundingBoxPX.TopLeft().X,
			Y1: boundingBoxPX.TopLeft().Y,
			X2: boundingBoxPX.BottomRight().X,
			Y2: boundingBoxPX.BottomRight().Y,
		},
		&ClickInput{
			X:      pointPX.X,
			Y:      pointPX.Y,
			Button: button,
		},
	)

}

func recordAction(ctx context.Context, tconn *chrome.TestConn, actionName ActionName, uiTarget string, boundingBox *BoundingBox, clickInput *ClickInput) {
	// Don't run in unit tests as the global var shouldRun is not initialized
	// there.
	if flag.Lookup("test.v") != nil {
		return
	}
	if alcommon.ShouldRun.Value() != "true" {
		return
	}
	if err := readLogger(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to read logger from tmp file")
		return
	}
	logger.recordAction(ctx, tconn, actionName, uiTarget, boundingBox, clickInput)

	// Set the output Dir, it has to be done here as Reset and Save are called
	// from remote fixture hook that has different output dir.
	var ok bool
	logger.OutDir, ok = testing.ContextOutDir(ctx)
	if !ok {
		testing.ContextLog(ctx, "Failed to get output dir")
	}
	if err := saveLogger(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save logger")
	}
}

// RecordClickAtLocationAction is for mouse click or touchscreen tap actions, without rectangle info.
// Assume the bounding box and point are in DPI.
func RecordClickAtLocationAction(ctx context.Context, tconn *chrome.TestConn, actionName ActionName, uiTarget string, point coords.Point, button mouse.Button) {
	if alcommon.ShouldRun.Value() != "true" {
		return
	}
	dsf := displayScaleFactor(ctx, tconn)
	pointPX := coords.ConvertPointFromDPToPX(point, dsf)
	clickInput := &ClickInput{
		X:      pointPX.X,
		Y:      pointPX.Y,
		Button: button,
	}
	recordAction(ctx, tconn, actionName, uiTarget, nil, clickInput)
}

// RecordKeyboardAction is for keyboard actions.
func RecordKeyboardAction(ctx context.Context, tconn *chrome.TestConn, actionName ActionName, uiTarget string) {
	recordAction(ctx, tconn, actionName, uiTarget, nil, nil)
}

// RecordWaitUIAction is for wait UI actions.
func RecordWaitUIAction(ctx context.Context, tconn *chrome.TestConn, actionName ActionName, uiTarget string) {
	recordAction(ctx, tconn, actionName, uiTarget, nil, nil)
}

func saveUITree(ctx context.Context, uiTree, dir, filename string) {
	filepath := path.Join(dir, filename)
	if err := os.WriteFile(filepath, []byte(uiTree), 0644); err != nil {
		testing.ContextLogf(ctx, "Failed to write UI tree file %s: %v", filename, err)
	}
}

// extractFilename gets the extractFilename from a path to a Go file.
func extractFilename(path string) (string, error) {
	r := regexp.MustCompile(`\/([^\/]+)\.go`)
	result := r.FindStringSubmatch(path)
	if result == nil {
		return "", errors.New("found no matches searching the call stack for the test name")
	}

	expectedGroups := 2
	if len(result) != expectedGroups {
		return "", errors.Errorf("found an incorrect number of matches searching the call stack for the calling test. Got %d, want %d", len(result), expectedGroups)
	}

	return result[1] + ".go", nil
}

func displayScaleFactor(ctx context.Context, tconn *chrome.TestConn) float64 {
	screens, err := display.GetInfo(ctx, tconn)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get the display info: ", err)
		return 0
	}

	// Find the ratio to convert coordinates in the screenshot to those in the screen.
	scaleFactor, err := screens[0].GetEffectiveDeviceScaleFactor()
	if err != nil {
		testing.ContextLog(ctx, "Failed to get the device scale factor: ", err)
		return 0
	}
	return scaleFactor
}
