package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/r4ulcl/api_template/utils/models"
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

	// 1. Determine the current database/schema
	dbName := c.BC.DB.Migrator().CurrentDatabase()

	// 2. Query information_schema.tables for metrics (excluding TABLE_ROWS itself)
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
		`, dbName).
		Scan(&rawStats).
		Error

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// 3. For each rawStat, run a SELECT COUNT(*) to get an exact row count.
	stats := make([]TableStats, 0, len(rawStats))
	for _, rs := range rawStats {
		var exactCount int64
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", rs.TableName)
		if err := c.BC.DB.Raw(countQuery).Scan(&exactCount).Error; err != nil {
			exactCount = -1
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

	// 4. Build pagination metadata and links (single page only)
	totalItems := len(stats)
	currentPage := 1
	perPage := totalItems
	totalPages := 1

	// Reconstruct the request’s base path + query (to fill “self”)
	basePath := r.URL.Path
	q := r.URL.Query()
	q.Set("page", fmt.Sprintf("%d", currentPage))
	q.Set("page_size", fmt.Sprintf("%d", perPage))
	selfURL := basePath + "?" + q.Encode()

	// First and Last are the same since only one page exists
	firstURL := selfURL
	lastURL := selfURL

	// No Prev/Next if only one page
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
			First: firstURL,
			Last:  lastURL,
		},
	}

	// 5. Return JSON
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
