package utils

import (
	"fmt"
	"os"
)

// Config struct holds the configuration variables needed for connecting to a database and managing JWT.
type Config struct {
	DBHost        string // Database host (e.g., "localhost")
	DBPort        string // Database port (e.g., "3306")
	DBUser        string // Database username (e.g., "root")
	DBPassword    string // Database password (e.g., "password")
	DBName        string // Database name (e.g., "demo_db")
	JWTSecret     string // JWT secret key for token signing
	AdminPassword string // Admin password (e.g., "admin_secret")
	UserGUI       string // Allow user to access stats
}

// LoadConfig loads environment variables or uses default values for database and authentication configuration.
func LoadConfig() *Config {
	return &Config{
		DBHost:        getEnv("DB_HOST", "localhost"),              // Default: localhost
		DBPort:        getEnv("DB_PORT", "3306"),                   // Default: 3306
		DBUser:        getEnv("DB_USER", "root"),                   // Default: root
		DBPassword:    getEnv("DB_PASSWORD", ""),                   // Default: empty string
		DBName:        getEnv("DB_NAME", "demo_db"),                // Default: demo_db
		JWTSecret:     getEnv("JWT_SECRET", "your_jwt_secret_key"), // Default: "your_jwt_secret_key"
		AdminPassword: getEnv("ADMIN_PASSWORD", ""),                // Default: empty string
		UserGUI:       getEnv("USER_GUI", "false"),
	}
}

// DSN constructs a Data Source Name (DSN) for the database connection string.
func (c *Config) DSN() string {
	// The format used in MySQL connection string is: user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser,
		c.DBPassword,
		c.DBHost,
		c.DBPort,
		c.DBName,
	)
}

// getEnv retrieves the value of an environment variable or returns a default value if the variable is not set.
func getEnv(key, defaultVal string) string {
	// os.LookupEnv checks if the environment variable exists and returns its value if found.
	// If not found, it returns the default value provided as the second argument.
	if val, ok := os.LookupEnv(key); ok {
		return val
	}

	return defaultVal
}
