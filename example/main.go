package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/kmlixh/crudo"
	"gopkg.in/yaml.v3"
)

func main() {
	// 加载 YAML 配置
	configPath := "config.yml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	yamlData, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("无法读取配置文件 %s: %v", configPath, err)
	}

	// 解析 YAML 到 ServiceConfig
	var config crudo.ServiceConfig
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		log.Fatalf("解析 YAML 失败: %v", err)
	}

	// 初始化 CrudManager
	manager, err := crudo.NewCrudManager(&config)
	if err != nil {
		log.Fatalf("初始化 CrudManager 失败: %v", err)
	}

	// 创建 Fiber 应用
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"code":    code,
				"message": err.Error(),
				"data":    nil,
			})
		},
	})

	// 添加中间件
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New())

	// 注册路由
	api := app.Group("/api")
	manager.RegisterRoutes(api)

	// 添加健康检查端点
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   fmt.Sprintf("%v", time.Now()),
		})
	})

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("服务器启动在端口 %s", port)
	log.Fatal(app.Listen(":" + port))
}
