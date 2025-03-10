package crudo

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type (
	CodeMsg struct {
		Code    int    `json:"code"`
		Data    any    `json:"data"`
		Message string `json:"msg"`
	}

	ParseRequestFunc     func(*fiber.Ctx) (any, error)
	DataOperationFunc    func(any) (any, error)
	TransferResultFunc   func(any) (any, error)
	RenderResponseFunc   = func(*fiber.Ctx, any, error) error
	ValidationMiddleware func(*fiber.Ctx) error
)

// RenderJson 渲染JSON响应
func RenderJson(c *fiber.Ctx, code int, msg string, data interface{}) error {
	return c.Status(http.StatusOK).JSON(CodeMsg{
		Code:    code,
		Message: msg,
		Data:    data,
	})
}

// RenderOk 渲染成功响应
func RenderOk(c *fiber.Ctx, data interface{}) error {
	return RenderJson(c, 200, "ok", data)
}

func JsonOk(c *fiber.Ctx, data interface{}) error {
	return RenderJson(c, 200, "ok", data)
}

func JsonErrs(c *fiber.Ctx, err error) error {
	return RenderErrs(c, err)
}

// RenderErrs 渲染错误响应
func RenderErrs(c *fiber.Ctx, err error) error {
	if err == nil {
		return RenderJson(c, 0, "ok", nil)
	}
	return RenderJson(c, 500, err.Error(), nil)
}

// RenderErr2 渲染错误响应
func RenderErr2(c *fiber.Ctx, code int, msg string) error {
	return RenderJson(c, code, msg, nil)
}

func Cors(allowList map[string]bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if origin := c.Get("Origin"); allowList[origin] {
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Access-Control-Allow-Headers", "Content-Type, AccessToken, X-CSRF-Token, Authorization, Token,token")
			c.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			c.Set("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
			c.Set("Access-Control-Allow-Credentials", "true")
		}

		// 允许放行OPTIONS请求
		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}
		return c.Next()
	}
}

type Server struct {
	app  *fiber.App
	addr string
}

func NewServer() *Server {
	app := fiber.New(fiber.Config{
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 30 * time.Second,
	})
	return NewServer2(app, ":80")
}

func NewServer2(app *fiber.App, addr string) *Server {
	return &Server{app: app, addr: addr}
}

func (s *Server) SetAddr(addr string) *Server {
	s.addr = addr
	return s
}

func (s *Server) SetReadTimeout(timeout time.Duration) *Server {
	config := s.app.Config()
	config.ReadTimeout = timeout
	s.app = fiber.New(config)
	return s
}

func (s *Server) SetWriteTimeout(timeout time.Duration) *Server {
	config := s.app.Config()
	config.WriteTimeout = timeout
	s.app = fiber.New(config)
	return s
}

func (s *Server) SetMaxHeaderBytes(max int) *Server {
	// Fiber doesn't have direct MaxHeaderBytes setting
	return s
}

func (s *Server) SetHttpServer(server *http.Server) *Server {
	// Not applicable for Fiber
	return s
}

func (s *Server) SetEngine(app *fiber.App) *Server {
	s.app = app
	return s
}

func (s Server) GetApp() *fiber.App {
	return s.app
}

func (s Server) ListenAndServe() error {
	return s.app.Listen(s.addr)
}

func GetMapFromRst(c *fiber.Ctx) (map[string]any, error) {
	var maps map[string]interface{}
	var er error

	if c.Method() == "POST" {
		contentType := c.Get("Content-Type")
		if strings.Contains(contentType, "application/x-www-form-urlencoded") {
			maps = make(map[string]interface{})
			form := c.Body()
			values, err := parseQuery(string(form))
			if err != nil {
				return nil, err
			}
			for k, v := range values {
				if len(v) == 1 {
					maps[k] = v[0]
				} else {
					maps[k] = v
				}
			}
		} else if strings.Contains(contentType, "application/json") {
			er = c.BodyParser(&maps)
		}
	} else if c.Method() == http.MethodGet {
		maps = make(map[string]interface{})
		c.QueryParser(&maps)
	}

	if er != nil {
		return nil, er
	}
	return maps, nil
}

func parseQuery(query string) (map[string][]string, error) {
	values := make(map[string][]string)
	for query != "" {
		key := query
		if i := strings.IndexAny(key, "&;"); i >= 0 {
			key, query = key[:i], key[i+1:]
		} else {
			query = ""
		}
		if key == "" {
			continue
		}
		value := ""
		if i := strings.Index(key, "="); i >= 0 {
			key, value = key[:i], key[i+1:]
		}
		key, err1 := queryUnescape(key)
		if err1 != nil {
			return nil, err1
		}
		value, err2 := queryUnescape(value)
		if err2 != nil {
			return nil, err2
		}
		values[key] = append(values[key], value)
	}
	return values, nil
}

func queryUnescape(s string) (string, error) {
	return s, nil // 简单实现，实际使用时可能需要更复杂的解码逻辑
}
