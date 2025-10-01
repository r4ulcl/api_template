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
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/r4ulcl/api_template/api/middlewares"
	"github.com/r4ulcl/api_template/database"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/gorm"
)

// Controller provides methods for handling CRUD operations.
// It encapsulates a reference to the BaseController for database interactions.
type Controller struct {
	BC *database.BaseController
}

const ownerColumn = "created_by"

// ------------------------------------------------------------------
// Audit helpers
// ------------------------------------------------------------------

// getRequestID returns an existing X-Request-ID or generates one.
func getRequestID(r *http.Request) string {
	rid := r.Header.Get("X-Request-ID")
	if rid == "" {
		rid = uuid.NewString()
	}
	return rid
}

// getClientIP tries common headers and falls back to RemoteAddr.
// If you run behind a proxy, terminate or sanitize X-Forwarded-For at the edge.
func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		parts := strings.Split(ip, ",")
		return strings.TrimSpace(parts[0])
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

// getIDString returns the value of a struct field named ID as a string.
// Works with string or numeric IDs. Returns empty string if not found.
func getIDString(model interface{}) string {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName("ID")
	if !f.IsValid() {
		return ""
	}
	switch f.Kind() {
	case reflect.String:
		return f.String()
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return fmt.Sprintf("%d", f.Uint())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return fmt.Sprintf("%d", f.Int())
	default:
		return ""
	}
}

// Always import "encoding/json" and "log"

type auditChange struct {
	Before interface{} `json:"before,omitempty"`
	After  interface{} `json:"after,omitempty"`
}

func logAudit(c *Controller, r *http.Request, actorID string, action string, resource string, resourceID string, status int, changes *auditChange) {
	changesJSON := "null" // valid JSON literal

	if changes != nil {
		if b, err := json.Marshal(changes); err == nil && len(b) > 0 {
			changesJSON = string(b)
		} else {
			// keep "null" if marshal fails or is empty
			changesJSON = "null"
		}
	}

	entry := models.AuditLog{
		ActorID:    actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Path:       r.URL.Path,
		Method:     r.Method,
		Status:     status,
		IP:         getClientIP(r),
		UserAgent:  r.UserAgent(),
		RequestID:  getRequestID(r),
		Changes:    changesJSON,
		CreatedAt:  time.Now(),
		LastUpdate: time.Now(),
		CreatedBy:  actorID,
		EditedBy:   actorID,
	}

	if err := c.BC.DB.Create(&entry).Error; err != nil {
		// Do not fail the request if audit logging fails
		log.Printf("audit log failed: %v", err)
	}
}

// ------------------------------------------------------------------
// Helpers for ownership and audit fields
// ------------------------------------------------------------------

func ownOnlyAndUserID(r *http.Request) (bool, string) {
	ownOnly := middlewares.IsOwnOnly(r.Context())
	uidVal := r.Context().Value(middlewares.ContextUserID)
	userID, _ := uidVal.(string)
	return ownOnly, userID
}

func readBodyBytes(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("request body is empty")
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func enforceCreatedBy(model interface{}, userID string) {
	v := reflect.ValueOf(model)
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	if f := v.FieldByName("CreatedBy"); f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(userID)
	}
}

func setAuditOnCreateWithOwner(model interface{}, userID string) {
	setAuditOnCreate(model, userID)
	enforceCreatedBy(model, userID)
}

func newModelInstance(model interface{}) (interface{}, bool) {
	modelType := reflect.TypeOf(model)
	if modelType == nil || modelType.Kind() != reflect.Ptr {
		return nil, false
	}
	return reflect.New(modelType.Elem()).Interface(), true
}

func setAuditForSliceElement(elem reflect.Value, userID string) {
	if !elem.IsValid() {
		return
	}
	// Unwrap interface values so Addr/Elem checks work consistently
	if elem.Kind() == reflect.Interface && !elem.IsNil() {
		elem = elem.Elem()
	}
	if elem.Kind() == reflect.Ptr {
		if !elem.IsNil() {
			setAuditOnCreateWithOwner(elem.Interface(), userID)
		}
		return
	}
	if elem.CanAddr() {
		setAuditOnCreateWithOwner(elem.Addr().Interface(), userID)
	}
}

// zeroAuditFields sets CreatedAt, LastUpdate, CreatedBy to their zero values
// and leaves EditedBy untouched so callers can set it when needed.
func zeroAuditFields(model interface{}) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	setIf := func(name string, zero interface{}) {
		f := v.FieldByName(name)
		if f.IsValid() && f.CanSet() {
			switch f.Kind() {
			case reflect.String:
				f.SetString("")
			case reflect.Struct:
				// usually time.Time
				if f.Type() == reflect.TypeOf(time.Time{}) {
					f.Set(reflect.ValueOf(time.Time{}))
				}
			}
		}
	}
	setIf("CreatedAt", time.Time{})
	setIf("LastUpdate", time.Time{})
	setIf("CreatedBy", "")
	// do not change EditedBy here
}

// setAuditOnCreate sets audit fields for create operations.
// CreatedAt and LastUpdate are left zero so GORM auto timestamps take effect.
func setAuditOnCreate(model interface{}, userID string) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	zeroAuditFields(model)
	if f := v.FieldByName("EditedBy"); f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(userID)
	}
	if f := v.FieldByName("CreatedBy"); f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(userID)
	}
}

// setAuditOnUpdate sets EditedBy and prevents audit fields from being overridden.
func setAuditOnUpdate(model interface{}, userID string) {
	zeroAuditFields(model)
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	if f := v.FieldByName("EditedBy"); f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
		f.SetString(userID)
	}
}

// getCreatedBy tries to read CreatedBy from a loaded model value.
func getCreatedBy(model interface{}) (string, bool) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}
	f := v.FieldByName("CreatedBy")
	if f.IsValid() && f.Kind() == reflect.String {
		return f.String(), true
	}
	return "", false
}

// hasOwnership checks CreatedBy on a loaded instance.
func hasOwnership(model interface{}, userID string) bool {
	if createdBy, ok := getCreatedBy(model); ok && createdBy == userID {
		return true
	}
	return false
}

// ownsByID loads the record by ID and returns true only if CreatedBy matches userID.
func ownsByID(c *Controller, model interface{}, tokenizedID string, userID string) bool {
	temp, ok := newModelInstance(model)
	if !ok {
		return false
	}
	if err := c.BC.GetRecordsByID(temp, tokenizedID); err != nil {
		return false
	}
	return hasOwnership(temp, userID)
}

// ------------------------------------------------------------------
// Create (supports single object OR array of objects)
// ------------------------------------------------------------------

// Create persists one or more new records into the database.
// @Summary     Create one or more records
// @Description Accepts either a single JSON object or an array of JSON objects for the given resource.
// If `overwrite=true` and a duplicate-key conflict occurs, existing records are updated.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       resource   path      string  true   "Resource name (e.g., users, items)"
// @Param       overwrite  query     bool    false  "If true, for single object duplicates update instead of error"
// @Param       payload    body      object  true   "A single JSON object or an array of JSON objects matching model schema"
// @Success     201        {object}  object              "The created record, or list of created records"
// @Failure     400        {object}  models.ErrorResponse "Bad request (invalid JSON or missing fields)"
// @Failure     409        {object}  models.ErrorResponse "Conflict (duplicate key and overwrite=false) for single object"
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource} [post]
func (c *Controller) Create(w http.ResponseWriter, r *http.Request, model interface{}, overwrite bool) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	resource := vars["resource"]

	_, userID := ownOnlyAndUserID(r)

	payload, err := readBodyBytes(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Unable to read request body"})
		logAudit(c, r, userID, "create", resource, "", http.StatusBadRequest, nil)
		return
	}

	// Restore body for potential downstream readers and simplify detection logic
	r.Body = io.NopCloser(bytes.NewReader(payload))
	trimmed := bytes.TrimSpace(payload)

	if len(trimmed) > 0 && trimmed[0] == '[' {
		c.handleBulkCreate(w, r, model, userID, resource, payload)
		return
	}

	c.handleSingleCreate(w, r, model, userID, resource, payload, overwrite)
}

func (c *Controller) handleBulkCreate(w http.ResponseWriter, r *http.Request, model interface{}, userID, resource string, payload []byte) {
	modelVal := reflect.ValueOf(model)
	if modelVal.Kind() != reflect.Ptr {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Model must be a pointer"})
		logAudit(c, r, userID, "create", resource, "", http.StatusInternalServerError, nil)
		return
	}

	slicePtr := reflect.New(reflect.SliceOf(modelVal.Type().Elem()))
	if err := json.Unmarshal(payload, slicePtr.Interface()); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid JSON array: " + err.Error()})
		logAudit(c, r, userID, "create", resource, "", http.StatusBadRequest, nil)
		return
	}

	slice := slicePtr.Elem()
	for i := 0; i < slice.Len(); i++ {
		setAuditForSliceElement(slice.Index(i), userID)
	}

	if tx := c.BC.DB.Create(slicePtr.Interface()); tx.Error != nil {
		status := http.StatusInternalServerError
		if strings.Contains(tx.Error.Error(), "duplicate") {
			status = http.StatusConflict
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: tx.Error.Error()})
		logAudit(c, r, userID, "create", resource, "", status, nil)
		return
	}

	for i := 0; i < slice.Len(); i++ {
		elem := slice.Index(i).Interface()
		rid := getIDString(elem)
		logAudit(c, r, userID, "create", resource, rid, http.StatusCreated, &auditChange{After: elem})
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(slicePtr.Interface())
}

func (c *Controller) handleSingleCreate(w http.ResponseWriter, r *http.Request, model interface{}, userID, resource string, payload []byte, overwrite bool) {
	if err := json.Unmarshal(payload, model); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Invalid JSON object: " + err.Error()})
		logAudit(c, r, userID, "create", resource, "", http.StatusBadRequest, nil)
		return
	}

	setAuditOnCreateWithOwner(model, userID)

	if err := c.BC.CreateOrUpdateRecord(model, overwrite); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "duplicate") {
			status = http.StatusConflict
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "create", resource, "", status, nil)
		return
	}

	rid := getIDString(model)
	logAudit(c, r, userID, "create", resource, rid, http.StatusCreated, &auditChange{After: model})

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// GetAll (supports advanced filters and sort and pagination)
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

func parsePaginationParams(query url.Values) (int, int) {
	page := 1
	perPage := 1000

	if v := query.Get("page"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if v := query.Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			perPage = parsed
		}
	}

	return page, perPage
}

func applyQueryFilters(db *gorm.DB, query url.Values) *gorm.DB {
	for key, vals := range query {
		if len(vals) == 0 || !strings.HasPrefix(key, "filter[") {
			continue
		}

		inside := strings.TrimPrefix(key, "filter[")
		if !strings.HasSuffix(inside, "]") {
			continue
		}
		inside = inside[:len(inside)-1]
		parts := strings.SplitN(inside, "][", 2)
		if len(parts) != 2 {
			continue
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
		}
	}
	return db
}

func applySortParam(db *gorm.DB, sortParam string) *gorm.DB {
	sortParam = strings.TrimSpace(sortParam)
	if sortParam == "" {
		return db
	}

	fields := strings.Split(sortParam, ",")
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if strings.HasPrefix(field, "-") {
			db = db.Order(fmt.Sprintf("%s DESC", strings.TrimPrefix(field, "-")))
		} else {
			db = db.Order(fmt.Sprintf("%s ASC", field))
		}
	}

	return db
}

func buildPaginationLinks(basePath string, query url.Values, page, perPage, totalPages int) paginationLinks {
	qs := copyQueryExcluding(query, []string{"page", "page_size"})

	makeLink := func(p int) string {
		local := url.Values{}
		for key, values := range qs {
			for _, v := range values {
				local.Add(key, v)
			}
		}
		local.Set("page", strconv.Itoa(p))
		local.Set("page_size", strconv.Itoa(perPage))
		return basePath + "?" + local.Encode()
	}

	links := paginationLinks{
		Self:  makeLink(page),
		First: makeLink(1),
		Last:  makeLink(totalPages),
	}

	if page > 1 {
		links.Prev = makeLink(page - 1)
	}
	if page < totalPages {
		links.Next = makeLink(page + 1)
	}

	return links
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
func (c *Controller) GetAll(w http.ResponseWriter, r *http.Request, model interface{}, readLog bool) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	resource := vars["resource"]

	ownOnly, userID := ownOnlyAndUserID(r)

	queryVals := r.URL.Query()
	page, perPage := parsePaginationParams(queryVals)

	baseModel := c.BC.DB.Model(model)
	if ownOnly && userID != "" {
		baseModel = baseModel.Where(ownerColumn+" = ?", userID)
	}

	countDB := applyQueryFilters(baseModel.Session(&gorm.Session{}), queryVals)

	var totalItems int64
	if err := countDB.Count(&totalItems).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "read", resource, "", http.StatusInternalServerError, nil)
		return
	}

	offset := (page - 1) * perPage
	totalPages := 0
	if perPage > 0 {
		totalPages = int((totalItems + int64(perPage) - 1) / int64(perPage))
	}

	dataDB := baseModel.Session(&gorm.Session{})
	dataDB = applyQueryFilters(dataDB, queryVals)
	dataDB = applySortParam(dataDB, queryVals.Get("sort"))
	dataDB = dataDB.Limit(perPage).Offset(offset)

	if err := dataDB.Find(model).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "read", resource, "", http.StatusInternalServerError, nil)
		return
	}

	resp := paginatedResponse{
		Data: model,
		Meta: paginationMeta{
			CurrentPage: page,
			PerPage:     perPage,
			TotalItems:  totalItems,
			TotalPages:  totalPages,
		},
		Links: buildPaginationLinks(r.URL.Path, queryVals, page, perPage, totalPages),
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)

	if readLog {
		logAudit(c, r, userID, "read", resource, "", http.StatusOK, nil)
	}
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
// @Description Fetches a single resource by its ID. Supports composite keys via hyphen-separated format.
// @Tags        user,admin
// @Accept      json
// @Produce     json
// @Param       resource   path      string  true  "Resource name (e.g., users, items)"
// @Param       id         path      string  true  "Primary key (or hyphen-separated composite key)"
// @Success     200        {object}  object  "The requested record"
// @Failure     404        {object}  models.ErrorResponse "Record not found or access denied."
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource}/{id} [get]
func (c *Controller) GetByID(w http.ResponseWriter, r *http.Request, model interface{}, readLog bool) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	resource := vars["resource"]
	tokenizedID := vars["id"]

	ownOnly, userID := ownOnlyAndUserID(r)

	// Load by ID using existing BaseController helper
	if err := c.BC.GetRecordsByID(model, tokenizedID); err != nil {
		if strings.Contains(err.Error(), "Record not found or access denied.") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			logAudit(c, r, userID, "read", resource, tokenizedID, http.StatusNotFound, nil)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "read", resource, tokenizedID, http.StatusInternalServerError, nil)
		return
	}

	// If ownOnly, verify ownership using CreatedBy
	if ownOnly && !hasOwnership(model, userID) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Record not found or access denied."})
		logAudit(c, r, userID, "read", resource, tokenizedID, http.StatusNotFound, nil)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model)

	if readLog {
		logAudit(c, r, userID, "read", resource, tokenizedID, http.StatusOK, nil)
	}
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
// @Param       payload    body      object  true  "JSON object with fields to update. Non-zero fields will be updated"
// @Success     200        {object}  object  "The updated record"
// @Failure     400        {object}  models.ErrorResponse "Invalid input JSON"
// @Failure     404        {object}  models.ErrorResponse "Record not found or access denied."
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource}/{id} [put]
func (c *Controller) Update(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	resource := vars["resource"]
	tokenizedID := vars["id"]

	ownOnly, userID := ownOnlyAndUserID(r)

	// If ownOnly, verify ownership before applying update
	if ownOnly && !ownsByID(c, model, tokenizedID, userID) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Record not found or access denied."})
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusNotFound, nil)
		return
	}

	// Load "before" state
	before, ok := newModelInstance(model)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Model must be a pointer"})
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusInternalServerError, nil)
		return
	}
	if err := c.BC.GetRecordsByID(before, tokenizedID); err != nil {
		// Could not load before
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Record not found or access denied."})
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusNotFound, nil)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(model); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusBadRequest, nil)
		return
	}

	// Prevent audit field tampering and set editor
	setAuditOnUpdate(model, userID)

	if err := c.BC.UpdateRecords(model, tokenizedID); err != nil {
		if strings.Contains(err.Error(), "Record not found or access denied.") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusNotFound, &auditChange{Before: before})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusInternalServerError, &auditChange{Before: before})
		return
	}

	// After state
	after, ok := newModelInstance(model)
	if !ok {
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusOK, &auditChange{Before: before})
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(model)
		return
	}
	if err := c.BC.GetRecordsByID(after, tokenizedID); err != nil {
		// If we cannot fetch after, still return success but log with before only
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusOK, &auditChange{Before: before})
	} else {
		logAudit(c, r, userID, "update", resource, tokenizedID, http.StatusOK, &auditChange{Before: before, After: after})
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// Delete
// ------------------------------------------------------------------

// Delete removes a record by its primary key.
// @Summary     Delete a record
// @Description Deletes a resource identified by its primary key. Supports composite keys via hyphen-separated composite format.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       resource   path      string  true  "Resource name (e.g., users, items)"
// @Param       id         path      string  true  "Primary key (or hyphen-separated composite key)"
// @Success     200        {object}  map[string]string  "Success message"
// @Failure     404        {object}  models.ErrorResponse "Record not found or access denied."
// @Failure     500        {object}  models.ErrorResponse "Internal server error"
// @Router      /{resource}/{id} [delete]
func (c *Controller) Delete(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	resource := vars["resource"]
	tokenizedID := vars["id"]

	ownOnly, userID := ownOnlyAndUserID(r)

	// If ownOnly, verify ownership before deletion
	if ownOnly && !ownsByID(c, model, tokenizedID, userID) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Record not found or access denied."})
		logAudit(c, r, userID, "delete", resource, tokenizedID, http.StatusNotFound, nil)
		return
	}

	// Load "before" snapshot
	before, ok := newModelInstance(model)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "Model must be a pointer"})
		logAudit(c, r, userID, "delete", resource, tokenizedID, http.StatusInternalServerError, nil)
		return
	}
	if err := c.BC.GetRecordsByID(before, tokenizedID); err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "delete", resource, tokenizedID, http.StatusNotFound, nil)
		return
	}

	if err := c.BC.DeleteRecords(model, tokenizedID); err != nil {
		if strings.Contains(err.Error(), "no records deleted") || strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
			logAudit(c, r, userID, "delete", resource, tokenizedID, http.StatusNotFound, &auditChange{Before: before})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		logAudit(c, r, userID, "delete", resource, tokenizedID, http.StatusInternalServerError, &auditChange{Before: before})
		return
	}

	// Success
	logAudit(c, r, userID, "delete", resource, tokenizedID, http.StatusOK, &auditChange{Before: before})

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Deleted successfully"})
}
