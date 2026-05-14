package device

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	wireVarint = 0
	wire64Bit  = 1
	wireLenDel = 2
	wire32Bit  = 5
)

type protoEncoder struct {
	buf []byte
}

func newEncoder() *protoEncoder {
	return &protoEncoder{}
}

func (e *protoEncoder) Bytes() []byte {
	return e.buf
}

func (e *protoEncoder) appendVarint(v uint64) {
	for v >= 0x80 {
		e.buf = append(e.buf, byte(v)|0x80)
		v >>= 7
	}
	e.buf = append(e.buf, byte(v))
}

func (e *protoEncoder) appendTag(fieldNum int, wireType int) {
	e.appendVarint(uint64(fieldNum<<3 | wireType))
}

func (e *protoEncoder) WriteString(fieldNum int, val string) {
	if val == "" {
		return
	}
	e.appendTag(fieldNum, wireLenDel)
	e.appendVarint(uint64(len(val)))
	e.buf = append(e.buf, val...)
}

func (e *protoEncoder) WriteBytes(fieldNum int, val []byte) {
	if len(val) == 0 {
		return
	}
	e.appendTag(fieldNum, wireLenDel)
	e.appendVarint(uint64(len(val)))
	e.buf = append(e.buf, val...)
}

func (e *protoEncoder) WriteUint32(fieldNum int, val uint32) {
	if val == 0 {
		return
	}
	e.appendTag(fieldNum, wireVarint)
	e.appendVarint(uint64(val))
}

func (e *protoEncoder) WriteFixed32(fieldNum int, val uint32) {
	e.appendTag(fieldNum, wire32Bit)
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, val)
	e.buf = append(e.buf, b...)
}

func (e *protoEncoder) WriteBool(fieldNum int, val bool) {
	e.appendTag(fieldNum, wireVarint)
	if val {
		e.buf = append(e.buf, 1)
	} else {
		e.buf = append(e.buf, 0)
	}
}

func (e *protoEncoder) WriteDouble(fieldNum int, val float64) {
	if val == 0 {
		return
	}
	e.appendTag(fieldNum, wire64Bit)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(val))
	e.buf = append(e.buf, b...)
}

func (e *protoEncoder) WriteMessage(fieldNum int, inner *protoEncoder) {
	data := inner.Bytes()
	if len(data) == 0 {
		return
	}
	e.appendTag(fieldNum, wireLenDel)
	e.appendVarint(uint64(len(data)))
	e.buf = append(e.buf, data...)
}

func (e *protoEncoder) WriteEmptyMessage(fieldNum int) {
	e.appendTag(fieldNum, wireLenDel)
	e.appendVarint(0)
}

type protoField struct {
	Number   int
	WireType int
	Varint   uint64
	Bytes    []byte
	Fixed32  uint32
	Fixed64  uint64
}

func decodeFields(data []byte) ([]protoField, error) {
	var fields []protoField
	pos := 0

	for pos < len(data) {
		tag, n := binary.Uvarint(data[pos:])
		if n <= 0 {
			return nil, fmt.Errorf("proto: invalid tag at position %d", pos)
		}
		pos += n

		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x07)

		field := protoField{Number: fieldNum, WireType: wireType}

		switch wireType {
		case wireVarint:
			val, n := binary.Uvarint(data[pos:])
			if n <= 0 {
				return nil, fmt.Errorf("proto: invalid varint at field %d", fieldNum)
			}
			pos += n
			field.Varint = val

		case wire64Bit:
			if pos+8 > len(data) {
				return nil, fmt.Errorf("proto: truncated fixed64 at field %d", fieldNum)
			}
			field.Fixed64 = binary.LittleEndian.Uint64(data[pos : pos+8])
			pos += 8

		case wireLenDel:
			length, n := binary.Uvarint(data[pos:])
			if n <= 0 {
				return nil, fmt.Errorf("proto: invalid length at field %d", fieldNum)
			}
			pos += n
			end := pos + int(length)
			if end > len(data) {
				return nil, fmt.Errorf("proto: truncated bytes at field %d", fieldNum)
			}
			field.Bytes = data[pos:end]
			pos = end

		case wire32Bit:
			if pos+4 > len(data) {
				return nil, fmt.Errorf("proto: truncated fixed32 at field %d", fieldNum)
			}
			field.Fixed32 = binary.LittleEndian.Uint32(data[pos : pos+4])
			pos += 4

		default:
			return nil, fmt.Errorf("proto: unknown wire type %d at field %d", wireType, fieldNum)
		}

		fields = append(fields, field)
	}

	return fields, nil
}

func getString(fields []protoField, num int) string {
	for _, f := range fields {
		if f.Number == num && f.WireType == wireLenDel {
			return string(f.Bytes)
		}
	}
	return ""
}

func getFixed32(fields []protoField, num int) uint32 {
	for _, f := range fields {
		if f.Number == num && f.WireType == wire32Bit {
			return f.Fixed32
		}
	}
	return 0
}

func getBytes(fields []protoField, num int) []byte {
	for _, f := range fields {
		if f.Number == num && f.WireType == wireLenDel {
			return f.Bytes
		}
	}
	return nil
}

func getBool(fields []protoField, num int) bool {
	for _, f := range fields {
		if f.Number == num && f.WireType == wireVarint {
			return f.Varint != 0
		}
	}
	return false
}

func getInt32Signed(fields []protoField, num int) int32 {
	for _, f := range fields {
		if f.Number == num {
			switch f.WireType {
			case wireVarint:
				n := f.Varint
				return int32((n >> 1) ^ -(n & 1))
			case wire32Bit:
				return int32(f.Fixed32)
			}
		}
	}
	return 0
}
