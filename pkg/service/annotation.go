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
	Timestamp time.Time
	Content   string
}

// TODO(eh-am): upsert
func (svc AnnotationsService) CreateAnnotation(ctx context.Context, params CreateAnnotationParams) (*model.Annotation, error) {
	var u model.Annotation

	u.AppName = params.AppName
	u.Content = params.Content
	u.From = params.Timestamp

	tx := svc.db.WithContext(ctx)
	if err := tx.Create(&u).Error; err != nil {
		return nil, err
	}

	return &u, nil
}

// FindAnnotationsByTimeRange finds all annotations for an app in a time range
func (svc AnnotationsService) FindAnnotationsByTimeRange(ctx context.Context, appName string, from time.Time, until time.Time) ([]model.Annotation, error) {
	tx := svc.db.WithContext(ctx)
	var u []model.Annotation

	//	if err := tx.Where("app_name = ?", appName).Where("from BETWEEN ? AND ?", from, until).Find(&u).Error; err != nil {
	if err := tx.Where("app_name = ?", appName).Find(&u).Error; err != nil {
		return u, err
	}

	return u, nil
}
