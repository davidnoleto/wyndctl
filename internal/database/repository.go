// Package database provides the data access layer using the repository pattern.
package database

import (
	"fmt"

	"github.com/hellowynd/wyndctl/internal/config"
	"github.com/hellowynd/wyndctl/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Repository provides database access methods.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new database repository from config.
func NewRepository(cfg config.DBConfig) (*Repository, error) {
	dsn := cfg.BuildDSN()
	if dsn == "" {
		return nil, fmt.Errorf("database connection is required; set PG_HOST/PG_USER/PG_PASSWORD env vars, WYND_DB_DSN, or db settings in wyndctl.yaml")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	return &Repository{db: db}, nil
}

// CreateUser creates a new user account and default notification profile.
// cognito_username is set to the email to match the Python CLI convention.
func (r *Repository) CreateUser(email, fullName string) (*models.User, error) {
	var existing models.User
	if err := r.db.Where("email = ?", email).First(&existing).Error; err == nil {
		return nil, fmt.Errorf("user with email %q already exists (user_id=%d)", email, existing.UserID)
	}

	cognitoUsername := email
	user := models.User{
		Email:           email,
		FullName:        fullName,
		Status:          models.UserStatusConfirmed,
		CognitoUsername: &cognitoUsername,
	}
	if err := r.db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	profile := models.UserProfile{
		UserID:          user.UserID,
		IsEmailEnabled:  true,
		IsPushEnabled:   true,
		IsSMSEnabled:    false,
		IsPhoneVerified: false,
	}
	if err := r.db.Create(&profile).Error; err != nil {
		return nil, fmt.Errorf("creating user profile: %w", err)
	}

	return &user, nil
}

// GetUserByEmail finds a user by their email address.
func (r *Repository) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	result := r.db.Where("email = ?", email).First(&user)
	if result.Error != nil {
		return nil, fmt.Errorf("finding user %q: %w", email, result.Error)
	}
	return &user, nil
}

// CreateLodging creates a new property for the given owner.
func (r *Repository) CreateLodging(ownerID int, name, address string, extraData map[string]interface{}) (*models.Lodging, error) {
	lodging := models.Lodging{
		OwnerID:   ownerID,
		Name:      name,
		Address:   address,
		ExtraData: extraData,
	}
	if err := r.db.Create(&lodging).Error; err != nil {
		return nil, fmt.Errorf("creating lodging: %w", err)
	}
	return &lodging, nil
}

// ListLodgings returns all lodgings owned by the given user.
func (r *Repository) ListLodgings(ownerID int) ([]models.Lodging, error) {
	var lodgings []models.Lodging
	if err := r.db.Where("owner_id = ?", ownerID).Find(&lodgings).Error; err != nil {
		return nil, fmt.Errorf("listing lodgings: %w", err)
	}
	return lodgings, nil
}

// GetLodging finds a lodging by ID and owner.
func (r *Repository) GetLodging(ownerID, lodgingID int) (*models.Lodging, error) {
	var lodging models.Lodging
	result := r.db.Where("owner_id = ? AND lodging_id = ?", ownerID, lodgingID).First(&lodging)
	if result.Error != nil {
		return nil, fmt.Errorf("finding lodging %d: %w", lodgingID, result.Error)
	}
	return &lodging, nil
}

// FindOrCreateZone gets or creates a room in a lodging.
func (r *Repository) FindOrCreateZone(lodgingID int, name, roomType string) (*models.Zone, error) {
	var zone models.Zone
	result := r.db.Where("lodging_id = ? AND name = ?", lodgingID, name).First(&zone)
	if result.Error == nil {
		return &zone, nil
	}

	zone = models.Zone{
		LodgingID: lodgingID,
		Name:      name,
		ExtraData: map[string]interface{}{"type": roomType},
	}
	if err := r.db.Create(&zone).Error; err != nil {
		return nil, fmt.Errorf("creating zone: %w", err)
	}

	return &zone, nil
}

// AssignDeviceToZone associates a device with a room.
func (r *Repository) AssignDeviceToZone(deviceID string, zoneID int, name string) error {
	extraData := map[string]interface{}{"name": name}
	device := models.Device{
		DeviceID:  deviceID,
		ZoneID:    &zoneID,
		ExtraData: extraData,
	}

	result := r.db.
		Where("device_id = ?", deviceID).
		Assign(models.Device{ZoneID: &zoneID, ExtraData: extraData}).
		FirstOrCreate(&device)
	return result.Error
}

// DeleteLodging removes one or all lodgings owned by ownerID, mirroring the
// Python delete_lodgings_of_user flow: LodgingIntegration rows are deleted
// first (no DB cascade), then Lodging rows (zones cascade at the DB level).
// Pass a non-nil lodgingID to scope deletion to a single property.
// Returns the IDs of every deleted lodging.
func (r *Repository) DeleteLodging(ownerID int, lodgingID *int) ([]int, error) {
	query := r.db.Where("owner_id = ?", ownerID)
	if lodgingID != nil {
		query = query.Where("lodging_id = ?", *lodgingID)
	}

	var lodgings []models.Lodging
	if err := query.Find(&lodgings).Error; err != nil {
		return nil, fmt.Errorf("finding lodgings: %w", err)
	}
	if len(lodgings) == 0 {
		return nil, nil
	}

	ids := make([]int, len(lodgings))
	for i, l := range lodgings {
		ids[i] = l.LodgingID
	}

	if err := r.db.Where("lodging_id IN ?", ids).Delete(&models.LodgingIntegration{}).Error; err != nil {
		return nil, fmt.Errorf("deleting lodging integrations: %w", err)
	}
	if err := r.db.Where("lodging_id IN ?", ids).Delete(&models.Lodging{}).Error; err != nil {
		return nil, fmt.Errorf("deleting lodgings: %w", err)
	}

	return ids, nil
}

// DeleteDevices clears device-zone associations for a user's lodging.
// Returns the device IDs (AWS Thing names) of all affected devices. The
// device rows themselves are kept so they can be redeployed; only zone_id
// is nulled out. If deleteRooms is true, the associated zone rows are also
// removed.
func (r *Repository) DeleteDevices(ownerID int, lodgingID *int, deviceID *string, deleteRooms bool) ([]string, error) {
	query := r.db.Model(&models.Device{}).
		Joins("JOIN zone ON device.zone_id = zone.zone_id").
		Joins("JOIN lodging ON zone.lodging_id = lodging.lodging_id").
		Where("lodging.owner_id = ?", ownerID)

	if lodgingID != nil {
		query = query.Where("lodging.lodging_id = ?", *lodgingID)
	}
	if deviceID != nil {
		query = query.Where("device.device_id = ?", *deviceID)
	}

	var devices []models.Device
	if err := query.Find(&devices).Error; err != nil {
		return nil, fmt.Errorf("finding devices: %w", err)
	}

	var thingNames []string
	for _, dev := range devices {
		zoneID := dev.ZoneID
		dev.ZoneID = nil
		if err := r.db.Save(&dev).Error; err != nil {
			return thingNames, fmt.Errorf("clearing zone on device %q: %w", dev.DeviceID, err)
		}

		if deleteRooms && zoneID != nil {
			if err := r.db.Delete(&models.Zone{}, *zoneID).Error; err != nil {
				return thingNames, fmt.Errorf("deleting zone %d: %w", *zoneID, err)
			}
		}

		thingNames = append(thingNames, dev.DeviceID)
	}

	return thingNames, nil
}

// Close closes the database connection.
func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
