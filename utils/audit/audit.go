package audit

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/r4ulcl/api_template/database"
	"github.com/r4ulcl/api_template/utils/models"
)

var LogReads bool // set from env

type Change struct {
	Before any `json:"before,omitempty"`
	After  any `json:"after,omitempty"`
	Diff   any `json:"diff,omitempty"` // optional
}

func Log(c *gin.Context, actorID string, action, resource, resourceID string, changes *Change, status int) {
	rid, _ := c.Get("request_id")
	ip := c.ClientIP()
	ua := c.Request.UserAgent()

	var changesJSON string
	if changes != nil {
		b, _ := json.Marshal(changes)
		changesJSON = string(b)
	}

	al := models.AuditLog{
		ActorID:    actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Path:       c.FullPath(),
		Method:     c.Request.Method,
		Status:     status,
		IP:         ip,
		UserAgent:  ua,
		RequestID:  rid.(string),
		Changes:    changesJSON,
	}
	database.DB.Create(&al)
}
