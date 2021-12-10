package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func createUserTableMigration() *gormigrate.Migration {
	type User struct {
		ID           uint    `gorm:"primarykey"`
		Name         string  `gorm:"type:varchar(255);not null;default:null;index:,unique"`
		Email        string  `gorm:"type:varchar(255);not null;default:null;index:,unique"`
		FullName     *string `gorm:"type:varchar(255);default:null"`
		PasswordHash []byte  `gorm:"type:varchar(255);not null;default:null"`
		Role         int     `gorm:"not null;default:null"`
		IsDisabled   *bool   `gorm:"not null;default:false"`

		LastSeenAt        *time.Time `gorm:"default:null"`
		PasswordChangedAt time.Time
		CreatedAt         time.Time
		UpdatedAt         time.Time
		DeletedAt         gorm.DeletedAt
	}

	return &gormigrate.Migration{
		ID: "1638496809",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&User{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&User{})
		},
	}
}
