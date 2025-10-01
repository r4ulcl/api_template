package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	Show          bool   `json:"show"`
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
	Show           bool       `json:"show"`
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

type tableAccess struct {
	tableName string
	mode      string
}

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

type statsContext struct {
	controller *Controller
	role       string
	userID     string
	access     map[string]*tableAccess
}

func newStatsContext(ctrl *Controller, r *http.Request) *statsContext {
	roleVal := r.Context().Value(middlewares.ContextRole)
	role, _ := roleVal.(string)
	uidVal := r.Context().Value(middlewares.ContextUserID)
	userID, _ := uidVal.(string)
	return &statsContext{
		controller: ctrl,
		role:       strings.TrimSpace(role),
		userID:     strings.TrimSpace(userID),
		access:     make(map[string]*tableAccess),
	}
}

func (s *statsContext) permissions() (models.Permissions, bool) {
	perms, ok := models.RolePermissions[s.role]
	return perms, ok
}

func (s *statsContext) collectAccessibleTables(perms models.Permissions) {
	appendResources := func(resources []string, mode string) {
		for _, res := range resources {
			modelPtr, exists := models.ModelMap[res]
			if !exists {
				continue
			}
			table, err := s.tableName(modelPtr)
			if err != nil {
				continue
			}
			if s.shouldSkip(table) {
				continue
			}
			s.addTable(table, mode)
		}
	}

	appendResources(perms.GetOwn, "own")
	appendResources(perms.Get, "full")

	for _, hidden := range models.StatsHiddenOriginalTables {
		s.addTable(hidden, "full")
	}
}

func (s *statsContext) tableName(model interface{}) (string, error) {
	stmt := &gorm.Statement{DB: s.controller.BC.DB}
	if err := stmt.Parse(model); err != nil {
		return "", err
	}
	return stmt.Schema.Table, nil
}

func (s *statsContext) shouldSkip(table string) bool {
	return strings.EqualFold(table, "api_key") && !models.IsStatsHiddenTable(table)
}

func (s *statsContext) addTable(table, mode string) {
	canonical := strings.ToLower(strings.TrimSpace(table))
	if canonical == "" {
		return
	}
	if existing, ok := s.access[canonical]; ok {
		if existing.mode == "full" {
			return
		}
		if mode == "full" {
			existing.mode = "full"
		}
		return
	}
	s.access[canonical] = &tableAccess{tableName: table, mode: mode}
}

func (s *statsContext) isAdmin() bool {
	return models.Role(strings.TrimSpace(s.role)) == models.AdminRole
}

func (s *statsContext) fetchRawStats() ([]rawStat, error) {
	allowed := make([]string, 0, len(s.access))
	for _, entry := range s.access {
		allowed = append(allowed, entry.tableName)
	}

	dbName := s.controller.BC.DB.Migrator().CurrentDatabase()
	var stats []rawStat
	err := s.controller.BC.DB.
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
		Scan(&stats).Error
	return stats, err
}

func (s *statsContext) tableMode(table string) string {
	entry, ok := s.access[strings.ToLower(table)]
	if !ok || entry == nil {
		return "full"
	}
	return entry.mode
}

func (s *statsContext) rowCount(table, mode string) int64 {
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)
	args := []interface{}{}
	if mode == "own" {
		query += " WHERE `created_by` = ?"
		args = append(args, s.userID)
	}
	var count int64
	if err := s.controller.BC.DB.Raw(query, args...).Scan(&count).Error; err != nil {
		return -1
	}
	return count
}

func (s *statsContext) buildAdminStats(raw []rawStat) []TableStatsAdmin {
	stats := make([]TableStatsAdmin, 0, len(raw))
	for _, rs := range raw {
		mode := s.tableMode(rs.TableName)
		count := s.rowCount(rs.TableName, mode)
		hidden := models.IsStatsHiddenTable(rs.TableName)
		stats = append(stats, TableStatsAdmin{
			TableName:      rs.TableName,
			ExactRowCount:  count,
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
			Show:           !hidden,
		})
	}
	return stats
}

func (s *statsContext) buildUserStats(raw []rawStat) []TableStats {
	stats := make([]TableStats, 0, len(raw))
	for _, rs := range raw {
		mode := s.tableMode(rs.TableName)
		count := s.rowCount(rs.TableName, mode)
		hidden := models.IsStatsHiddenTable(rs.TableName)
		stats = append(stats, TableStats{
			TableName:     rs.TableName,
			ExactRowCount: count,
			PrimaryKey:    rs.PKColumns,
			Show:          !hidden,
		})
	}
	return stats
}

func (c *Controller) GetDBStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx := newStatsContext(c, r)
	perms, ok := ctx.permissions()
	if !ok {
		writeStats(w, r, []TableStats{})
		return
	}

	ctx.collectAccessibleTables(perms)
	if len(ctx.access) == 0 {
		writeStats(w, r, []TableStats{})
		return
	}

	stats, err := ctx.fetchRawStats()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: err.Error()})
		return
	}

	if ctx.isAdmin() {
		writeStats(w, r, ctx.buildAdminStats(stats))
		return
	}
	writeStats(w, r, ctx.buildUserStats(stats))
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
