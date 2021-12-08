package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
)

func createUserTableMigration() *gormigrate.Migration {
	type User struct {
		gorm.Model

		FullName     string     `gorm:"type:varchar(255);default:null"`
		Email        string     `gorm:"type:varchar(255);not null;default:null;index:,unique"`
		PasswordHash []byte     `gorm:"type:varchar(255);not null;default:null"`
		Role         model.Role `gorm:"not null;default:null"`
		IsDisabled   *bool      `gorm:"not null;default:false"`

		LastSeenAt        time.Time `gorm:"default:null"`
		PasswordChangedAt time.Time `gorm:"not null;default:null"`
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
