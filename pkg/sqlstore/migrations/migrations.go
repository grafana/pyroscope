package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// Migrate executes all migrations UP that did not run yet.
//
// 1. Migrations must be backward compatible and only extend the schema.
// 2. Migration ID must be a unix epoch time in seconds (use 'date +%s').
// 3. Although in the current shape schema downgrades are not supported,
//    Rollback function must be also provided, and migrations must not import
//    any models in order to avoid side effects: instead, the type should be
//    explicitly defined within the migration body.
// 4. Migration code must be tested within the corresponding service package.
//
// A note on schema downgrades (not supported yet):
//
// As long as migrations are backward compatible, support for schema downgrade
// is not required. Once we introduce a breaking change (e.g, change column
// name or modify the data in any way), downgrades must be supported.
//
// The problem is that this requires migrator to know the future schema:
// v0.4.4 can't know which actions should be taken to revert the database from
// v0.4.5, but not vice-versa (unless they use the same source of migration
// scripts). There are some options:
//  - Changes can be reverted with the later version (that is installed before
//    the application downgrade) or with an independent tool – in containerized
//    deployments this can cause significant difficulties.
//  - Store migrations (as raw SQL scripts) in the database itself or elsewhere
//    locally/remotely.
//
// Before we have a very strong reason to perform a schema downgrade or violate
// the schema backward compatibility guaranties, we should follow the basic
// principle: "... to maintain backwards compatibility between the DB and all
// versions of the code currently deployed in production."
// (https://flywaydb.org/documentation/concepts/migrations#important-notes).
// On the other hand, "the lack of an effective rollback script can be a gating
// factor in the integration and deployment process" (Database Reliability
// Engineering by Laine Campbell & Charity Majors).
func Migrate(db *gorm.DB) error {
	return gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		createUserTableMigration(),
		createAPIKeyTableMigration(),
	}).Migrate()
}

func createUserTableMigration() *gormigrate.Migration {
	type user struct {
		ID                uint       `gorm:"primarykey"`
		Name              string     `gorm:"type:varchar(255);not null;default:null;index:,unique"`
		Email             string     `gorm:"type:varchar(255);not null;default:null;index:,unique"`
		FullName          *string    `gorm:"type:varchar(255);default:null"`
		PasswordHash      []byte     `gorm:"type:varchar(255);not null;default:null"`
		Role              int        `gorm:"not null;default:null"`
		IsDisabled        *bool      `gorm:"not null;default:false"`
		LastSeenAt        *time.Time `gorm:"default:null"`
		PasswordChangedAt time.Time

		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt gorm.DeletedAt
	}

	return &gormigrate.Migration{
		ID: "1638496809",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&user{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&user{})
		},
	}
}

func createAPIKeyTableMigration() *gormigrate.Migration {
	type apiKey struct {
		ID         uint       `gorm:"primarykey"`
		Name       string     `gorm:"type:varchar(255);not null;default:null;index:,unique"`
		Signature  string     `gorm:"type:varchar(255);not null;default:null"`
		Role       int        `gorm:"not null;default:null"`
		ExpiresAt  *time.Time `gorm:"default:null"`
		LastSeenAt *time.Time `gorm:"default:null"`

		CreatedAt time.Time
		DeletedAt gorm.DeletedAt
	}

	return &gormigrate.Migration{
		ID: "1641917891",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&apiKey{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&apiKey{})
		},
	}
}
