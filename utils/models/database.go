package models

import (
	"reflect"
	"strings"
)

// RelationalModelKeys lists which ModelMap entries need their own AutoMigrate pass.
// NormalModelKeys lists models to migrate in the first (non-relational) pass.
var (
	RelationalModelKeys []string
	NormalModelKeys     []string
)

func init() {
	// Automatically detect relational vs. normal tables by checking for gorm foreignKey tags
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

// ModelMap maps resource‐names to model pointers. (Update this with all models)
var ModelMap = map[string]interface{}{
	"user":              &User{}, // Do not delete
	"example1":          &Example1{},
	"example2":          &Example2{},
	"exampleRelational": &ExampleRelational{},
}

// Example1 represents a database table storing example data.
//
// This struct is mapped to a table where Field1 serves as the primary key.
type Example1 struct {
	// Field1 is the primary key of the Example1 table.
	Field1 string `gorm:"column:field1;primaryKey" json:"field1"`

	// Field2 stores additional data related to Example1.
	Field2 string `gorm:"column:field2" json:"field2"`
}

// Example2 represents another database table storing example data.
type Example2 struct {
	Field1 string `gorm:"column:field1;primaryKey" json:"field1"`
	Field2 string `gorm:"column:field2"            json:"field2"`
}

// ExampleRelational represents a relational table connecting Example1 and Example2.
//
// This struct defines a many-to-many relationship between Example1 and Example2.
type ExampleRelational struct {
	// Example1Field1 is a foreign key referencing Example1.
	Example1Field1 string `gorm:"primaryKey;column:example1_field1" json:"example1_field1"`

	// Example2Field1 is a foreign key referencing Example2.
	Example2Field1 string `gorm:"primaryKey;column:example2_field1" json:"example2_field1"`

	// Field3 stores additional relationship-related information.
	Field3 string `gorm:"column:field3" json:"field3"`

	// Example1Reference establishes a foreign key relationship with Example1.
	// Updates and deletions on Example1 cascade to ExampleRelational.
	Example1Reference Example1 `gorm:"foreignKey:Example1Field1;references:Field1;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`

	// Example2Reference establishes a foreign key relationship with Example2.
	// Updates and deletions on Example2 cascade to ExampleRelational.
	Example2Reference Example2 `gorm:"foreignKey:Example2Field1;references:Field1;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}
