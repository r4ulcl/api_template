package models

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"
)

// ModelDefinition declares how a resource name maps to a concrete Go model.
type ModelDefinition struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

const (
	modelMapEnvKey      = "MODEL_MAP_FILE"
	defaultModelMapFile = "example_tables/models.json"
)

var (
	// ModelMap maps resource names to model prototypes.
	ModelMap = map[string]interface{}{}
	// RelationalModelKeys lists which ModelMap entries need their own AutoMigrate pass.
	RelationalModelKeys []string
	// NormalModelKeys lists models to migrate in the first (non-relational) pass.
	NormalModelKeys []string

	modelRegistry = map[string]interface{}{
		"User":              &User{},
		"APIKey":            &APIKey{},
		"AuditLog":          &AuditLog{},
		"Example1":          &Example1{},
		"Example2":          &Example2{},
		"ExampleRelational": &ExampleRelational{},
	}
)

func init() {
	if err := LoadModelMap(); err != nil {
		log.Printf("models: %v", err)
	}
}

// RegisterModel lets callers extend the registry so JSON definitions can reference new types.
func RegisterModel(typeKey string, prototype interface{}) {
	modelRegistry[typeKey] = prototype
}

// LoadModelMap refreshes ModelMap from disk or the embedded defaults.
func LoadModelMap() error {
	data, err := readConfig(modelMapEnvKey, defaultModelMapFile)
	if err != nil {
		return err
	}

	var defs []ModelDefinition
	if err := json.Unmarshal(data, &defs); err != nil {
		return fmt.Errorf("models: parse model definitions: %w", err)
	}

	newMap := make(map[string]interface{}, len(defs))
	for _, def := range defs {
		prototype, ok := modelRegistry[def.Type]
		if !ok {
			return fmt.Errorf("models: unknown model type %q for resource %q", def.Type, def.Name)
		}
		newMap[def.Name] = prototype
	}

	ModelMap = newMap
	rebuildModelKeys()
	return nil
}

func rebuildModelKeys() {
	RelationalModelKeys = RelationalModelKeys[:0]
	NormalModelKeys = NormalModelKeys[:0]

	for name, model := range ModelMap {
		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		hasFK := false
		for i := 0; i < t.NumField(); i++ {
			gormTag := t.Field(i).Tag.Get("gorm")
			if strings.Contains(gormTag, "foreignKey:") {
				hasFK = true
				break
			}
		}
		if hasFK {
			RelationalModelKeys = append(RelationalModelKeys, name)
		} else {
			NormalModelKeys = append(NormalModelKeys, name)
		}
	}
}

// Example1 represents a database table storing example data.
//
// This struct is mapped to a table where Field1 serves as the primary key.
type Example1 struct {
	// Field1 is the primary key of the Example1 table.
	Field1 string `gorm:"column:field1;primaryKey" json:"field1"`

	// Field2 stores additional data related to Example1.
	Field2 string `gorm:"column:field2" json:"field2"`

	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUpdate time.Time `gorm:"column:last_update;autoUpdateTime" json:"last_update"`
	CreatedBy  string    `gorm:"column:created_by" json:"created_by"`
	EditedBy   string    `gorm:"column:edited_by" json:"edited_by"`
}

// Example2 represents another database table storing example data.
type Example2 struct {
	Field1 string `gorm:"column:field1;primaryKey" json:"field1"`
	Field2 string `gorm:"column:field2" json:"field2"`

	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUpdate time.Time `gorm:"column:last_update;autoUpdateTime" json:"last_update"`
	CreatedBy  string    `gorm:"column:created_by" json:"created_by"`
	EditedBy   string    `gorm:"column:edited_by" json:"edited_by"`
}

// ExampleRelational represents a relational table connecting Example1 and Example2.
//
// This struct defines a many-to-many relationship between Example1 and Example2.
type ExampleRelational struct {
	Example1Field1 string `gorm:"primaryKey;column:example1_field1" json:"example1_field1"`
	Example2Field1 string `gorm:"primaryKey;column:example2_field1" json:"example2_field1"`
	Field3         string `gorm:"column:field3" json:"field3"`

	// Use pointers and omit when empty
	Example1Reference *Example1 `gorm:"foreignKey:Example1Field1;references:Field1;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"example1,omitempty"`
	Example2Reference *Example2 `gorm:"foreignKey:Example2Field1;references:Field1;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"example2,omitempty"`

	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	LastUpdate time.Time `gorm:"column:last_update;autoUpdateTime" json:"last_update"`
	CreatedBy  string    `gorm:"column:created_by" json:"created_by"`
	EditedBy   string    `gorm:"column:edited_by" json:"edited_by"`
}
