package crudo

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kmlixh/gom/v4"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestRunServer 测试服务器启动和基本功能
func TestRunServer(t *testing.T) {
	// Load YAML config
	yamlData, err := os.ReadFile("test_config.yml")
	if err != nil {
		t.Fatalf("Failed to read YAML config: %v", err)
	}

	// Parse YAML into ServiceConfig
	var config ServiceConfig
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	// 首先创建一个直接的数据库连接来创建表
	dbConfig := config.Databases[0]
	dsn := ""
	if dbConfig.Driver == "postgres" {
		dsn = "host=" + dbConfig.Host + " port=" + "5432" + " user=" + dbConfig.User + " password=" + dbConfig.Password + " dbname=" + dbConfig.Database + " sslmode=disable"
	}

	db, err := gom.Open(dbConfig.Driver, dsn, nil)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 创建测试表
	result := db.Chain().Raw(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(100) NOT NULL,
			email VARCHAR(100) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`).Exec()
	if result.Error != nil {
		t.Fatalf("Failed to create test table: %v", result.Error)
	}

	// 确保表中有一些数据
	result = db.Chain().Raw(`
		INSERT INTO users (username, email) 
		VALUES ('testuser', 'test@example.com')
	`).Exec()
	if result.Error != nil {
		t.Fatalf("Failed to insert test data: %v", result.Error)
	}

	// 等待一下确保表结构已经被数据库完全处理
	time.Sleep(100 * time.Millisecond)

	// Initialize CrudManager
	manager, err := NewCrudManager(&config)
	if err != nil {
		t.Fatalf("Failed to initialize CrudManager: %v", err)
	}

	// Create Fiber app
	app := fiber.New()

	// Register routes
	api := app.Group("/api")
	manager.RegisterRoutes(api)

	// Add health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   fmt.Sprintf("%v", time.Now()),
		})
	})

	// 启动服务器，但不阻塞测试
	go func() {
		port := "8081" // 使用不同的端口避免冲突
		log.Printf("Server starting on port %s", port)
		if err := app.Listen(":" + port); err != nil {
			if err != http.ErrServerClosed {
				log.Fatalf("Server error: %v", err)
			}
		}
	}()

	// 等待服务器启动
	time.Sleep(500 * time.Millisecond)

	// 测试健康检查端点
	resp, err := http.Get("http://localhost:8081/health")
	if err != nil {
		t.Fatalf("Failed to call health endpoint: %v", err)
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "status")
	assert.Contains(t, string(body), "ok")

	// 测试API端点
	resp, err = http.Get("http://localhost:8081/api/users/list")
	if err != nil {
		t.Fatalf("Failed to call list endpoint: %v", err)
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), "List")
	assert.Contains(t, string(body), "testuser")

	// 关闭服务器
	app.Shutdown()

	// 清理测试表
	result = db.Chain().Raw("DROP TABLE IF EXISTS users").Exec()
	if result.Error != nil {
		t.Logf("Failed to drop test table: %v", result.Error)
	}
}
