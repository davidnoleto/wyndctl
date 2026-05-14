// Package device implements the Sentry device commander.
package device

import "github.com/hellowynd/wyndctl/internal/transport"

// Sentry RPC method definitions. Package=10, Service=1 matches wynd.sentry.v1.Sentry.
var (
	MethodGetDeviceInfo = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 1,
	}
	MethodGetBatteryInfo = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 2,
	}
	MethodSetProvision = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 3,
	}
	MethodGetStatus = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 4,
	}
	MethodIndicate = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 5,
	}
	MethodReboot = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 6,
	}
	MethodSetPower = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 7,
	}
	MethodSetAdvertising = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 8,
	}
	MethodWrite = &transport.Method{
		PackageID: 10, ServiceID: 1, MethodID: 9,
		ServerStreaming: true, ClientStreaming: true,
	}
)

// IndicatorID identifies which LED on the Sentry device to control.
type IndicatorID int

const (
	IndicatorLeft  IndicatorID = 0
	IndicatorTop   IndicatorID = 1
	IndicatorRight IndicatorID = 2
)

// Protobuf field numbers for manual encoding — must match sentry.proto exactly.
const (
	fieldManufacturer      = 1
	fieldModel             = 2
	fieldName              = 3
	fieldSerialNumber      = 4
	fieldHardwareRevision  = 5
	fieldFirmwareRevision  = 6
	fieldManufacturingDate = 7
	fieldAWSThingName      = 8
	fieldWiFiMAC           = 9
	fieldWiFiFWRevision    = 10
	fieldPMFWRevision      = 11

	fieldInformation = 1

	fieldSSID            = 1
	fieldPSK             = 2
	fieldMQTTTopic       = 3
	fieldMQTTURL         = 4
	fieldMQTTPort        = 5
	fieldMQTTThingsTopic = 6
	fieldMQTTLogTopic    = 7

	fieldState                  = 1
	fieldError                  = 2
	fieldDidProvisioningSucceed = 3

	fieldIdentifier = 1
	fieldValue      = 2

	fieldIndicateList = 1
	fieldTimeout      = 2

	fieldOn = 1

	fieldOpen      = 1
	fieldData      = 2
	fieldClose     = 3
	fieldPath      = 1
	fieldOffset    = 1
	fieldDataBytes = 2
)
