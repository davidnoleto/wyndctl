package transport

import "fmt"

// EncodeVarint encodes a uint64 as a protobuf-style varint and appends to dst.
func EncodeVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	dst = append(dst, byte(v))
	return dst
}

// DecodeVarint decodes a varint from data starting at position.
// Returns the value and the new position after the varint.
func DecodeVarint(data []byte, pos int) (uint64, int, error) {
	var result uint64
	var shift uint
	for {
		if pos >= len(data) {
			return 0, pos, fmt.Errorf("varint: unexpected end of data")
		}
		b := data[pos]
		pos++
		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return result, pos, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, pos, fmt.Errorf("varint: too many bytes")
		}
	}
}
