package crudo

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kmlixh/gom/v4"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestCrudManagerWithYAMLConfig(t *testing.T) {
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

	// Test endpoints
	t.Run("Test Table Info Endpoint", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users/table", nil)
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.NotEmpty(t, body)
	})

	t.Run("Test List Endpoint", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/users/list", nil)
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, fiber.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.NotEmpty(t, body)
	})

	// 清理测试表
	result = db.Chain().Raw("DROP TABLE IF EXISTS users").Exec()
	if result.Error != nil {
		t.Logf("Failed to drop test table: %v", result.Error)
	}
}
