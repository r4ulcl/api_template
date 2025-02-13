package database

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/r4ulcl/api_template/utils"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
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
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("Connected to MySQL successfully.")
			break
		}

		if attempts == 5 {
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

// CreateUser creates a new user in the database.
//
// If the username already exists, user creation is skipped.
// If an admin flag is set, the user will be assigned an admin role.
//
// Parameters:
// - username: The username for the new user.
// - password: The plaintext password (will be hashed before storage).
// - admin: A boolean indicating if the user should have admin privileges.
//
// Returns:
// - An error if the user already exists or if an issue occurs during creation.
func CreateUser(username, password string, admin bool) error {
	if password == "" {
		return fmt.Errorf("password not set, skipping user creation")
	}

	// Check if the user already exists
	var existingUser models.User
	if err := DB.Where("username = ?", username).First(&existingUser).Error; err == nil {
		return fmt.Errorf("user already exists, skipping creation")
	}

	// Hash the password before storing it
	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %v", err)
	}

	// Determine the user's role
	role := models.UserRole
	if admin {
		role = models.AdminRole
	}

	// Create and save the new user
	user := models.User{
		Username: username,
		Password: hashedPassword,
		Role:     role,
	}

	if err := DB.Create(&user).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return fmt.Errorf("user with this username already exists")
		}
		return fmt.Errorf("failed to create user: %v", err)
	}

	log.Printf("User created with username: %s", username)
	return nil
}

// CreateRecord inserts a new record into the database.
//
// This is a generic function that accepts any model struct.
//
// Parameters:
// - model: A pointer to the struct representing the database table.
//
// Returns:
// - An error if record creation fails.
func (bc *BaseController) CreateRecord(model interface{}) error {
	return bc.DB.Create(model).Error
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
	for i := 0; i < modelType.NumField(); i++ {
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
	parts := strings.Split(id, "-")
	primaryKeys := getPrimaryKeyFields(model)

	if len(primaryKeys) != len(parts) {
		return fmt.Errorf("Mismatch between primary keys and tokenized ID")
	}

	conditions := []interface{}{}
	for i := range primaryKeys {
		conditions = append(conditions, parts[i])
	}

	if err := bc.DB.First(model, conditions...).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("Record not found")
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
			return fmt.Errorf("mismatch between number of primary keys and ID parts")
		}

		keyValues = parts
	} else {
		// When ID is empty, extract primary key values from the model
		var err error
		keyValues, err = getPrimaryKeyValues(model)
		if err != nil {
			return fmt.Errorf("failed to get primary key values from model: %v", err)
		}
		primaryKeys = getJSONPrimaryKeys(model)

		if len(primaryKeys) == 0 {
			return fmt.Errorf("no primary keys found in the model")
		}
	}

	// Construct the query based on primary keys and their values
	query := bc.DB.Model(model)
	for i, pk := range primaryKeys {
		query = query.Where(pk+" = ?", keyValues[i])
	}

	// Attempt to find the existing record
	if err := query.First(model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("record not found")
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
		return fmt.Errorf("model must be a non-nil pointer to a struct")
	}
	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return fmt.Errorf("model must point to a struct")
	}

	// We'll iterate through fields in the struct in the same order as `getJSONPrimaryKeys`.
	// Each time we find a primaryKey field, we assign the corresponding `parts[i]`.
	pkCount := 0
	for i := 0; i < elem.NumField(); i++ {
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

	for i := 0; i < val.NumField(); i++ {
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

	for i := 0; i < typ.NumField(); i++ {
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
