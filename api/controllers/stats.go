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

// TableStats holds a compact view for non admin users.
type TableStats struct {
	TableName     string `json:"table_name"`
	ExactRowCount int64  `json:"exact_row_count"`
	PrimaryKey    string `json:"primary_key"`
}

// TableStatsAdmin is the detailed view for admins.
type TableStatsAdmin struct {
	TableName      string     `json:"table_name"`
	ExactRowCount  int64      `json:"exact_row_count"`
	DataSize       uint64     `json:"data_size_bytes"`
	IndexSize      uint64     `json:"index_size_bytes"`
	DataFree       uint64     `json:"data_free_bytes"`
	MaxDataLength  uint64     `json:"max_data_length_bytes"`
	AutoIncrement  uint64     `json:"auto_increment"`
	Engine         string     `json:"engine"`
	TableCollation string     `json:"table_collation"`
	RowFormat      string     `json:"row_format"`
	TableType      string     `json:"table_type"`
	TableComment   string     `json:"table_comment"`
	CreateTime     *time.Time `json:"create_time,omitempty"`
	UpdateTime     *time.Time `json:"update_time,omitempty"`
	CheckTime      *time.Time `json:"check_time,omitempty"`
	ColumnCount    uint64     `json:"column_count"`
	IndexCount     uint64     `json:"index_count"`
	TotalSize      uint64     `json:"total_size_bytes"`
	PrimaryKey     string     `json:"primary_key"`
}

// paginatedStatsResponse wraps the stats slice.
type paginatedStatsResponse struct {
	Data  interface{}     `json:"data"`
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

func (c *Controller) GetDBStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	roleVal := r.Context().Value(middlewares.ContextRole)
	role, _ := roleVal.(string)
	uidVal := r.Context().Value(middlewares.ContextUserID)
	userID, _ := uidVal.(string)

	perms, ok := models.RolePermissions[role]
	if !ok {
		// Unknown role means no access
		writeStats(w, r, []TableStats{})
		return
	}

	// Map: table name -> access mode ("full" or "own")
	accessByTable := map[string]string{}

	getTableName := func(db *gorm.DB, model interface{}) (string, error) {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			return "", err
		}
		return stmt.Schema.Table, nil
	}

	for _, res := range perms.GetOwn {
		if modelPtr, exists := models.ModelMap[res]; exists {
			if tbl, err := getTableName(c.BC.DB, modelPtr); err == nil {
				if accessByTable[tbl] == "" {
					accessByTable[tbl] = "own"
				}
			}
		}
	}
	for _, res := range perms.Get {
		if modelPtr, exists := models.ModelMap[res]; exists {
			if tbl, err := getTableName(c.BC.DB, modelPtr); err == nil {
				accessByTable[tbl] = "full"
			}
		}
	}

	if len(accessByTable) == 0 {
		writeStats(w, r, []TableStats{})
		return
	}

	dbName := c.BC.DB.Migrator().CurrentDatabase()

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
		PKColumns      string     `gorm:"column:PRIMARY_KEY"`
	}

	allowed := make([]string, 0, len(accessByTable))
	for tbl := range accessByTable {
		allowed = append(allowed, tbl)
	}

	var rawStats []rawStat
	err := c.BC.DB.
		Raw(`
			SELECT
				t.TABLE_NAME,
				IFNULL(t.DATA_LENGTH, 0)      AS DATA_LENGTH,
				IFNULL(t.INDEX_LENGTH, 0)     AS INDEX_LENGTH,
				IFNULL(t.DATA_FREE, 0)        AS DATA_FREE,
				IFNULL(t.MAX_DATA_LENGTH, 0)  AS MAX_DATA_LENGTH,
				IFNULL(t.AUTO_INCREMENT, 0)   AS AUTO_INCREMENT,
				IFNULL(t.ENGINE, '')          AS ENGINE,
				IFNULL(t.TABLE_COLLATION, '') AS TABLE_COLLATION,
				IFNULL(t.ROW_FORMAT, '')      AS ROW_FORMAT,
				IFNULL(t.TABLE_TYPE, '')      AS TABLE_TYPE,
				IFNULL(t.TABLE_COMMENT, '')   AS TABLE_COMMENT,
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
		`, dbName, allowed).
		Scan(&rawStats).Error

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	// Decide the shape based on the user role
	isAdmin := role == "admin"

	if isAdmin {
		stats := make([]TableStatsAdmin, 0, len(rawStats))
		for _, rs := range rawStats {
			mode := accessByTable[rs.TableName]
			var exactCount int64

			if mode == "own" {
				q := fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE `created_by` = ?", rs.TableName)
				if err := c.BC.DB.Raw(q, userID).Scan(&exactCount).Error; err != nil {
					exactCount = -1
				}
			} else {
				q := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", rs.TableName)
				if err := c.BC.DB.Raw(q).Scan(&exactCount).Error; err != nil {
					exactCount = -1
				}
			}

			stats = append(stats, TableStatsAdmin{
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
		writeStats(w, r, stats)
		return
	}

	// Non admin payload
	stats := make([]TableStats, 0, len(rawStats))
	for _, rs := range rawStats {
		mode := accessByTable[rs.TableName]
		var exactCount int64

		if mode == "own" {
			q := fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE `created_by` = ?", rs.TableName)
			if err := c.BC.DB.Raw(q, userID).Scan(&exactCount).Error; err != nil {
				exactCount = -1
			}
		} else {
			q := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", rs.TableName)
			if err := c.BC.DB.Raw(q).Scan(&exactCount).Error; err != nil {
				exactCount = -1
			}
		}

		stats = append(stats, TableStats{
			TableName:     rs.TableName,
			ExactRowCount: exactCount,
			PrimaryKey:    rs.PKColumns,
		})
	}

	writeStats(w, r, stats)
}

// writeStats paginates and writes any slice as data.
func writeStats(w http.ResponseWriter, r *http.Request, data interface{}) {
	// Determine total items from the slice length
	totalItems := 0
	switch v := data.(type) {
	case []TableStats:
		totalItems = len(v)
	case []TableStatsAdmin:
		totalItems = len(v)
	case nil:
		totalItems = 0
	default:
		// Fallback when an unexpected type is passed
		totalItems = 0
	}

	currentPage := 1
	perPage := totalItems
	totalPages := 1

	basePath := r.URL.Path
	q := r.URL.Query()
	q.Set("page", fmt.Sprintf("%d", currentPage))
	q.Set("page_size", fmt.Sprintf("%d", perPage))
	selfURL := basePath + "?" + q.Encode()

	resp := paginatedStatsResponse{
		Data: data,
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
