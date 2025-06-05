// file: controllers/base_controller.go

package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/database"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/gorm"
)

// Controller provides methods for handling CRUD operations.
//
// It encapsulates a reference to the BaseController for database interactions.
type Controller struct {
	BC *database.BaseController
}

// ------------------------------------------------------------------
// Create (supports single object OR array of objects)
// ------------------------------------------------------------------

// Create persists one or more new records into the database.
// @Summary     Create one or more records
// @Description Accepts either a single JSON object or an array of JSON objects for the given resource.
//
//	If `overwrite=true` and a duplicate-key conflict occurs, existing records are updated.
//
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       resource   path      string  true   "Resource name (e.g., users, items)"
// @Param       overwrite  query     bool    false  "If true, for single object duplicates → update instead of error"
// @Param       payload    body      object  true   "A single JSON object or an array of JSON objects matching model schema"
// @Success     201        {object}  object            "The created record, or list of created records"
// @Failure     400        {object}  models.ErrorResponse "Bad request (invalid JSON or missing fields)"
// @Failure     409        {object}  models.ErrorResponse "Conflict (duplicate key and overwrite=false) for single object"
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource} [post]
func (c *Controller) Create(w http.ResponseWriter, r *http.Request, model interface{}, overwrite bool) {
	w.Header().Set("Condent-Type", "application/json")

	// 1) Read raw body to detect if it's an array or single object
	var buf bytes.Buffer
	tee := io.TeeReader(r.Body, &buf) // tee lets us inspect then reuse
	firstBytes := make([]byte, 1)

	// Read first non-whitespace byte
	for {
		n, err := tee.Read(firstBytes)
		if err != nil && err != io.EOF {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unable to read request body"})
			return
		}
		if n == 0 {
			break
		}
		if bytes.TrimSpace(firstBytes)[0] == '[' || bytes.TrimSpace(firstBytes)[0] == '{' {
			break
		}
	}
	// Reconstruct r.Body so that full JSON is available below
	r.Body = io.NopCloser(io.MultiReader(&buf, r.Body))

	trimmedFirst := bytes.TrimSpace(firstBytes)
	if len(trimmedFirst) > 0 && trimmedFirst[0] == '[' {
		// ---- BULK INSERT PATH ----

		// 2) Build a slice type whose element is the underlying model type
		//    E.g. if model is *User, elemType = reflect.TypeOf(User{}), then sliceType = []User
		modelVal := reflect.ValueOf(model)
		if modelVal.Kind() != reflect.Ptr {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Model must be a pointer"})
			return
		}
		elemType := modelVal.Type().Elem()     // e.g. User
		sliceType := reflect.SliceOf(elemType) // []User
		slicePtr := reflect.New(sliceType)     // *[]User

		// 3) Unmarshal entire body into *[]T
		if err := json.NewDecoder(r.Body).Decode(slicePtr.Interface()); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid JSON array: " + err.Error()})
			return
		}

		// 4) Bulk-create using GORM (no Overwrite support here—pure INSERT)
		tx := c.BC.DB.Create(slicePtr.Interface())
		if tx.Error != nil {
			// If this is a duplicate-key error, return 409
			if strings.Contains(tx.Error.Error(), "duplicate") {
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: tx.Error.Error()})
				return
			}
			// Other DB errors → 500
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: tx.Error.Error()})
			return
		}

		// 5) Return the slice of created records (GORM does fill any default columns)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(slicePtr.Interface())
		return
	}

	// ---- SINGLE OBJECT PATH ----

	// 6) Decode into the single model instance
	if err := json.NewDecoder(r.Body).Decode(model); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("r.Body", r.Body)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid JSON object: " + err.Error()})
		return
	}

	// 7) Attempt to CreateOrUpdateRecord (honoring 'overwrite' flag)
	if err := c.BC.CreateOrUpdateRecord(model, overwrite); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			// Duplicate key + overwrite==false → conflict
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			return
		}
		// Other errors → 500
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// 8) Return the newly created/updated single object
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// GetAll (supports advanced filters + sort + pagination)
// ------------------------------------------------------------------

// paginatedResponse is the shape of our JSON response when returning a paginated list.
type paginatedResponse struct {
	Data  interface{}     `json:"data"`
	Meta  paginationMeta  `json:"meta"`
	Links paginationLinks `json:"links"`
}

type paginationMeta struct {
	CurrentPage int   `json:"current_page"`
	PerPage     int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
}

type paginationLinks struct {
	Self  string `json:"self"`
	First string `json:"first"`
	Prev  string `json:"prev,omitempty"`
	Next  string `json:"next,omitempty"`
	Last  string `json:"last"`
}

// GetAll retrieves all records with optional filtering, sorting, and pagination.
// @Summary     Get a paginated list of records
// @Description Retrieves records of a given resource, supporting complex filters, sorting, and pagination.
//   - Filters use `filter[field][operator]=value` (e.g. `filter[name][contains]=john`).
//   - Sorting uses `sort=field1,-field2` (prefix `-` for descending).
//   - Pagination uses `page` and `page_size`.
//
// @Tags        user,admin
// @Accept      json
// @Produce     json
// @Param       resource    path      string  true   "Resource name (e.g., users, items)"
// @Param       page        query     int     false  "Page number (default is 1)"
// @Param       page_size   query     int     false  "Items per page (default is 1000)"
// @Param       sort        query     string  false  "Comma-separated sort fields, prefix with '-' for DESC"
// @Param       filter      query     string  false  "Filter parameters of the form filter[field][op]=value (repeatable)"
// @Success     200         {object}  paginatedResponse    "Paginated list of records"
// @Failure     400         {object}  models.ErrorResponse "Invalid query parameters"
// @Failure     500         {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource} [get]
func (c *Controller) GetAll(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// 1) Parse "page" and "page_size" parameters (with defaults)
	queryVals := r.URL.Query()
	pageParam := queryVals.Get("page")
	perPageParam := queryVals.Get("page_size")

	page := 1
	perPage := 1000

	if p, err := strconv.Atoi(pageParam); err == nil && p > 0 {
		page = p
	}
	if pp, err := strconv.Atoi(perPageParam); err == nil && pp > 0 {
		perPage = pp
	}

	// 2) Prepare base GORM instance and apply filters/sort
	baseModel := c.BC.DB.Model(model)

	// 2a) Apply advanced filters
	applyFilters := func(db *gorm.DB) *gorm.DB {
		for rawKey, vals := range queryVals {
			// Skip pagination & sort keys
			if rawKey == "page" || rawKey == "page_size" || rawKey == "sort" {
				continue
			}

			// Expect keys of the form: filter[field][operator]
			if !strings.HasPrefix(rawKey, "filter[") {
				continue
			}

			inside := strings.TrimPrefix(rawKey, "filter[")
			if !strings.HasSuffix(inside, "]") {
				continue // malformed
			}
			inside = inside[:len(inside)-1] // e.g. "field][contains"

			parts := strings.SplitN(inside, "][", 2)
			if len(parts) != 2 {
				continue // malformed
			}
			field := parts[0]
			operator := parts[1]
			value := vals[0]

			switch operator {
			case "eq":
				db = db.Where(fmt.Sprintf("%s = ?", field), value)
			case "ne", "neq":
				db = db.Where(fmt.Sprintf("%s <> ?", field), value)

			case "contains":
				db = db.Where(fmt.Sprintf("%s LIKE ?", field), "%"+value+"%")
			case "ncontains":
				db = db.Where(fmt.Sprintf("%s NOT LIKE ?", field), "%"+value+"%")

			case "gt":
				db = db.Where(fmt.Sprintf("%s > ?", field), value)
			case "gte":
				db = db.Where(fmt.Sprintf("%s >= ?", field), value)
			case "lt":
				db = db.Where(fmt.Sprintf("%s < ?", field), value)
			case "lte":
				db = db.Where(fmt.Sprintf("%s <= ?", field), value)

			case "in":
				list := strings.Split(value, ",")
				db = db.Where(fmt.Sprintf("%s IN ?", field), list)
			case "nin":
				list := strings.Split(value, ",")
				db = db.Where(fmt.Sprintf("%s NOT IN ?", field), list)

			case "isnull":
				vLower := strings.ToLower(value)
				if vLower == "true" || vLower == "1" {
					db = db.Where(fmt.Sprintf("%s IS NULL", field))
				} else {
					db = db.Where(fmt.Sprintf("%s IS NOT NULL", field))
				}

			default:
				// Unknown operator → skip
				continue
			}
		}
		return db
	}

	// 2b) Apply sorting (if provided)
	applySort := func(db *gorm.DB) *gorm.DB {
		sortParam := queryVals.Get("sort")
		if strings.TrimSpace(sortParam) == "" {
			return db
		}

		fields := strings.Split(sortParam, ",")
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if strings.HasPrefix(f, "-") {
				fieldName := strings.TrimPrefix(f, "-")
				db = db.Order(fmt.Sprintf("%s DESC", fieldName))
			} else {
				db = db.Order(fmt.Sprintf("%s ASC", f))
			}
		}
		return db
	}

	// 3) Count total items (apply filters only)
	countDB := baseModel.Session(&gorm.Session{})
	countDB = applyFilters(countDB)

	var totalItems int64
	if err := countDB.Count(&totalItems).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// 4) Calculate pagination offsets
	offset := (page - 1) * perPage
	totalPages := int((totalItems + int64(perPage) - 1) / int64(perPage)) // ceil

	// 5) Fetch the actual page of data (apply filters, sort, limit, offset)
	dataDB := baseModel.Session(&gorm.Session{})
	dataDB = applyFilters(dataDB)
	dataDB = applySort(dataDB)
	dataDB = dataDB.Limit(perPage).Offset(offset)

	if err := dataDB.Find(model).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// 6) Build pagination links
	basePath := r.URL.Path
	qs := copyQueryExcluding(queryVals, []string{"page", "page_size"})

	makeLink := func(p int) string {
		local := url.Values{}
		for key, vals := range qs {
			for _, v := range vals {
				local.Add(key, v)
			}
		}
		local.Set("page", strconv.Itoa(p))
		local.Set("page_size", strconv.Itoa(perPage))
		return basePath + "?" + local.Encode()
	}

	selfLink := makeLink(page)
	firstLink := makeLink(1)
	lastLink := makeLink(totalPages)

	prevLink := ""
	if page > 1 {
		prevLink = makeLink(page - 1)
	}

	nextLink := ""
	if page < totalPages {
		nextLink = makeLink(page + 1)
	}

	// 7) Return paginated response
	resp := paginatedResponse{
		Data: model,
		Meta: paginationMeta{
			CurrentPage: page,
			PerPage:     perPage,
			TotalItems:  totalItems,
			TotalPages:  totalPages,
		},
		Links: paginationLinks{
			Self:  selfLink,
			First: firstLink,
			Prev:  prevLink,
			Next:  nextLink,
			Last:  lastLink,
		},
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// copyQueryExcluding returns a copy of url.Values without the specified keys.
func copyQueryExcluding(src url.Values, keysToSkip []string) url.Values {
	out := url.Values{}
	skip := make(map[string]bool)
	for _, k := range keysToSkip {
		skip[k] = true
	}
	for key, vals := range src {
		if skip[key] {
			continue
		}
		for _, v := range vals {
			out.Add(key, v)
		}
	}
	return out
}

// ------------------------------------------------------------------
// GetByID
// ------------------------------------------------------------------

// GetByID retrieves a single record by its primary key.
// @Summary     Get a record by ID
// @Description Fetches a single resource by its ID (supports composite keys via hyphen-separated format).
// @Tags        user,admin
// @Accept      json
// @Produce     json
// @Param       resource   path      string  true  "Resource name (e.g., users, items)"
// @Param       id         path      string  true  "Primary key (or hyphen-separated composite key)"
// @Success     200        {object}  object  "The requested record"
// @Failure     404        {object}  models.ErrorResponse "Record not found"
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource}/{id} [get]
func (c *Controller) GetByID(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	if err := c.BC.GetRecordsByID(model, tokenizedID); err != nil {
		// If it's a “not found” error, return 404
		if strings.Contains(err.Error(), "record not found") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			return
		}
		// Otherwise, 500
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// Update
// ------------------------------------------------------------------

// Update modifies an existing record identified by its primary key.
// @Summary     Update a record
// @Description Accepts a JSON payload to update an existing resource. The `id` in the path is used to locate the record.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       resource   path      string  true  "Resource name (e.g., users, items)"
// @Param       id         path      string  true  "Primary key (or hyphen-separated composite key)"
// @Param       payload    body      object  true  "JSON object with fields to update (non-zero fields will be updated)"
// @Success     200        {object}  object  "The updated record"
// @Failure     400        {object}  models.ErrorResponse "Invalid input JSON"
// @Failure     404        {object}  models.ErrorResponse "Record not found"
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource}/{id} [put]
func (c *Controller) Update(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	if err := json.NewDecoder(r.Body).Decode(model); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	if err := c.BC.UpdateRecords(model, tokenizedID); err != nil {
		if strings.Contains(err.Error(), "record not found") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// Delete
// ------------------------------------------------------------------

// Delete removes a record by its primary key.
// @Summary     Delete a record
// @Description Deletes a resource identified by its primary key (or hyphen-separated composite key).
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       resource   path      string  true  "Resource name (e.g., users, items)"
// @Param       id         path      string  true  "Primary key (or hyphen-separated composite key)"
// @Success     200        {object}  map[string]string  "Success message"
// @Failure     404        {object}  models.ErrorResponse "Record not found"
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource}/{id} [delete]
func (c *Controller) Delete(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	if err := c.BC.DeleteRecords(model, tokenizedID); err != nil {
		if strings.Contains(err.Error(), "no records deleted") || strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Deleted successfully"})
}
