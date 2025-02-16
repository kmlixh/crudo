package crudo

import (
	"github.com/gin-gonic/gin"
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
type BuildResponseFunc func(*gin.Context, any, error) *CodeMsg

type RequestHandler struct {
	Method     string
	PreHandler gin.HandlerFunc
	ParseRequestFunc
	DataOperationFunc
	TransferResultFunc
	BuildResponseFunc
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

	response := h.BuildResponseFunc(c, result, err)
	c.JSON(response.Code, response)
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
