package crudo

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kmlixh/gom/v4"
	"github.com/kmlixh/gom/v4/define"
)

// config.go
type DatabaseConfig struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

type TableConfig struct {
	Name         string            `yaml:"name"`
	Database     string            `yaml:"database"`
	PathPrefix   string            `yaml:"path_prefix"`
	FieldMap     map[string]string `yaml:"field_map"`
	ListFields   []string          `yaml:"list_fields"`
	DetailFields []string          `yaml:"detail_fields"`
}

type ServiceConfig struct {
	Databases []DatabaseConfig `yaml:"databases"`
	Tables    []TableConfig    `yaml:"tables"`
}

// crud_manager.go
type CrudManager struct {
	config   *ServiceConfig
	dbs      map[string]*gom.DB
	tables   map[string]*Crud // key: table name
	tableMap map[string]*define.TableInfo
	mu       sync.RWMutex
}

func NewCrudManager(conf *ServiceConfig) (*CrudManager, error) {
	cm := &CrudManager{
		config:   conf,
		dbs:      make(map[string]*gom.DB),
		tables:   make(map[string]*Crud),
		tableMap: make(map[string]*define.TableInfo),
	}

	if err := cm.init(); err != nil {
		return nil, err
	}
	return cm, nil
}

func (cm *CrudManager) init() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 初始化数据库连接
	for _, dbConf := range cm.config.Databases {
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			dbConf.Host, dbConf.Port, dbConf.User, dbConf.Password, dbConf.Database)

		db, err := gom.Open("postgres", dsn, &define.DBOptions{Debug: true})
		if err != nil {
			return fmt.Errorf("failed to connect %s: %v", dbConf.Name, err)
		}
		cm.dbs[dbConf.Name] = db
	}

	// 初始化表配置
	for _, tblConf := range cm.config.Tables {
		db, exists := cm.dbs[tblConf.Database]
		if !exists {
			return fmt.Errorf("database %s not found for table %s", tblConf.Database, tblConf.Name)
		}
		tableInfo, err := db.GetTableInfo(tblConf.Name)
		if err != nil {
			return fmt.Errorf("failed to get table info for %s: %v", tblConf.Name, err)
		}
		cm.tableMap[tblConf.Name] = tableInfo
		// 构建字段映射
		revMap := make(map[string]string, len(tblConf.FieldMap))
		for k, v := range tblConf.FieldMap {
			revMap[v] = k
		}

		crud, err := NewCrud(tblConf.PathPrefix, tblConf.Name, db, revMap, tblConf.ListFields, tblConf.DetailFields)
		if err != nil {
			return fmt.Errorf("failed to create crud for %s: %v", tblConf.Name, err)
		}
		cm.tables[tblConf.Name] = crud
	}
	return nil
}

// 注册统一路由
func (cm *CrudManager) RegisterRoutes(r *gin.Engine) {
	group := r.Group("/:table")
	{
		group.POST("/"+PathSave, cm.handleSave)
		group.DELETE("/"+PathDelete, cm.handleDelete)
		group.GET("/"+PathGet, cm.handleGet)
		group.GET("/"+PathList, cm.handleList)
		group.GET("/"+PathTable, cm.handleTable)
	}
}

// 通用请求处理
func (cm *CrudManager) getCrudInstance(c *gin.Context) (*Crud, bool) {
	tableName := c.Param("table")

	cm.mu.RLock()
	instance, exists := cm.tables[tableName]
	cm.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "table not configured"})
		return nil, false
	}
	return instance, true
}
func (cm *CrudManager) handleTable(c *gin.Context) {
	instance, ok := cm.getCrudInstance(c)
	if !ok {
		return
	}
	requestHandler, ok := instance.HandlerMap[PathTable]
	if !ok || requestHandler == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "table not configured"})
	}
	if requestHandler.PreHandle != nil {
		requestHandler.PreHandle(c)
	}
	if c.IsAborted() {
		return
	}
	requestHandler.Handle(c)
}

// 示例处理函数：保存
func (cm *CrudManager) handleSave(c *gin.Context) {
	instance, ok := cm.getCrudInstance(c)
	if !ok {
		return
	}
	instance.HandlerMap[PathSave].Handle(c)
}

// 添加缺失的处理方法
func (cm *CrudManager) handleDelete(c *gin.Context) {

}

func (cm *CrudManager) handleGet(c *gin.Context) {

}

func (cm *CrudManager) handleList(c *gin.Context) {

}

// 辅助函数检查slice包含
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// 更新配置（线程安全）
func (cm *CrudManager) UpdateConfig(newConf *ServiceConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 关闭旧连接
	for _, db := range cm.dbs {
		db.Close()
	}

	// 应用新配置
	cm.config = newConf
	cm.dbs = make(map[string]*gom.DB)
	cm.tables = make(map[string]*Crud)
	cm.tableMap = make(map[string]*define.TableInfo)
	return cm.init()
}
