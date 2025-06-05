// file: controllers/base_controller.go

package controllers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

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

// ------------------------------------------------------------------
// Create (unchanged)
// ------------------------------------------------------------------

func (c *Controller) Create(w http.ResponseWriter, r *http.Request, model interface{}, overwrite bool) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewDecoder(r.Body).Decode(model); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// Use the new CreateOrUpdateRecord function
	if err := c.BC.CreateOrUpdateRecord(model, overwrite); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// GetAll (UPDATED to support filters + pagination)
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

// GetAll retrieves all records with optional filtering and pagination.
//
// It parses query parameters to apply filters dynamically and returns
// a paginated JSON response containing the matching records.
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

	// 2) Build a copy of the query parameters for filtering (exclude page & page_size)
	filters := make(map[string]interface{})
	for key, vals := range queryVals {
		// Skip pagination keys
		if key == "page" || key == "page_size" {
			continue
		}
		if len(vals) > 0 {
			filters[key] = vals[0]
		}
	}

	// 3) Determine the underlying element type so we can run GORM
	//    Example: model is *[]Example1, so GORM knows to use Example1 as the table.
	//    GORM can infer from model, but we'll be explicit for Count().
	db := c.BC.DB.Model(model)

	// 4) Apply filters to the "count" query
	for key, val := range filters {
		// Use simple equality filter: WHERE key = val
		db = db.Where(key+" = ?", val)
	}

	// 5) Count total items (ignoring Limit/Offset)
	var totalItems int64
	if err := db.Count(&totalItems).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// 6) Calculate offset and total pages
	offset := (page - 1) * perPage
	totalPages := int((totalItems + int64(perPage) - 1) / int64(perPage)) // ceil(totalItems / perPage)

	// 7) Actually fetch this page's “slice” of data
	//    We need a fresh GORM chain because we mutated `db` for counting.
	dataDB := c.BC.DB.Model(model)
	for key, val := range filters {
		dataDB = dataDB.Where(key+" = ?", val)
	}

	dataDB = dataDB.Limit(perPage).Offset(offset)
	if err := dataDB.Find(model).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// 8) Build pagination links (self, first, prev, next, last)
	//    We take the original URL Path + rebuilt Query (adjusting page).
	basePath := r.URL.Path
	qs := copyQueryExcluding(queryVals, []string{"page", "page_size"})

	// a) helper to construct a URL string with updated page
	makeLink := func(p int) string {
		// Clone the existing filter‐only query params
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

	// 9) Serialize everything as JSON
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

// copyQueryExcluding returns a copy of the url.Values without any of the keysToSkip.
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
// GetByID (unchanged)
// ------------------------------------------------------------------

func (c *Controller) GetByID(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	if err := c.BC.GetRecordsByID(model, tokenizedID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// Update (unchanged)
// ------------------------------------------------------------------

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
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(model)
}

// ------------------------------------------------------------------
// Delete (unchanged)
// ------------------------------------------------------------------

func (c *Controller) Delete(w http.ResponseWriter, r *http.Request, model interface{}) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	tokenizedID := vars["id"]

	if err := c.BC.DeleteRecords(model, tokenizedID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Deleted successfully"})
}
