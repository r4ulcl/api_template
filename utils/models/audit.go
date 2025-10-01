package models

import "time"

type AuditLog struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	ActorID    string `gorm:"index" json:"actor_id"`
	Action     string `gorm:"size:128;index" json:"action"` // create, read, update, delete
	Resource   string `gorm:"size:64;index" json:"resource"`
	ResourceID string `gorm:"size:64;index" json:"resource_id"`
	// minimal request context
	Path      string `gorm:"size:255" json:"path"`
	Method    string `gorm:"size:8" json:"method"`
	Status    int    `json:"status"`
	IP        string `gorm:"size:64" json:"ip"`
	UserAgent string `gorm:"size:255" json:"user_agent"`
	RequestID string `gorm:"size:64;index" json:"request_id"`
	// payload changes captured as JSON
	Changes string `gorm:"type:json" json:"changes"`

	// Audit
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUpdate time.Time `gorm:"column:last_update;autoUpdateTime" json:"last_update"`
	CreatedBy  string    `gorm:"column:created_by" json:"created_by"`
	EditedBy   string    `gorm:"column:edited_by" json:"edited_by"`
}
