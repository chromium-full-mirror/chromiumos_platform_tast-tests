// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
)

const (
	// TopazAppName is the name of the Topaz application.
	TopazAppName AppName = "DemoOCX32"

	topazClearIcon = "citrix/topaz_clear.png"
)

// TopazSignaturePad implements the SignaturePad interface for Topaz.
type TopazSignaturePad struct {
	ud       *uidetection.Context
	kb       *input.KeyboardEventWriter
	tconn    *chrome.TestConn
	dataPath func(string) string
}

// NewTopazSignaturePad creates a new TopazSignaturePad instance.
func NewTopazSignaturePad(ud *uidetection.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, dataPath func(string) string) *TopazSignaturePad {
	return &TopazSignaturePad{ud: ud, kb: kb, tconn: tconn, dataPath: dataPath}
}

// GetCanvasBounds returns Topaz canvas bounds.
func (t *TopazSignaturePad) GetCanvasBounds(ctx context.Context) (coords.Rect, error) {
	appTitleText := uidetection.TextBlockFromSentence("Topaz SigPlus Demonstration").First()
	topazClearCanvas := uidetection.CustomIcon(t.dataPath(topazClearIcon)).Below(appTitleText)
	return getSignaturePadCanvasBounds(ctx, t.ud, t.tconn, t.dataPath, topazClearCanvas)
}

// StartSignature starts Topaz signature.
func (t *TopazSignaturePad) StartSignature() uiauto.Action {
	ud := t.ud
	startText := uidetection.Word("Start").First()
	return uiauto.NamedCombine("start Topaz signature",
		t.ClearSignature(),
		ud.LeftClick(startText),
	)
}

// SaveSignature saves Topaz signature.
func (t *TopazSignaturePad) SaveSignature(fileName string) uiauto.Action {
	ud := t.ud
	// Sometimes "Save Sig" will detect the wrong coordinate, just use "Save".
	saveSigText := uidetection.Word("Save").First()
	saveAsText := uidetection.TextBlockFromSentence("Save As").First()
	quickAccessText := uidetection.TextBlockFromSentence("Quick access").First()
	desktopText := uidetection.Word("Desktop").Below(quickAccessText).First()
	saveAsTypeText := uidetection.TextBlockFromSentence("Save as type").First()
	saveText := uidetection.Word("Save").Below(saveAsTypeText).First()
	return uiauto.NamedCombine("save signature",
		ud.LeftClickUntil(saveSigText, ud.Exists(saveAsText)),
		t.kb.TypeAction(fileName),
		ud.LeftClick(desktopText),
		ud.LeftClickUntil(saveText, ud.Gone(saveAsText)),
	)
}

// ClearSignature clears Topaz signature.
func (t *TopazSignaturePad) ClearSignature() uiauto.Action {
	ud := t.ud
	clearText := uidetection.Word("Clear").First()
	topazClearCanvas := uidetection.CustomIcon(t.dataPath(topazClearIcon))
	return uiauto.NamedAction("clear signature",
		ud.LeftClickUntil(clearText, ud.Exists(topazClearCanvas)),
	)
}

// LoadSignature loads Topaz signature.
func (t *TopazSignaturePad) LoadSignature(fileName string) uiauto.Action {
	ud := t.ud
	// Sometimes "Load Sig" will detect the wrong coordinate, just use "Load".
	loadSigText := uidetection.Word("Load").First()
	nameText := uidetection.Word("Name").First()
	signaturesText := uidetection.Word("Signatures").First()
	openText := uidetection.Word("Open").Below(signaturesText).First()
	fileNameText := uidetection.Word(fileName).Below(nameText).Above(openText).First()
	signatureText := uidetection.Word("Signatures").First()
	defaultText := uidetection.Word("*.SIG").LeftOf(signatureText).First()
	return uiauto.NamedCombine("load signature",
		ud.LeftClickUntil(loadSigText, ud.Exists(openText)),
		ud.LeftClickUntil(fileNameText, ud.Gone(defaultText)),
		ud.LeftClickUntil(openText, ud.Gone(openText)),
	)
}

var _ SignaturePad = (*TopazSignaturePad)(nil)
