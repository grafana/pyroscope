package model

import "time"

type Annotation struct {
	ID        uint
	AppName   string
	Content   string
	From      time.Time
	Until     time.Time
	CreatedAt time.Time
}
