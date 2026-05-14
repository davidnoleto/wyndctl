package models

import "fmt"

// DeploymentSetting represents configuration for a single bay during deployment.
type DeploymentSetting struct {
	Bay      int    `csv:"bay"`
	WiFiSSID string `csv:"wifi_ssid"`
	WiFiPSK  string `csv:"wifi_psk"`
	Account  string `csv:"account"`
	LodgingID int   `csv:"lodging_id"`
	Room     string `csv:"room"`
	RoomType string `csv:"room_type"`
}

// Validate checks that required fields are populated.
func (s *DeploymentSetting) Validate() error {
	if s.WiFiSSID == "" {
		return fmt.Errorf("bay %d: wifi_ssid is required", s.Bay)
	}
	if s.Account == "" {
		return fmt.Errorf("bay %d: account is required", s.Bay)
	}
	return nil
}

// DeploymentResult represents the outcome of deploying a single device.
type DeploymentResult struct {
	Bay       int    `csv:"bay"`
	DeviceID  string `csv:"device_id"`
	MACAddr   string `csv:"mac_addr"`
	Succeeded bool   `csv:"succeeded"`
	RoomID    int    `csv:"room_id,omitempty"`
	RoomName  string `csv:"room_name,omitempty"`
	Reason    string `csv:"reason,omitempty"`
}

// LocationMapping maps bay numbers to USB serial port locations.
type LocationMapping struct {
	Bay      int    `csv:"bay"`
	Location string `csv:"location"`
	DeviceID string `csv:"device_id"`
}

// RGBColor represents a 24-bit color for device LED indicators.
type RGBColor uint32

// NewRGB creates a color from R, G, B components (0-255 each).
func NewRGB(r, g, b uint8) RGBColor {
	return RGBColor(uint32(r)<<16 | uint32(g)<<8 | uint32(b))
}

// ParseRGB parses a "R,G,B" string into an RGBColor.
func ParseRGB(s string) (RGBColor, error) {
	var r, g, b int
	_, err := fmt.Sscanf(s, "%d,%d,%d", &r, &g, &b)
	if err != nil {
		return 0, fmt.Errorf("invalid RGB color %q: expected R,G,B format: %w", s, err)
	}
	if r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
		return 0, fmt.Errorf("RGB values must be 0-255, got %d,%d,%d", r, g, b)
	}
	return NewRGB(uint8(r), uint8(g), uint8(b)), nil
}
