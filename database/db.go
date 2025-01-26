package database

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/r4ulcl/api_template/config"
	"github.com/r4ulcl/api_template/models"
	"github.com/r4ulcl/api_template/utils"
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
func ConnectDB(cfg *config.Config) {
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
// - id: A string representing the primary key(s).
//
// Returns:
// - An error if the record is not found or update fails.
func (bc *BaseController) UpdateRecords(model interface{}, id string) error {
	parts := strings.Split(id, "-")
	primaryKeys := getJSONPrimaryKeys(model)

	if len(primaryKeys) != len(parts) {
		return fmt.Errorf("Mismatch between primary keys and tokenized ID")
	}

	query := bc.DB.Model(model)
	for i, pk := range primaryKeys {
		query = query.Where(pk+" = ?", parts[i])
	}

	if err := query.First(model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("Record not found")
		}
		return err
	}

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
	parts := strings.Split(id, "-")
	primaryKeys := getJSONPrimaryKeys(model)

	if len(primaryKeys) != len(parts) {
		return fmt.Errorf("Mismatch between primary keys and tokenized ID")
	}

	query := bc.DB.Model(model)
	for i, pk := range primaryKeys {
		query = query.Where(pk+" = ?", parts[i])
	}

	return query.Delete(model).Error
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
			keys = append(keys, field.Tag.Get("json"))
		}
	}
	return keys
}
