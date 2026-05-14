// Package models defines the core domain types used across the application.
package models

import "fmt"

// DeviceInfo holds the information returned by the Sentry device.
type DeviceInfo struct {
	Manufacturer      string
	Model             string
	Name              string
	SerialNumber      string
	HardwareRevision  string
	FirmwareRevision  string
	ManufacturingDate string
	AWSThingName      string
	WiFiMAC           string
	WiFiFWRevision    string
	PMFWRevision      string
}

// BatteryInfo holds battery status from a Sentry device.
type BatteryInfo struct {
	IsPluggedIn bool
	IsCharging  bool
	Voltage     float64
	Level       float64
}

// ProvisionStatus represents the provisioning state machine.
type ProvisionStatus int

const (
	ProvisionOff            ProvisionStatus = 0
	ProvisionUnprovisioned  ProvisionStatus = 1
	ProvisionWiFiWait       ProvisionStatus = 2
	ProvisionWiFiDisconnect ProvisionStatus = 3
	ProvisionWiFiConnect    ProvisionStatus = 4
	ProvisionWiFiPing       ProvisionStatus = 5
	ProvisionMQTTConnect    ProvisionStatus = 6
	ProvisionMQTTPublish    ProvisionStatus = 7
	ProvisionMQTTWait       ProvisionStatus = 8
)

func (s ProvisionStatus) String() string {
	switch s {
	case ProvisionOff:
		return "Off"
	case ProvisionUnprovisioned:
		return "Unprovisioned"
	case ProvisionWiFiWait:
		return "WiFiWait"
	case ProvisionWiFiDisconnect:
		return "WiFiDisconnect"
	case ProvisionWiFiConnect:
		return "WiFiConnect"
	case ProvisionWiFiPing:
		return "WiFiPing"
	case ProvisionMQTTConnect:
		return "MQTTConnect"
	case ProvisionMQTTPublish:
		return "MQTTPublish"
	case ProvisionMQTTWait:
		return "MQTTWait"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// SentryStatus holds the current device state.
type SentryStatus struct {
	State                  ProvisionStatus
	Error                  int
	DidProvisioningSucceed bool
}
