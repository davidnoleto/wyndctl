package transport

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// SerialPort holds metadata about a discovered USB serial port.
type SerialPort struct {
	Device       string
	Name         string
	Location     string
	SerialNumber string
	VID          string
	PID          string
}

// SerialChannel manages a single USB serial connection to a Sentry device.
type SerialChannel struct {
	transport *Transport
	port      string
	baudRate  int
	serial    serial.Port
	buffer    []byte
	mu        sync.Mutex
	running   bool
	closed    bool
	done      chan struct{}
	log       *slog.Logger
}

// NewSerialChannel creates a new channel for the given port.
func NewSerialChannel(t *Transport, port string, baudRate int, log *slog.Logger) *SerialChannel {
	return &SerialChannel{
		transport: t,
		port:      port,
		baudRate:  baudRate,
		buffer:    make([]byte, 0, 4096),
		done:      make(chan struct{}),
		log:       log,
	}
}

// Port returns the serial port path.
func (c *SerialChannel) Port() string {
	return c.port
}

// Open opens the serial port and starts the read loop in a background goroutine.
func (c *SerialChannel) Open() error {
	mode := &serial.Mode{
		BaudRate: c.baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(c.port, mode)
	if err != nil {
		return fmt.Errorf("opening serial port %s: %w", c.port, err)
	}

	c.serial = p
	c.running = true
	c.closed = false
	c.done = make(chan struct{})
	c.buffer = c.buffer[:0]

	go c.readLoop()
	return nil
}

// Close stops the read loop and closes the serial port.
func (c *SerialChannel) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.running = false
	c.mu.Unlock()

	if c.serial != nil {
		err := c.serial.Close()
		<-c.done
		c.serial = nil
		return err
	}
	return nil
}

// WriteRaw sends raw bytes over the serial port.
func (c *SerialChannel) WriteRaw(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.serial == nil {
		return fmt.Errorf("serial port not open")
	}

	_, err := c.serial.Write(data)
	return err
}

// readLoop continuously reads from the serial port and processes incoming packets.
func (c *SerialChannel) readLoop() {
	defer close(c.done)

	buf := make([]byte, 256)
	for c.running {
		n, err := c.serial.Read(buf)
		if err != nil {
			if c.running {
				c.log.Debug("serial read error", "port", c.port, "error", err)
			}
			return
		}

		if n > 0 {
			c.processBytes(buf[:n])
		}
	}
}

// processBytes handles COBS framing by splitting on the 0x00 delimiter.
func (c *SerialChannel) processBytes(data []byte) {
	for _, b := range data {
		if b != packetDelimiter {
			c.buffer = append(c.buffer, b)
			continue
		}

		if len(c.buffer) == 0 {
			continue
		}

		decoded, err := COBSDecode(c.buffer)
		if err != nil {
			c.log.Debug("COBS decode error", "error", err)
			c.buffer = c.buffer[:0]
			continue
		}

		if err := c.transport.Receive(c, decoded); err != nil {
			c.log.Debug("transport receive error", "error", err)
		}

		c.buffer = c.buffer[:0]
	}
}

// FindSerialPorts discovers USB serial ports matching the given VID/PID.
func FindSerialPorts(vid, pid uint16) ([]*SerialPort, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("enumerating serial ports: %w", err)
	}

	vidStr := fmt.Sprintf("%04x", vid)
	pidStr := fmt.Sprintf("%04x", pid)

	var result []*SerialPort
	for _, p := range ports {
		if !p.IsUSB {
			continue
		}
		if strings.EqualFold(p.VID, vidStr) && strings.EqualFold(p.PID, pidStr) {
			result = append(result, &SerialPort{
				Device:       p.Name,
				Name:         p.Name,
				Location:     p.Name,
				SerialNumber: p.SerialNumber,
				VID:          p.VID,
				PID:          p.PID,
			})
		}
	}

	return result, nil
}
