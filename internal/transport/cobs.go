// Package transport implements the low-level serial communication protocol:
// COBS framing, CRC16 integrity, and packet multiplexing.
package transport

import "fmt"

// COBSEncode encodes data using Consistent Overhead Byte Stuffing.
// The encoded output will not contain any zero bytes.
func COBSEncode(data []byte) []byte {
	if len(data) == 0 {
		return []byte{0x01}
	}

	encoded := make([]byte, 0, len(data)+len(data)/254+1)
	codeIdx := len(encoded)
	encoded = append(encoded, 0) // placeholder for first code byte
	code := byte(1)

	for _, b := range data {
		if b == 0 {
			encoded[codeIdx] = code
			codeIdx = len(encoded)
			encoded = append(encoded, 0) // placeholder for next code
			code = 1
		} else {
			encoded = append(encoded, b)
			code++
			if code == 0xFF {
				encoded[codeIdx] = code
				codeIdx = len(encoded)
				encoded = append(encoded, 0)
				code = 1
			}
		}
	}
	encoded[codeIdx] = code

	return encoded
}

// COBSDecode decodes a COBS-encoded byte slice.
func COBSDecode(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cobs: empty input")
	}

	decoded := make([]byte, 0, len(data))
	idx := 0

	for idx < len(data) {
		code := data[idx]
		idx++

		if code == 0 {
			return nil, fmt.Errorf("cobs: unexpected zero byte at position %d", idx-1)
		}

		for i := byte(1); i < code; i++ {
			if idx >= len(data) {
				return nil, fmt.Errorf("cobs: truncated data")
			}
			decoded = append(decoded, data[idx])
			idx++
		}

		// Add zero byte delimiter unless this is the last group or code was 0xFF
		if code < 0xFF && idx < len(data) {
			decoded = append(decoded, 0)
		}
	}

	return decoded, nil
}
