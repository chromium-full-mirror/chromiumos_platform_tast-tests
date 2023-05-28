// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/flashrom"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterBiosServiceServer(srv, &BiosService{s: s})
		},
	})
}

// BiosService implements tast.cros.firmware.BiosService.
type BiosService struct {
	s *testing.ServiceState
}

// programmerEnumToProgrammer maps the enum from FWSectionInfo to a bios FlashromProgramer.
var programmerEnumToProgrammer = map[pb.Programmer]flashrom.Programmer{
	pb.Programmer_BIOSProgrammer: flashrom.ProgrammerHost,
	pb.Programmer_ECProgrammer:   flashrom.ProgrammerEc,
}

// sectionEnumToSection maps the enum from FWSectionInfo to a bios ImageSection.
var sectionEnumToSection = map[pb.ImageSection]bios.ImageSection{
	pb.ImageSection_EmptyImageSection: bios.EmptyImageSection,

	pb.ImageSection_BOOTSTUBImageSection:        bios.BOOTSTUBImageSection,
	pb.ImageSection_COREBOOTImageSection:        bios.COREBOOTImageSection,
	pb.ImageSection_SHAREDDATAImageSection:      bios.SHAREDDATAImageSection,
	pb.ImageSection_FMAPImageSection:            bios.FMAPImageSection,
	pb.ImageSection_SMMSTOREImageSection:        bios.SMMSTOREImageSection,
	pb.ImageSection_FPFSTATUSImageSection:       bios.FPFSTATUSImageSection,
	pb.ImageSection_DEVICEEXTENSIONImageSection: bios.DEVICEEXTENSIONImageSection,
	pb.ImageSection_UNUSEDHOLEImageSection:      bios.UNUSEDHOLEImageSection,
	pb.ImageSection_IFWIImageSection:            bios.IFWIImageSection,

	pb.ImageSection_RECOVERYMRCCACHEImageSection: bios.RECOVERYMRCCACHEImageSection,
	pb.ImageSection_UNIFIEDMRCCACHEImageSection:  bios.UNIFIEDMRCCACHEImageSection,
	pb.ImageSection_RWMRCCACHEImageSection:       bios.RWMRCCACHEImageSection,
	pb.ImageSection_RWVARMRCCACHEImageSection:    bios.RWVARMRCCACHEImageSection,

	pb.ImageSection_RWELOGImageSection:   bios.RWELOGImageSection,
	pb.ImageSection_RWSHAREDImageSection: bios.RWSHAREDImageSection,
	pb.ImageSection_RWNVRAMImageSection:  bios.RWNVRAMImageSection,

	pb.ImageSection_ECRWImageSection:  bios.ECRWImageSection,
	pb.ImageSection_ECRWBImageSection: bios.ECRWBImageSection,
	pb.ImageSection_ECROImageSection:  bios.ECROImageSection,

	pb.ImageSection_APWPROImageSection:       bios.APWPROImageSection,
	pb.ImageSection_APROImageSection:         bios.APROImageSection,
	pb.ImageSection_GBBImageSection:          bios.GBBImageSection,
	pb.ImageSection_FRMAINImageSection:       bios.FRMAINImageSection,
	pb.ImageSection_RWLEGACYImageSection:     bios.RWLEGACYImageSection,
	pb.ImageSection_RWMISCImageSection:       bios.RWMISCImageSection,
	pb.ImageSection_MISCRWImageSection:       bios.MISCRWImageSection,
	pb.ImageSection_BIOSUNUSABLEImageSection: bios.BIOSUNUSABLEImageSection,
	pb.ImageSection_RWPRESERVEImageSection:   bios.RWPRESERVEImageSection,
	pb.ImageSection_VBLOCKDEVImageSection:    bios.VBLOCKDEVImageSection,

	pb.ImageSection_ROFRIDImageSection:    bios.ROFRIDImageSection,
	pb.ImageSection_ROFRIDPADImageSection: bios.ROFRIDPADImageSection,
	pb.ImageSection_RWFWIDImageSection:    bios.RWFWIDImageSection,

	pb.ImageSection_FWSignAImageSection: bios.FWSignAImageSection,
	pb.ImageSection_FWBodyAImageSection: bios.FWBodyAImageSection,
	pb.ImageSection_APRWAImageSection:   bios.APRWAImageSection,
	pb.ImageSection_RWFWIDAImageSection: bios.RWFWIDAImageSection,

	pb.ImageSection_FWSignBImageSection: bios.FWSignBImageSection,
	pb.ImageSection_FWBodyBImageSection: bios.FWBodyBImageSection,
	pb.ImageSection_APRWBImageSection:   bios.APRWBImageSection,
	pb.ImageSection_RWFWIDBImageSection: bios.RWFWIDBImageSection,

	pb.ImageSection_ROVPDImageSection: bios.ROVPDImageSection,
	pb.ImageSection_RWVPDImageSection: bios.RWVPDImageSection,

	pb.ImageSection_IntelCSERWAImageSection: bios.IntelCSERWAImageSection,
	pb.ImageSection_IntelCSERWBImageSection: bios.IntelCSERWBImageSection,

	pb.ImageSection_SIALLImageSection:  bios.SIALLImageSection,
	pb.ImageSection_SIDESCImageSection: bios.SIDESCImageSection,
	pb.ImageSection_SIMEImageSection:   bios.SIMEImageSection,
	pb.ImageSection_SIBIOSImageSection: bios.SIBIOSImageSection,

	pb.ImageSection_SignedAMDFWAImageSection: bios.SignedAMDFWAImageSection,
	pb.ImageSection_SignedAMDFWBImageSection: bios.SignedAMDFWBImageSection,
}

// updateModeEnumtoMode maps the enum from FirmwareUpdateModeRequest to a bios FirmwareUpdateMode.
var updateModeEnumtoMode = map[pb.UpdateMode]bios.FirmwareUpdateMode{
	pb.UpdateMode_RecoveryMode: bios.RecoveryMode,
}

// BackupImageSection dumps the image region into temporary file locally and returns its path.
func (*BiosService) BackupImageSection(ctx context.Context, req *pb.FWSectionInfo) (*pb.FWSectionInfo, error) {
	var flashromConfig flashrom.Config
	flashromInstance, ctx, cleanup, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(programmerEnumToProgrammer[req.Programmer], "").
		Probe(ctx)
	defer cleanup()

	if err != nil {
		return nil, errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	path, err := bios.NewImageToFile(ctx, sectionEnumToSection[req.Section], flashromInstance, req.Path)
	if err != nil {
		return nil, errors.Wrapf(err, "could not backup %s region with programmer %s", sectionEnumToSection[req.Section], programmerEnumToProgrammer[req.Programmer])
	}
	return &pb.FWSectionInfo{Path: path, Section: req.Section, Programmer: req.Programmer}, nil
}

// RestoreImageSection restores image region from temporary file locally and restores fw with it.
func (bs *BiosService) RestoreImageSection(ctx context.Context, req *pb.FWSectionInfo) (*empty.Empty, error) {
	var flashromConfig flashrom.Config
	flashromInstance, ctx, cleanup, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(programmerEnumToProgrammer[req.Programmer], "").
		Probe(ctx)
	defer cleanup()

	if err != nil {
		return nil, errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	if req.Section != pb.ImageSection_EmptyImageSection {
		if err := bios.WriteImageFromSingleSectionFile(ctx, req.Path, sectionEnumToSection[req.Section], flashromInstance); err != nil {
			return nil, errors.Wrapf(err, "could not restore %q region with programmer %q from path %q", sectionEnumToSection[req.Section], programmerEnumToProgrammer[req.Programmer], req.Path)
		}
	} else {
		if err := bios.WriteImageFromMultiSectionFile(ctx, req.Path, sectionEnumToSection[req.Section], flashromInstance); err != nil {
			return nil, errors.Wrapf(err, "could not restore %q region with programmer %q from path %q", sectionEnumToSection[req.Section], programmerEnumToProgrammer[req.Programmer], req.Path)
		}
	}
	return &empty.Empty{}, nil
}

// SetAPSoftwareWriteProtect sets the AP software write protect.
func (bs *BiosService) SetAPSoftwareWriteProtect(ctx context.Context, req *pb.WPRequest) (*empty.Empty, error) {
	if err := bios.SetAPSoftwareWriteProtect(ctx, req.Enable); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

func copyAndFlash(ctx context.Context, img *bios.Image, req *pb.FWSectionInfo, flashromInstance *flashrom.Instance) (*pb.FWSectionInfo, error) {
	// Save copy of data to file before writing.
	imgPath, err := img.WriteImageToFile(ctx, sectionEnumToSection[req.Section], req.Path)
	if err != nil {
		return nil, errors.Wrap(err, "failed writing image contents to file")
	}
	// Delete temporary image file if saving not requested with req.Path.
	if req.Path == "" {
		defer os.Remove(imgPath)
	}

	// Write image with flashrom.
	err = bios.WriteImageFromSingleSectionFile(ctx, imgPath, sectionEnumToSection[req.Section], flashromInstance)
	if err != nil {
		return nil, errors.Wrap(err, "could not write firmware")
	}

	// Return path to fw file if save path provided.
	if req.Path == "" {
		return &pb.FWSectionInfo{Section: req.Section, Programmer: req.Programmer}, nil
	}
	return &pb.FWSectionInfo{Path: imgPath, Section: req.Section, Programmer: req.Programmer}, nil
}

// CorruptFWSection writes garbage over part of the specified firmware section.
// Provide a dir to save corrupted image in the request, else temp image file will be cleaned up.
func (bs *BiosService) CorruptFWSection(ctx context.Context, req *pb.FWSectionInfo) (*pb.FWSectionInfo, error) {
	var flashromConfig flashrom.Config
	flashromInstance, ctx, cleanup, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(programmerEnumToProgrammer[req.Programmer], "").
		Probe(ctx)
	defer cleanup()

	if err != nil {
		return nil, errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	img, err := bios.NewImage(ctx, sectionEnumToSection[req.Section], flashromInstance)
	if err != nil {
		return nil, errors.Wrap(err, "could not read firmware")
	}
	for i, v := range img.Data {
		img.Data[i] = (v + 1) & 0xff
	}

	return copyAndFlash(ctx, img, req, flashromInstance)
}

// WriteImageFromMultiSectionFile writes the provided multi section file in the specified section.
func (bs *BiosService) WriteImageFromMultiSectionFile(ctx context.Context, req *pb.FWSectionInfo) (*empty.Empty, error) {
	var flashromConfig flashrom.Config
	flashromInstance, ctx, cleanup, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(programmerEnumToProgrammer[req.Programmer], "").
		Probe(ctx)
	defer cleanup()

	if err != nil {
		return nil, errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	if err := bios.WriteImageFromMultiSectionFile(ctx, req.Path, sectionEnumToSection[req.Section], flashromInstance); err != nil {
		return nil, errors.Wrapf(err, "could not write %s region with programmer %s from path %s", sectionEnumToSection[req.Section], programmerEnumToProgrammer[req.Programmer], req.Path)
	}
	return &empty.Empty{}, nil
}

// ChromeosFirmwareUpdate will perform the firmware update in the desired mode.
func (*BiosService) ChromeosFirmwareUpdate(ctx context.Context, req *pb.FirmwareUpdateModeRequest) (*empty.Empty, error) {
	switch req.Options {
	case "":
		if err := bios.ChromeosFirmwareUpdate(ctx, updateModeEnumtoMode[req.Mode]); err != nil {
			return nil, err
		}
	default:
		if err := bios.ChromeosFirmwareUpdate(ctx, updateModeEnumtoMode[req.Mode], req.Options); err != nil {
			return nil, err
		}
	}
	return &empty.Empty{}, nil
}

func (bs *BiosService) ParseFMAP(ctx context.Context, req *pb.FMAP) (*pb.FMAP, error) {
	var flashromConfig flashrom.Config
	flashromInstance, ctx, cleanup, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(programmerEnumToProgrammer[req.Programmer], "").
		Probe(ctx)
	defer cleanup()

	if err != nil {
		return nil, errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}

	fmap, err := bios.ParseFMAP(ctx, flashromInstance)
	if err != nil {
		return nil, err
	}
	req.Fmap = fmap
	return req, nil
}

// CorruptCBFSFWSection corrupts CBFS file in the specified way and in specified section.
// Provide a dir to save corrupted image in the request, else temp image file will be cleaned up.
func (bs *BiosService) CorruptCBFSFWSection(ctx context.Context, req *pb.CBFSCorruptInfo) (*pb.FWSectionInfo, error) {
	if req.Type == pb.CBFSCorruptType_NONE {
		return nil, errors.New("NONE corruption type is not allowed")
	}

	var flashromConfig flashrom.Config
	flashromInstance, ctx, cleanup, _, err := flashromConfig.
		FlashromInit("").
		ProgrammerInit(programmerEnumToProgrammer[req.SectionInfo.Programmer], "").
		Probe(ctx)
	defer cleanup()

	if err != nil {
		return nil, errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
	}
	img, err := bios.NewImage(ctx, sectionEnumToSection[req.SectionInfo.Section], flashromInstance)
	if err != nil {
		return nil, errors.Wrap(err, "could not read firmware")
	}

	var corruptorWalker bios.CBFSWalker

	switch req.Type {
	case pb.CBFSCorruptType_MAGIC:
		corruptorWalker = func(ctxb bios.CBFSWalkerContext) bool {
			header := ctxb.File.Header
			for i, v := range header.Magic {
				header.Magic[i] = v + 1 // Corrupt file magic so file is not recognized.
			}

			buf := new(bytes.Buffer)
			binary.Write(buf, bios.CBFSEndianness, header)
			copy(ctxb.Buffer, buf.Bytes())

			return true
		}
	case pb.CBFSCorruptType_LENGTH:
		corruptorWalker = func(ctxb bios.CBFSWalkerContext) bool {
			header := ctxb.File.Header
			header.Len = ^uint32(0) - 1 // Set file data length to UINT32_MAX - 1.

			buf := new(bytes.Buffer)
			binary.Write(buf, bios.CBFSEndianness, header)
			copy(ctxb.Buffer, buf.Bytes())

			return true
		}
	case pb.CBFSCorruptType_ATTRIBUTES:
		corruptorWalker = func(ctxb bios.CBFSWalkerContext) bool {
			header := ctxb.File.Header

			if header.AttributesOffset == 0 {
				return true
			}

			for i := int(header.AttributesOffset); i < int(header.Offset); i++ {
				ctxb.Buffer[i] = ctxb.Buffer[i] + 1 // Corrupt whole attributes data range.
			}

			return true
		}
	case pb.CBFSCorruptType_DATA:
		corruptorWalker = func(ctxb bios.CBFSWalkerContext) bool {
			header := ctxb.File.Header

			for i := int(header.Offset); i < int(header.Offset+header.Len); i++ {
				ctxb.Buffer[i] = ctxb.Buffer[i] + 1 // Corrupt whole file data.
			}

			return true
		}
	default:
		return nil, errors.Errorf("%v corruption type is not allowed", req.Type.String())
	}

	if err := bios.CBFSWalk(img.Data, bios.CBFSCreateSingleFileWalker(req.Filename, corruptorWalker)); err != nil {
		return nil, errors.Wrap(err, "failed to work on CBFS")
	}

	return copyAndFlash(ctx, img, req.SectionInfo, flashromInstance)
}
