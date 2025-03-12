package crudo

import (
	"errors"
	"fmt"
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
}

type Crud struct {
	Prefix         string
	Table          string
	Db             *gom.DB
	TransferMap    map[string]string
	FieldOfList    []string
	FieldOfDetail  []string
	HandlerMap     map[string]*RequestHandler
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
	var result any
	if err == nil {
		result, err = h.DataOperationFunc(input)
	}
	if err == nil && h.TransferResultFunc != nil {
		result, err = h.TransferResultFunc(result)
	}
	return h.RenderResponseFunc(c, result, err)
}

func (c *Crud) RegisterRoutes(r fiber.Router) {
	for path, handler := range c.HandlerMap {
		if handler.PreHandle != nil {
			r.Add(handler.Method, c.Prefix+"/"+path, handler.PreHandle, handler.Handle)
		} else {
			r.Add(handler.Method, c.Prefix+"/"+path, handler.Handle)
		}
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

	// 定义所有可能的处理器映射
	allHandlers := map[string]*RequestHandler{
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

	// 初始化 HandlerMap
	c.HandlerMap = make(map[string]*RequestHandler)

	// 如果没有指定过滤器，添加所有处理器
	if len(c.handlerFilters) == 0 {
		c.HandlerMap = allHandlers
		return nil
	}

	// 只添加指定的处理器
	for _, path := range c.handlerFilters {
		if handler, exists := allHandlers[path]; exists {
			c.HandlerMap[path] = handler
		}
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
	return func(ctx *fiber.Ctx) (any, error) {
		data := make(map[string]any)
		if err := ctx.BodyParser(&data); err != nil {
			return nil, fmt.Errorf("invalid request body: %w", err)
		}

		if idParam := ctx.Params("id"); idParam != "" {
			if id, err := strconv.ParseInt(idParam, 10, 64); err == nil {
				data["id"] = id
			}
		}

		return c.transferData(data, false)
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
			if result.Error != nil {
				return nil, result.Error
			}
			// 对于更新操作，重新查询获取更新后的数据
			queryResult := chain.First()
			if queryResult.Error != nil {
				return nil, queryResult.Error
			}
			if len(queryResult.Data) == 0 {
				return nil, errors.New("failed to retrieve saved data")
			}
			// 转换字段名称并返回
			return c.transferData(queryResult.Data[0], true)
		} else {
			// 对于插入操作，使用 RETURNING * 获取插入的数据
			result = chain.Values(data).Raw("RETURNING *").Save()
			if result.Error != nil {
				return nil, result.Error
			}
			if len(result.Data) == 0 {
				return nil, errors.New("failed to retrieve saved data")
			}
			// 转换字段名称并返回
			return c.transferData(result.Data[0], true)
		}
	}
}

// 其他操作实现类似，限于篇幅省略...
// 完整实现需要补充deleteOperation/getOperation/listOperation

func doNothingTransfer(input any) (any, error) {
	return input, nil
}

func renderJSON(ctx *fiber.Ctx, data any, err error) error {
	code := SuccessCode
	msg := SuccessMsg
	if err != nil {
		code = ErrorCode
		msg = err.Error()
		data = nil
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	return ctx.Status(http.StatusOK).JSON(CodeMsg{
		Code:    code,
		Data:    data,
		Message: msg,
	})
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
	if err := crud.InitDefaultHandler(); err != nil {
		return nil, err
	}
	return crud, nil
}

func RequestToQueryParamsTransfer(tableName string, transferMap map[string]string, columnMap map[string]define.ColumnInfo) ParseRequestFunc {
	return func(c *fiber.Ctx) (any, error) {
		queryParams := QueryParams{}
		queryParams.Table = tableName

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

		result := chain.First()
		if result.Error != nil {
			return nil, fmt.Errorf("get failed: %w", result.Error)
		}

		if len(result.Data) == 0 {
			return nil, errors.New("record not found")
		}

		// 转换字段名称
		return c.transferData(result.Data[0], true)
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

		// 获取总数
		total, err := chain.Count()
		if err != nil {
			return nil, fmt.Errorf("count failed: %w", err)
		}

		// 执行分页查询
		result := chain.Limit(pageSize).Offset((page - 1) * pageSize).List()
		if result.Error != nil {
			return nil, fmt.Errorf("list query failed: %w", result.Error)
		}

		// 转换分页结果
		var converted []map[string]any
		for _, item := range result.Data {
			if convertedData, err := c.transferData(item, true); err == nil {
				converted = append(converted, convertedData)
			}
		}

		// 构建分页响应
		return map[string]any{
			"Page":     page,
			"PageSize": pageSize,
			"Total":    total,
			"List":     converted,
		}, nil
	}
}
