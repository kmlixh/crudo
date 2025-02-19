package crudo

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kmlixh/gom/v4"
	"github.com/stretchr/testify/assert"
)

func setupRouter() (*gin.Engine, *Crud) {
	// 初始化内存数据库
	db, _ := gom.Open("sqlite3", ":memory:", nil)
	db.Chain().Raw(`
		CREATE TABLE test_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			field1 TEXT,
			field2 INTEGER
		)`).Exec()

	// 创建CRUD实例
	crud, _ := NewCrud(
		"/data",
		"test_data",
		db,
		map[string]string{
			"apiField1": "field1",
			"apiField2": "field2",
		},
	)

	// 初始化Gin路由
	router := gin.Default()
	crud.RegisterRoutes(router.Group("/api"))

	return router, crud
}

func TestCRUDIntegration(t *testing.T) {
	router, crud := setupRouter()
	defer crud.Db.Close()

	// 测试数据模板
	baseData := map[string]interface{}{
		"apiField1": "testValue",
		"apiField2": 100,
	}

	t.Run("CreateAndRetrieve", func(t *testing.T) {
		// 创建记录
		createBody, _ := json.Marshal(baseData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var createRes CodeMsg
		json.Unmarshal(w.Body.Bytes(), &createRes)
		createdID := int(createRes.Data.(map[string]interface{})["id"].(float64))

		// 查询记录
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/data/get?id="+strconv.Itoa(createdID), nil)
		router.ServeHTTP(w, req)

		var getRes CodeMsg
		json.Unmarshal(w.Body.Bytes(), &getRes)
		assert.Equal(t, "testValue", getRes.Data.(map[string]interface{})["apiField1"])
	})

	t.Run("UpdateRecord", func(t *testing.T) {
		// 先创建记录
		createBody, _ := json.Marshal(baseData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
		router.ServeHTTP(w, req)
		createdID := int(w.Result().Body.(*bytes.Buffer).Bytes()["id"].(float64))

		// 更新记录
		updateData := map[string]interface{}{
			"id":        createdID,
			"apiField1": "updatedValue",
		}
		updateBody, _ := json.Marshal(updateData)
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/data/save", bytes.NewReader(updateBody))
		router.ServeHTTP(w, req)

		// 验证更新
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/data/get?id="+strconv.Itoa(createdID), nil)
		router.ServeHTTP(w, req)
		var getRes CodeMsg
		json.Unmarshal(w.Body.Bytes(), &getRes)
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
			req, _ := http.NewRequest("POST", "/api/data/save", bytes.NewReader(body))
			router.ServeHTTP(httptest.NewRecorder(), req)
		}

		// 测试分页
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/data/list?page=2&pageSize=10", nil)
		router.ServeHTTP(w, req)

		var listRes CodeMsg
		json.Unmarshal(w.Body.Bytes(), &listRes)
		pageInfo := listRes.Data.(map[string]interface{})

		assert.Equal(t, 2, int(pageInfo["Page"].(float64)))
		assert.Equal(t, 10, int(pageInfo["PageSize"].(float64)))
		assert.Equal(t, 27, int(pageInfo["Total"].(float64))) // 25 + 之前测试的2条
		assert.Len(t, pageInfo["List"], 10)
	})

	t.Run("DeleteRecord", func(t *testing.T) {
		// 创建测试记录
		createBody, _ := json.Marshal(baseData)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
		router.ServeHTTP(w, req)
		createdID := int(w.Result().Body.(*bytes.Buffer).Bytes()["id"].(float64))

		// 删除记录
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("DELETE", "/api/data/delete?id="+strconv.Itoa(createdID), nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 验证删除
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/data/get?id="+strconv.Itoa(createdID), nil)
		router.ServeHTTP(w, req)
		var getRes CodeMsg
		json.Unmarshal(w.Body.Bytes(), &getRes)
		assert.Equal(t, ErrorCode, getRes.Code)
	})
}

func TestFieldMapping(t *testing.T) {
	router, _ := setupRouter()

	// 测试字段映射
	testData := map[string]interface{}{
		"apiField1": "mappingTest",
		"apiField2": 200,
	}

	// 创建记录
	createBody, _ := json.Marshal(testData)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/data/save", bytes.NewReader(createBody))
	router.ServeHTTP(w, req)

	// 直接查询数据库验证字段映射
	var dbResult struct {
		Field1 string `gorm:"column:field1"`
		Field2 int    `gorm:"column:field2"`
	}
	router.GET("/api/data/get?id=1", func(c *gin.Context) {
		c.JSON(200, dbResult)
	})

	assert.Equal(t, "mappingTest", dbResult.Field1)
	assert.Equal(t, 200, dbResult.Field2)
}
