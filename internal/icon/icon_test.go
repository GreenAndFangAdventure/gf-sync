package icon

import (
	"encoding/binary"
	"testing"
)

func TestICOHeader(t *testing.T) {
	for _, ico := range [][]byte{Normal, Paused, Error, Pairing, ICO(256, Green, Red)} {
		if binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 1 {
			t.Fatal("not an icon")
		}
		size := binary.LittleEndian.Uint32(ico[14:])
		off := binary.LittleEndian.Uint32(ico[18:])
		if int(off+size) != len(ico) {
			t.Fatalf("size %d + off %d != len %d", size, off, len(ico))
		}
	}
	if string(Normal) == string(Error) {
		t.Fatal("states must differ")
	}
}
