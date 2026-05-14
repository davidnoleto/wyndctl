package transport

import "fmt"

// PacketType identifies the RPC packet direction/phase.
type PacketType uint64

const (
	PacketServerInitialize PacketType = 0
	PacketServerRequest    PacketType = 1
	PacketServerFinalize   PacketType = 2
	PacketClientResponse   PacketType = 3
	PacketClientFinalize   PacketType = 4
	PacketTypeCount        PacketType = 5
)

const packetDelimiter = 0x00

// PacketHandler processes an incoming packet payload for a given channel.
type PacketHandler func(channel *SerialChannel, data []byte)

// Transport manages packet framing, CRC validation, and handler dispatch.
type Transport struct {
	handlers map[PacketType]PacketHandler
}

// NewTransport creates a new Transport instance.
func NewTransport() *Transport {
	return &Transport{
		handlers: make(map[PacketType]PacketHandler),
	}
}

// SetHandler registers a handler for a specific packet type.
func (t *Transport) SetHandler(pType PacketType, handler PacketHandler) {
	t.handlers[pType] = handler
}

// Write encodes and sends a packet through the given channel.
func (t *Transport) Write(channel *SerialChannel, pType PacketType, data []byte) error {
	typeData := EncodeVarint(nil, uint64(pType))

	length := len(typeData) + len(data)
	lengthData := []byte{byte(length & 0xFF), byte((length >> 8) & 0xFF)}

	payload := append(lengthData, typeData...)
	payload = append(payload, data...)
	crc := CRC16CCITT(payload)
	crcData := []byte{byte(crc & 0xFF), byte((crc >> 8) & 0xFF)}

	packet := append(crcData, payload...)

	encoded := COBSEncode(packet)
	encoded = append(encoded, packetDelimiter)

	return channel.WriteRaw(encoded)
}

// Receive parses an incoming raw (already COBS-decoded) packet.
func (t *Transport) Receive(channel *SerialChannel, data []byte) error {
	const headerSize = 4 // CRC (2) + length (2)

	if len(data) < headerSize {
		return fmt.Errorf("transport: packet too short (%d bytes)", len(data))
	}

	crc := uint16(data[0]) | uint16(data[1])<<8
	length := int(data[2]) | int(data[3])<<8

	actualLength := len(data) - headerSize
	if actualLength != length {
		return fmt.Errorf("transport: length mismatch (header=%d, actual=%d)", length, actualLength)
	}

	actualCRC := CRC16CCITT(data[2:])
	if actualCRC != crc {
		return fmt.Errorf("transport: CRC mismatch (expected=0x%04X, actual=0x%04X)", crc, actualCRC)
	}

	pType, pos, err := DecodeVarint(data[4:], 0)
	if err != nil {
		return fmt.Errorf("transport: decoding packet type: %w", err)
	}

	if PacketType(pType) >= PacketTypeCount {
		return fmt.Errorf("transport: unknown packet type %d", pType)
	}

	handler, ok := t.handlers[PacketType(pType)]
	if !ok {
		return fmt.Errorf("transport: no handler for packet type %d", pType)
	}

	handler(channel, data[4+pos:])
	return nil
}
