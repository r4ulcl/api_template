package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/database"
	"github.com/r4ulcl/api_template/utils/models"
)

// Controller provides methods for handling CRUD operations.
//
// It encapsulates a reference to the BaseController for database interactions.
type Controller struct {
	BC *database.BaseController
}

// Create inserts a new record into the database.
//
// It decodes the request body into the provided model, validates the input,
// and creates a new record in the database.
//
// Parameters:
// - w: The HTTP response writer.
// - r: The HTTP request containing the JSON payload.
// - model: A pointer to the struct representing the database entity.
// - overwrite: Bool to create and overwrite if already exists
//
// Returns:
// - HTTP 400 if the request body is invalid.
// - HTTP 201 if the record is successfully created.
func (c *Controller) Create(w http.ResponseWriter, r *http.Request, model interface{}, overwrite bool) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewDecoder(r.Body).Decode(model); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})

		return
	}

	// Use the new CreateOrUpdateRecord function
	if err := c.BC.CreateOrUpdateRecord(model, overwrite); err != nil {
		// If it's a duplicate key error and overwrite == false, or any other DB error
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})

		return
	}

	// If the create (or update) succeeded
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(model)
}

// GetAll retrieves all records with optional filtering.
//
// It parses query parameters to apply filters dynamically and returns the matching records.
//
// Parameters:
// - w: The HTTP response writer.
// - r: The HTTP request containing optional filters as query parameters.
// - model: A pointer to a slice of structs representing the database entity.
//
// Returns:
// - HTTP 500 if the retrieval fails.
// - JSON array of records if successful.
func (c *Controller) GetAll(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// Parse query parameters into filters
	filters := make(map[string]interface{})
	queryParams := r.URL.Query()

	for key, values := range queryParams {
		if len(values) > 0 {
			filters[key] = values[0] // Assuming single value per key
		}
	}

	if err := c.BC.GetAllRecords(model, filters); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})

		return
	}

	_ = json.NewEncoder(w).Encode(model)
}

// GetByID retrieves a single record using composite primary keys.
//
// It extracts the tokenized ID from the URL and fetches the corresponding record.
//
// Parameters:
// - w: The HTTP response writer.
// - r: The HTTP request containing the tokenized ID as a URL parameter.
// - model: A pointer to a struct representing the database entity.
//
// Returns:
// - HTTP 500 if the record is not found or retrieval fails.
// - JSON object of the record if successful.
func (c *Controller) GetByID(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// Extract the tokenized ID from the URL (e.g., "employee_name-server_name")
	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	if err := c.BC.GetRecordsByID(model, tokenizedID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})

		return
	}

	_ = json.NewEncoder(w).Encode(model)
}

// Update modifies an existing record identified by its tokenized ID.
//
// It extracts the ID from the URL, decodes the request body, and updates the record.
//
// Parameters:
// - w: The HTTP response writer.
// - r: The HTTP request containing the updated JSON payload.
// - model: A pointer to the struct representing the database entity.
//
// Returns:
// - HTTP 400 if the request body is invalid.
// - HTTP 500 if the update fails.
// - JSON object of the updated record if successful.
func (c *Controller) Update(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// Extract the tokenized ID from the URL
	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	// Decode the incoming request body
	if err := json.NewDecoder(r.Body).Decode(model); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})

		return
	}

	if err := c.BC.UpdateRecords(model, tokenizedID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})

		return
	}

	_ = json.NewEncoder(w).Encode(model)
}

// Delete removes a record identified by its tokenized ID.
//
// Parameters:
// - w: The HTTP response writer.
// - r: The HTTP request containing the tokenized ID as a URL parameter.
// - model: A pointer to the struct representing the database entity.
//
// Returns:
// - HTTP 500 if deletion fails.
// - JSON confirmation message if successful.
func (c *Controller) Delete(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// Extract the tokenized ID from the URL
	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	if err := c.BC.DeleteRecords(model, tokenizedID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})

		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Deleted successfully"})
}
