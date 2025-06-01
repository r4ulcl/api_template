package database

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// DB is the global database connection instance.
var DB *gorm.DB

// BaseController provides a wrapper around database operations.
//
// It embeds the GORM database instance to facilitate CRUD operations.
type BaseController struct {
	DB *gorm.DB
}

// ConnectDB initializes and establishes a connection to the database.
//
// It attempts to connect up to 5 times with a 5-second delay between attempts.
// If the connection fails after 5 attempts, the application exits with an error.
//
// Parameters:
// - cfg: A pointer to the configuration containing database credentials.
//
// This function also performs automatic migrations for all registered models.
func ConnectDB(cfg *utils.Config) {
	dsn := cfg.DSN() // Generate the database connection string

	var db *gorm.DB

	var err error

	seconds := 5

	// Retry connection up to 5 times
	for attempts := 1; attempts <= 5; attempts++ {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			SkipDefaultTransaction: true,
			NamingStrategy:         schema.NamingStrategy{},
			Logger:                 logger.Default.LogMode(logger.Silent),
			NowFunc:                time.Now,
		})
		if err == nil {
			log.Println("Connected to MySQL successfully.")

			break
		}

		const maxRetries = 5
		if attempts == maxRetries {
			log.Fatalf("Failed to connect to MySQL after %d attempts: %v", attempts, err)
		}

		log.Printf("Failed to connect to MySQL, retrying in %d seconds... (Attempt %d/5)", seconds, attempts)
		time.Sleep(time.Duration(seconds) * time.Second)
	}

	// AutoMigrate all models
	err = db.Debug().AutoMigrate(&models.Example1{}, &models.Example2{}, &models.User{})
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	// AutoMigrate relational models separately
	err = db.Debug().AutoMigrate(&models.ExampleRelational{})
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	// Assign the global database instance
	DB = db
}

// CreateOrUpdateRecord attempts to create a new record. If a duplicate key error
// is encountered (and overwrite == true), it falls back to an update.
//
// Parameters:
// - model: A pointer to the struct representing the database entity.
// - overwrite: Whether to update the record on duplicate key conflict.
//
// Returns:
// - An error if creation fails and overwrite is false, or if the update fails.
func (bc *BaseController) CreateOrUpdateRecord(model interface{}, overwrite bool) error {
	// Try to create the record
	if err := bc.DB.Create(model).Error; err != nil {
		// Check if it's a duplicate key error
		if isDuplicateKeyError(err) {
			// Only overwrite (update) if the overwrite flag is true
			if overwrite {
				// Pass an empty string as ID here, so UpdateRecords reads
				// the primary key from the struct itself
				if updateErr := bc.UpdateRecords(model, ""); updateErr != nil {
					return updateErr
				}

				return nil
			}
		}
		// Return any other error (or the duplicate key error if overwrite==false)
		return err
	}

	// If record is created successfully, return nil
	return nil
}

// isDuplicateKeyError checks if the error indicates a unique constraint violation.
// Adjust the checks for your specific DB engine (MySQL, PostgreSQL, etc.).
func isDuplicateKeyError(err error) bool {
	// For PostgreSQL (error code 23505)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return true
	}

	// For MySQL, error code 1062 means 'Duplicate entry'
	// A simple check could be:
	if strings.Contains(err.Error(), "1062") {
		return true
	}

	return false
}

// GetAllRecords retrieves all records of a given type with optional filters.
//
// Filters are applied dynamically, and relationships are preloaded if foreign keys exist.
//
// Parameters:
// - model: A pointer to a slice where retrieved records will be stored.
// - filters: A map of key-value pairs used for filtering results.
//
// Returns:
// - An error if retrieval fails.
func (bc *BaseController) GetAllRecords(model interface{}, filters map[string]interface{}) error {
	tx := bc.DB
	modelType := reflect.TypeOf(model).Elem().Elem() // Get slice element type

	// Apply dynamic filters
	for key, value := range filters {
		tx = tx.Where(key+" = ?", value)
	}

	// Preload relationships dynamically
	for i := range modelType.NumField() {
		field := modelType.Field(i)
		if gormTag, ok := field.Tag.Lookup("gorm"); ok && strings.Contains(gormTag, "foreignKey") {
			tx = tx.Preload(field.Name)
		}
	}

	// Execute query
	return tx.Find(model).Error
}

// GetRecordsByID retrieves a record by its primary key(s).
//
// If the ID is a composite key, it must be provided in a hyphen-separated format.
//
// Parameters:
// - model: A pointer to the struct where the retrieved record will be stored.
// - id: A string representing the primary key(s).
//
// Returns:
// - An error if the record is not found.
func (bc *BaseController) GetRecordsByID(model interface{}, id string) error {
	log.Println("GetRecordsByID", model, id)
	parts := strings.Split(id, "-")
	primaryKeys := getPrimaryKeyFields(model)

	log.Println("GetRecordsByID primaryKeys", primaryKeys)

	if len(primaryKeys) != len(parts) {
		return fmt.Errorf("mismatch between primary keys and tokenized ID")
	}

	// Build a map[columnName]value
	pkMap := make(map[string]interface{}, len(primaryKeys))
	for i, col := range primaryKeys {
		pkMap[col] = parts[i]
	}
	log.Println("GetRecordsByID pkMap", pkMap)

	// GORM will translate the map into `WHERE col1 = ? AND col2 = ? ...`
	if err := bc.DB.First(model, pkMap).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("record not found")
		}
		return err
	}

	return nil
}

// UpdateRecords updates an existing record identified by its primary key(s).
//
// Parameters:
// - model: A pointer to the struct representing the updated data.
// - id: A string representing the primary key(s), separated by "-" if multiple.
//
// Returns:
// - An error if the record is not found or update fails.
func (bc *BaseController) UpdateRecords(model interface{}, id string) error {
	var primaryKeys []string

	var keyValues []string

	if id != "" {
		// When ID is provided, split it and use it
		parts := strings.Split(id, "-")
		primaryKeys = getJSONPrimaryKeys(model)

		if len(primaryKeys) != len(parts) {
			ErrMismatch := errors.New("mismatch between number of primary keys and ID parts")

			return fmt.Errorf("%w", ErrMismatch)
		}

		keyValues = parts
	} else {
		// When ID is empty, extract primary key values from the model
		var err error

		keyValues, err = getPrimaryKeyValues(model)
		if err != nil {
			ErrMismatch := errors.New("failed to get primary key values from model")

			return fmt.Errorf("%w", ErrMismatch)
		}

		primaryKeys = getJSONPrimaryKeys(model)

		if len(primaryKeys) == 0 {
			return errors.New("no primary keys found in the model")
		}
	}

	// Construct the query based on primary keys and their values
	query := bc.DB.Model(model)
	for i, pk := range primaryKeys {
		query = query.Where(pk+" = ?", keyValues[i])
	}

	// Attempt to find the existing record
	if err := query.First(model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("record not found")
		}

		return err
	}

	// Save the updated model
	return bc.DB.Save(model).Error
}

// DeleteRecords deletes a record identified by its primary key(s).
//
// Parameters:
// - model: A pointer to the struct representing the record.
// - id: A string representing the primary key(s).
//
// Returns:
// - An error if deletion fails.
func (bc *BaseController) DeleteRecords(model interface{}, id string) error {
	tx := bc.DB.Debug().
		Session(&gorm.Session{NewDB: true}).
		Model(model)

	// Split the incoming ID by "-" for potential composite keys.
	parts := strings.Split(id, "-")

	// Get all JSON field names where GORM tag includes "primaryKey".
	primaryKeys := getJSONPrimaryKeys(model)

	if len(primaryKeys) != len(parts) {
		return fmt.Errorf("mismatch between primary keys (%d) and tokenized ID parts (%d)",
			len(primaryKeys), len(parts))
	}

	// Reflect on the `model` pointer to reach its underlying struct fields.
	val := reflect.ValueOf(model)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return errors.New("model must be a non-nil pointer to a struct")
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return errors.New("model must point to a struct")
	}

	// We'll iterate through fields in the struct in the same order as `getJSONPrimaryKeys`.
	// Each time we find a primaryKey field, we assign the corresponding `parts[i]`.
	pkCount := 0

	for i := range elem.NumField() {
		fieldType := elem.Type().Field(i)

		gormTag := fieldType.Tag.Get("gorm")
		if strings.Contains(gormTag, "primaryKey") {
			// This field is a primary key. We set its value to parts[pkCount].
			// NOTE: If your PK is an integer, parse parts[pkCount] accordingly.
			fieldValue := elem.Field(i)
			if !fieldValue.CanSet() {
				return fmt.Errorf("cannot set value for field %s", fieldType.Name)
			}
			// For simplicity, assume string primary keys. Adjust if numeric.
			fieldValue.SetString(parts[pkCount])

			pkCount++
		}
	}

	// Now that the primary key fields are updated to match `id`,
	// GORM will generate a delete statement like:
	//    DELETE FROM `example1` WHERE `example1`.`field1` = 'id'
	res := tx.Delete(model)
	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected == 0 {
		return fmt.Errorf("no records deleted for ID %s", id)
	}

	return nil
}

// getPrimaryKeyFields extracts the GORM primary key fields from a struct.
func getPrimaryKeyFields(model interface{}) []string {
	var primaryKeys []string

	val := reflect.ValueOf(model).Elem()
	typ := val.Type()

	for i := range val.NumField() {
		field := typ.Field(i)
		if tag := field.Tag.Get("gorm"); strings.Contains(tag, "primaryKey") {
			primaryKeys = append(primaryKeys, field.Name)
		}
	}

	return primaryKeys
}

// getJSONPrimaryKeys extracts JSON field names for primary keys.
func getJSONPrimaryKeys(model interface{}) []string {
	var keys []string

	typ := reflect.TypeOf(model)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	for i := range typ.NumField() {
		field := typ.Field(i)
		if strings.Contains(field.Tag.Get("gorm"), "primaryKey") {
			jsonTag := field.Tag.Get("json")
			// Handle cases where json tag might have options like "id,omitempty"
			jsonField := strings.Split(jsonTag, ",")[0]
			keys = append(keys, jsonField)
		}
	}

	return keys
}

// getPrimaryKeyValues extracts the primary key values from the model.
func getPrimaryKeyValues(model interface{}) ([]string, error) {
	var values []string

	val := reflect.ValueOf(model).Elem()
	primaryKeys := getPrimaryKeyFields(model)

	for _, pk := range primaryKeys {
		fieldVal := val.FieldByName(pk)
		if !fieldVal.IsValid() {
			return nil, fmt.Errorf("primary key field %s not found in model", pk)
		}
		// Convert the field value to string
		values = append(values, fmt.Sprintf("%v", fieldVal.Interface()))
	}

	return values, nil
}
