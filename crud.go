package crudo

import (
	"fmt"
	"strconv"
	"strings"
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
	PathInsert = "insert"
	PathUpdate = "update"
	PathDelete = "delete"
	PathGet    = "get"
	PathList   = "list"
)

type CodeMsg struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"msg"`
}
type ParseRequestFunc func(*gin.Context) (any, error)
type DataOperationFunc func(any) (any, error)
type TransferResultFunc func(any) (any, error)
type RenderResponseFunc func(*gin.Context, any, error)

type RequestHandler struct {
	Method     string
	PreHandler gin.HandlerFunc
	ParseRequestFunc
	DataOperationFunc
	TransferResultFunc
	RenderResponseFunc
}

func (h *RequestHandler) Handle(c *gin.Context) {
	var result any
	var err error

	input, err := h.ParseRequestFunc(c)
	if err == nil {
		result, err = h.DataOperationFunc(input)
	}
	if err == nil && h.TransferResultFunc != nil {
		result, err = h.TransferResultFunc(result)
	}
	h.RenderResponseFunc(c, result, err)
}

func (h *RequestHandler) Register(r *gin.RouterGroup, path string) {

	if h.PreHandler != nil {
		r.Handle(h.Method, path, h.PreHandler, h.Handle)
	} else {
		r.Handle(h.Method, path, h.Handle)
	}
}

type ICrud interface {
	AddHandler(path string, h *RequestHandler)
	RemoveHandler(path string)
	RegisterRoutes(r *gin.RouterGroup)
}
type Crud struct {
	HandlerMap map[string]RequestHandler
}

func (c *Crud) AddHandler(path string, h *RequestHandler) {
	c.HandlerMap[path] = *h
}

func (c *Crud) RemoveHandler(path string) {
	delete(c.HandlerMap, path)
}

func (c *Crud) RegisterRoutes(r *gin.RouterGroup) {
	for k, v := range c.HandlerMap {
		v.Register(r, k)
	}
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
	Values []any         `json:"values"`
}

func NewCrud(table string, db *gom.DB) (*Crud, error) {
	tableInfo, err := db.GetTableInfo(table)
	columnMap := TableInfoToColumnMap(tableInfo)
	if err != nil {
		return nil, err
	}
	return &Crud{HandlerMap: map[string]RequestHandler{
		PathInsert: {
			Method:             "POST",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToMap,
			DataOperationFunc:  QueryInsertFunc(db, tableInfo.TableName),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJsonResponse,
		},
		PathUpdate: {
			Method:             "POST",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToMap,
			DataOperationFunc:  QueryUpdateFunc(db, tableInfo.TableName),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJsonResponse,
		},
		PathDelete: {
			Method:             "GET",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToQueryParams(tableInfo.TableName, columnMap),
			DataOperationFunc:  QueryDeleteFunc(db, tableInfo.TableName),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJsonResponse,
		},
		PathGet: {
			Method:             "GET",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToQueryParams(tableInfo.TableName, columnMap),
			DataOperationFunc:  QueryGetFunc(db),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJsonResponse,
		},
		PathList: {
			Method:             "GET",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToQueryParams(tableInfo.TableName, columnMap),
			DataOperationFunc:  QueryListFunc(db),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJsonResponse,
		},
	}}, nil
}
func RequestToMap(c *gin.Context) (any, error) {
	mapData := make(map[string]any)
	if err := c.ShouldBindJSON(&mapData); err != nil {
		return nil, err
	}
	return mapData, nil
}
func RequestToQueryParams(tableName string, columnMap map[string]define.ColumnInfo) ParseRequestFunc {
	//  从request中
	return func(c *gin.Context) (any, error) {
		queryParams := QueryParams{}
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
				queryParams.OrderBy = v
			} else if k == "orderByDesc" {
				queryParams.OrderByDesc = v
			} else {
				// 从k中解析出key和op
				key, op := KeyToKeyOp(k)
				column, ok := columnMap[key]
				if !ok {
					return nil, fmt.Errorf("column %s not found", key)
				}
				values, err := QueryValuesToValues(v, column)
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
func KeyToKeyOp(key string) (string, define.OpType) {
	keys := strings.Split(key, "_")
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
func TableInfoToColumnMap(tableInfo *define.TableInfo) map[string]define.ColumnInfo {
	columnMap := make(map[string]define.ColumnInfo)
	for _, column := range tableInfo.Columns {
		columnMap[column.Name] = column
	}
	return columnMap
}
func QueryValuesToValues(values []string, column define.ColumnInfo) ([]any, error) {
	//将values转换为[]any
	anyValues := make([]any, len(values))
	for i, v := range values {
		switch column.DataType {
		case "sql.NullString":
			anyValues[i] = v
		case "sql.NullInt64":
			val, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, err
			}
			anyValues[i] = val
		case "sql.NullFloat64":
			val, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, err
			}
			anyValues[i] = val
		case "sql.NullBool":
			val, err := strconv.ParseBool(v)
			if err != nil {
				return nil, err
			}
			anyValues[i] = val
		case "sql.NullTime":
			val, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, err
			}
			anyValues[i] = val
		case "sql.NullInt32":
			val, err := strconv.ParseInt(v, 10, 32)
			if err != nil {
				return nil, err
			}
			anyValues[i] = int32(val)
		case "sql.NullInt16":
			val, err := strconv.ParseInt(v, 10, 16)
			if err != nil {
				return nil, err
			}
			anyValues[i] = int16(val)
		default:
			anyValues[i] = v
		}
	}
	return anyValues, nil
}
func QueryListFunc(db *gom.DB) DataOperationFunc {
	return func(i any) (any, error) {
		if queryParams, ok := i.(QueryParams); ok {
			chain := db.Chain()
			chain.Table(queryParams.Table)
			for _, param := range queryParams.ConditionParams {
				chain.Where(param.Key, param.Op, param.Values)
			}
			for _, orderBy := range queryParams.OrderBy {
				chain.OrderBy(orderBy)
			}
			for _, orderByDesc := range queryParams.OrderByDesc {
				chain.OrderByDesc(orderByDesc)
			}
			chain.Page(queryParams.Page, queryParams.PageSize)
			result := chain.List()
			return result, nil
		}
		return nil, fmt.Errorf("")
	}
}
func QueryGetFunc(db *gom.DB) DataOperationFunc {
	return func(i any) (any, error) {
		if queryParams, ok := i.(QueryParams); ok {
			chain := db.Chain()
			chain.Table(queryParams.Table)
			for _, param := range queryParams.ConditionParams {
				chain.Where(param.Key, param.Op, param.Values)
			}
			result := chain.First()
			return result.Data, nil
		}
		return nil, fmt.Errorf("")
	}
}
func QueryInsertFunc(db *gom.DB, table string) DataOperationFunc {
	return func(i any) (any, error) {
		if mapData, ok := i.(map[string]any); ok {
			chain := db.Chain()
			chain.Table(table)
			chain.Values(mapData)
			result := chain.Save()
			return result, nil
		}
		return nil, fmt.Errorf("")
	}
}
func QueryUpdateFunc(db *gom.DB, table string) DataOperationFunc {
	return func(i any) (any, error) {
		if queryParams, ok := i.(QueryParams); ok {
			chain := db.Chain()
			chain.Table(queryParams.Table)
			for _, param := range queryParams.ConditionParams {
				chain.Where(param.Key, param.Op, param.Values)
			}
			result := chain.Delete()
			return result.Data, nil
		}
		return nil, fmt.Errorf("")
	}
}
func QueryDeleteFunc(db *gom.DB, table string) DataOperationFunc {
	return func(i any) (any, error) {
		if mapData, ok := i.(map[string]any); ok {
			chain := db.Chain()
			chain.Table(table)
			chain.Where(mapData["id"].(string), define.OpEq, mapData["id"])
			result := chain.Delete()
			return result, nil
		}
		return nil, fmt.Errorf("")
	}
}

func QueryParamsToCondition(queryParams QueryParams) *define.Condition {
	var condition *define.Condition
	for i, param := range queryParams.ConditionParams {
		if i == 0 {
			condition = define.Eq(param.Key, param.Values[0])
		} else {
			condition = condition.And(define.Eq(param.Key, param.Values[0]))
		}
	}
	return condition
}
func DoNothingTransferResultFunc(i any) (any, error) {
	return i, nil
}
func RenderJsonResponse(c *gin.Context, data any, err error) {
	if err != nil {
		c.JSON(ErrorCode, CodeMsg{
			Code:    ErrorCode,
			Data:    nil,
			Message: err.Error(),
		})
	} else {
		c.JSON(SuccessCode, CodeMsg{
			Code:    SuccessCode,
			Data:    data,
			Message: SuccessMsg,
		})
	}
}
