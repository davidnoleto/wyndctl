package deployment

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hellowynd/wyndctl/internal/database"
	"github.com/hellowynd/wyndctl/internal/device"
	"github.com/hellowynd/wyndctl/internal/models"
	"github.com/hellowynd/wyndctl/internal/transport"
)

// Service orchestrates device deployment operations.
type Service struct {
	commander  *device.Commander
	repo       *database.Repository
	log        *slog.Logger
	outputFile string
	outputMu   sync.Mutex
}

// NewService creates a new deployment service.
// repo is optional — if nil, room assignment is skipped.
func NewService(commander *device.Commander, repo *database.Repository, outputFile string, log *slog.Logger) *Service {
	return &Service{
		commander:  commander,
		repo:       repo,
		log:        log,
		outputFile: outputFile,
	}
}

// DeployOptions configures a deployment run.
type DeployOptions struct {
	Timeout      time.Duration
	MaxRetries   int
	SuccessColor uint32
	FailColor    uint32
}

// DeployDevice provisions a single device with full retry logic.
func (s *Service) DeployDevice(
	ch *transport.SerialChannel,
	setting models.DeploymentSetting,
	opts DeployOptions,
) {
	bay := setting.Bay
	log := s.log.With("bay", bay)

	log.Info("starting deployment")

	info, err := s.commander.GetDeviceInfo(ch)
	if err != nil {
		log.Error("failed to get device info", "error", err)
		s.writeResult(bay, &models.DeviceInfo{}, false, 0, "", err.Error())
		return
	}

	log = log.With("device_id", info.AWSThingName)
	log.Info("device identified",
		"serial", info.SerialNumber,
		"firmware", info.FirmwareRevision,
		"wifi_fw", info.WiFiFWRevision,
		"pm_fw", info.PMFWRevision)

	var (
		succeeded     bool
		failureReason string
		roomName      string
		roomID        int
	)

	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		if attempt > 1 {
			log.Info("rebooting device before retry", "attempt", attempt)
			_ = s.commander.Reboot(ch)
			_ = s.commander.CloseChannel(ch)
			time.Sleep(10 * time.Second)
			if err := s.commander.ActivateChannel(ch); err != nil {
				log.Error("failed to reactivate after reboot", "error", err)
				failureReason = err.Error()
				continue
			}
		}

		log.Info("provisioning", "attempt", attempt, "ssid", setting.WiFiSSID)

		if err := s.commander.Unprovision(ch); err != nil {
			log.Warn("unprovision failed", "error", err)
		}
		time.Sleep(1 * time.Second)

		if err := s.commander.SetAdvertising(ch, false); err != nil {
			log.Warn("disable advertising failed", "error", err)
		}

		ok, lastState, err := s.commander.SetProvision(ch, setting.WiFiSSID, setting.WiFiPSK, opts.Timeout)
		if err != nil {
			log.Warn("provisioning error", "error", err)
			failureReason = err.Error()
			continue
		}
		if !ok {
			log.Warn("provisioning timed out", "stuck_state", lastState.String(), "stuck_state_id", int(lastState))
			failureReason = fmt.Sprintf("provisioning timed out (stuck at %s)", lastState)
			continue
		}

		log.Info("provisioning succeeded")

		if setting.LodgingID > 0 && setting.Room != "" {
			if s.repo == nil {
				log.Warn("skipping room assignment — no database connection configured")
				roomName = setting.Room
			} else {
				log.Info("assigning room", "room", setting.Room, "lodging_id", setting.LodgingID)

				user, err := s.repo.GetUserByEmail(setting.Account)
				if err != nil {
					log.Error("user not found", "account", setting.Account, "error", err)
					failureReason = fmt.Sprintf("user %s not found", setting.Account)
					break
				}

				_, err = s.repo.GetLodging(user.UserID, setting.LodgingID)
				if err != nil {
					log.Error("lodging not found", "lodging_id", setting.LodgingID, "error", err)
					failureReason = fmt.Sprintf("lodging %d not found for user %s", setting.LodgingID, setting.Account)
					break
				}

				roomType := setting.RoomType
				if roomType == "" {
					roomType = "room"
				}
				zone, err := s.repo.FindOrCreateZone(setting.LodgingID, setting.Room, roomType)
				if err != nil {
					log.Error("failed to create room", "error", err)
					failureReason = err.Error()
					continue
				}

				if err := s.repo.AssignDeviceToZone(info.AWSThingName, zone.ZoneID, info.Name); err != nil {
					log.Error("failed to assign device to room", "error", err)
					failureReason = err.Error()
					continue
				}

				roomName = setting.Room
				roomID = zone.ZoneID
				log.Info("room assigned", "zone_id", zone.ZoneID)
			}
		}

		succeeded = true
		break
	}

	_ = s.commander.CancelIndication(ch)
	time.Sleep(1500 * time.Millisecond)

	if succeeded {
		log.Info("deployment completed successfully")
		_ = s.commander.SetIndicate(ch, opts.SuccessColor, opts.SuccessColor, opts.SuccessColor, 3600)
		s.writeResult(bay, info, true, roomID, roomName, "")
	} else {
		log.Error("deployment failed", "reason", failureReason)
		_ = s.commander.SetIndicate(ch, opts.FailColor, opts.FailColor, opts.FailColor, 3600)
		s.writeResult(bay, info, false, roomID, roomName, failureReason)
	}
}

func (s *Service) writeResult(bay int, info *models.DeviceInfo, succeeded bool, roomID int, roomName, reason string) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()

	result := &models.DeploymentResult{
		Bay:       bay,
		DeviceID:  info.AWSThingName,
		MACAddr:   info.WiFiMAC,
		Succeeded: succeeded,
		RoomID:    roomID,
		RoomName:  roomName,
		Reason:    reason,
	}

	if err := AppendResult(s.outputFile, result); err != nil {
		s.log.Error("failed to write deployment result", "error", err)
	}
}
