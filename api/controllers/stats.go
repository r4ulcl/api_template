package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/r4ulcl/api_template/api/middlewares"
	"github.com/r4ulcl/api_template/utils/models"
	"gorm.io/gorm"
)

// TableStats holds detailed statistics for a single table, including its PK columns.
type TableStats struct {
	TableName      string     `json:"table_name"`
	ExactRowCount  int64      `json:"exact_row_count"`       // exact COUNT(*) at the time of the request
	DataSize       uint64     `json:"data_size_bytes"`       // DATA_LENGTH
	IndexSize      uint64     `json:"index_size_bytes"`      // INDEX_LENGTH
	DataFree       uint64     `json:"data_free_bytes"`       // DATA_FREE
	MaxDataLength  uint64     `json:"max_data_length_bytes"` // MAX_DATA_LENGTH
	AutoIncrement  uint64     `json:"auto_increment"`        // AUTO_INCREMENT
	Engine         string     `json:"engine"`                // storage engine (InnoDB, MyISAM, etc.)
	TableCollation string     `json:"table_collation"`       // TABLE_COLLATION
	RowFormat      string     `json:"row_format"`            // ROW_FORMAT
	TableType      string     `json:"table_type"`            // BASE TABLE, VIEW, etc.
	TableComment   string     `json:"table_comment"`         // any comment on the table
	CreateTime     *time.Time `json:"create_time,omitempty"` // CREATE_TIME (can be null)
	UpdateTime     *time.Time `json:"update_time,omitempty"` // UPDATE_TIME (can be null)
	CheckTime      *time.Time `json:"check_time,omitempty"`  // CHECK_TIME (can be null)
	ColumnCount    uint64     `json:"column_count"`          // number of columns in the table
	IndexCount     uint64     `json:"index_count"`           // number of distinct indexes on that table
	TotalSize      uint64     `json:"total_size_bytes"`      // DataSize + IndexSize
	PrimaryKey     string     `json:"primary_key"`           // comma-separated list of PK column(s)
}

// paginatedStatsResponse wraps the stats slice in the new JSON format.
type paginatedStatsResponse struct {
	Data  []TableStats    `json:"data"`
	Meta  statsPagination `json:"meta"`
	Links statsLinks      `json:"links"`
}

type statsPagination struct {
	CurrentPage int `json:"current_page"`
	PerPage     int `json:"page_size"`
	TotalItems  int `json:"total_items"`
	TotalPages  int `json:"total_pages"`
}

type statsLinks struct {
	Self  string `json:"self"`
	First string `json:"first"`
	Prev  string `json:"prev,omitempty"`
	Next  string `json:"next,omitempty"`
	Last  string `json:"last"`
}

// GetDBStats retrieves, for each table in the current schema:
//   - exact row count (via SELECT COUNT(*))
//   - DATA_LENGTH, INDEX_LENGTH, DATA_FREE, MAX_DATA_LENGTH, AUTO_INCREMENT
//   - ENGINE, TABLE_COLLATION, ROW_FORMAT, TABLE_TYPE, TABLE_COMMENT
//   - CREATE_TIME, UPDATE_TIME, CHECK_TIME
//   - column_count (number of columns in that table)
//   - index_count (number of distinct indexes on that table)
//   - primary_key (all PK columns comma‐separated)
//   - total_size_bytes (data + index size)
//
// Returns a paginated JSON response with “data”, “meta”, and “links”. On any error,
// it responds with HTTP 500 + ErrorResponse.
func (c *Controller) GetDBStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Role and user ID from context
	roleVal := r.Context().Value(middlewares.ContextRole)
	role, _ := roleVal.(string)
	uidVal := r.Context().Value(middlewares.ContextUserID)
	userID, _ := uidVal.(string)

	// Resolve which resources this role can access, and how
	perms, ok := models.RolePermissions[role]
	if !ok {
		// Unknown role → no access to any table
		writeStats(w, r, nil)
		return
	}

	// Build a map of tableName → accessMode ("full" or "own")
	accessByTable := map[string]string{}

	// Helper to get GORM table name for a model pointer
	getTableName := func(db *gorm.DB, model interface{}) (string, error) {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			return "", err
		}
		return stmt.Schema.Table, nil
	}

	// First, mark own access
	for _, res := range perms.GetOwn {
		modelPtr, exists := models.ModelMap[res]
		if !exists {
			continue
		}
		if tbl, err := getTableName(c.BC.DB, modelPtr); err == nil {
			// Only set to own if not already full
			if accessByTable[tbl] == "" {
				accessByTable[tbl] = "own"
			}
		}
	}

	// Then, upgrade to full where applicable
	for _, res := range perms.Get {
		modelPtr, exists := models.ModelMap[res]
		if !exists {
			continue
		}
		if tbl, err := getTableName(c.BC.DB, modelPtr); err == nil {
			accessByTable[tbl] = "full"
		}
	}

	// If no allowed tables, respond with empty data
	if len(accessByTable) == 0 {
		writeStats(w, r, nil)
		return
	}

	// Current schema
	dbName := c.BC.DB.Migrator().CurrentDatabase()

	// Raw row for information_schema details
	type rawStat struct {
		TableName      string     `gorm:"column:TABLE_NAME"`
		DataLength     uint64     `gorm:"column:DATA_LENGTH"`
		IndexLength    uint64     `gorm:"column:INDEX_LENGTH"`
		DataFree       uint64     `gorm:"column:DATA_FREE"`
		MaxDataLength  uint64     `gorm:"column:MAX_DATA_LENGTH"`
		AutoIncrement  uint64     `gorm:"column:AUTO_INCREMENT"`
		Engine         string     `gorm:"column:ENGINE"`
		TableCollation string     `gorm:"column:TABLE_COLLATION"`
		RowFormat      string     `gorm:"column:ROW_FORMAT"`
		TableType      string     `gorm:"column:TABLE_TYPE"`
		TableComment   string     `gorm:"column:TABLE_COMMENT"`
		CreateTime     *time.Time `gorm:"column:CREATE_TIME"`
		UpdateTime     *time.Time `gorm:"column:UPDATE_TIME"`
		CheckTime      *time.Time `gorm:"column:CHECK_TIME"`
		ColumnCount    uint64     `gorm:"column:COLUMN_COUNT"`
		IndexCount     uint64     `gorm:"column:INDEX_COUNT"`
		PKColumns      string     `gorm:"column:PRIMARY_KEY"` // comma‐separated PK names
	}

	// Collect the allowed table names to pass into the IN clause
	allowedTables := make([]string, 0, len(accessByTable))
	for tbl := range accessByTable {
		allowedTables = append(allowedTables, tbl)
	}

	var rawStats []rawStat
	err := c.BC.DB.
		Raw(`
			SELECT
				t.TABLE_NAME,
				IFNULL(t.DATA_LENGTH, 0)          AS DATA_LENGTH,
				IFNULL(t.INDEX_LENGTH, 0)         AS INDEX_LENGTH,
				IFNULL(t.DATA_FREE, 0)            AS DATA_FREE,
				IFNULL(t.MAX_DATA_LENGTH, 0)      AS MAX_DATA_LENGTH,
				IFNULL(t.AUTO_INCREMENT, 0)       AS AUTO_INCREMENT,
				IFNULL(t.ENGINE, '')              AS ENGINE,
				IFNULL(t.TABLE_COLLATION, '')     AS TABLE_COLLATION,
				IFNULL(t.ROW_FORMAT, '')          AS ROW_FORMAT,
				IFNULL(t.TABLE_TYPE, '')          AS TABLE_TYPE,
				IFNULL(t.TABLE_COMMENT, '')       AS TABLE_COMMENT,
				t.CREATE_TIME,
				t.UPDATE_TIME,
				t.CHECK_TIME,
				(
					SELECT COUNT(*)
					FROM information_schema.columns c
					WHERE c.table_schema = t.table_schema
					  AND c.table_name   = t.table_name
				) AS COLUMN_COUNT,
				(
					SELECT COUNT(DISTINCT s.INDEX_NAME)
					FROM information_schema.statistics s
					WHERE s.table_schema = t.table_schema
					  AND s.table_name   = t.table_name
				) AS INDEX_COUNT,
				(
					SELECT IFNULL(
						GROUP_CONCAT(k.COLUMN_NAME ORDER BY k.ORDINAL_POSITION SEPARATOR ','),
						''
					)
					FROM information_schema.key_column_usage k
					WHERE k.table_schema    = t.table_schema
					  AND k.table_name      = t.table_name
					  AND k.constraint_name = 'PRIMARY'
				) AS PRIMARY_KEY
			FROM information_schema.tables t
			WHERE t.table_schema = ?
			  AND t.table_name IN (?)
		`, dbName, allowedTables).
		Scan(&rawStats).
		Error

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// For each row, compute exact count with access mode
	stats := make([]TableStats, 0, len(rawStats))
	for _, rs := range rawStats {
		mode := accessByTable[rs.TableName]
		var exactCount int64

		if mode == "own" {
			countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE `created_by` = ?", rs.TableName)
			if err := c.BC.DB.Raw(countQuery, userID).Scan(&exactCount).Error; err != nil {
				exactCount = -1
			}
		} else {
			countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", rs.TableName)
			if err := c.BC.DB.Raw(countQuery).Scan(&exactCount).Error; err != nil {
				exactCount = -1
			}
		}

		stats = append(stats, TableStats{
			TableName:      rs.TableName,
			ExactRowCount:  exactCount,
			DataSize:       rs.DataLength,
			IndexSize:      rs.IndexLength,
			DataFree:       rs.DataFree,
			MaxDataLength:  rs.MaxDataLength,
			AutoIncrement:  rs.AutoIncrement,
			Engine:         rs.Engine,
			TableCollation: rs.TableCollation,
			RowFormat:      rs.RowFormat,
			TableType:      rs.TableType,
			TableComment:   rs.TableComment,
			CreateTime:     rs.CreateTime,
			UpdateTime:     rs.UpdateTime,
			CheckTime:      rs.CheckTime,
			ColumnCount:    rs.ColumnCount,
			IndexCount:     rs.IndexCount,
			TotalSize:      rs.DataLength + rs.IndexLength,
			PrimaryKey:     rs.PKColumns,
		})
	}

	// Respond with one page containing only the permitted tables
	writeStats(w, r, stats)
}

// writeStats builds the pagination envelope and writes the response.
func writeStats(w http.ResponseWriter, r *http.Request, stats []TableStats) {
	totalItems := 0
	if stats != nil {
		totalItems = len(stats)
	}
	currentPage := 1
	perPage := totalItems
	if perPage == 0 {
		perPage = 0
	}
	totalPages := 1

	basePath := r.URL.Path
	q := r.URL.Query()
	q.Set("page", fmt.Sprintf("%d", currentPage))
	q.Set("page_size", fmt.Sprintf("%d", perPage))
	selfURL := basePath + "?" + q.Encode()

	resp := paginatedStatsResponse{
		Data: stats,
		Meta: statsPagination{
			CurrentPage: currentPage,
			PerPage:     perPage,
			TotalItems:  totalItems,
			TotalPages:  totalPages,
		},
		Links: statsLinks{
			Self:  selfURL,
			First: selfURL,
			Last:  selfURL,
		},
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
