// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bios

import (
	"bytes"
	"encoding/binary"
	"io"

	"go.chromium.org/tast/core/errors"
)

// CBFSFileCompression describes compression algorithm.
type CBFSFileCompression uint32

// Available compression types.
const (
	CBFSFileCompressionNone CBFSFileCompression = 0
	CBFSFileCompressionLZMA CBFSFileCompression = 1
	CBFSFileCompressionLZ4  CBFSFileCompression = 2
)

// Enum value maps for CBFSFileCompression.
var (
	cbfsFileCompressionName = map[CBFSFileCompression]string{
		CBFSFileCompressionNone: "None",
		CBFSFileCompressionLZMA: "LZMA",
		CBFSFileCompressionLZ4:  "LZ4",
	}
)

func (x CBFSFileCompression) String() string {
	if val, ok := cbfsFileCompressionName[x]; ok {
		return val
	}
	return "unknown"
}

// CBFSFileType describes CBFS file type.
type CBFSFileType uint32

// Commonly used CBFS file types.
const (
	CBFSFileTypeQuery       CBFSFileType = 0
	CBFSFileTypeDeleted     CBFSFileType = 0
	CBFSFileTypeNull        CBFSFileType = 0xffffffff
	CBFSFileTypeBootblock   CBFSFileType = 0x01
	CBFSFileTypeCBFSHeader  CBFSFileType = 0x02
	CBFSFileTypeLegacyStage CBFSFileType = 0x10
	CBFSFileTypeStage       CBFSFileType = 0x11
	CBFSFileTypeSELF        CBFSFileType = 0x20
	CBFSFileTypeFITPayload  CBFSFileType = 0x21
	CBFSFileTypeOptionROM   CBFSFileType = 0x30
	CBFSFileTypeBootsplash  CBFSFileType = 0x40
	CBFSFileTypeRaw         CBFSFileType = 0x50
	CBFSFileTypeVSA         CBFSFileType = 0x51
	CBFSFileTypeMBI         CBFSFileType = 0x52
	CBFSFileTypeMicrocode   CBFSFileType = 0x53
	CBFSFileTypeIntelFIT    CBFSFileType = 0x54
	CBFSFileTypeFSP         CBFSFileType = 0x60
	CBFSFileTypeMRC         CBFSFileType = 0x61
	CBFSFileTypeMMA         CBFSFileType = 0x62
	CBFSFileTypeEFI         CBFSFileType = 0x63
	CBFSFileTypeStruct      CBFSFileType = 0x70
	CBFSFileTypeAMDFW       CBFSFileType = 0x80
	CBFSFileTypeCMOSDefault CBFSFileType = 0xaa
	CBFSFileTypeSPD         CBFSFileType = 0xab
	CBFSFileTypeMRCCache    CBFSFileType = 0xac
	CBFSFileTypeCMOSLayout  CBFSFileType = 0x01aa
)

// Enum value maps for CBFSFileType.
var (
	cbfsFileTypeName = map[CBFSFileType]string{
		CBFSFileTypeDeleted:     "Deleted",
		CBFSFileTypeNull:        "Null",
		CBFSFileTypeBootblock:   "Bootblock",
		CBFSFileTypeCBFSHeader:  "CBFSHeader",
		CBFSFileTypeLegacyStage: "LegacyStage",
		CBFSFileTypeStage:       "Stage",
		CBFSFileTypeSELF:        "SELF",
		CBFSFileTypeFITPayload:  "FITPayload",
		CBFSFileTypeOptionROM:   "OptionROM",
		CBFSFileTypeBootsplash:  "Bootsplash",
		CBFSFileTypeRaw:         "Raw",
		CBFSFileTypeVSA:         "VSA",
		CBFSFileTypeMBI:         "MBI",
		CBFSFileTypeMicrocode:   "Microcode",
		CBFSFileTypeIntelFIT:    "IntelFIT",
		CBFSFileTypeFSP:         "FSP",
		CBFSFileTypeMRC:         "MRC",
		CBFSFileTypeMMA:         "MMA",
		CBFSFileTypeEFI:         "EFI",
		CBFSFileTypeStruct:      "Struct",
		CBFSFileTypeAMDFW:       "AMDFW",
		CBFSFileTypeCMOSDefault: "CMOSDefault",
		CBFSFileTypeSPD:         "SPD",
		CBFSFileTypeMRCCache:    "MRCCache",
		CBFSFileTypeCMOSLayout:  "CMOSLayout",
	}
)

func (x CBFSFileType) String() string {
	if val, ok := cbfsFileTypeName[x]; ok {
		return val
	}
	return "unknown"
}

// CBFSAlignment describes CBFS files alignment in flash.
const CBFSAlignment = 64

// CBFSFileMagic is CBFS file magic value.
const CBFSFileMagic = "LARCHIVE"

// CBFSMetadataMaxSize is maximum size of CBFS file header with attributes.
const CBFSMetadataMaxSize = 256

// CBFSFileHeader describes basic CBFS file header fields.
type CBFSFileHeader struct {
	Magic            [8]byte
	Len              uint32
	FileType         CBFSFileType
	AttributesOffset uint32
	Offset           uint32
}

// CBFSFileHeaderSize = sizeof(struct cbfs_file).
const CBFSFileHeaderSize = 24

// CBFSFileAttributeTag describes attribute type.
type CBFSFileAttributeTag uint32

// Commonly used CBFS file attribute types.
const (
	CBFSFileAttributeTagUnused      CBFSFileAttributeTag = 0
	CBFSFileAttributeTagUnused2     CBFSFileAttributeTag = 0xffffffff
	CBFSFileAttributeTagCompression CBFSFileAttributeTag = 0x42435a4c // BE: 'BCZL'.
	CBFSFileAttributeTagHash        CBFSFileAttributeTag = 0x68736148 // BE: 'hsaH'.
	CBFSFileAttributeTagPosition    CBFSFileAttributeTag = 0x42435350 // BE: 'BCSP'.
	CBFSFileAttributeTagAlignment   CBFSFileAttributeTag = 0x42434c41 // BE: 'BCLA'.
	CBFSFileAttributeTagIBB         CBFSFileAttributeTag = 0x32494242 // BE: '2IBB'.
	CBFSFileAttributeTagPadding     CBFSFileAttributeTag = 0x47444150 // BE: 'GNDP'.
	CBFSFileAttributeTagStageHeader CBFSFileAttributeTag = 0x53746748 // BE: 'StgH'.
)

// Enum value maps for CBFSFileCompression.
var (
	cbfsFileAttributeTagName = map[CBFSFileAttributeTag]string{
		CBFSFileAttributeTagUnused:      "TagUnused",
		CBFSFileAttributeTagUnused2:     "TagUnused2",
		CBFSFileAttributeTagCompression: "TagCompression",
		CBFSFileAttributeTagHash:        "TagHash",
		CBFSFileAttributeTagPosition:    "TagPosition",
		CBFSFileAttributeTagAlignment:   "TagAlignment",
		CBFSFileAttributeTagIBB:         "TagIBB",
		CBFSFileAttributeTagPadding:     "TagPadding",
		CBFSFileAttributeTagStageHeader: "TagStageHeader",
	}
)

func (x CBFSFileAttributeTag) String() string {
	if val, ok := cbfsFileAttributeTagName[x]; ok {
		return val
	}
	return "unknown"
}

// CBFSAttributeAlign is alignment of CBFS file attributes.
const CBFSAttributeAlign = 4

// CBFSFileAttributeHeader is basic struct containing tag and length of CBFS file attribute.
type CBFSFileAttributeHeader struct {
	Tag CBFSFileAttributeTag
	Len uint32 // Covers whole structure.
}

// CBFSFileAttributeHeaderSize = sizeof(struct cbfs_file_attribute).
const CBFSFileAttributeHeaderSize = 8

// CBFSFileAttribute is interface allowing to group and generalize CBFS file attributes.
type CBFSFileAttribute interface {
	GetTag() CBFSFileAttributeTag
}

// CBFSFileAttributeCompression describes CBFS file data compression algorithm used.
type CBFSFileAttributeCompression struct {
	CBFSFileAttributeHeader
	Compression      CBFSFileCompression
	DecompressedSize uint32
}

// GetTag returns attribute tag value.
func (a CBFSFileAttributeCompression) GetTag() CBFSFileAttributeTag {
	return a.Tag
}

// CBFSFileHash is based on vb2_hash with support for sha1, sha256 and sha512.
type CBFSFileHash struct {
	_         [3]byte // Padding to match existing 4-byte big-endian from CBFS.
	Algorithm uint8
	Digest    []byte
}

// CBFSFileAttributeHash contains CBFS file data hash.
type CBFSFileAttributeHash struct {
	CBFSFileAttributeHeader
	Hash CBFSFileHash
}

// GetTag returns attribute tag value.
func (a CBFSFileAttributeHash) GetTag() CBFSFileAttributeTag {
	return a.Tag
}

// CBFSFileAttributePosition contains position attribute.
type CBFSFileAttributePosition struct {
	CBFSFileAttributeHeader
	Position uint32
}

// GetTag returns attribute tag value.
func (a CBFSFileAttributePosition) GetTag() CBFSFileAttributeTag {
	return a.Tag
}

// CBFSFileAttributeAlignment contains alignment attribute.
type CBFSFileAttributeAlignment struct {
	CBFSFileAttributeHeader
	Alignment uint32
}

// GetTag returns attribute tag value.
func (a CBFSFileAttributeAlignment) GetTag() CBFSFileAttributeTag {
	return a.Tag
}

// CBFSFileAttributeStageHeader contains stage description.
type CBFSFileAttributeStageHeader struct {
	CBFSFileAttributeHeader
	LoadAddress uint64
	EntryOffset uint32
	MemLen      uint32
}

// GetTag returns attribute tag value.
func (a CBFSFileAttributeStageHeader) GetTag() CBFSFileAttributeTag {
	return a.Tag
}

// CBFSFileAttributeGeneric is generic container for unrecognized attributes.
type CBFSFileAttributeGeneric struct {
	CBFSFileAttributeHeader
	Data []byte
}

// GetTag returns attribute tag value.
func (a CBFSFileAttributeGeneric) GetTag() CBFSFileAttributeTag {
	return a.Tag
}

// CBFSFile contains deserialized CBFS file.
type CBFSFile struct {
	FileType   CBFSFileType
	Filename   string
	Attributes []CBFSFileAttribute
	Data       []byte
	Header     CBFSFileHeader
}

// CBFSEndianness should be used with binary.Read() to correctly unpack CBFS structures
var CBFSEndianness = binary.BigEndian

func binaryToAttributes(data []byte) ([]CBFSFileAttribute, error) {
	var attrs []CBFSFileAttribute
	var offset = 0
	buf := bytes.NewReader(data)
	var attributeHeader CBFSFileAttributeHeader

	for offset+CBFSFileAttributeHeaderSize <= len(data) {
		if _, err := buf.Seek(int64(offset), io.SeekStart); err != nil {
			return nil, errors.Wrap(err, "failed to seek at offset")
		}
		if err := binary.Read(buf, CBFSEndianness, &attributeHeader); err != nil {
			return nil, errors.Wrap(err, "failed to read CBFS file attribute header")
		}

		if attributeHeader.Len < CBFSFileAttributeHeaderSize || int(attributeHeader.Len) > len(data)-offset {
			return nil, errors.Errorf("file attribute with incorrect size %v at offset %v", attributeHeader.Len, offset)
		}

		// Reset offset.
		if _, err := buf.Seek(int64(offset), io.SeekStart); err != nil {
			return nil, errors.Wrap(err, "failed to seek at offset")
		}

		switch attributeHeader.Tag {
		case CBFSFileAttributeTagCompression:
			var attr CBFSFileAttributeCompression
			if err := binary.Read(buf, CBFSEndianness, &attr); err != nil {
				return nil, errors.Wrap(err, "failed to read compression attribute")
			}
			attrs = append(attrs, attr)
		case CBFSFileAttributeTagHash:
			var attr CBFSFileAttributeHash
			attr.Tag = attributeHeader.Tag
			attr.Len = attributeHeader.Len
			attr.Hash.Algorithm = data[offset+int(CBFSFileAttributeHeaderSize)+3]
			copy(attr.Hash.Digest, data[(offset + CBFSFileAttributeHeaderSize + 4):][:(attributeHeader.Len-CBFSFileAttributeHeaderSize-4)])
			attrs = append(attrs, attr)
		case CBFSFileAttributeTagPosition:
			var attr CBFSFileAttributePosition
			if err := binary.Read(buf, CBFSEndianness, &attr); err != nil {
				return nil, errors.Wrap(err, "failed to read position attribute")
			}
			attrs = append(attrs, attr)
		case CBFSFileAttributeTagAlignment:
			var attr CBFSFileAttributeAlignment
			if err := binary.Read(buf, CBFSEndianness, &attr); err != nil {
				return nil, errors.Wrap(err, "failed to read alignment attribute")
			}
			attrs = append(attrs, attr)
		case CBFSFileAttributeTagStageHeader:
			var attr CBFSFileAttributeStageHeader
			if err := binary.Read(buf, CBFSEndianness, &attr); err != nil {
				return nil, errors.Wrap(err, "failed to read stage header attribute")
			}
			attrs = append(attrs, attr)
		default:
			var attr CBFSFileAttributeGeneric
			attr.CBFSFileAttributeHeader = attributeHeader
			copy(attr.Data, data[offset:][:(attr.Len-CBFSFileAttributeHeaderSize)])
			attrs = append(attrs, attr)
		}

		offset += int(attributeHeader.Len)
	}

	return attrs, nil
}

// CBFSWalkerContext holds context of CBFSWalker functions.
type CBFSWalkerContext struct {
	File   *CBFSFile
	Buffer []byte // Buffer containing raw CBFS file header, attributes and data.
}

// CBFSWalker is function type used to implement walkers for CBFSWalk().
type CBFSWalker func(ctx CBFSWalkerContext) bool

func alignUp(i, mask int) int {
	return (i + (mask - 1)) & ^(mask - 1)
}

// CBFSWalk is based on cbfs_walk() from coreboot. Provide data and walker.
// On true returned from walker function will return.
func CBFSWalk(data []byte, walker CBFSWalker) error {
	var offset = 0
	var fileHeader = CBFSFileHeader{}
	var filenameSlice []byte
	var file *CBFSFile
	var fileData []byte
	buf := bytes.NewReader(data)

	for offset+CBFSFileHeaderSize <= len(data) {
		offset = alignUp(offset, CBFSAlignment)
		if _, err := buf.Seek(int64(offset), io.SeekStart); err != nil {
			return errors.Wrap(err, "failed to seek at offset")
		}
		if err := binary.Read(buf, CBFSEndianness, &fileHeader); err != nil {
			return errors.Wrap(err, "failed to read full CBFS file header")

		}

		if string(fileHeader.Magic[:]) != CBFSFileMagic {
			offset += CBFSAlignment
			continue
		}

		if fileHeader.Offset > CBFSMetadataMaxSize || int(fileHeader.Len) > len(data)-offset || int(fileHeader.Offset)+int(fileHeader.Len) > len(data)-offset {
			// File too large, omit.
			offset += CBFSAlignment
			continue
		}

		var filenameLength int
		if fileHeader.AttributesOffset == 0 {
			filenameLength = int(fileHeader.Offset) - CBFSFileHeaderSize
		} else {
			filenameLength = int(fileHeader.AttributesOffset) - CBFSFileHeaderSize
		}

		if fileHeader.Offset <= fileHeader.AttributesOffset || filenameLength <= 0 || filenameLength > CBFSMetadataMaxSize-CBFSFileHeaderSize {
			// Corrupted file header.
			goto next_file
		}

		fileData = data[offset:]
		filenameSlice = fileData[CBFSFileHeaderSize:][:filenameLength]
		file = new(CBFSFile)
		file.Header = fileHeader
		file.FileType = fileHeader.FileType
		file.Filename = string(filenameSlice[:bytes.Index(filenameSlice, []byte{0})])

		if fileHeader.AttributesOffset > 0 {
			// Parse attributes.
			attrs, err := binaryToAttributes(fileData[fileHeader.AttributesOffset:fileHeader.Offset])
			if err != nil {
				return errors.Wrap(err, "failed to read file attributes")
			}
			file.Attributes = attrs
		}

		// Copy file contents.
		copy(file.Data, fileData[fileHeader.Offset:][:fileHeader.Len])

		if ok := walker(CBFSWalkerContext{file, fileData[:(fileHeader.Offset + fileHeader.Len)]}); ok {
			return nil
		}

	next_file:
		offset += int(fileHeader.Offset) + int(fileHeader.Len)
	}

	return nil
}

// CBFSGetFiles returns list of CBFS files excluding empty ones.
func CBFSGetFiles(data []byte) (files []CBFSFile, err error) {
	filesListWalker := func(ctx CBFSWalkerContext) bool {
		if ctx.File.FileType != CBFSFileTypeDeleted && ctx.File.FileType != CBFSFileTypeNull && ctx.File.FileType != CBFSFileTypeQuery {
			files = append(files, *ctx.File)
		}
		return false
	}

	return files, CBFSWalk(data, filesListWalker)
}

// CBFSGetAllFiles returns list of CBFS files including empty ones.
func CBFSGetAllFiles(data []byte) (files []CBFSFile, err error) {
	filesListWalker := func(ctx CBFSWalkerContext) bool {
		files = append(files, *ctx.File)
		return false
	}

	return files, CBFSWalk(data, filesListWalker)
}

// CBFSCreateSingleFileWalker returns wrapper for CBFSWalker.
// Provided walker function will be executed only if it finds non-empty file with provided name.
func CBFSCreateSingleFileWalker(filename string, walker CBFSWalker) CBFSWalker {
	return func(ctx CBFSWalkerContext) bool {
		if ctx.File.Filename == filename && ctx.File.FileType != CBFSFileTypeDeleted && ctx.File.FileType != CBFSFileTypeNull && ctx.File.FileType != CBFSFileTypeQuery {
			return walker(ctx)
		}
		return false
	}
}

// CBFSGetFile returns one non-empty file if it exists or nil if it does not.
func CBFSGetFile(data []byte, filename string) (file *CBFSFile, err error) {
	singleFileWalker := CBFSCreateSingleFileWalker(filename,
		func(ctx CBFSWalkerContext) bool {
			file = ctx.File
			return true
		})

	return file, CBFSWalk(data, singleFileWalker)
}
