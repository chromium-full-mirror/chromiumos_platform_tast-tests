// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pdfocr provides constants that are used in tests interacting with the
// PDF OCR feature on ChromeOS.
package pdfocr

// Strings used in go.chromium.org/tast-tests/cros/local/bundles/cros/a11y/pdfocr*.go
const (
	// Status node message when PDF OCR finished converting image to text
	StatusReadyMessage = "Image converted to text"
	// Testing PDF's filename
	TestPDFName = "inaccessible-text.pdf"
	// Inaccessible text embedded in an image in the testing PDF file
	TextInPDFImage = "Hello, world!"
)
