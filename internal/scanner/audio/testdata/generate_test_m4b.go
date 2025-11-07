package main

import (
	"bytes"
	"encoding/binary"
	"os"
)

// createAtom creates an MP4 atom with size, type, and data
func createAtom(atomType string, data []byte) []byte {
	buf := &bytes.Buffer{}
	size := uint32(8 + len(data))
	binary.Write(buf, binary.BigEndian, size)
	buf.WriteString(atomType)
	buf.Write(data)
	return buf.Bytes()
}

// createDataAtom creates a data atom with UTF-8 text
func createDataAtom(value string) []byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.BigEndian, uint32(1)) // type = UTF-8
	binary.Write(buf, binary.BigEndian, uint32(0)) // reserved
	buf.WriteString(value)
	return createAtom("data", buf.Bytes())
}

// createMetadataItem creates a metadata item (tag + data atom)
func createMetadataItem(tag []byte, value string) []byte {
	dataAtom := createDataAtom(value)
	return createAtom(string(tag), dataAtom)
}

func main() {
	// Create ftyp atom (M4B file type)
	ftypData := []byte("M4B ")         // major brand
	ftypData = append(ftypData, 0, 0, 0, 0) // minor version
	ftypData = append(ftypData, []byte("M4B M4A mp42isom")...) // compatible brands
	ftypAtom := createAtom("ftyp", ftypData)

	// Create metadata items
	titleItem := createMetadataItem([]byte{0xA9, 'n', 'a', 'm'}, "Test Book")
	artistItem := createMetadataItem([]byte{0xA9, 'A', 'R', 'T'}, "Test Author")

	// Create ilst (metadata list)
	ilstData := append(titleItem, artistItem...)
	ilstAtom := createAtom("ilst", ilstData)

	// Create meta atom (needs version+flags before data)
	metaBuf := &bytes.Buffer{}
	binary.Write(metaBuf, binary.BigEndian, uint32(0)) // version+flags
	metaBuf.Write(ilstAtom)
	metaAtom := createAtom("meta", metaBuf.Bytes())

	// Create udta (user data)
	udtaAtom := createAtom("udta", metaAtom)

	// Create mvhd (movie header) for duration
	mvhdBuf := &bytes.Buffer{}
	binary.Write(mvhdBuf, binary.BigEndian, uint8(0))  // version
	binary.Write(mvhdBuf, binary.BigEndian, uint8(0))  // flags
	binary.Write(mvhdBuf, binary.BigEndian, uint16(0)) // flags
	binary.Write(mvhdBuf, binary.BigEndian, uint32(0)) // creation time
	binary.Write(mvhdBuf, binary.BigEndian, uint32(0)) // modification time
	binary.Write(mvhdBuf, binary.BigEndian, uint32(1000)) // timescale (1000 = 1ms)
	binary.Write(mvhdBuf, binary.BigEndian, uint32(60000)) // duration (60 seconds)
	// Skip remaining fields (not needed for test)
	mvhdAtom := createAtom("mvhd", mvhdBuf.Bytes())

	// Create moov (movie container)
	moovData := append(mvhdAtom, udtaAtom...)
	moovAtom := createAtom("moov", moovData)

	// Create minimal mdat (media data) - just empty
	mdatAtom := createAtom("mdat", []byte{})

	// Write complete file
	file, err := os.Create("test.m4b")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	file.Write(ftypAtom)
	file.Write(moovAtom)
	file.Write(mdatAtom)

	println("Created test.m4b")
}