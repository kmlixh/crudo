package crudo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kmlixh/gom/v4"
	_ "github.com/kmlixh/gom/v4/factory/postgres"
	"github.com/stretchr/testify/assert"
)

// 基础测试数据
var baseData = map[string]interface{}{
	"apiField1": "testValue",
	"apiField2": 100,
}

func setupRouter() (*fiber.App, *Crud) {
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
				Name:       "test_data",
				Database:   "test_db",
				PathPrefix: "/data",
				TransferMap: map[string]string{
					"apiField1": "field1",
					"apiField2": "field2",
				},
			},
		},
	}

	// Initialize CrudManager
	manager, err := NewCrudManager(config)
	if err != nil {
		panic(fmt.Errorf("failed to create CrudManager: %v", err))
	}

	// Initialize the manager
	if err := manager.init(); err != nil {
		panic(fmt.Errorf("failed to initialize manager: %v", err))
	}

	// Create test table
	db := manager.dbs["test_db"]

	// 清理和创建测试表
	cleanupTestTable(db)
	createTestTable(db)

	// Create Fiber app and register routes
	app := fiber.New()
	manager.RegisterRoutes(app)

	// Get the crud instance
	crud, ok := manager.routes["/data"]
	if !ok {
		panic("Failed to get CRUD instance for test_data")
	}

	crudInstance, ok := crud.(*Crud)
	if !ok {
		panic("Failed to convert to Crud type")
	}

	return app, crudInstance
}

// 辅助函数：清理测试表
func cleanupTestTable(db *gom.DB) {
	result := db.Chain().Raw("DROP TABLE IF EXISTS test_data").Exec()
	if result.Error != nil {
		panic(fmt.Errorf("failed to drop table: %v", result.Error))
	}
}

// 辅助函数：创建测试表
func createTestTable(db *gom.DB) {
	result := db.Chain().Raw(`
		CREATE TABLE IF NOT EXISTS test_data (
			id SERIAL PRIMARY KEY,
			field1 TEXT,
			field2 INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`).Exec()
	if result.Error != nil {
		panic(fmt.Errorf("failed to create table: %v", result.Error))
	}
}

// TestMain 用于设置和清理测试环境
func TestMain(m *testing.M) {
	// 运行测试并直接退出
	os.Exit(m.Run())
}

func TestCRUDIntegration(t *testing.T) {
	app, crud := setupRouter()
	defer crud.Db.Close()

	t.Run("CreateAndRetrieve", func(t *testing.T) {
		// Create record
		createBody, _ := json.Marshal(baseData)
		req := httptest.NewRequest("POST", "/data/save", bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, _ := io.ReadAll(resp.Body)
		var createRes CodeMsg
		err = json.Unmarshal(body, &createRes)
		assert.NoError(t, err)
		assert.Equal(t, SuccessCode, createRes.Code)
		assert.NotNil(t, createRes.Data, "Response data should not be nil")

		responseData, ok := createRes.Data.(map[string]interface{})
		assert.True(t, ok, "Response data should be a map")
		assert.NotNil(t, responseData["id"], "Response should contain an ID")

		createdID := int(responseData["id"].(float64))
		assert.Greater(t, createdID, 0, "Created ID should be greater than 0")

		// 查询记录
		req = httptest.NewRequest("GET", "/data/get?id="+strconv.Itoa(createdID), nil)
		resp, err = app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, _ = io.ReadAll(resp.Body)
		var getRes CodeMsg
		err = json.Unmarshal(body, &getRes)
		assert.NoError(t, err)
		assert.Equal(t, SuccessCode, getRes.Code)
		assert.NotNil(t, getRes.Data, "Response data should not be nil")

		getData, ok := getRes.Data.(map[string]interface{})
		assert.True(t, ok, "Response data should be a map")
		assert.Equal(t, float64(createdID), getData["id"], "Retrieved ID should match created ID")
		assert.Equal(t, baseData["apiField1"], getData["apiField1"], "Retrieved apiField1 should match created value")
		assert.Equal(t, float64(baseData["apiField2"].(int)), getData["apiField2"], "Retrieved apiField2 should match created value")
	})

	t.Run("UpdateRecord", func(t *testing.T) {
		// 先创建记录
		createBody, _ := json.Marshal(baseData)
		req := httptest.NewRequest("POST", "/data/save", bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		assert.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		var createRes CodeMsg
		json.Unmarshal(body, &createRes)

		// 添加安全断言
		if createRes.Data == nil {
			t.Fatal("Create response data is empty")
		}
		dataMap, ok := createRes.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Invalid create response format")
		}
		createdID := int(dataMap["id"].(float64))

		// 更新记录
		updateData := map[string]interface{}{
			"id":        createdID,
			"apiField1": "updatedValue",
		}
		updateBody, _ := json.Marshal(updateData)
		req = httptest.NewRequest("POST", "/data/save", bytes.NewReader(updateBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err = app.Test(req)
		assert.NoError(t, err)

		// 验证更新
		req = httptest.NewRequest("GET", "/data/get?id="+strconv.Itoa(createdID), nil)
		resp, err = app.Test(req)
		assert.NoError(t, err)

		body, _ = io.ReadAll(resp.Body)
		var getRes CodeMsg
		json.Unmarshal(body, &getRes)
		assert.Equal(t, "updatedValue", getRes.Data.(map[string]interface{})["apiField1"])
	})

	t.Run("PaginationTest", func(t *testing.T) {
		// 插入25条测试数据
		for i := 0; i < 25; i++ {
			data := map[string]interface{}{
				"apiField1": "item" + strconv.Itoa(i),
				"apiField2": i,
			}
			body, _ := json.Marshal(data)
			req := httptest.NewRequest("POST", "/data/save", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			app.Test(req)
		}

		// 测试分页
		req := httptest.NewRequest("GET", "/data/list?page=2&pageSize=10", nil)
		resp, err := app.Test(req)
		assert.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		var listRes CodeMsg
		json.Unmarshal(body, &listRes)
		pageInfo := listRes.Data.(map[string]interface{})

		assert.Equal(t, 2, int(pageInfo["Page"].(float64)))
		assert.Equal(t, 10, int(pageInfo["PageSize"].(float64)))
		assert.Equal(t, 27, int(pageInfo["Total"].(float64))) // 25 + 之前测试的2条
		assert.Len(t, pageInfo["List"], 10)
	})

	t.Run("DeleteRecord", func(t *testing.T) {
		// 创建测试记录
		createBody, _ := json.Marshal(baseData)
		req := httptest.NewRequest("POST", "/data/save", bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		assert.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		var createRes CodeMsg
		json.Unmarshal(body, &createRes)
		createdID := int(createRes.Data.(map[string]interface{})["id"].(float64))

		// 删除记录
		req = httptest.NewRequest("DELETE", "/data/delete?id="+strconv.Itoa(createdID), nil)
		resp, err = app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 验证删除
		req = httptest.NewRequest("GET", "/data/get?id="+strconv.Itoa(createdID), nil)
		resp, err = app.Test(req)
		assert.NoError(t, err)

		body, _ = io.ReadAll(resp.Body)
		var getRes CodeMsg
		json.Unmarshal(body, &getRes)
		assert.Equal(t, ErrorCode, getRes.Code)
	})
}

func TestFieldMapping(t *testing.T) {
	app, crud := setupRouter()
	defer crud.Db.Close()

	// 测试字段映射
	testData := map[string]interface{}{
		"apiField1": "mappingTest",
		"apiField2": 200,
	}

	// 创建记录
	createBody, _ := json.Marshal(testData)
	req := httptest.NewRequest("POST", "/data/save", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	assert.NoError(t, err)

	// 验证响应
	body, _ := io.ReadAll(resp.Body)
	var createRes CodeMsg
	err = json.Unmarshal(body, &createRes)
	assert.NoError(t, err)
	assert.NotNil(t, createRes.Data)

	responseData, ok := createRes.Data.(map[string]interface{})
	assert.True(t, ok, "Response data should be a map")
	assert.NotNil(t, responseData["id"], "Response should contain an ID")

	createdID := int(responseData["id"].(float64))

	// 查询记录
	req = httptest.NewRequest("GET", "/data/get?id="+strconv.Itoa(createdID), nil)
	resp, err = app.Test(req)
	assert.NoError(t, err)

	body, _ = io.ReadAll(resp.Body)
	var getRes CodeMsg
	json.Unmarshal(body, &getRes)
	getData := getRes.Data.(map[string]interface{})

	assert.Equal(t, "mappingTest", getData["apiField1"])
	assert.Equal(t, float64(200), getData["apiField2"])
}
