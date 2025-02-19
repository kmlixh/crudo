package crudo

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
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
)

type (
	CodeMsg struct {
		Code    int    `json:"code"`
		Data    any    `json:"data"`
		Message string `json:"msg"`
	}

	ParseRequestFunc     func(*gin.Context) (any, error)
	DataOperationFunc    func(any) (any, error)
	TransferResultFunc   func(any) (any, error)
	RenderResponseFunc   func(*gin.Context, any, error)
	ValidationMiddleware func(*gin.Context) error
)

type RequestHandler struct {
	Method     string
	PreHandler gin.HandlerFunc
	ParseRequestFunc
	DataOperationFunc
	TransferResultFunc
	RenderResponseFunc
}

type ICrud interface {
	AddHandler(path string, h *RequestHandler)
	RemoveHandler(path string)
	GetHandler(path string) (*RequestHandler, bool)
	RegisterRoutes(r *gin.RouterGroup)
}

type Crud struct {
	Prefix        string
	Table         string
	Db            *gom.DB
	TransferMap   map[string]string
	FieldOfList   []string
	FieldOfDetail []string
	HandlerMap    map[string]*RequestHandler
	queryBuilder  *QueryBuilder
	mu            sync.RWMutex
}

type QueryBuilder struct {
	db          *gom.DB
	table       string
	columnCache map[string]define.ColumnInfo
	columnLock  sync.RWMutex
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

func (c *Crud) InitDefaultHandler() error {
	tableInfo, err := c.Db.GetTableInfo(c.Table)
	if err != nil {
		return fmt.Errorf("failed to get table info: %w", err)
	}

	columnMap := make(map[string]define.ColumnInfo)
	for _, col := range tableInfo.Columns {
		columnMap[col.Name] = col
	}

	c.HandlerMap = map[string]*RequestHandler{
		c.Prefix + "/" + PathSave: {
			Method:             http.MethodPost,
			ParseRequestFunc:   c.requestToMap(),
			DataOperationFunc:  c.saveOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
		c.Prefix + "/" + PathDelete: {
			Method:             http.MethodDelete,
			ParseRequestFunc:   c.queryParamsParser(columnMap),
			DataOperationFunc:  c.deleteOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
		c.Prefix + "/" + PathGet: {
			Method:             http.MethodGet,
			ParseRequestFunc:   c.queryParamsParser(columnMap),
			DataOperationFunc:  c.getOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
		c.Prefix + "/" + PathList: {
			Method:             http.MethodGet,
			ParseRequestFunc:   c.queryParamsParser(columnMap),
			DataOperationFunc:  c.listOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
	}
	return nil
}

func (c *Crud) requestToMap() ParseRequestFunc {
	return func(ctx *gin.Context) (any, error) {
		data := make(map[string]any)
		if err := ctx.ShouldBindJSON(&data); err != nil {
			return nil, fmt.Errorf("invalid request body: %w", err)
		}

		if idParam := ctx.Param("id"); idParam != "" {
			if id, err := strconv.ParseInt(idParam, 10, 64); err == nil {
				data["id"] = id
			}
		}

		return c.transferData(data, false)
	}
}

func (c *Crud) transferData(input map[string]any, reverse bool) (map[string]any, error) {
	output := make(map[string]any)
	for k, v := range input {
		if k == "id" {
			output[k] = v
			continue
		}

		mappedKey := c.TransferMap[k]
		if reverse {
			if origKey, exists := c.reverseMap()[k]; exists {
				output[origKey] = v
			}
		} else if mappedKey != "" {
			output[mappedKey] = v
		} else {
			output[k] = v
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

		result := c.Db.Chain().Table(c.Table).Values(data).Save()
		if result.Error != nil {
			return nil, fmt.Errorf("save operation failed: %w", result.Error)
		}
		return result.Data, nil
	}
}

// 其他操作实现类似，限于篇幅省略...
// 完整实现需要补充deleteOperation/getOperation/listOperation

func (c *Crud) AddHandler(path string, h *RequestHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HandlerMap[path] = h
}

func (c *Crud) RegisterRoutes(r *gin.RouterGroup) {
	for path, handler := range c.HandlerMap {
		handlers := make([]gin.HandlerFunc, 0)
		if handler.PreHandler != nil {
			handlers = append(handlers, handler.PreHandler)
		}
		handlers = append(handlers, handler.Handle)
		r.Handle(handler.Method, path, handlers...)
	}
}

func doNothingTransfer(input any) (any, error) {
	return input, nil
}

func renderJSON(ctx *gin.Context, data any, err error) {
	code := SuccessCode
	msg := SuccessMsg
	if err != nil {
		code = ErrorCode
		msg = err.Error()
	}
	ctx.JSON(http.StatusOK, CodeMsg{
		Code:    code,
		Data:    data,
		Message: msg,
	})
}

// 使用示例
func NewCrud(prefix, table string, db *gom.DB, transferMap map[string]string) (*Crud, error) {
	crud := &Crud{
		Prefix:       prefix,
		Table:        table,
		Db:           db,
		TransferMap:  transferMap,
		queryBuilder: NewQueryBuilder(db, table),
	}
	if err := crud.InitDefaultHandler(); err != nil {
		return nil, err
	}
	return crud, nil
}

func (h *RequestHandler) Handle(c *gin.Context) {
	input, err := h.ParseRequestFunc(c)
	var result any
	if err == nil {
		result, err = h.DataOperationFunc(input)
	}
	if err == nil && h.TransferResultFunc != nil {
		result, err = h.TransferResultFunc(result)
	}
	h.RenderResponseFunc(c, result, err)
}

// 添加缺失的queryParamsParser方法
func (c *Crud) queryParamsParser(columns map[string]define.ColumnInfo) ParseRequestFunc {
	return func(ctx *gin.Context) (any, error) {
		params := make(map[string]any)
		for k, v := range ctx.Request.URL.Query() {
			if len(v) > 0 {
				// 自动转换数字类型
				if col, ok := columns[k]; ok {
					switch col.DataType {
					case "int", "integer":
						if num, err := strconv.Atoi(v[0]); err == nil {
							params[k] = num
							continue
						}
					case "bigint":
						if num, err := strconv.ParseInt(v[0], 10, 64); err == nil {
							params[k] = num
							continue
						}
					}
				}
				params[k] = v[0]
			}
		}
		return c.transferData(params, false)
	}
}

// 添加deleteOperation方法
func (c *Crud) deleteOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(map[string]any)
		if !ok {
			return nil, errors.New("invalid delete parameters")
		}

		chain := c.Db.Chain().Table(c.Table)
		for k, v := range params {
			chain.Where(k, define.OpEq, v)
		}

		result := chain.Delete()
		if result.Error != nil {
			return nil, fmt.Errorf("delete failed: %w", result.Error)
		}
		return result.RowsAffected, nil
	}
}

// 添加getOperation方法
func (c *Crud) getOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(map[string]any)
		if !ok {
			return nil, errors.New("invalid get parameters")
		}

		chain := c.Db.Chain().Table(c.Table)
		for k, v := range params {
			chain.Where(k, define.OpEq, v)
		}

		var result map[string]any
		if err := chain.First(&result).Error; err != nil {
			return nil, fmt.Errorf("get failed: %w", err)
		}
		return c.transferData(result, true) // 反向转换字段
	}
}

// 修改后的分页处理逻辑
func (c *Crud) listOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(map[string]any)
		if !ok {
			return nil, errors.New("invalid list parameters")
		}

		chain := c.Db.Chain().Table(c.Table)
		for k, v := range params {
			if k == "page" || k == "pageSize" {
				continue
			}
			chain.Where(k, define.OpEq, v)
		}

		// 分页参数处理
		page := 1
		if p, ok := params["page"].(int); ok {
			page = p
		}

		pageSize := 10
		if ps, ok := params["pageSize"].(int); ok {
			pageSize = ps
		}

		// 执行分页查询
		pageInfo, err := chain.Page(page, pageSize).PageInfo()
		if err != nil {
			return nil, fmt.Errorf("list query failed: %w", err)
		}
		originData, ok := pageInfo.List.([]any)
		if !ok {
			return nil, errors.New("invalid list data")
		}
		// 转换分页结果
		var converted []map[string]any
		for _, item := range originData {
			if data, ok := item.(map[string]any); ok {
				if convertedData, err := c.transferData(data, true); err == nil {
					converted = append(converted, convertedData)
				}
			}
		}

		// 重建分页结构
		pageInfo.List = converted
		return pageInfo, nil
	}
}
