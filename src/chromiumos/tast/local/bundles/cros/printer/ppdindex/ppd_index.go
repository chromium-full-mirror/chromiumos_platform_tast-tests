// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ppdindex implements a server that mimics the functionality of the
// real PPD Index.  The server works in conjunction with the
// printing-ppd-channel=localhost flag.  Example usage:
//
// server := ppdindex.New(s)
// defer server.Stop()
// server.AddEntry(ppdindex.Entry{Manufacturer: "Brother", Model: "Brother DCP-1200"})
// server.AddEntry(ppdindex.Entry{Manufacturer: "Xerox", Model: "Xerox	B230", License: "xerox-printing-license"})
//
// When adding entries using AddEntry, the Entry Manufacturer and Model are
// required.  License and Ppd are optional.  If License is not provided it means
// there is no license for that PPD, otherwise, it is a string containing the
// name of the license.  If Ppd is empty a default PPD will be used, otherwise,
// Ppd needs to be a string containing VALID PPD data.  ppdFilename is only used
// internally and should not be provided by a user.
//
// This is expected to be used in a controlled testing environment.  For
// example, it does not handle concurrent requests.
package ppdindex

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"chromiumos/tast/testing"
)

// Entry is used to add new PPD entries and metadata to the PPD Index Server.
type Entry struct {
	Manufacturer string
	Model        string
	License      string
	Ppd          string
	ppdFilename  string
}

func (entry *Entry) createManufacturerMetadataName() string {
	return fmt.Sprintf("%s-en.json", entry.Manufacturer)
}

func extractManufacturerFromMetadataName(name string) string {
	return strings.TrimSuffix(name, "-en.json")
}

func (entry *Entry) createEmm() string {
	emm := fmt.Sprintf("%s-%s", entry.Manufacturer, entry.Model)
	emm = strings.ReplaceAll(emm, " ", "_")
	emm = strings.ToLower(emm)
	return emm
}

// Server will create a local server that functions similarly to the actual PPD
// index.
type Server struct {
	server http.Server
	// A map with Manufacturer as the key and a list of Entries for that
	// manufacturer as the value.
	entries map[string][]Entry
	// A map with ppdFilename as the key (not an actual file on disk - rather just
	// a name to distinguish between different PPD data.  For example, the
	// ppdFilename data member of the Entry) and the PPD contents as the value.
	ppdData map[string]string
}

const serverAddress = ":7002"
const defaultPpdFilename = "default-ppd.ppd"
const defaultPpdData = `*PPD-Adobe: "4.3"
*FormatVersion: "4.3"
*FileVersion: "1.0"
*LanguageVersion: English
*LanguageEncoding: ISOLatin1
*PCFileName: "SAMPLE.PPD"
*Product: "(Sample)"
*PSVersion: "(1) 1"
*ModelName: "Sample"
*ShortNickName: "Sample"
*NickName: "Sample"
*Manufacturer: "Sample"
*OpenUI *PageSize: PickOne
*DefaultPageSize: A4
*PageSize A4/A4: "<</PageSize[595.20 841.68]>>setpagedevice"
*CloseUI: *PageSize
*OpenUI *PageRegion: PickOne
*DefaultPageRegion: A4
*PageRegion A4/A4: "<</PageRegion[595.20 841.68]>>setpagedevice"
*CloseUI: *PageRegion
*DefaultImageableArea: A4
*ImageableArea A4/A4: "8.40 8.40 586.80 833.28"
*DefaultPaperDimension: A4
*PaperDimension A4/A4: "595.20 841.68"`

func (server *Server) fetchLocales(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"locales":["de", "en", "es"]}`))
}

func (server *Server) fetchManufacturers(w http.ResponseWriter, req *http.Request) {
	var sb strings.Builder
	sb.WriteString(`{"filesMap": {`)
	firstEntry := true
	for k, v := range server.entries {
		if !firstEntry {
			sb.WriteString(",")
		}
		firstEntry = false
		fmt.Fprintf(&sb, `"%s": "%s"`, k, v[0].createManufacturerMetadataName())
	}
	sb.WriteString("}}")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(sb.String()))
}

func (server *Server) fetchPrinters(w http.ResponseWriter, req *http.Request) {
	manufacturerMedatdataName := path.Base(req.URL.Path)
	manufacturer := extractManufacturerFromMetadataName(manufacturerMedatdataName)

	var sb strings.Builder
	sb.WriteString(`{"printers": [ `)
	firstEntry := true
	printers := server.entries[manufacturer]
	for _, entry := range printers {
		if !firstEntry {
			sb.WriteString(",")
		}
		firstEntry = false
		fmt.Fprintf(&sb, `{"name": "%s","emm": "%s"}`, entry.Model, entry.createEmm())
	}
	sb.WriteString("] }")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(sb.String()))
}

func (server *Server) fetchIndex(w http.ResponseWriter, req *http.Request) {
	var sb strings.Builder
	sb.WriteString(`{"ppdIndex": {`)
	firstEntry := true
	// Loop over all of our manufacturers.
	for _, entries := range server.entries {
		// Loop over all printers for that manufacturer.
		for _, entry := range entries {
			if !firstEntry {
				sb.WriteString(",")
			}
			firstEntry = false
			fmt.Fprintf(&sb, `"%s": {"ppdMetadata": [ {"name": "%s"`,
				entry.createEmm(), entry.ppdFilename)
			if len(entry.License) > 0 {
				fmt.Fprintf(&sb, `, "license": "%s"`, entry.License)
			}
			sb.WriteString("} ] }")
		}
	}
	sb.WriteString("}}")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(sb.String()))
}

func (server *Server) fetchReverseIndex(w http.ResponseWriter, req *http.Request) {
	var sb strings.Builder
	sb.WriteString(`{"locale": "en", "reverseIndex": {`)
	firstEntry := true
	// Loop over all manufacturers.
	for _, entries := range server.entries {
		// Loop over all printers for that manufacturer.
		for _, entry := range entries {
			if !firstEntry {
				sb.WriteString(",")
			}
			firstEntry = false
			fmt.Fprintf(&sb, `"%s": { "manufacturer": "%s", "model": "%s"}`,
				entry.createEmm(), entry.Manufacturer, entry.Model)
		}
	}
	sb.WriteString("}}")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(sb.String()))
}

func (server *Server) fetchPpd(w http.ResponseWriter, req *http.Request) {
	ppdFilename := path.Base(req.URL.Path)

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	w.Write([]byte(server.ppdData[ppdFilename]))
}

// New creates and starts a PPD Index server.
func New(s *testing.State) *Server {
	// Normally this isn't needed when creating an HTTP server.  However, when
	// this runs in tast, if there are multiple tests running that use this server
	// we need to clear this out so we don't add duplicate patterns to the
	// HandleFunc functions.
	http.DefaultServeMux = new(http.ServeMux)
	server := &Server{http.Server{Addr: serverAddress},
		make(map[string][]Entry), make(map[string]string)}

	http.HandleFunc("/metadata_v3/locales.json", server.fetchLocales)
	http.HandleFunc("/metadata_v3/manufacturers-en.json", server.fetchManufacturers)

	// Instead of figuring out where each emm hashes to, just make all the shards
	// serve the same data.
	kNumShards := 20
	for i := 0; i < kNumShards; i++ {
		http.HandleFunc(fmt.Sprintf("/metadata_v3/index-%02d.json", i), server.fetchIndex)
		http.HandleFunc(fmt.Sprintf("/metadata_v3/reverse_index-en-%02d.json", i), server.fetchReverseIndex)
	}

	s.Log("Starting PPD Index server")
	go func(server *Server) {
		if err := server.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Fatal("Error during ListenAndServe: ", err)
		}
	}(server)

	return server
}

// AddEntry adds the given entry to our PPD Index.
func (server *Server) AddEntry(entry Entry) {
	// We can only add a handler function once for each manufacturer.
	if _, exists := server.entries[entry.Manufacturer]; !exists {
		http.HandleFunc(fmt.Sprintf("/metadata_v3/%s-en.json", entry.Manufacturer),
			server.fetchPrinters)
	}

	// If the entry has an empty PPD string, use the default PPD.
	if len(entry.Ppd) == 0 {
		entry.ppdFilename = defaultPpdFilename
		entry.Ppd = defaultPpdData
	} else {
		entry.ppdFilename = fmt.Sprintf("ppd-%d.ppd", len(server.ppdData))
	}

	if _, exists := server.ppdData[entry.ppdFilename]; !exists {
		server.ppdData[entry.ppdFilename] = entry.Ppd
		http.HandleFunc(fmt.Sprintf("/ppds_for_metadata_v3/%s", entry.ppdFilename), server.fetchPpd)
	}

	server.entries[entry.Manufacturer] =
		append(server.entries[entry.Manufacturer], entry)
}

// Stop will stop the server and close all connections.
func (server *Server) Stop() {
	server.server.Close()
}
