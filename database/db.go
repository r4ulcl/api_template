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
	dsn := cfg.DSN()

	var (
		db  *gorm.DB
		err error
	)

	const (
		maxRetries = 5
		retryDelay = 5 * time.Second
	)

	// --- 1) CONNECT WITH RETRIES ---
	for attempt := 1; attempt <= maxRetries; attempt++ {
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			SkipDefaultTransaction: true,
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
			Logger:  logger.Default.LogMode(logger.Silent),
			NowFunc: time.Now,
		})
		if err == nil {
			log.Println("Connected to MySQL successfully.")
			break
		}

		if attempt == maxRetries {
			log.Fatalf("Failed to connect after %d attempts: %v", attempt, err)
		}
		log.Printf("Connection failed, retrying in %v (attempt %d/%d)…", retryDelay, attempt, maxRetries)
		time.Sleep(retryDelay)
	}

	// --- 2) COLLECT MODELS FOR MIGRATION ---
	regular := make([]interface{}, 0, len(models.ModelMap))
	relational := make([]interface{}, 0, len(models.RelationalModelKeys))

	// Build a quick lookup for relational keys
	relKeys := map[string]struct{}{}
	for _, k := range models.RelationalModelKeys {
		relKeys[k] = struct{}{}
	}

	// Split ModelMap entries into regular vs relational
	for name, mdl := range models.ModelMap {
		if _, isRel := relKeys[name]; isRel {
			relational = append(relational, mdl)
		} else {
			regular = append(regular, mdl)
		}
	}

	// --- 3) MIGRATE REGULAR MODELS ---
	if err := db.Debug().AutoMigrate(regular...); err != nil {
		log.Fatalf("AutoMigrate (regular models) failed: %v", err)
	}
	log.Printf("AutoMigrate: %d regular models", len(regular))

	// --- 4) MIGRATE RELATIONAL MODELS ---
	if len(relational) > 0 {
		if err := db.Debug().AutoMigrate(relational...); err != nil {
			log.Fatalf("AutoMigrate (relational models) failed: %v", err)
		}
		log.Printf("AutoMigrate: %d relational models", len(relational))
	}

	// --- 5) ASSIGN GLOBAL INSTANCE ---
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
	modelType := reflect.TypeOf(model).Elem().Elem()

	// Apply dynamic filters
	for key, value := range filters {
		tx = tx.Where(key+" = ?", value)
	}

	// Preload relationships dynamically
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if gormTag, ok := field.Tag.Lookup("gorm"); ok && strings.Contains(gormTag, "foreignKey:") {
			tx = tx.Preload(field.Name)
		}
	}

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
	parts := strings.Split(id, "-")

	pkCols, err := getDBPrimaryKeyColumns(bc.DB, model)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}
	if len(pkCols) != len(parts) {
		return fmt.Errorf("mismatch between primary keys and tokenized ID")
	}

	pkMap := make(map[string]interface{}, len(pkCols))
	for i, col := range pkCols {
		pkMap[col] = parts[i]
	}

	// Preload belongs-to relations if present
	modelType := reflect.TypeOf(model).Elem()
	tx := bc.DB
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if gormTag, ok := field.Tag.Lookup("gorm"); ok && strings.Contains(gormTag, "foreignKey:") {
			tx = tx.Preload(field.Name)
		}
	}

	if err := tx.First(model, pkMap).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("record not found")
		}
		return err
	}
	return nil
}

func (bc *BaseController) GetRecordsByIDWithSensitive(model interface{}, id string) error {
	if model == nil {
		return errors.New("model must be a non-nil pointer to a struct")
	}

	parts := strings.Split(id, "-")
	pkCols, err := getDBPrimaryKeyColumns(bc.DB, model)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}
	if len(pkCols) != len(parts) {
		return fmt.Errorf("mismatch between primary keys and tokenized ID")
	}

	pkMap := make(map[string]interface{}, len(pkCols))
	for i, col := range pkCols {
		pkMap[col] = parts[i]
	}

	tx := bc.DB.Session(&gorm.Session{NewDB: true}).Select("*")

	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if gTag, ok := field.Tag.Lookup("gorm"); ok && strings.Contains(gTag, "foreignKey:") {
			tx = tx.Preload(field.Name)
		}
	}

	if err := tx.First(model, pkMap).Error; err != nil {
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
		parts := strings.Split(id, "-")

		dbPKCols, err := getDBPrimaryKeyColumns(bc.DB, model)
		if err != nil {
			return fmt.Errorf("failed to parse schema: %w", err)
		}
		if len(dbPKCols) != len(parts) {
			return fmt.Errorf("mismatch between number of primary keys and ID parts")
		}
		primaryKeys = dbPKCols
		keyValues = parts
	} else {
		// extract PKs from model itself
		var err error
		keyValues, err = getPrimaryKeyValues(model)
		if err != nil {
			return fmt.Errorf("failed to get primary key values from model: %w", err)
		}
		// map struct field PKs to DB column names
		dbPKCols, err := getDBPrimaryKeyColumns(bc.DB, model)
		if err != nil {
			return fmt.Errorf("failed to parse schema: %w", err)
		}
		if len(dbPKCols) == 0 {
			return errors.New("no primary keys found in the model")
		}
		primaryKeys = dbPKCols
	}

	tx := bc.DB.Model(model)
	for i, col := range primaryKeys {
		tx = tx.Where(col+" = ?", keyValues[i])
	}

	if err := tx.Updates(model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("record not found")
		}
		return err
	}
	return nil
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
	parts := strings.Split(id, "-")

	pkCols, err := getDBPrimaryKeyColumns(bc.DB, model)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}
	if len(pkCols) != len(parts) {
		return fmt.Errorf("mismatch between primary keys (%d) and tokenized ID parts (%d)", len(pkCols), len(parts))
	}

	tx := bc.DB.Debug().Session(&gorm.Session{NewDB: true}).Model(model)
	for i, col := range pkCols {
		tx = tx.Where(col+" = ?", parts[i])
	}

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

// helpers: use GORM schema to get PK db column names in correct order
func getDBPrimaryKeyColumns(db *gorm.DB, model interface{}) ([]string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(stmt.Schema.PrimaryFields))
	for _, f := range stmt.Schema.PrimaryFields {
		cols = append(cols, f.DBName) // real db column name
	}
	return cols, nil
}
