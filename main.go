package crud2

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 标准响应结构
type StandardResponse struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

// 请求处理阶段类型
type ProcessStage int

const (
	StageParseRequest ProcessStage = iota + 1
	StageValidate
	StageDBOperation
	StageProcessResult
	StageBuildResponse
)

// 请求处理器
type RequestHandler struct {
	// 路由配置
	Method string   // HTTP方法
	Paths  []string // 注册路径

	// 处理流程组件
	ParseRequestFunc  func(c *gin.Context) (interface{}, error)
	ValidateFunc      func(input interface{}) error
	DBOperationFunc   func(ctx context.Context, input interface{}) (interface{}, error)
	ProcessResultFunc func(dbResult interface{}) (interface{}, error)
	BuildResponseFunc func(c *gin.Context, processedResult interface{}) *StandardResponse
	OnErrorFunc       func(c *gin.Context, stage ProcessStage, err error)

	// 数据库上下文
	DBCtx context.Context
}

// 默认组件实现
func DefaultParseRequest(c *gin.Context) (interface{}, error) {
	var input map[string]interface{}
	if err := c.ShouldBind(&input); err != nil {
		return nil, err
	}
	return input, nil
}

func DefaultValidate(input interface{}) error {
	// 示例：检查必要字段
	params, ok := input.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid parameters format")
	}
	if _, exists := params["id"]; !exists {
		return fmt.Errorf("missing required field: id")
	}
	return nil
}

func DefaultBuildResponse(c *gin.Context, result interface{}) *StandardResponse {
	return &StandardResponse{
		Code:    http.StatusOK,
		Data:    result,
		Message: "success",
	}
}

// 核心处理方法
func (h *RequestHandler) Handle(c *gin.Context) {
	var finalResult interface{}
	var err error

	// 1. 解析请求
	input, err := h.ParseRequestFunc(c)
	if err != nil {
		h.handleError(c, StageParseRequest, err)
		return
	}

	// 2. 参数验证
	if err = h.ValidateFunc(input); err != nil {
		h.handleError(c, StageValidate, err)
		return
	}

	// 3. 数据库操作
	dbResult, err := h.DBOperationFunc(h.DBCtx, input)
	if err != nil {
		h.handleError(c, StageDBOperation, err)
		return
	}

	// 4. 结果处理
	if h.ProcessResultFunc != nil {
		finalResult, err = h.ProcessResultFunc(dbResult)
		if err != nil {
			h.handleError(c, StageProcessResult, err)
			return
		}
	} else {
		finalResult = dbResult
	}

	// 5. 构建响应
	response := h.BuildResponseFunc(c, finalResult)
	c.JSON(response.Code, response)
}

// 错误处理
func (h *RequestHandler) handleError(c *gin.Context, stage ProcessStage, err error) {
	if h.OnErrorFunc != nil {
		h.OnErrorFunc(c, stage, err)
		return
	}

	// 默认错误处理
	code := http.StatusInternalServerError
	message := "server error"

	switch stage {
	case StageParseRequest:
		code = http.StatusBadRequest
		message = "invalid request"
	case StageValidate:
		code = http.StatusUnprocessableEntity
		message = "validation failed"
	case StageDBOperation:
		code = http.StatusServiceUnavailable
		message = "database error"
	}

	c.JSON(code, StandardResponse{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// 注册路由
func (h *RequestHandler) Register(r *gin.Engine) {
	for _, path := range h.Paths {
		r.Handle(h.Method, path, h.Handle)
	}
}

// 示例使用
func main() {
	r := gin.Default()

	// 创建用户查询处理器
	userHandler := &RequestHandler{
		Method: http.MethodGet,
		Paths:  []string{"/user/:id"},
		DBCtx:  context.Background(),

		ParseRequestFunc: func(c *gin.Context) (interface{}, error) {
			return map[string]interface{}{
				"id": c.Param("id"),
			}, nil
		},

		DBOperationFunc: func(ctx context.Context, input interface{}) (interface{}, error) {
			params := input.(map[string]interface{})
			// 执行数据库查询...
			return map[string]string{
				"id":   params["id"].(string),
				"name": "John Doe",
			}, nil
		},

		OnErrorFunc: func(c *gin.Context, stage ProcessStage, err error) {
			// 自定义错误处理
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
		},
	}

	// 注册默认组件
	userHandler.ValidateFunc = DefaultValidate
	userHandler.BuildResponseFunc = DefaultBuildResponse

	// 注册路由
	userHandler.Register(r)

	r.Run(":8080")
}
