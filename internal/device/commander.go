package device

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/hellowynd/wyndctl/internal/config"
	"github.com/hellowynd/wyndctl/internal/models"
	"github.com/hellowynd/wyndctl/internal/transport"
)

// Commander provides a high-level API for interacting with Sentry devices.
type Commander struct {
	transport *transport.Transport
	rpc       *transport.RPC
	channels  map[string]*transport.SerialChannel
	cfg       config.USBConfig
	mqttCfg   config.MQTTConfig
	env       string
	log       *slog.Logger
}

// NewCommander creates a new device commander.
func NewCommander(usbCfg config.USBConfig, mqttCfg config.MQTTConfig, env string, log *slog.Logger) *Commander {
	t := transport.NewTransport()
	rpc := transport.NewRPC(t)

	return &Commander{
		transport: t,
		rpc:       rpc,
		channels:  make(map[string]*transport.SerialChannel),
		cfg:       usbCfg,
		mqttCfg:   mqttCfg,
		env:       env,
		log:       log,
	}
}

// Channels returns the map of location -> channel for discovered devices.
func (c *Commander) Channels() map[string]*transport.SerialChannel {
	return c.channels
}

// Scan discovers connected Sentry devices and creates serial channels.
func (c *Commander) Scan() (map[string]*transport.SerialChannel, error) {
	c.log.Info("scanning for Sentry devices",
		"vid", fmt.Sprintf("0x%04X", c.cfg.VendorID),
		"pid", fmt.Sprintf("0x%04X", c.cfg.ProductID))

	ports, err := transport.FindSerialPorts(c.cfg.VendorID, c.cfg.ProductID)
	if err != nil {
		return nil, fmt.Errorf("scanning USB ports: %w", err)
	}

	if len(ports) == 0 {
		c.log.Warn("no Sentry devices found")
		return c.channels, nil
	}

	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Location < ports[j].Location
	})

	for _, port := range ports {
		c.log.Info("found device",
			"device", port.Device,
			"location", port.Location,
			"serial", port.SerialNumber)

		ch := transport.NewSerialChannel(c.transport, port.Device, c.cfg.BaudRate, c.log)
		c.channels[port.Location] = ch
	}

	c.log.Info("scan complete", "devices_found", len(c.channels))
	return c.channels, nil
}

// ActivateChannel opens a single serial channel.
func (c *Commander) ActivateChannel(ch *transport.SerialChannel) error {
	return ch.Open()
}

// ActivateAll opens all discovered serial channels.
func (c *Commander) ActivateAll() error {
	for loc, ch := range c.channels {
		if err := ch.Open(); err != nil {
			return fmt.Errorf("activating channel at %s: %w", loc, err)
		}
	}
	return nil
}

// CloseChannel closes a single serial channel.
func (c *Commander) CloseChannel(ch *transport.SerialChannel) error {
	return ch.Close()
}

// CloseAll closes all serial channels.
func (c *Commander) CloseAll() {
	for _, ch := range c.channels {
		if err := ch.Close(); err != nil {
			c.log.Warn("error closing channel", "error", err)
		}
	}
}

// GetDeviceInfo retrieves the device information from a Sentry.
func (c *Commander) GetDeviceInfo(ch *transport.SerialChannel) (*models.DeviceInfo, error) {
	resp, err := c.rpc.UnaryCall(MethodGetDeviceInfo, nil, ch, 0)
	if err != nil {
		return nil, fmt.Errorf("get device info: %w", err)
	}

	return parseDeviceInfoResponse(resp)
}

// GetBatteryInfo retrieves battery status from a Sentry.
func (c *Commander) GetBatteryInfo(ch *transport.SerialChannel) (*models.BatteryInfo, error) {
	resp, err := c.rpc.UnaryCall(MethodGetBatteryInfo, nil, ch, 0)
	if err != nil {
		return nil, fmt.Errorf("get battery info: %w", err)
	}

	return parseBatteryInfoResponse(resp)
}

// SetPower turns the device on or off.
func (c *Commander) SetPower(ch *transport.SerialChannel, on bool) error {
	enc := newEncoder()
	enc.WriteBool(fieldOn, on)

	_, err := c.rpc.UnaryCall(MethodSetPower, enc.Bytes(), ch, 0)
	return err
}

// Reboot restarts the device.
func (c *Commander) Reboot(ch *transport.SerialChannel) error {
	_, err := c.rpc.UnaryCall(MethodReboot, nil, ch, 0)
	if err != nil {
		if _, ok := err.(*transport.TimeoutError); ok {
			return nil
		}
		return err
	}
	return nil
}

// SetAdvertising enables or disables BLE advertising on the device.
func (c *Commander) SetAdvertising(ch *transport.SerialChannel, on bool) error {
	enc := newEncoder()
	enc.WriteBool(fieldOn, on)

	_, err := c.rpc.UnaryCall(MethodSetAdvertising, enc.Bytes(), ch, 0)
	return err
}

// SetIndicate sets the LED colors on the device.
func (c *Commander) SetIndicate(ch *transport.SerialChannel, left, top, right uint32, timeout float64) error {
	enc := newEncoder()

	for _, ind := range []struct {
		id    IndicatorID
		color uint32
	}{
		{IndicatorLeft, left},
		{IndicatorTop, top},
		{IndicatorRight, right},
	} {
		inner := newEncoder()
		inner.appendTag(fieldIdentifier, wireVarint)
		inner.appendVarint(uint64(ind.id))
		inner.WriteFixed32(fieldValue, ind.color)

		enc.WriteMessage(fieldIndicateList, inner)
	}

	enc.WriteDouble(fieldTimeout, timeout)

	_, err := c.rpc.UnaryCall(MethodIndicate, enc.Bytes(), ch, 0)
	return err
}

// CancelIndication turns off all LEDs on the device.
func (c *Commander) CancelIndication(ch *transport.SerialChannel) error {
	return c.SetIndicate(ch, 1, 1, 1, 0.3)
}

// SetProvision configures WiFi and MQTT on the device, then waits for connection.
// On timeout, the second return value is the last provisioning state observed
// from the device (or ProvisionOff if no status poll succeeded), which lets
// callers tell whether the device got stuck on WiFi vs MQTT.
//
// SECURITY: never log the psk argument or any value derived from it.
func (c *Commander) SetProvision(ch *transport.SerialChannel, ssid, psk string, timeout time.Duration) (bool, models.ProvisionStatus, error) {
	escapedPSK := psk
	if psk != "" {
		escapedPSK = `"` + psk + `"`
	}

	mqttTopic := fmt.Sprintf(c.mqttCfg.TopicPattern, c.env)
	logTopic := fmt.Sprintf(c.mqttCfg.LogTopicPattern, c.env)

	enc := newEncoder()
	inner := newEncoder()
	inner.WriteString(fieldSSID, ssid)
	inner.WriteString(fieldPSK, escapedPSK)
	inner.WriteString(fieldMQTTTopic, mqttTopic)
	inner.WriteString(fieldMQTTURL, c.mqttCfg.BrokerURL)
	inner.WriteUint32(fieldMQTTPort, uint32(c.mqttCfg.Port))
	inner.WriteString(fieldMQTTThingsTopic, c.mqttCfg.ThingsPattern)
	inner.WriteString(fieldMQTTLogTopic, logTopic)
	enc.WriteMessage(fieldInformation, inner)

	_, err := c.rpc.UnaryCall(MethodSetProvision, enc.Bytes(), ch, 0)
	if err != nil {
		return false, models.ProvisionOff, fmt.Errorf("sending provision request: %w", err)
	}

	var lastState models.ProvisionStatus
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := c.GetStatus(ch)
		if err != nil {
			c.log.Debug("error polling status during provisioning", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		lastState = status.State
		if status.State >= models.ProvisionMQTTPublish {
			return true, status.State, nil
		}

		time.Sleep(1 * time.Second)
	}

	return false, lastState, nil
}

// Unprovision clears WiFi and MQTT configuration from the device.
func (c *Commander) Unprovision(ch *transport.SerialChannel) error {
	_, err := c.rpc.UnaryCall(MethodSetProvision, nil, ch, 0)
	return err
}

// GetStatus retrieves the current provisioning state from the device.
func (c *Commander) GetStatus(ch *transport.SerialChannel) (*models.SentryStatus, error) {
	resp, err := c.rpc.UnaryCall(MethodGetStatus, nil, ch, 0)
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}

	return parseStatusResponse(resp)
}

// WriteFirmware uploads a firmware binary to the device via streaming RPC.
func (c *Commander) WriteFirmware(ch *transport.SerialChannel, path string, data []byte, timeout time.Duration) error {
	ctx := c.rpc.StreamingCall(MethodWrite, ch)
	defer c.rpc.StreamingFinalize(ctx)

	if err := c.writeOpen(ctx, path, timeout); err != nil {
		return fmt.Errorf("firmware write open: %w", err)
	}

	const chunkSize = 512
	offset := 0
	for offset < len(data) {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}

		if err := c.writeData(ctx, data[offset:end], offset, timeout); err != nil {
			return fmt.Errorf("firmware write data at offset %d: %w", offset, err)
		}

		offset = end
	}

	if err := c.writeClose(ctx, timeout); err != nil {
		return fmt.Errorf("firmware write close: %w", err)
	}

	return nil
}

func (c *Commander) writeOpen(ctx *transport.ClientContext, path string, timeout time.Duration) error {
	enc := newEncoder()
	inner := newEncoder()
	inner.WriteString(fieldPath, path)
	enc.WriteMessage(fieldOpen, inner)

	if err := c.rpc.StreamingSend(ctx, enc.Bytes()); err != nil {
		return err
	}

	return c.checkWriteResponse(ctx, timeout)
}

func (c *Commander) writeData(ctx *transport.ClientContext, data []byte, offset int, timeout time.Duration) error {
	enc := newEncoder()
	inner := newEncoder()
	inner.appendTag(fieldOffset, wireVarint)
	inner.appendVarint(uint64(offset))
	inner.WriteBytes(fieldDataBytes, data)
	enc.WriteMessage(fieldData, inner)

	if err := c.rpc.StreamingSend(ctx, enc.Bytes()); err != nil {
		return err
	}

	return c.checkWriteResponse(ctx, timeout)
}

func (c *Commander) writeClose(ctx *transport.ClientContext, timeout time.Duration) error {
	enc := newEncoder()
	enc.WriteEmptyMessage(fieldClose)

	if err := c.rpc.StreamingSend(ctx, enc.Bytes()); err != nil {
		return err
	}

	return c.checkWriteResponse(ctx, timeout)
}

func (c *Commander) checkWriteResponse(ctx *transport.ClientContext, timeout time.Duration) error {
	resp, err := c.rpc.StreamingReceive(ctx, timeout)
	if err != nil {
		return err
	}

	fields, err := decodeFields(resp)
	if err != nil {
		return fmt.Errorf("decoding write response: %w", err)
	}

	errBytes := getBytes(fields, 1)
	if errBytes != nil {
		errFields, _ := decodeFields(errBytes)
		reason := getString(errFields, 1)
		if reason != "" {
			return fmt.Errorf("device write error: %s", reason)
		}
	}

	return nil
}

func parseDeviceInfoResponse(data []byte) (*models.DeviceInfo, error) {
	fields, err := decodeFields(data)
	if err != nil {
		return nil, fmt.Errorf("decoding device info response: %w", err)
	}

	infoBytes := getBytes(fields, fieldInformation)
	if infoBytes == nil {
		return nil, fmt.Errorf("device info response missing information field")
	}

	infoFields, err := decodeFields(infoBytes)
	if err != nil {
		return nil, fmt.Errorf("decoding device information: %w", err)
	}

	return &models.DeviceInfo{
		Manufacturer:      getString(infoFields, fieldManufacturer),
		Model:             getString(infoFields, fieldModel),
		Name:              getString(infoFields, fieldName),
		SerialNumber:      getString(infoFields, fieldSerialNumber),
		HardwareRevision:  getString(infoFields, fieldHardwareRevision),
		FirmwareRevision:  getString(infoFields, fieldFirmwareRevision),
		ManufacturingDate: getString(infoFields, fieldManufacturingDate),
		AWSThingName:      getString(infoFields, fieldAWSThingName),
		WiFiMAC:           getString(infoFields, fieldWiFiMAC),
		WiFiFWRevision:    getString(infoFields, fieldWiFiFWRevision),
		PMFWRevision:      getString(infoFields, fieldPMFWRevision),
	}, nil
}

func parseBatteryInfoResponse(data []byte) (*models.BatteryInfo, error) {
	fields, err := decodeFields(data)
	if err != nil {
		return nil, fmt.Errorf("decoding battery info response: %w", err)
	}

	infoBytes := getBytes(fields, fieldInformation)
	if infoBytes == nil {
		return nil, fmt.Errorf("battery info response missing information field")
	}

	infoFields, err := decodeFields(infoBytes)
	if err != nil {
		return nil, fmt.Errorf("decoding battery information: %w", err)
	}

	return &models.BatteryInfo{
		IsPluggedIn: getBool(infoFields, 1),
		IsCharging:  getBool(infoFields, 2),
	}, nil
}

func parseStatusResponse(data []byte) (*models.SentryStatus, error) {
	fields, err := decodeFields(data)
	if err != nil {
		return nil, fmt.Errorf("decoding status response: %w", err)
	}

	statusBytes := getBytes(fields, 1)
	if statusBytes == nil {
		return nil, fmt.Errorf("status response missing status field")
	}

	statusFields, err := decodeFields(statusBytes)
	if err != nil {
		return nil, fmt.Errorf("decoding status: %w", err)
	}

	return &models.SentryStatus{
		State:                  models.ProvisionStatus(getFixed32(statusFields, fieldState)),
		Error:                  int(getInt32Signed(statusFields, fieldError)),
		DidProvisioningSucceed: getBool(statusFields, fieldDidProvisioningSucceed),
	}, nil
}
