package crudo

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
	PathTable  = "table"
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
	Method    string
	PreHandle gin.HandlerFunc
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
func (c *Crud) RegisterRoutes(r *gin.RouterGroup) {
	for path, handler := range c.HandlerMap {
		handlers := make([]gin.HandlerFunc, 0)
		if handler.PreHandle != nil {
			handlers = append(handlers, handler.PreHandle)
		}
		handlers = append(handlers, handler.Handle)
		r.Handle(handler.Method, c.Prefix+"/"+path, handlers...)
	}
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
		PathSave: {
			Method:             http.MethodPost,
			ParseRequestFunc:   c.requestToMap(),
			DataOperationFunc:  c.saveOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
		PathDelete: {
			Method:             http.MethodDelete,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, columnMap),
			DataOperationFunc:  c.deleteOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
		PathGet: {
			Method:             http.MethodGet,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, columnMap),
			DataOperationFunc:  c.getOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
		PathList: {
			Method:             http.MethodGet,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, columnMap),
			DataOperationFunc:  c.listOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
		PathTable: {
			Method:             http.MethodGet,
			DataOperationFunc:  c.tableOperation(),
			TransferResultFunc: doNothingTransfer,
			RenderResponseFunc: renderJSON,
		},
	}
	return nil
}
func (c *Crud) tableOperation() DataOperationFunc {
	return func(input any) (any, error) {
		tableInfo, er := c.Db.GetTableInfo(c.Table)
		if er != nil {
			return nil, fmt.Errorf("failed to get table info: %w", er)
		}
		columns := make([]Column, 0)
		primaryKeyAuto := make([]string, 0)
		for _, col := range tableInfo.Columns {
			columns = append(columns, Column{
				Name:    col.Name,
				Type:    col.DataType,
				Comment: col.Comment,
				IsKey:   col.IsPrimaryKey,
				IsAuto:  col.IsAutoIncrement,
			})
			if col.IsAutoIncrement {
				primaryKeyAuto = append(primaryKeyAuto, col.Name)
			}
		}
		table := TableInfo{
			TableName:      tableInfo.TableName,
			Comment:        tableInfo.TableComment,
			PrimaryKey:     tableInfo.PrimaryKeys,
			PrimaryKeyAuto: primaryKeyAuto,
			Columns:        columns,
		}
		return table, nil
	}
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

		chain := c.Db.Chain().Table(c.Table)

		// 检查是否是更新操作
		var isUpdate bool
		if id, hasID := data["id"]; hasID {
			isUpdate = true
			chain.Where("id", define.OpEq, id)
			delete(data, "id")
		}

		// 执行保存操作
		var result *define.Result
		if isUpdate {
			result = chain.Values(data).Update()
		} else {
			result = chain.Values(data).Save()
		}
		return result, nil
	}
}

// 其他操作实现类似，限于篇幅省略...
// 完整实现需要补充deleteOperation/getOperation/listOperation

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
func NewCrud(prefix, table string, db *gom.DB, transferMap map[string]string, fieldOfList []string, fieldOfDetail []string) (*Crud, error) {
	crud := &Crud{
		Prefix:        prefix,
		Table:         table,
		Db:            db,
		TransferMap:   transferMap,
		FieldOfList:   fieldOfList,
		FieldOfDetail: fieldOfDetail,
		queryBuilder:  NewQueryBuilder(db, table),
	}
	if err := crud.InitDefaultHandler(); err != nil {
		return nil, err
	}
	return crud, nil
}
func RequestToQueryParamsTransfer(tableName string, transferMap map[string]string, columnMap map[string]define.ColumnInfo) ParseRequestFunc {
	//  从request中
	return func(c *gin.Context) (any, error) {
		queryParams := QueryParams{}
		queryParams.Table = tableName
		// 从Request的Query生成一个Map
		for k, v := range c.Request.URL.Query() {
			if k == "page" {
				page, err := strconv.Atoi(v[0])
				if err != nil {
					return nil, err
				}
				if page < 1 {
					page = 1
				}
				queryParams.Page = page
			} else if k == "pageSize" {
				pageSize, err := strconv.Atoi(v[0])
				if err != nil {
					return nil, err
				}
				if pageSize < 1 {
					pageSize = 10
				}
				queryParams.PageSize = pageSize
			} else if k == "orderBy" {
				vv := make([]string, 0)
				for _, vi := range v {
					if vk, ok := transferMap[vi]; ok {
						vv = append(vv, vk)
					} else {
						vv = append(vv, vi)
					}
				}
				queryParams.OrderBy = vv
			} else if k == "orderByDesc" {
				vv := make([]string, 0)
				for _, vi := range v {
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
					return nil, fmt.Errorf("column %s not found", key)
				}
				values, err := QueryValuesToValues(op, v, column)
				if err != nil {
					return nil, err
				}
				queryParams.ConditionParams = append(queryParams.ConditionParams, ConditionParam{
					Key:    key,
					Op:     op,
					Values: values,
				})
			}
		}

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
			val, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, err
			}
			return val, nil
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
	keys := []string{key[:strings.LastIndex(key, "_")], key[strings.LastIndex(key, "_")+1:]}
	key = keys[0]
	opStr := keys[1]
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
	return key, op
}

// 添加缺失的queryParamsParser方法

// 添加deleteOperation方法
func (c *Crud) deleteOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(QueryParams)
		if !ok {
			return nil, errors.New("invalid delete parameters")
		}

		chain := c.Db.Chain().Table(c.Table)
		for _, v := range params.ConditionParams {
			chain.Where(v.Key, v.Op, v.Values)
		}

		result := chain.Delete()
		if result.Error != nil {
			return nil, fmt.Errorf("delete failed: %w", result.Error)
		}
		return result, nil
	}
}

// 添加getOperation方法
func (c *Crud) getOperation() DataOperationFunc {
	return func(input any) (any, error) {
		params, ok := input.(QueryParams)
		if !ok {
			return nil, errors.New("invalid get parameters")
		}

		chain := c.Db.Chain().Table(c.Table)
		for _, v := range params.ConditionParams {
			chain.Where(v.Key, v.Op, v.Values)
		}
		if len(c.FieldOfDetail) > 0 {
			chain.Fields(c.FieldOfDetail...)
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
		params, ok := input.(QueryParams)
		if !ok {
			return nil, errors.New("invalid list parameters")
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
