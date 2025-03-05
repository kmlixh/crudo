package crudo

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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

// RenderJson 渲染JSON响应
func RenderJson(c *gin.Context, code int, msg string, data interface{}) {
	c.JSON(http.StatusOK, CodeMsg{
		Code:    code,
		Message: msg,
		Data:    data,
	})
	c.Abort()
}

// RenderOk 渲染成功响应
func RenderOk(c *gin.Context, data interface{}) {
	RenderJson(c, 200, "ok", data)
}
func JsonOk(c *gin.Context, data interface{}) {
	RenderJson(c, 200, "ok", data)
}
func JsonErrs(c *gin.Context, err error) {
	RenderErrs(c, err)
}

// RenderErrs 渲染错误响应
func RenderErrs(c *gin.Context, err error) {
	if err == nil {
		RenderJson(c, 0, "ok", nil)
		return
	}
	RenderJson(c, 500, err.Error(), nil)
}

// RenderErr2 渲染错误响应
func RenderErr2(c *gin.Context, code int, msg string) {
	RenderJson(c, code, msg, nil)
}

func Cors(allowList map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := c.Request.Header.Get("Origin"); allowList[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Headers", "Content-Type, AccessToken, X-CSRF-Token, Authorization, Token,token")
			c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		// 允许放行OPTIONS请求
		method := c.Request.Method
		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
		}
		c.Next()
	}
}

type Server struct {
	server *http.Server
	engine *gin.Engine
}

func NewServer() *Server {
	engine := gin.Default()
	return NewServer2(engine, ":80", 60*time.Second, 30*time.Second, 1<<20)
}

func NewServer2(engine *gin.Engine, addr string, readTimeout time.Duration, writeTimeout time.Duration, maxHeaderBytes int) *Server {
	return &Server{&http.Server{
		Addr:           addr,
		Handler:        engine,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: maxHeaderBytes,
	}, engine}
}
func (s *Server) SetAddr(addr string) *Server {
	s.server.Addr = addr
	return s

}
func (s *Server) SetReadTimeout(time time.Duration) *Server {
	s.server.ReadTimeout = time
	return s

}
func (s *Server) SetWriteTimeout(time time.Duration) *Server {
	s.server.WriteTimeout = time
	return s

}
func (s *Server) SetMaxHeaderBytes(max int) *Server {
	s.server.MaxHeaderBytes = max
	return s
}
func (s *Server) SetHttpServer(server *http.Server) *Server {
	s.server = server
	s.server.Handler = s.engine
	return s

}
func (s *Server) SetEngine(engine *gin.Engine) *Server {
	s.engine = engine
	s.server.Handler = s.engine
	return s
}

func (s Server) GetEngine() *gin.Engine {
	return s.engine
}
func (s Server) GetServer() *http.Server {
	return s.server
}
func (s Server) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func GetMapFromRst(c *gin.Context) (map[string]any, error) {
	var maps map[string]interface{}
	var er error
	if c.Request.Method == "POST" {
		contentType := c.GetHeader("Content-Type")
		if strings.Contains(contentType, "application/x-www-form-urlencoded") {
			maps = make(map[string]interface{})
			er = c.Request.ParseForm()
			if er != nil {
				return nil, er
			}
			values := c.Request.Form
			for k, v := range values {
				if len(v) == 1 {
					maps[k] = v[0]
				} else {
					maps[k] = v
				}
			}

		} else if strings.Contains(contentType, "application/json") {
			bbs, er := io.ReadAll(c.Request.Body)
			if er != nil {
				return nil, er
			}
			er = json.Unmarshal(bbs, &maps)
		}
	} else if c.Request.Method == http.MethodGet {
		maps = make(map[string]interface{})
		values := c.Request.URL.Query()
		for k, v := range values {
			if len(v) == 1 {
				maps[k] = v[0]
			} else {
				maps[k] = v
			}
		}
	}
	if er != nil {
		return nil, er
	}
	return maps, nil

}
