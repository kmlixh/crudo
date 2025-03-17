package crudo

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kmlixh/gom/v4"
	"github.com/kmlixh/gom/v4/define"
	"github.com/stretchr/testify/assert"
)

// 使用与 crud_test.go 相同的测试环境配置
var (
	testDBHost     = getEnvOrDefault("TEST_DB_HOST", "192.168.111.20")
	testDBPort     = getEnvOrDefault("TEST_DB_PORT", "5432")
	testDBUser     = getEnvOrDefault("TEST_DB_USER", "postgres")
	testDBPassword = getEnvOrDefault("TEST_DB_PASSWORD", "yzy123")
	testDBName     = getEnvOrDefault("TEST_DB_NAME", "crud_test")
)

func TestCrudManagerRouting(t *testing.T) {
	// 创建测试配置
	config := &ServiceConfig{
		Databases: []DatabaseConfig{
			{
				Name:     "test_db",
				Driver:   "postgres",
				Host:     testDBHost,
				Port:     mustParseInt(testDBPort),
				User:     testDBUser,
				Password: testDBPassword,
				Database: testDBName,
				Options: &DBOptions{
					Debug: true,
				},
			},
		},
		Tables: []TableConfig{
			{
				Name:       "products",
				Database:   "test_db",
				PathPrefix: "/ecommerce/products",
				TransferMap: map[string]string{
					"name": "product_name",
					"desc": "description",
				},
				HandlerFilters: []string{"save", "list"},
			},
			{
				Name:       "categories",
				Database:   "test_db",
				PathPrefix: "/ecommerce/categories",
				TransferMap: map[string]string{
					"name": "category_name",
					"desc": "description",
				},
				HandlerFilters: []string{"list", "get"},
			},
		},
	}

	// 创建测试数据库连接
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.Databases[0].Host,
		config.Databases[0].Port,
		config.Databases[0].User,
		config.Databases[0].Password,
		config.Databases[0].Database,
	)
	db, err := gom.Open("postgres", dsn, &define.DBOptions{Debug: true})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 创建测试表
	err = setupTestTables(db)
	if err != nil {
		t.Fatalf("Failed to setup test tables: %v", err)
	}

	// 初始化 CrudManager
	manager, err := NewCrudManager(config)
	if err != nil {
		t.Fatalf("Failed to initialize CrudManager: %v", err)
	}

	// 初始化 manager
	if err := manager.init(); err != nil {
		t.Fatalf("Failed to initialize CrudManager: %v", err)
	}

	// 创建 Fiber app 用于测试
	app := fiber.New()
	api := app.Group("/api")
	manager.RegisterRoutes(api)

	// 测试用例
	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		expectedStatus int
	}{
		{
			name:           "Products Save - Valid",
			method:         "POST",
			path:           "/api/ecommerce/products/save",
			body:           `{"name":"Test Product","desc":"Test Description","price":99.99}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Products List - Valid",
			method:         "GET",
			path:           "/api/ecommerce/products/list",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Products Get - Not Configured",
			method:         "GET",
			path:           "/api/ecommerce/products/get",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Categories List - Valid",
			method:         "GET",
			path:           "/api/ecommerce/categories/list",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Categories Get - Valid",
			method:         "GET",
			path:           "/api/ecommerce/categories/get",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Categories Save - Not Configured",
			method:         "POST",
			path:           "/api/ecommerce/categories/save",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Invalid Path",
			method:         "GET",
			path:           "/api/invalid/path",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Wrong Method",
			method:         "POST",
			path:           "/api/ecommerce/products/list",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
				req = httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			}

			// 设置测试超时时间，特别是在调试模式下
			resp, err := app.Test(req, -1) // -1 表示不设置超时限制
			assert.NoError(t, err, "Failed to test request")
			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "Unexpected status code")
		})
	}

	//// 清理测试表
	//err = cleanupTestTables(db)
	//if err != nil {
	//	t.Errorf("Failed to clean up test tables: %v", err)
	//}
}

// 辅助函数：设置测试表
func setupTestTables(db *gom.DB) error {
	createProductsTable := `
		CREATE TABLE IF NOT EXISTS products (
			id SERIAL PRIMARY KEY,
			product_name VARCHAR(100) NOT NULL,
			description TEXT,
			price DECIMAL(10,2) NOT NULL DEFAULT 0.00,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`

	createCategoriesTable := `
		CREATE TABLE IF NOT EXISTS categories (
			id SERIAL PRIMARY KEY,
			category_name VARCHAR(100) NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`

	if err := db.Chain().Raw(createProductsTable).Exec().Error; err != nil {
		return fmt.Errorf("failed to create products table: %v", err)
	}
	if err := db.Chain().Raw(createCategoriesTable).Exec().Error; err != nil {
		return fmt.Errorf("failed to create categories table: %v", err)
	}
	return nil
}

// 辅助函数：清理测试表
func cleanupTestTables(db *gom.DB) error {
	if err := db.Chain().Raw("DROP TABLE IF EXISTS products, categories").Exec().Error; err != nil {
		return fmt.Errorf("failed to drop test tables: %v", err)
	}
	return nil
}

// 辅助函数：获取环境变量
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// 辅助函数：解析整数
func mustParseInt(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
