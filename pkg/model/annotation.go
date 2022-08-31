package model

import "time"

type Annotation struct {
	ID        uint      `json:"-"`
	AppName   string    `json:"appName"`
	Content   string    `json:"content"`
	From      time.Time `json:"timestamp"`
	Until     time.Time `json:"-"`
	CreatedAt time.Time `json:"-"`
}
