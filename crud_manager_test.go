package crudo

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestManager() (*CrudManager, func()) {
	conf := &ServiceConfig{
		Databases: []DatabaseConfig{{
			Name:     "test_db",
			Host:     "localhost",
			Port:     5432,
			User:     "test",
			Password: "test",
			Database: "test_db",
		}},
		Tables: []TableConfig{{
			Name:       "test_table",
			Database:   "test_db",
			PathPrefix: "/data",
			FieldMap:   map[string]string{"apiField": "db_field"},
		}},
	}

	manager, err := NewCrudManager(conf)
	if err != nil {
		panic(err)
	}

	// 创建测试表
	db := manager.dbs["test_db"]
	db.Chain().Raw(`CREATE TABLE IF NOT EXISTS test_table (
		id SERIAL PRIMARY KEY,
		db_field VARCHAR(255)
	)`).Exec()

	return manager, func() {
		db.Close()
		db.Chain().Raw("DROP TABLE test_table").Exec()
	}
}

func TestCrudManager_Initialization(t *testing.T) {
	t.Run("正常初始化", func(t *testing.T) {
		manager, cleanup := setupTestManager()
		defer cleanup()

		assert.NotNil(t, manager.dbs["test_db"])
		assert.NotNil(t, manager.tables["test_table"])
		assert.Equal(t, 1, len(manager.tableMap))
	})

	t.Run("数据库连接失败", func(t *testing.T) {
		invalidConf := &ServiceConfig{
			Databases: []DatabaseConfig{{
				Name:     "invalid_db",
				Host:     "invalid_host",
				Port:     5432,
				User:     "root",
				Password: "wrong",
				Database: "test",
			}},
		}

		_, err := NewCrudManager(invalidConf)
		assert.ErrorContains(t, err, "failed to connect")
	})
}

func TestCrudManager_Routing(t *testing.T) {
	manager, cleanup := setupTestManager()
	defer cleanup()

	router := gin.New()
	manager.RegisterRoutes(router)

	tests := []struct {
		method string
		path   string
		status int
	}{
		{"POST", "/test_table/save", http.StatusOK},
		{"DELETE", "/test_table/delete", http.StatusOK},
		{"GET", "/test_table/get", http.StatusOK},
		{"GET", "/test_table/list", http.StatusOK},
		{"GET", "/test_table/table", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, tt.status, w.Code)
		})
	}
}

func TestCrudManager_UpdateConfig(t *testing.T) {
	manager, cleanup := setupTestManager()
	defer cleanup()

	newConf := &ServiceConfig{
		Databases: []DatabaseConfig{{
			Name:     "new_db",
			Host:     "localhost",
			Port:     5432,
			User:     "test",
			Password: "test",
			Database: "new_db",
		}},
		Tables: []TableConfig{{
			Name:       "new_table",
			Database:   "new_db",
			PathPrefix: "/new",
		}},
	}

	t.Run("配置更新验证", func(t *testing.T) {
		err := manager.UpdateConfig(newConf)
		assert.NoError(t, err)

		assert.NotNil(t, manager.dbs["new_db"])
		assert.NotNil(t, manager.tables["new_table"])
		assert.Equal(t, 1, len(manager.tableMap))
	})
}

func TestCrudManager_InstanceHandling(t *testing.T) {
	manager, cleanup := setupTestManager()
	defer cleanup()

	t.Run("存在实例查询", func(t *testing.T) {
		instance, ok := manager.getCrudInstance(&gin.Context{
			Params: gin.Params{{Key: "table", Value: "test_table"}},
		})
		assert.True(t, ok)
		assert.NotNil(t, instance)
	})

	t.Run("不存在实例处理", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "table", Value: "non_existent"}}

		instance, ok := manager.getCrudInstance(c)
		assert.False(t, ok)
		assert.Nil(t, instance)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCrudManager_Concurrency(t *testing.T) {
	manager, cleanup := setupTestManager()
	defer cleanup()

	t.Run("并发配置更新", func(t *testing.T) {
		// 测试并发读写安全性
		go func() {
			newConf := &ServiceConfig{
				Databases: manager.config.Databases,
				Tables:    append(manager.config.Tables, TableConfig{Name: "temp_table"}),
			}
			manager.UpdateConfig(newConf)
		}()

		// 并发访问
		_, ok := manager.getCrudInstance(&gin.Context{
			Params: gin.Params{{Key: "table", Value: "test_table"}},
		})
		assert.True(t, ok)
	})
}
