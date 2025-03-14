package crudo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/kmlixh/gom/v4"
	"github.com/kmlixh/gom/v4/define"
	_ "github.com/kmlixh/gom/v4/factory/postgres"
	"github.com/stretchr/testify/assert"
)

func setupRouter() (*fiber.App, *Crud) {
	// 初始化内存数据库
	db, er := gom.Open("postgres", "host=10.0.1.5 user=postgres password=123456 dbname=crud_test port=5432 sslmode=disable", &define.DBOptions{Debug: true})
	if er != nil {
		panic(er)
	}

	// 清理表
	result := db.Chain().Raw("DROP TABLE IF EXISTS test_data").Exec()
	if result.Error != nil {
		panic(fmt.Errorf("failed to drop table: %v", result.Error))
	}

	// 创建表时使用PostgreSQL语法
	result = db.Chain().Raw(`
		CREATE TABLE IF NOT EXISTS test_data (
			id BIGSERIAL PRIMARY KEY,
			field1 TEXT,
			field2 INTEGER
		)`).Exec()
	if result.Error != nil {
		panic(fmt.Errorf("failed to create table: %v", result.Error))
	}

	// 创建CRUD实例
	crud, _ := NewCrud(
		"/data",
		"test_data",
		db,
		map[string]string{
			"apiField1": "field1",
			"apiField2": "field2",
		},
		nil,
		nil,
		nil,
	)

	// 初始化Fiber路由
	app := fiber.New()
	crud.RegisterRoutes(app.Group("/api"))

	return app, crud
}

func TestCRUDIntegration(t *testing.T) {
	app, crud := setupRouter()
	defer crud.Db.Close()

	// 测试数据模板
	baseData := map[string]interface{}{
		"apiField1": "testValue",
		"apiField2": 100,
	}

	t.Run("CreateAndRetrieve", func(t *testing.T) {
		// Create record
		createData := map[string]interface{}{
			"apiField1": "testValue",
			"apiField2": 100,
		}

		createBody, _ := json.Marshal(createData)
		req := httptest.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, _ := io.ReadAll(resp.Body)
		var createRes CodeMsg
		err = json.Unmarshal(body, &createRes)
		assert.NoError(t, err)
		assert.NotNil(t, createRes.Data, "Response data should not be nil")

		responseData, ok := createRes.Data.(map[string]interface{})
		assert.True(t, ok, "Response data should be a map")
		assert.NotNil(t, responseData["id"], "Response should contain an ID")

		createdID := int(responseData["id"].(float64))

		// 查询记录
		req = httptest.NewRequest("GET", "/api/data/get?id="+strconv.Itoa(createdID), nil)
		resp, err = app.Test(req)
		assert.NoError(t, err)

		body, _ = io.ReadAll(resp.Body)
		var getRes CodeMsg
		json.Unmarshal(body, &getRes)
		assert.Equal(t, "testValue", getRes.Data.(map[string]interface{})["apiField1"])
	})

	t.Run("UpdateRecord", func(t *testing.T) {
		// 先创建记录
		createBody, _ := json.Marshal(baseData)
		req := httptest.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
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
		req = httptest.NewRequest("POST", "/api/data/save", bytes.NewReader(updateBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err = app.Test(req)
		assert.NoError(t, err)

		// 验证更新
		req = httptest.NewRequest("GET", "/api/data/get?id="+strconv.Itoa(createdID), nil)
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
			req := httptest.NewRequest("POST", "/api/data/save", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			app.Test(req)
		}

		// 测试分页
		req := httptest.NewRequest("GET", "/api/data/list?page=2&pageSize=10", nil)
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
		req := httptest.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		assert.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		var createRes CodeMsg
		json.Unmarshal(body, &createRes)
		createdID := int(createRes.Data.(map[string]interface{})["id"].(float64))

		// 删除记录
		req = httptest.NewRequest("DELETE", "/api/data/delete?id="+strconv.Itoa(createdID), nil)
		resp, err = app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 验证删除
		req = httptest.NewRequest("GET", "/api/data/get?id="+strconv.Itoa(createdID), nil)
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
	req := httptest.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
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
	req = httptest.NewRequest("GET", "/api/data/get?id="+strconv.Itoa(createdID), nil)
	resp, err = app.Test(req)
	assert.NoError(t, err)

	body, _ = io.ReadAll(resp.Body)
	var getRes CodeMsg
	json.Unmarshal(body, &getRes)
	getData := getRes.Data.(map[string]interface{})

	assert.Equal(t, "mappingTest", getData["apiField1"])
	assert.Equal(t, float64(200), getData["apiField2"])
}
