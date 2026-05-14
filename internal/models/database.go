package models

// UserStatus mirrors the Python UserStatus enum stored in the DB.
type UserStatus string

const (
	UserStatusConfirmed UserStatus = "CONFIRMED"
)

// User represents a Wynd platform user account.
type User struct {
	UserID   int        `gorm:"primaryKey;column:user_id"`
	Email    string     `gorm:"column:email"`
	FullName string     `gorm:"column:full_name"`
	Status   UserStatus `gorm:"column:status"`
}

func (User) TableName() string { return "user" }

// Lodging represents a property owned by a user.
type Lodging struct {
	LodgingID int    `gorm:"primaryKey;column:lodging_id"`
	OwnerID   int    `gorm:"column:owner_id"`
	Name      string `gorm:"column:name"`
}

func (Lodging) TableName() string { return "lodging" }

// Zone represents a room within a lodging.
type Zone struct {
	ZoneID    int                    `gorm:"primaryKey;column:zone_id"`
	LodgingID int                    `gorm:"column:lodging_id"`
	Name      string                 `gorm:"column:name"`
	ExtraData map[string]interface{} `gorm:"column:extra_data;serializer:json"`
}

func (Zone) TableName() string { return "zone" }

// Device represents a Sentry device registered in the platform.
type Device struct {
	DeviceID  string                 `gorm:"primaryKey;column:device_id"`
	ZoneID    *int                   `gorm:"column:zone_id"`
	Zone      *Zone                  `gorm:"foreignKey:ZoneID"`
	ExtraData map[string]interface{} `gorm:"column:extra_data;serializer:json"`
}

func (Device) TableName() string { return "device" }
