package publication

import (
	"bytes"
	"encoding/binary"
)

const (
	zipEndSize       = 22
	zip64LocatorSize = 20
	zip64EndSize     = 56
	zipCentralSize   = 46
)

// The standard ZIP reader can mistake a signature inside the archive comment
// for its end record. Locate an end record backed by the actual directory first,
// then give the reader a view without the opaque comment. The stored and sent
// pack bytes stay untouched. Java still validates the complete original archive.
func zipDiscoveryView(data []byte) []byte {
	last := len(data) - zipEndSize
	for end := max(0, last-0xffff); end <= last; end++ {
		footer := data[end : end+zipEndSize]
		if string(footer[:4]) != "PK\x05\x06" || int(binary.LittleEndian.Uint16(footer[20:])) != last-end {
			continue
		}
		if !hasZIPDirectory(data, end) {
			continue
		}
		little := binary.LittleEndian
		// archive/zip also probes for a ZIP64 locator when the ordinary
		// directory size is exactly 65,535. A locator-shaped member comment
		// must not override that ordinary directory's validated metadata.
		start, size := little.Uint32(footer[16:]), little.Uint32(footer[12:])
		ordinary := uint64(start)+uint64(size) == uint64(end)
		if size == 0xffff && ordinary &&
			string(data[end-zip64LocatorSize:end-zip64LocatorSize+4]) == "PK\x06\x07" {
			return zip64DiscoveryView(data[:end], footer)
		}
		// A valid ZIP64 archive can mark only the classic directory size
		// with 0xffffffff. archive/zip doesn't recognize that marker, so
		// use the corresponding record-count markers in its private view.
		sizeOnlyZIP64 := size == 0xffffffff && little.Uint16(footer[10:]) != 0xffff && start != 0xffffffff
		if end == last && !sizeOnlyZIP64 {
			return data
		}
		view := bytes.Clone(data[:end+zipEndSize])
		clear(view[len(view)-2:])
		if sizeOnlyZIP64 {
			little.PutUint32(view[end+8:], 0xffffffff)
		}
		return view
	}
	return data
}

func zip64DiscoveryView(directory []byte, footer []byte) []byte {
	end := len(directory)
	view := make([]byte, end+zip64EndSize+zip64LocatorSize+zipEndSize)
	copy(view, directory)
	little := binary.LittleEndian
	wide := view[end : end+zip64EndSize]
	copy(wide, "PK\x06\x06")
	little.PutUint64(wide[4:], zip64EndSize-12)
	little.PutUint16(wide[12:], 45)
	little.PutUint16(wide[14:], 45)
	count := uint64(little.Uint16(footer[10:]))
	little.PutUint64(wide[24:], count)
	little.PutUint64(wide[32:], count)
	little.PutUint64(wide[40:], uint64(little.Uint32(footer[12:])))
	little.PutUint64(wide[48:], uint64(little.Uint32(footer[16:])))
	locator := view[end+zip64EndSize : end+zip64EndSize+zip64LocatorSize]
	copy(locator, "PK\x06\x07")
	little.PutUint64(locator[8:], uint64(end))
	little.PutUint32(locator[16:], 1)
	copy(view[len(view)-zipEndSize:], footer)
	clear(view[len(view)-2:])
	return view
}

func hasZIPDirectory(data []byte, end int) bool {
	little := binary.LittleEndian
	footer := data[end : end+zipEndSize]
	if little.Uint32(footer[4:]) != 0 || little.Uint16(footer[8:]) != little.Uint16(footer[10:]) {
		return false
	}
	count := uint64(little.Uint16(footer[10:]))
	size := uint64(little.Uint32(footer[12:]))
	start := uint64(little.Uint32(footer[16:]))
	directoryEnd := uint64(end)
	if start+size != directoryEnd {
		if end < zip64EndSize+zip64LocatorSize {
			return false
		}
		locator := data[end-zip64LocatorSize : end]
		if string(locator[:4]) != "PK\x06\x07" || little.Uint32(locator[4:]) != 0 || little.Uint32(locator[16:]) != 1 {
			return false
		}
		directoryEnd = little.Uint64(locator[8:])
		if directoryEnd > uint64(end-zip64LocatorSize-zip64EndSize) {
			return false
		}
		wide := data[directoryEnd : end-zip64LocatorSize]
		if string(wide[:4]) != "PK\x06\x06" || little.Uint64(wide[4:]) != uint64(len(wide)-12) ||
			little.Uint64(wide[16:]) != 0 || little.Uint64(wide[24:]) != little.Uint64(wide[32:]) {
			return false
		}
		wideCount, wideSize, wideStart := little.Uint64(wide[32:]), little.Uint64(wide[40:]), little.Uint64(wide[48:])
		if count != 0xffff && count != wideCount || size != 0xffffffff && size != wideSize || start != 0xffffffff && start != wideStart {
			return false
		}
		count, size, start = wideCount, wideSize, wideStart
	}
	if start > directoryEnd || size != directoryEnd-start {
		return false
	}
	for pos := int(start); pos < int(directoryEnd); {
		header := data[pos:directoryEnd]
		if len(header) < zipCentralSize || string(header[:4]) != "PK\x01\x02" || count == 0 {
			return false
		}
		recordSize := zipCentralSize + int(little.Uint16(header[28:])) + int(little.Uint16(header[30:])) + int(little.Uint16(header[32:]))
		if recordSize > len(header) {
			return false
		}
		pos += recordSize
		count--
	}
	return count == 0
}
