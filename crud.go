package crudo

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kmlixh/gom/v4"
	"github.com/kmlixh/gom/v4/define"
)

const (
	SuccessCode  = 200
	SuccessMsg   = "success"
	ErrorCode    = 500
	ErrorMessage = "error"
)

const (
	PathSave   = "save"
	PathDelete = "delete"
	PathGet    = "get"
	PathList   = "list"
	PathPage   = "page"
	PathTable  = "table"
)

type RequestHandler struct {
	Method    string
	PreHandle fiber.Handler
	ParseRequestFunc
	DataOperationFunc
	TransferResultFunc
	RenderResponseFunc
}

type Column struct {
	Name    string
	Type    string
	Comment string
	IsKey   bool
	IsAuto  bool
}

type TableInfo struct {
	TableName      string
	Comment        string
	PrimaryKey     []string
	PrimaryKeyAuto []string
	Columns        []Column
}

type ICrud interface {
	AddHandler(path string, h *RequestHandler)
	RemoveHandler(path string)
	GetHandler(path string) (*RequestHandler, bool)
	RegisterRoutes(r fiber.Router)
	GetPrefix() string
	GetAvailablePaths() []string
	Handle(c *fiber.Ctx) error
}

type Crud struct {
	Prefix         string
	Table          string
	Db             *gom.DB
	TransferMap    map[string]string
	FieldOfList    []string
	FieldOfDetail  []string
	HandlerMap     map[string]*RequestHandler // key is now full path: prefix + "/" + operation
	handlerFilters []string
	queryBuilder   *QueryBuilder
	mu             sync.RWMutex
}

type QueryBuilder struct {
	db          *gom.DB
	table       string
	columnCache map[string]define.ColumnInfo
	columnLock  sync.RWMutex
}

type QueryParams struct {
	Table           string           `json:"table"`
	Page            int              `json:"page"`
	PageSize        int              `json:"pageSize"`
	ConditionParams []ConditionParam `json:"conditionParams"`
	OrderBy         []string         `json:"orderBy"`
	OrderByDesc     []string         `json:"orderByDesc"`
}

type ConditionParam struct {
	Key    string        `json:"key"`
	Op     define.OpType `json:"op"`
	Values any           `json:"values"`
}

// 添加批量删除的请求结构
type DeleteRequest struct {
	IDs []any `json:"ids"` // 要删除的记录ID列表，支持字符串和数字类型
}

func NewQueryBuilder(db *gom.DB, table string) *QueryBuilder {
	return &QueryBuilder{
		db:          db,
		table:       table,
		columnCache: make(map[string]define.ColumnInfo),
	}
}

func (qb *QueryBuilder) CacheTableInfo() (map[string]define.ColumnInfo, error) {
	qb.columnLock.Lock()
	defer qb.columnLock.Unlock()

	if len(qb.columnCache) > 0 {
		return qb.columnCache, nil
	}

	tableInfo, err := qb.db.GetTableInfo(qb.table)
	if err != nil {
		return nil, err
	}

	for _, col := range tableInfo.Columns {
		qb.columnCache[col.Name] = col
	}
	return qb.columnCache, nil
}

func (c *Crud) AddHandler(path string, h *RequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HandlerMap[path] = h
}

func (h *RequestHandler) Handle(c *fiber.Ctx) error {
	input, err := h.ParseRequestFunc(c)
	fmt.Printf("ParseRequestFunc result: input=%+v, err=%v\n", input, err)
	var result any
	if err == nil {
		result, err = h.DataOperationFunc(input)
		fmt.Printf("DataOperationFunc result: result=%+v, err=%v\n", result, err)
	}
	if err == nil && h.TransferResultFunc != nil {
		result, err = h.TransferResultFunc(result)
		fmt.Printf("TransferResultFunc result: result=%+v, err=%v\n", result, err)
	}
	return h.RenderResponseFunc(c, result, err)
}

func (c *Crud) RegisterRoutes(r fiber.Router) {
	for path, handler := range c.HandlerMap {
		if handler.PreHandle != nil {
			r.Add(handler.Method, path, handler.PreHandle, handler.Handle)
		} else {
			r.Add(handler.Method, path, handler.Handle)
		}
	}
}

func (c *Crud) GetAvailablePaths() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	paths := make([]string, 0, len(c.HandlerMap))
	for path := range c.HandlerMap {
		paths = append(paths, path)
	}
	return paths
}

func (c *Crud) InitDefaultHandler() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.HandlerMap = make(map[string]*RequestHandler)

	// Define all possible handlers
	allHandlers := map[string]*RequestHandler{
		PathSave: {
			Method:            http.MethodPost,
			ParseRequestFunc:  c.requestToMap(),
			DataOperationFunc: c.saveOperation(),
			RenderResponseFunc: func(ctx *fiber.Ctx, data any, err error) error {
				if err != nil {
					return RenderErrs(ctx, err)
				}
				return RenderOk(ctx, data)
			},
		},
		PathDelete: {
			Method: http.MethodPost,
			ParseRequestFunc: func(ctx *fiber.Ctx) (any, error) {
				// 尝试解析批量删除请求
				var deleteReq DeleteRequest
				if err := ctx.BodyParser(&deleteReq); err == nil {
					return deleteReq, nil
				}

				// 回退到查询参数方式
				return RequestToQueryParamsTransfer(c.Table, c.TransferMap, c.queryBuilder.columnCache)(ctx)
			},
			DataOperationFunc: c.deleteOperation(),
			RenderResponseFunc: func(ctx *fiber.Ctx, data any, err error) error {
				if err != nil {
					return RenderErrs(ctx, err)
				}
				return RenderOk(ctx, data)
			},
		},
		PathGet: {
			Method:            http.MethodGet,
			ParseRequestFunc:  RequestToQueryParamsTransfer(c.Table, c.TransferMap, c.queryBuilder.columnCache),
			DataOperationFunc: c.getOperation(),
			TransferResultFunc: func(data any) (any, error) {
				if data == nil {
					return nil, nil
				}
				result, ok := data.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("unexpected data type: %T", data)
				}
				return c.transferData(result, true)
			},
			RenderResponseFunc: func(ctx *fiber.Ctx, data any, err error) error {
				if err != nil {
					return RenderErrs(ctx, err)
				}
				return RenderOk(ctx, data)
			},
		},
		PathList: {
			Method:             http.MethodGet,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, c.queryBuilder.columnCache),
			DataOperationFunc:  c.listOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: func(ctx *fiber.Ctx, data any, err error) error {
				if err != nil {
					return RenderErrs(ctx, err)
				}
				return RenderOk(ctx, data)
			},
		},
		PathPage: {
			Method:             http.MethodGet,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, c.queryBuilder.columnCache),
			DataOperationFunc:  c.pageOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: func(ctx *fiber.Ctx, data any, err error) error {
				if err != nil {
					return RenderErrs(ctx, err)
				}
				return RenderOk(ctx, data)
			},
		},
		PathTable: {
			Method:            http.MethodGet,
			ParseRequestFunc:  func(c *fiber.Ctx) (any, error) { return nil, nil },
			DataOperationFunc: c.tableOperation(),
			RenderResponseFunc: func(ctx *fiber.Ctx, data any, err error) error {
				if err != nil {
					return RenderErrs(ctx, err)
				}
				return RenderOk(ctx, data)
			},
		},
	}

	// If no filters specified, use all handlers
	if len(c.handlerFilters) == 0 {
		for path, handler := range allHandlers {
			c.HandlerMap[path] = handler
		}
		return nil
	}

	// Only add handlers that are in the filter list
	for _, path := range c.handlerFilters {
		if handler, exists := allHandlers[path]; exists {
			c.HandlerMap[path] = handler
		}
	}

	return nil
}

func (c *Crud) tableOperation() DataOperationFunc {
	return func(input any) (any, error) {
		return c.Db.GetTableInfo(c.Table)
	}
}

func (c *Crud) requestToMap() ParseRequestFunc {
	return func(ctx *fiber.Ctx) (any, error) {
		fmt.Printf("requestToMap: method=%s, path=%s\n", ctx.Method(), ctx.Path())

		// 原有的处理逻辑
		data := make(map[string]any)
		if err := ctx.BodyParser(&data); err != nil {
			fmt.Printf("requestToMap: body parse error: %v\n", err)
			return nil, fmt.Errorf("invalid request body: %w", err)
		}

		if idParam := ctx.Params("id"); idParam != "" {
			if id, err := strconv.ParseInt(idParam, 10, 64); err == nil {
				data["id"] = id
			}
		}

		fmt.Printf("requestToMap: data before transfer: %+v\n", data)
		result, err := c.transferData(data, false)
		fmt.Printf("requestToMap: data after transfer: %+v, err=%v\n", result, err)
		return result, err
	}
}

func (c *Crud) transferData(input map[string]any, reverse bool) (map[string]any, error) {
	output := make(map[string]any)

	// 总是保留 id 字段
	if id, ok := input["id"]; ok {
		output["id"] = id
	}

	// 如果没有映射配置，直接返回原始数据
	if len(c.TransferMap) == 0 {
		for k, v := range input {
			if k != "id" { // 避免重复添加 id
				output[k] = v
			}
		}
		return output, nil
	}

	// 应用字段映射
	if reverse {
		// 数据库字段名 -> API 字段名
		for apiField, dbField := range c.TransferMap {
			if val, ok := input[dbField]; ok {
				output[apiField] = val
			}
		}
		// 保留未映射的字段
		for k, v := range input {
			if _, mapped := c.reverseMap()[k]; !mapped && k != "id" {
				output[k] = v
			}
		}
	} else {
		// API 字段名 -> 数据库字段名
		for apiField, dbField := range c.TransferMap {
			if val, ok := input[apiField]; ok {
				output[dbField] = val
			}
		}
		// 保留未映射的字段
		for k, v := range input {
			if _, mapped := c.TransferMap[k]; !mapped && k != "id" {
				output[k] = v
			}
		}
	}

	return output, nil
}

func (c *Crud) reverseMap() map[string]string {
	rm := make(map[string]string)
	for k, v := range c.TransferMap {
		rm[v] = k
	}
	return rm
}

func (c *Crud) saveOperation() DataOperationFunc {
	return func(input any) (any, error) {
		data, ok := input.(map[string]any)
		if !ok {
			return nil, errors.New("invalid data format")
		}

		chain := c.Db.Chain().Table(c.Table)

		// 获取表结构信息，包括主键信息
		tableInfo, err := c.Db.GetTableInfo(c.Table)
		if err != nil {
			return nil, fmt.Errorf("failed to get table info: %w", err)
		}

		// 检查表是否有主键
		if len(tableInfo.PrimaryKeys) == 0 {
			return nil, errors.New("table has no primary key")
		}

		// 获取第一个主键字段
		primaryKey := tableInfo.PrimaryKeys[0]

		// 检查是否是更新操作
		var isUpdate bool
		var primaryKeyValue any
		if pkVal, hasPK := data[primaryKey]; hasPK && isPrimaryKeyValid(pkVal) {
			isUpdate = true
			primaryKeyValue = pkVal
			delete(data, primaryKey)
		}

		// 获取表结构信息，用于自动填充时间字段
		columnInfo, err := c.queryBuilder.CacheTableInfo()
		if err == nil {
			now := time.Now()

			// 当主键为空时（新增记录时）处理所有时间字段
			if !isUpdate {
				// 遍历所有字段
				for fieldName, colInfo := range columnInfo {
					// 检查字段是否为时间类型
					if isTimeField(colInfo.DataType) {
						// 检查字段是否需要自动填充（为空或值不合法）
						shouldFill := false

						if fieldVal, exists := data[fieldName]; !exists || fieldVal == nil {
							// 字段不存在或为nil
							shouldFill = true
						} else {
							// 检查字段值是否为合法的时间值
							switch v := fieldVal.(type) {
							case string:
								if v == "" {
									shouldFill = true
								} else {
									// 尝试解析时间字符串
									_, err := parseTimeWithMultipleFormats(v)
									if err != nil {
										// 时间格式不合法，标记为需要填充
										shouldFill = true
									}
								}
							case time.Time:
								// 已经是time.Time类型，检查是否为零值
								if v.IsZero() {
									shouldFill = true
								}
							default:
								// 非时间类型值，标记为需要填充
								shouldFill = true
							}
						}

						// 如果需要填充，设置为当前时间
						if shouldFill {
							data[fieldName] = now
						}
					}
				}
			} else {
				// 更新操作时，只自动填充更新时间相关字段
				updateTimeFieldNames := []string{"update_at", "updated_at", "update_time", "modification_time", "modified_at"}
				for _, fieldName := range updateTimeFieldNames {
					if col, exists := columnInfo[fieldName]; exists {
						if isTimeField(col.DataType) {
							if _, hasField := data[fieldName]; !hasField || data[fieldName] == nil {
								data[fieldName] = now
							}
						}
					}
				}
			}
		}

		// 执行保存操作
		if isUpdate {
			// 更新操作
			chain.Where(primaryKey, define.OpEq, primaryKeyValue)
			result := chain.Values(data).Update()
			if result.Error != nil {
				return nil, result.Error
			}
			// 重新查询获取更新后的数据
			queryResult := chain.First()
			if queryResult.Error != nil {
				return nil, queryResult.Error
			}
			if len(queryResult.Data) == 0 {
				return nil, errors.New("failed to retrieve updated data")
			}
			return c.transferData(queryResult.Data[0], true)
		} else {
			// 插入操作 - 直接使用原始 SQL 和预处理语句
			columns := make([]string, 0, len(data))
			values := make([]any, 0, len(data))
			placeholders := make([]string, 0, len(data))

			i := 1
			for k, v := range data {
				columns = append(columns, k)
				values = append(values, v)
				placeholders = append(placeholders, fmt.Sprintf("$%d", i))
				i++
			}

			// 为 PostgreSQL 使用 RETURNING 语法
			query := fmt.Sprintf("INSERT INTO \"%s\" (%s) VALUES (%s) RETURNING *",
				c.Table,
				strings.Join(columns, ", "),
				strings.Join(placeholders, ", "))

			result := chain.Raw(query, values...).Exec()
			if result.Error != nil {
				return nil, result.Error
			}

			if len(result.Data) == 0 {
				// 如果没有返回数据
				return map[string]interface{}{
					"success": true,
				}, nil
			}

			return c.transferData(result.Data[0], true)
		}
	}
}

// 判断主键值是否有效（不为nil、空字符串、0等）
func isPrimaryKeyValid(value any) bool {
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case string:
		return v != "" && v != "0"
	case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		// 整数类型统一处理
		return v != 0
	case float32:
		return math.Abs(float64(v)) > 1e-10
	case float64:
		return math.Abs(v) > 1e-10
	default:
		// 对于其他类型，转为字符串比较
		str := fmt.Sprintf("%v", v)
		return str != "" && str != "0"
	}
}

// 尝试使用多种格式解析时间字符串
func parseTimeWithMultipleFormats(v string) (time.Time, error) {
	timeFormats := []string{
		time.RFC3339,          // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05", // ISO8601
		"2006-01-02 15:04:05", // 常见日期时间格式
		"2006-01-02 15:04",    // 日期时间不含秒
		"2006-01-02",          // 仅日期
		"01/02/2006 15:04:05", // 美式日期时间
		"01/02/2006",          // 美式日期
		"02/01/2006 15:04:05", // 欧式日期时间
		"02/01/2006",          // 欧式日期
		"20060102150405",      // 紧凑格式
		"20060102",            // 紧凑日期
		time.ANSIC,
		time.UnixDate,
		time.RubyDate,
		time.RFC822,
		time.RFC822Z,
		time.RFC850,
		time.RFC1123,
		time.RFC1123Z,
		time.RFC3339Nano,
		time.Kitchen,
		time.Stamp,
		time.StampMilli,
		time.StampMicro,
		time.StampNano,
	}

	var lastErr error
	for _, format := range timeFormats {
		parsed, err := time.Parse(format, v)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}

	return time.Time{}, fmt.Errorf("无法解析为时间格式: %s (错误: %v)", v, lastErr)
}

// 修改 deleteOperation 方法
func (c *Crud) deleteOperation() DataOperationFunc {
	return func(input any) (any, error) {
		// 获取表的主键信息
		tableInfo, err := c.Db.GetTableInfo(c.Table)
		if err != nil {
			return nil, fmt.Errorf("failed to get table info: %w", err)
		}

		// 查找主键列
		primaryKey := tableInfo.PrimaryKeys[0]
		if primaryKey == "" {
			return nil, errors.New("table has no primary key")
		}

		// 批量删除模式
		if deleteReq, ok := input.(DeleteRequest); ok {
			if len(deleteReq.IDs) == 0 {
				return nil, errors.New("ids cannot be empty")
			}

			// 批量删除 - 构建 WHERE primaryKey IN (...) 条件
			placeholders := make([]string, len(deleteReq.IDs))
			values := make([]any, len(deleteReq.IDs))
			for i, id := range deleteReq.IDs {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
				values[i] = id
			}

			query := fmt.Sprintf("DELETE FROM \"%s\" WHERE \"%s\" IN (%s)",
				c.Table,
				primaryKey,
				strings.Join(placeholders, ", "))

			result := c.Db.Chain().Raw(query, values...).Exec()
			if result.Error != nil {
				return nil, fmt.Errorf("batch delete failed: %w", result.Error)
			}

			// Call the RowsAffected function to get the actual count
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return nil, fmt.Errorf("failed to get affected rows: %w", err)
			}

			return map[string]interface{}{
				"deleted_count": rowsAffected,
				"ids":           deleteReq.IDs,
			}, nil
		}

		// 单个ID或条件删除模式
		params, ok := input.(QueryParams)
		if !ok {
			return nil, errors.New("invalid delete parameters")
		}

		// 使用 DELETE 语句但不带 RETURNING
		query := fmt.Sprintf("DELETE FROM \"%s\"", c.Table)
		values := make([]any, 0)
		var conditions []string

		valueIndex := 1
		for _, v := range params.ConditionParams {
			condition, condValues := buildCondition(v, valueIndex)
			if condition != "" {
				conditions = append(conditions, condition)
				values = append(values, condValues...)
				valueIndex += len(condValues)
			}
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}

		result := c.Db.Chain().Raw(query, values...).Exec()
		if result.Error != nil {
			return nil, fmt.Errorf("delete failed: %w", result.Error)
		}

		// Call the RowsAffected function to get the actual count
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to get affected rows: %w", err)
		}

		return map[string]interface{}{
			"deleted_count": rowsAffected,
		}, nil
	}
}

// 构建 SQL 条件
func buildCondition(param ConditionParam, startIndex int) (string, []any) {
	var condition string
	var values []any

	switch param.Op {
	case define.OpEq:
		condition = fmt.Sprintf("%s = $%d", param.Key, startIndex)
		values = []any{param.Values}
	case define.OpNe:
		condition = fmt.Sprintf("%s != $%d", param.Key, startIndex)
		values = []any{param.Values}
	case define.OpGt:
		condition = fmt.Sprintf("%s > $%d", param.Key, startIndex)
		values = []any{param.Values}
	case define.OpGe:
		condition = fmt.Sprintf("%s >= $%d", param.Key, startIndex)
		values = []any{param.Values}
	case define.OpLt:
		condition = fmt.Sprintf("%s < $%d", param.Key, startIndex)
		values = []any{param.Values}
	case define.OpLe:
		condition = fmt.Sprintf("%s <= $%d", param.Key, startIndex)
		values = []any{param.Values}
	case define.OpIn:
		// 处理 IN 操作
		if vals, ok := param.Values.([]any); ok && len(vals) > 0 {
			placeholders := make([]string, len(vals))
			for i := range vals {
				placeholders[i] = fmt.Sprintf("$%d", startIndex+i)
			}
			condition = fmt.Sprintf("%s IN (%s)", param.Key, strings.Join(placeholders, ", "))
			values = vals
		}
	default:
		// 对于其他操作，暂时不处理
		return "", nil
	}

	return condition, values
}

// 修改 getOperation 方法
func (c *Crud) getOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(QueryParams)
		if !ok {
			// 如果无法解析为 QueryParams，使用默认值
			params = QueryParams{
				Table: c.Table,
			}
		}

		chain := c.Db.Chain().Table(c.Table)
		for _, v := range params.ConditionParams {
			chain.Where(v.Key, v.Op, v.Values)
		}
		if len(c.FieldOfDetail) > 0 {
			chain.Fields(c.FieldOfDetail...)
		}

		result := chain.First()
		if result.Error != nil {
			// 对于"没有行"的情况返回空对象而不是错误
			if strings.Contains(result.Error.Error(), "no rows") {
				return map[string]interface{}{}, nil
			}
			return nil, fmt.Errorf("get failed: %w", result.Error)
		}

		if len(result.Data) == 0 {
			return map[string]interface{}{}, nil
		}

		// 转换字段名称
		return c.transferData(result.Data[0], true)
	}
}
func (c *Crud) pageOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(QueryParams)
		if !ok {
			// 如果无法解析为 QueryParams，使用默认值
			params = QueryParams{
				Table:    c.Table,
				Page:     1,
				PageSize: 10,
			}
		}

		chain := c.Db.Chain().Table(c.Table)
		for _, v := range params.ConditionParams {
			chain.Where(v.Key, v.Op, v.Values)
		}
		page := params.Page
		pageSize := params.PageSize
		if pageSize == 0 {
			pageSize = 10
		}
		if page == 0 {
			page = 1
		}
		if len(params.OrderBy) > 0 {
			for _, v := range params.OrderBy {
				chain.OrderBy(v)
			}
		}
		if len(params.OrderByDesc) > 0 {
			for _, v := range params.OrderByDesc {
				chain.OrderByDesc(v)
			}
		}
		if len(c.FieldOfList) > 0 {
			chain.Fields(c.FieldOfList...)
		}
		return chain.Page(page, pageSize).PageInfo()
	}
}

// 修改 listOperation 方法
func (c *Crud) listOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(QueryParams)
		if !ok {
			// 如果无法解析为 QueryParams，使用默认值
			params = QueryParams{
				Table: c.Table,
			}
		}

		chain := c.Db.Chain().Table(c.Table)
		for _, v := range params.ConditionParams {
			chain.Where(v.Key, v.Op, v.Values)
		}
		if len(params.OrderBy) > 0 {
			for _, v := range params.OrderBy {
				chain.OrderBy(v)
			}
		}
		if len(params.OrderByDesc) > 0 {
			for _, v := range params.OrderByDesc {
				chain.OrderByDesc(v)
			}
		}
		if len(c.FieldOfList) > 0 {
			chain.Fields(c.FieldOfList...)
		}
		result := chain.List()
		if result.Error != nil {
			return nil, fmt.Errorf("list failed: %w", result.Error)
		}
		return result.Data, nil
	}
}

func doNothingTransfer(input any) (any, error) {
	return input, nil
}

// 使用示例
func NewCrud(prefix, table string, db *gom.DB, transferMap map[string]string, fieldOfList []string, fieldOfDetail []string, handlerFilters []string) (*Crud, error) {
	crud := &Crud{
		Prefix:         prefix,
		Table:          table,
		Db:             db,
		TransferMap:    transferMap,
		FieldOfList:    fieldOfList,
		FieldOfDetail:  fieldOfDetail,
		handlerFilters: handlerFilters,
		queryBuilder:   NewQueryBuilder(db, table),
	}

	// Cache table column information
	_, err := crud.queryBuilder.CacheTableInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to cache table info: %v", err)
	}

	if err := crud.InitDefaultHandler(); err != nil {
		return nil, err
	}
	return crud, nil
}

func RequestToQueryParamsTransfer(tableName string, transferMap map[string]string, columnMap map[string]define.ColumnInfo) ParseRequestFunc {
	return func(c *fiber.Ctx) (any, error) {
		fmt.Printf("RequestToQueryParamsTransfer: tableName=%s\n", tableName)
		queryParams := QueryParams{
			Table: tableName,
		}

		// 从Request的Query生成一个Map
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			k := string(key)
			v := string(value)

			if k == "page" {
				page, err := strconv.Atoi(v)
				if err != nil {
					return
				}
				if page < 1 {
					page = 1
				}
				queryParams.Page = page
			} else if k == "pageSize" {
				pageSize, err := strconv.Atoi(v)
				if err != nil {
					return
				}
				if pageSize < 1 {
					pageSize = 10
				}
				queryParams.PageSize = pageSize
			} else if k == "orderBy" {
				values := strings.Split(v, ",")
				vv := make([]string, 0)
				for _, vi := range values {
					if vk, ok := transferMap[vi]; ok {
						vv = append(vv, vk)
					} else {
						vv = append(vv, vi)
					}
				}
				queryParams.OrderBy = vv
			} else if k == "orderByDesc" {
				values := strings.Split(v, ",")
				vv := make([]string, 0)
				for _, vi := range values {
					if vk, ok := transferMap[vi]; ok {
						vv = append(vv, vk)
					} else {
						vv = append(vv, vi)
					}
				}
				queryParams.OrderByDesc = vv
			} else {
				// 从k中解析出key和op
				key, op := KeyToKeyOp(k)
				if newKey, ok := transferMap[key]; ok {
					key = newKey
				}
				column, ok := columnMap[key]
				if !ok {
					return
				}
				values := strings.Split(v, ",")
				val, err := QueryValuesToValues(op, values, column)
				if err != nil {
					return
				}
				queryParams.ConditionParams = append(queryParams.ConditionParams, ConditionParam{
					Key:    key,
					Op:     op,
					Values: val,
				})
			}
		})

		fmt.Printf("RequestToQueryParamsTransfer: queryParams=%+v\n", queryParams)
		return queryParams, nil
	}
}

func QueryValuesToValues(op define.OpType, values []string, column define.ColumnInfo) (any, error) {
	//将values转换为[]any
	var err error
	transferTypeFunc := TransferType(column)
	anyValues := make([]any, len(values))
	for i, v := range values {
		anyValues[i], err = transferTypeFunc(v)
		if err != nil {
			return nil, err
		}
	}
	if len(values) == 1 {
		return anyValues[0], nil
	}
	return anyValues, nil
}

type TransferTypeFunc func(any string) (any, error)

func TransferType(column define.ColumnInfo) TransferTypeFunc {
	switch column.DataType {
	case "string":
		return func(v string) (any, error) {
			return v, nil
		}
	case "int8":
		return func(v string) (any, error) {
			val, err := strconv.ParseInt(v, 10, 8)
			if err != nil {
				return nil, err
			}
			return val, nil
		}
	case "int16":
		return func(v string) (any, error) {
			val, err := strconv.ParseInt(v, 10, 16)
			if err != nil {
				return nil, err
			}
			return val, nil
		}
	case "int32":
		return func(v string) (any, error) {
			val, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return nil, err
			}
			return val, nil
		}
	case "bool":
		return func(v string) (any, error) {
			val, err := strconv.ParseBool(v)
			if err != nil {
				return nil, err
			}
			return val, nil
		}
	case "time.Time":
		return func(v string) (any, error) {
			// 支持多种日期时间格式
			timeFormats := []string{
				time.RFC3339,          // 2006-01-02T15:04:05Z07:00
				"2006-01-02T15:04:05", // ISO8601
				"2006-01-02 15:04:05", // 常见日期时间格式
				"2006-01-02 15:04",    // 日期时间不含秒
				"2006-01-02",          // 仅日期
				"01/02/2006 15:04:05", // 美式日期时间
				"01/02/2006",          // 美式日期
				"02/01/2006 15:04:05", // 欧式日期时间
				"02/01/2006",          // 欧式日期
				"20060102150405",      // 紧凑格式
				"20060102",            // 紧凑日期
				time.ANSIC,
				time.UnixDate,
				time.RubyDate,
				time.RFC822,
				time.RFC822Z,
				time.RFC850,
				time.RFC1123,
				time.RFC1123Z,
				time.RFC3339Nano,
				time.Kitchen,
				time.Stamp,
				time.StampMilli,
				time.StampMicro,
				time.StampNano,
			}

			var err error
			for _, format := range timeFormats {
				val, err := time.Parse(format, v)
				if err == nil {
					return val, nil
				}
			}

			// 如果所有格式都无法解析，返回最后一个错误
			return nil, fmt.Errorf("无法解析为时间格式: %s (错误: %v)", v, err)
		}
	case "uint8":
		return func(v string) (any, error) {
			val, err := strconv.ParseUint(v, 10, 8)
			if err != nil {
				return nil, err
			}
			return uint8(val), nil
		}
	case "uint16":
		return func(v string) (any, error) {
			val, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				return nil, err
			}
			return uint16(val), nil
		}
	case "uint32":
		return func(v string) (any, error) {
			val, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return nil, err
			}
			return uint32(val), nil
		}
	case "uint64":
		return func(v string) (any, error) {
			val, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, err
			}
			return uint64(val), nil
		}
	case "float32":
		return func(v string) (any, error) {
			val, err := strconv.ParseFloat(v, 32)
			if err != nil {
				return nil, err
			}
			return float32(val), nil
		}
	case "float64":
		return func(v string) (any, error) {
			val, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, err
			}
			return val, nil
		}
	case "[]byte":
		return func(v string) (any, error) {
			return []byte(v), nil
		}
	case "[]uint8":
		return func(v string) (any, error) {
			return []uint8(v), nil
		}
	default:
		return func(v string) (any, error) {
			return v, nil
		}
	}
}

func KeyToKeyOp(key string) (string, define.OpType) {
	lastIndex := strings.LastIndex(key, "_")
	if lastIndex == -1 {
		return key, define.OpEq
	}

	field := key[:lastIndex]
	opStr := key[lastIndex+1:]
	op := define.OpEq

	switch opStr {
	case "eq":
		op = define.OpEq
	case "ne":
		op = define.OpNe
	case "gt":
		op = define.OpGt
	case "ge":
		op = define.OpGe
	case "lt":
		op = define.OpLt
	case "le":
		op = define.OpLe
	case "in":
		op = define.OpIn
	case "notIn":
		op = define.OpNotIn
	case "isNull":
		op = define.OpIsNull
	case "isNotNull":
		op = define.OpIsNotNull
	case "between":
		op = define.OpBetween
	case "notBetween":
		op = define.OpNotBetween
	case "like":
		op = define.OpLike
	case "notLike":
		op = define.OpNotLike
	}

	return field, op
}

func (c *Crud) Handle(ctx *fiber.Ctx) error {
	// 获取请求路径
	path := ctx.Path()

	// 提取操作部分
	if !strings.Contains(path, c.Prefix) {
		return errors.New("path not configured")
	}
	path = path[strings.Index(path, c.Prefix):]
	operation := strings.TrimPrefix(path, c.Prefix)
	operation = strings.TrimPrefix(operation, "/")

	// 查找对应的处理器
	c.mu.RLock()
	handler, exists := c.HandlerMap[operation]
	c.mu.RUnlock()

	if !exists || handler == nil {
		return ctx.Status(http.StatusNotFound).JSON(fiber.Map{"error": "operation not configured"})
	}

	if ctx.Method() != handler.Method {
		return ctx.Status(http.StatusMethodNotAllowed).JSON(fiber.Map{"error": "method not allowed"})
	}

	if handler.PreHandle != nil {
		if err := handler.PreHandle(ctx); err != nil {
			return err
		}
	}

	return handler.Handle(ctx)
}

func (c *Crud) GetHandler(path string) (*RequestHandler, bool) {
	c.mu.RLock()
	operation := strings.TrimPrefix(path, c.Prefix)
	operation = strings.TrimPrefix(operation, "/")
	handler, exists := c.HandlerMap[operation]
	c.mu.RUnlock()
	return handler, exists
}

func (c *Crud) RemoveHandler(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.HandlerMap, path)
}

func (c *Crud) GetPrefix() string {
	return c.Prefix
}

// isTimeField 判断字段类型是否为时间相关类型
func isTimeField(dataType string) bool {
	timeTypes := []string{
		"time", "date", "timestamp", "datetime", "timestamptz",
		"time.Time", "Time", "DATE", "TIMESTAMP", "DATETIME",
	}

	dataTypeLower := strings.ToLower(dataType)
	for _, tt := range timeTypes {
		if strings.Contains(dataTypeLower, strings.ToLower(tt)) {
			return true
		}
	}

	return false
}
