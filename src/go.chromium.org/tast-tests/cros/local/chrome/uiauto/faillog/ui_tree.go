// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package faillog provides helper functions for dumping UI data on test failures.
package faillog

import (
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/testing"
)

const (
	faillogDir     = "faillog"
	uiTreeFileName = "ui_tree.txt"
)

// DumpUITreeWithScreenshotWithTestAPIOnError checks the given hasError function and dumps the whole UI tree data
// into 'filePrefix'.txt and a screenshot into 'filePrefix'.png when the test fails. It does nothing when the test succeeds.
func DumpUITreeWithScreenshotWithTestAPIOnError(ctx context.Context, outDir string, hasError func() bool, tconn *chrome.TestConn, filePrefix string) {
	SaveScreenshotToFileWithTestAPIOnError(ctx, tconn, outDir, hasError, filePrefix+".png")
	DumpUITreeOnErrorToFile(ctx, outDir, hasError, tconn, filePrefix+".txt")
}

// DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir checks the given hasError function and dumps the whole UI tree data
// into 'filePrefix'.txt and a screenshot into 'filePrefix'.png when the test fails. It does nothing when the test succeeds.
func DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(ctx context.Context, hasError func() bool, tconn *chrome.TestConn, filePrefix string) {
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		testing.ContextLog(ctx, "Failed to obtain context output directory")
		return
	}
	DumpUITreeWithScreenshotWithTestAPIOnError(ctx, outDir, hasError, tconn, filePrefix)
}

// DumpUITreeWithScreenshotOnError checks the given hasError function and dumps the whole UI tree data
// into 'filePrefix'.txt and a screenshot into 'filePrefix'.png when the test fails. It does nothing when the test succeeds.
func DumpUITreeWithScreenshotOnError(ctx context.Context, outDir string, hasError func() bool, cr *chrome.Chrome, filePrefix string) {
	if !hasError() {
		return
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to obtain test API conn to dump UI tree: ", err)
		return
	}
	DumpUITreeWithScreenshotWithTestAPIOnError(ctx, outDir, hasError, tconn, filePrefix)
}

// DumpUITreeOnError dumps tree to 'ui_tree.txt', when the test fails.
// Use DumpUITreeOnErrorToFile, if you want to specify the fileName.
func DumpUITreeOnError(ctx context.Context, outDir string, hasError func() bool, tconn *chrome.TestConn) {
	DumpUITreeOnErrorToFile(ctx, outDir, hasError, tconn, uiTreeFileName)
}

// DumpUITreeOnErrorToFile checks the given hasError function and dumps the whole UI tree data
// into a file 'fileName' when the test fails. It does nothing when the test succeeds.
// TODO(b/201247306): The dump content may not include ARC UI tree due to timing issue.
func DumpUITreeOnErrorToFile(ctx context.Context, outDir string, hasError func() bool, tconn *chrome.TestConn, fileName string) {
	if !hasError() {
		return
	}

	DumpUITreeToFile(ctx, outDir, tconn, fileName)
}

// DumpUITree Dumps the whole UI tree data to 'ui_tree.txt'.
func DumpUITree(ctx context.Context, outDir string, tconn *chrome.TestConn) {
	DumpUITreeToFile(ctx, outDir, tconn, uiTreeFileName)
}

// DumpUITreeToFile Dumps the whole UI tree data into a file 'fileName'.
func DumpUITreeToFile(ctx context.Context, outDir string, tconn *chrome.TestConn, fileName string) {
	dir := filepath.Join(outDir, faillogDir)
	if err := os.MkdirAll(dir, 0777); err != nil {
		testing.ContextLogf(ctx, "Failed to create directory %s: %v", dir, err)
		return
	}

	filePath := filepath.Join(dir, fileName)
	testing.ContextLog(ctx, "Test failed. Dumping the automation node tree into ", fileName)
	if err := uiauto.LogRootDebugInfo(ctx, tconn, filePath); err != nil {
		testing.ContextLog(ctx, "Failed to dump: ", err)
	}
}
