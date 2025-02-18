package crudo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmlixh/gom/v4"
	"github.com/kmlixh/gom/v4/define"
	_ "github.com/kmlixh/gom/v4/factory/postgres"
	"github.com/stretchr/testify/assert"
)

type TestUser struct {
	ID       int64  `json:"id"`
	Name     string `json:"user_name"`
	Age      int    `json:"user_age"`
	Email    string `json:"user_email"`
	IsActive bool   `json:"user_active"`
}

func setupTestDB(t *testing.T) *gom.DB {
	// 配置测试数据库连接，更新密码为yzy123
	db, err := gom.Open("postgres", "postgres://postgres:123456@10.0.1.5:5432/crud_test", &define.DBOptions{
		Debug: true,
	})
	assert.NoError(t, err)

	// 创建测试表 - 使用PostgreSQL语法
	result := db.Chain().Raw(`
		CREATE TABLE  IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			user_name VARCHAR(100) NOT NULL,
			user_age INTEGER,
			user_email VARCHAR(100),
			user_active BOOLEAN DEFAULT true
		)
	`).Exec()
	assert.NoError(t, result.Error)

	return db
}

func TestNewCrud2(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 设置转换映射 - 从API字段映射到数据库字段
	transferMap := map[string]string{
		"name":      "user_name", // API字段 -> 数据库字段
		"age":       "user_age",
		"email":     "user_email",
		"is_active": "user_active",
	}

	// 设置查询列 - 使用数据库字段名
	queryListCols := []string{"id", "user_name", "user_age", "user_email", "user_active"}
	queryDetailCols := []string{"id", "user_name", "user_age", "user_email", "user_active"}

	crud, err := NewCrud2("/users", "users", db, transferMap, queryListCols, queryDetailCols)
	assert.NoError(t, err)
	assert.NotNil(t, crud)

	// 设置 Gin 路由
	gin.SetMode(gin.TestMode)
	router := gin.New()
	apiGroup := router.Group("/api")
	crud.RegisterRoutes(apiGroup)

	t.Run("Test Insert", func(t *testing.T) {
		userData := map[string]interface{}{
			"name":      "John Doe",
			"age":       30,
			"email":     "john@example.com",
			"is_active": true,
		}
		jsonData, _ := json.Marshal(userData)

		req := httptest.NewRequest("POST", "/api/users/save", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response CodeMsg
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, SuccessCode, response.Code)

		// 验证插入的数据
		result := db.Chain().Table("users").Where("user_name", define.OpEq, "John Doe").First()
		assert.NoError(t, result.Error)
		assert.NotNil(t, result.Data)
		if len(result.Data) > 0 {
			assert.Equal(t, "John Doe", result.Data[0]["user_name"])
			assert.Equal(t, int64(30), result.Data[0]["user_age"])
			assert.Equal(t, "john@example.com", result.Data[0]["user_email"])
			assert.Equal(t, true, result.Data[0]["user_active"])
		}
	})

	t.Run("Test List with Data", func(t *testing.T) {
		// 先插入测试数据
		testData := []map[string]interface{}{
			{
				"name":      "Alice Smith",
				"age":       25,
				"email":     "alice@example.com",
				"is_active": true,
			},
			{
				"name":      "Bob Johnson",
				"age":       35,
				"email":     "bob@example.com",
				"is_active": true,
			},
		}

		for _, data := range testData {
			jsonData, _ := json.Marshal(data)
			req := httptest.NewRequest("POST", "/api/users/save", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		}

		// 测试列表查询
		req := httptest.NewRequest("GET", "/api/users/list?page=1&pageSize=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response CodeMsg
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, SuccessCode, response.Code)

		// 验证返回的数据列表
		resultData, ok := response.Data.(map[string]interface{})
		assert.True(t, ok)
		dataList, ok := resultData["data"].([]interface{})
		assert.True(t, ok)
		assert.GreaterOrEqual(t, len(dataList), 2)
	})

	t.Run("Test Update with Verification", func(t *testing.T) {
		// 先插入一条数据
		insertData := map[string]interface{}{
			"name":      "Update Test",
			"age":       40,
			"email":     "update@example.com",
			"is_active": true,
		}
		jsonData, _ := json.Marshal(insertData)
		req := httptest.NewRequest("POST", "/api/users/save", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 获取插入的记录ID
		var insertResponse CodeMsg
		err := json.Unmarshal(w.Body.Bytes(), &insertResponse)
		assert.NoError(t, err)
		insertResult := insertResponse.Data.(map[string]interface{})
		insertedID := insertResult["id"]

		// Debugging: Log the insertedID and result before verification
		fmt.Printf("Inserted ID: %v\n", insertedID)

		// 更新数据时需要包含ID
		updateData := map[string]interface{}{
			"id":        insertedID,
			"name":      "Updated Name",
			"age":       41,
			"email":     "updated@example.com",
			"is_active": false,
		}
		jsonData, _ = json.Marshal(updateData)
		req = httptest.NewRequest("POST", "/api/users/save", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证更新后的数据
		result := db.Chain().Table("users").Where("id", define.OpEq, insertedID).First()
		assert.NoError(t, result.Error)
		assert.NotEmpty(t, result.Data)
		assert.Equal(t, "Updated Name", result.Data[0]["user_name"])
		assert.Equal(t, "updated@example.com", result.Data[0]["user_email"])
		assert.Equal(t, int64(41), result.Data[0]["user_age"])
	})

	t.Run("Test Get", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users/get?id_eq=1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response CodeMsg
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, SuccessCode, response.Code)
	})

	t.Run("Test Delete", func(t *testing.T) {
		// 先插入一条测试数据
		insertData := map[string]interface{}{
			"user_name":   "Delete Test",
			"user_age":    50,
			"user_email":  "delete@example.com",
			"user_active": true,
		}
		jsonData, _ := json.Marshal(insertData)
		req := httptest.NewRequest("POST", "/api/users/save", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 获取插入的记录ID
		var insertResponse CodeMsg
		err := json.Unmarshal(w.Body.Bytes(), &insertResponse)
		assert.NoError(t, err)
		insertResult := insertResponse.Data.(map[string]interface{})
		insertedID := insertResult["id"]

		// 执行删除
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/users/delete?id_eq=%v", insertedID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证删除是否成功
		result := db.Chain().Table("users").Where("id", define.OpEq, insertedID).First()
		assert.NoError(t, result.Error)
		assert.Empty(t, result.Data)
	})

	t.Run("Test Complex Query", func(t *testing.T) {
		// 使用API字段名进行查询
		url := "/api/users/list?name_like=John&age_gt=25&is_active_eq=true&orderBy=name&orderByDesc=age&page=1&pageSize=10"
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response CodeMsg
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, SuccessCode, response.Code)
	})

	t.Run("Test Field Mapping", func(t *testing.T) {
		// 插入数据
		userData := map[string]interface{}{
			"user_name":   "Test Mapping",
			"user_age":    28,
			"user_email":  "mapping@example.com",
			"user_active": true,
		}
		jsonData, _ := json.Marshal(userData)

		req := httptest.NewRequest("POST", "/api/users/save", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 获取数据并验证字段映射
		var response CodeMsg
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		insertResult := response.Data.(map[string]interface{})
		insertedID := insertResult["id"]

		// 使用 GET 接口获取数据
		req = httptest.NewRequest("GET", fmt.Sprintf("/api/users/get?id_eq=%v", insertedID), nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var getResponse CodeMsg
		err = json.Unmarshal(w.Body.Bytes(), &getResponse)
		assert.NoError(t, err)
		getData := getResponse.Data.(map[string]interface{})

		// 验证字段映射 - 应该使用API字段名
		assert.Equal(t, "Test Mapping", getData["name"]) // 使用API字段名
		assert.Equal(t, 28.0, getData["age"])            // 使用API字段名
		assert.Equal(t, "mapping@example.com", getData["email"])
		assert.Equal(t, true, getData["is_active"])
	})
}

func TestTableInfoToColumnMap(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tableInfo, err := db.GetTableInfo("users")
	assert.NoError(t, err)

	columnMap := TableInfoToColumnMap(tableInfo)
	assert.NotNil(t, columnMap)

	// 修改验证列信息为实际的字段名
	expectedColumns := []string{"id", "user_name", "user_age", "user_email", "user_active"}
	for _, colName := range expectedColumns {
		_, exists := columnMap[colName]
		assert.True(t, exists, fmt.Sprintf("Column %s should exist in columnMap", colName))
	}
}
