package service

import (
	"context"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"gorm.io/gorm"
)

type AnnotationsService struct{ db *gorm.DB }

func NewAnnotationsService(db *gorm.DB) AnnotationsService {
	return AnnotationsService{db: db}
}

type CreateAnnotationParams struct {
	AppName   string
	Content   string
	Timestamp time.Time
}

// CreateAnnotation creates a single annotation for a given application
// It does not check if the application has consumed any data
func (svc AnnotationsService) CreateAnnotation(ctx context.Context, params CreateAnnotationParams) (*model.Annotation, error) {
	var u model.Annotation

	u.AppName = params.AppName
	u.Content = params.Content
	u.Timestamp = params.Timestamp

	tx := svc.db.WithContext(ctx)

	// Upsert
	if err := tx.Where(model.Annotation{
		AppName: params.AppName, Timestamp: params.Timestamp,
	}).Attrs(u).FirstOrCreate(&u).Error; err != nil {
		return nil, err
	}

	return &u, nil
}

// FindAnnotationsByTimeRange finds all annotations for an app in a time range
func (svc AnnotationsService) FindAnnotationsByTimeRange(ctx context.Context, appName string, startTime time.Time, endTime time.Time) ([]model.Annotation, error) {
	tx := svc.db.WithContext(ctx)
	var u []model.Annotation

	if err := tx.Where("app_name = ?", appName).Where("timestamp between ? and ?", startTime, endTime).Find(&u).Error; err != nil {
		return u, err
	}

	return u, nil
}
