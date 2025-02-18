package crudo

import "C"
import (
	"errors"
	"fmt"
	"net/http"
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
	PathSave   = "save"
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
}

func (c *Crud) InitDefaultHandler() {
	tableInfo, er := c.Db.GetTableInfo(c.Table)
	if er != nil {
		panic(er)
	}
	columnMap := TableInfoToColumnMap(tableInfo)
	c.HandlerMap = map[string]*RequestHandler{
		c.Prefix + "/" + PathSave: {
			Method:             "POST",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToMapAndTransfer(c.TransferMap, false),
			DataOperationFunc:  SaveFunc(c.Db, c.Table),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJson,
		},
		c.Prefix + "/" + PathDelete: {
			Method:             "GET",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, columnMap),
			DataOperationFunc:  QueryDeleteFunc(c.Db, c.Table),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJson,
		},
		c.Prefix + "/" + PathGet: {
			Method:             "GET",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, columnMap),
			DataOperationFunc:  QueryGetFunc(c.Db, c.FieldOfDetail, c.TransferMap),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJson,
		},
		c.Prefix + "/" + PathList: {
			Method:             "GET",
			PreHandler:         nil,
			ParseRequestFunc:   RequestToQueryParamsTransfer(c.Table, c.TransferMap, columnMap),
			DataOperationFunc:  QueryListFunc(c.Db, c.FieldOfDetail, c.TransferMap),
			TransferResultFunc: DoNothingTransferResultFunc,
			RenderResponseFunc: RenderJson,
		},
	}
}

func (c *Crud) AddHandler(path string, h *RequestHandler) {
	c.HandlerMap[path] = h
}

func (c *Crud) RemoveHandler(path string) {
	delete(c.HandlerMap, path)
}
func (c *Crud) GetHandler(path string) (*RequestHandler, bool) {
	h, ok := c.HandlerMap[path]
	return h, ok
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
	Values any           `json:"values"`
}

func NewCrudOfStruct(prefix string, db *gom.DB, i any) (*Crud, error) {
	tableStruct, er := db.GetTableStruct2(i)
	if er != nil {
		return nil, er
	}
	return NewCrud2(prefix, tableStruct.TableName, db, tableStruct.FieldToColMap, nil, nil)
}

func NewCrud(prefix string, table string, db *gom.DB) (*Crud, error) {
	return NewCrud2(prefix, table, db, nil, nil, nil)
}
func NewCrud2(prefix string, table string, db *gom.DB, transferMap map[string]string, queryListCols []string, queryDetailCols []string) (*Crud, error) {
	crud := &Crud{
		Prefix:        prefix,
		Db:            db,
		Table:         table,
		TransferMap:   transferMap,
		FieldOfList:   queryListCols,
		FieldOfDetail: queryDetailCols,
	}
	crud.InitDefaultHandler()
	return crud, nil
}

func RequestToMapAndTransfer(transferMap map[string]string, reverse bool) func(c *gin.Context) (any, error) {
	return func(c *gin.Context) (any, error) {
		inputData := make(map[string]any)
		if err := c.ShouldBindJSON(&inputData); err != nil {
			return nil, fmt.Errorf("failed to bind JSON: %v", err)
		}

		// If this is a save operation and we have an ID in the URL, add it to the input data
		if c.Request.Method == "POST" && c.Param("id") != "" {
			if id, err := strconv.ParseInt(c.Param("id"), 10, 64); err == nil {
				inputData["id"] = id
			}
		}

		// Convert between API and DB field names
		if len(transferMap) > 0 {
			outputData, err := TransferDataMap(inputData, transferMap, reverse)
			if err != nil {
				return nil, fmt.Errorf("failed to transfer data: %v", err)
			}
			return outputData, nil
		}

		return inputData, nil
	}
}

func TransferDataMap(inputData map[string]any, transferMap map[string]string, reverse bool) (map[string]any, error) {
	outputData := make(map[string]any)
	if len(transferMap) > 0 {
		// Handle special fields that should not be mapped
		if id, hasID := inputData["id"]; hasID {
			outputData["id"] = id
		}

		// Apply mapping
		for k, v := range transferMap {
			if reverse {
				// API field name <- DB field name
				if val, ok := inputData[v]; ok {
					outputData[k] = val
				}
			} else {
				// DB field name <- API field name
				if val, ok := inputData[k]; ok {
					outputData[v] = val
				}
			}
		}
	} else {
		// If no mapping is provided, copy all fields as is
		for k, v := range inputData {
			outputData[k] = v
		}
	}
	return outputData, nil
}

func TransferMapFunc(transferMap map[string]string) func(any) (any, error) {
	return func(input any) (any, error) {
		if inputData, ok := input.(map[string]any); ok {
			return TransferDataMap(inputData, transferMap, false)
		} else {
			return nil, errors.New("input is not map[string]any")
		}
	}
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
func TableInfoToColumnMap(tableInfo *define.TableInfo) map[string]define.ColumnInfo {
	columnMap := make(map[string]define.ColumnInfo)
	for _, column := range tableInfo.Columns {
		columnMap[column.Name] = column
	}
	return columnMap
}
func QueryValuesToValues(op define.OpType, values []string, column define.ColumnInfo) (any, error) {
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
	if len(values) == 1 {
		return anyValues[0], nil
	}
	return anyValues, nil
}
func QueryListFunc(db *gom.DB, queryCols []string, transferMap map[string]string) DataOperationFunc {
	return func(i any) (any, error) {
		if queryParams, ok := i.(QueryParams); ok {
			chain := db.Chain()
			chain.Table(queryParams.Table)
			chain.Table(queryParams.Table)
			if len(queryCols) > 0 {
				chain.Fields(queryCols...)
			}
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
			pageInfo, err := chain.PageInfo()
			if err != nil {
				return nil, err
			}
			if pageInfo.List != nil && len(transferMap) > 0 {
				dataListMap := make([]map[string]any, len(pageInfo.List.([]map[string]interface{})))
				for i, data := range pageInfo.List.([]map[string]any) {
					transferData, er := TransferDataMap(data, transferMap, true)
					if er != nil {
						return nil, er
					}
					dataListMap[i] = transferData
				}
				pageInfo.List = dataListMap
			}
			return pageInfo, nil
		}
		return nil, fmt.Errorf("invalid input data type")
	}
}
func QueryGetFunc(db *gom.DB, queryCols []string, transferMap map[string]string) DataOperationFunc {
	return func(i any) (any, error) {
		if queryParams, ok := i.(QueryParams); ok {
			chain := db.Chain()
			chain.Table(queryParams.Table)
			if len(queryCols) > 0 {
				chain.Fields(queryCols...)
			}
			for _, param := range queryParams.ConditionParams {
				chain.Where(param.Key, param.Op, param.Values)
			}
			result := chain.First()
			if result.Error != nil {
				return nil, result.Error
			}
			if len(result.Data) == 0 {
				return nil, fmt.Errorf("record not found")
			}
			// Transfer the result back to API field names
			return TransferDataMap(result.Data[0], transferMap, true)
		}
		return nil, fmt.Errorf("invalid input data type")
	}
}
func SaveFunc(db *gom.DB, table string) DataOperationFunc {
	return func(i any) (any, error) {
		tableInfo, er := db.GetTableInfo(table)
		if er != nil {
			return nil, er
		}
		if mapData, ok := i.(map[string]any); ok {
			hasPrimaryKey := false
			currentPrimaryKeys := make([]string, 0)
			for _, key := range tableInfo.PrimaryKeys {
				if _, ok := mapData[key]; ok {
					hasPrimaryKey = true
					currentPrimaryKeys = append(currentPrimaryKeys, key)
				}
			}
			chain := db.Chain()
			chain.Table(table)
			if hasPrimaryKey {
				for _, key := range currentPrimaryKeys {
					chain.Where(key, define.OpEq, mapData[key])
				}
			}
			result := chain.Values(mapData).Save()
			if result.Error != nil {
				return nil, result.Error
			}
			return result, nil
		}
		return nil, fmt.Errorf("invalid input data type")
	}
}
func QueryDeleteFunc(db *gom.DB, table string) DataOperationFunc {
	return func(i any) (any, error) {
		if queryParams, ok := i.(QueryParams); ok {
			chain := db.Chain()
			chain.Table(table)
			for _, param := range queryParams.ConditionParams {
				chain.Where(param.Key, param.Op, param.Values)
			}
			result := chain.Delete()
			if result.Error != nil {
				return nil, result.Error
			}
			return map[string]interface{}{
				"affected": result.RowsAffected,
			}, nil
		}
		return nil, fmt.Errorf("invalid input data type")
	}
}

func QueryParamsToCondition(queryParams QueryParams) *define.Condition {
	var condition *define.Condition
	for i, param := range queryParams.ConditionParams {
		if i == 0 {
			condition = define.Eq(param.Key, param.Values)
		} else {
			condition = condition.And(define.Eq(param.Key, param.Values))
		}
	}
	return condition
}
func DoNothingTransferResultFunc(i any) (any, error) {
	return i, nil
}
func RenderJson(c *gin.Context, data any, err error) {
	if err != nil {
		c.JSON(http.StatusOK, CodeMsg{
			Code:    ErrorCode,
			Data:    nil,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, CodeMsg{
		Code:    SuccessCode,
		Data:    data,
		Message: SuccessMsg,
	})
}
