package crudo

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gofiber/fiber/v2"
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
func (cm *CrudManager) RegisterRoutes(r fiber.Router) {
	r.All("/:table/:operation", cm.handle)
}

func (cm *CrudManager) handle(c *fiber.Ctx) error {
	table := c.Params("table")
	operation := c.Params("operation")
	instance, ok := cm.getCrudInstance(table)
	if !ok {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "table not configured"})
	}

	requestHandler, ok := instance.HandlerMap[operation]
	if !ok || requestHandler == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "operation not configured"})
	}
	if c.Method() != requestHandler.Method {
		return c.Status(http.StatusMethodNotAllowed).JSON(fiber.Map{"error": "method not allowed"})
	}
	if requestHandler.PreHandle != nil {
		if err := requestHandler.PreHandle(c); err != nil {
			return err
		}
	}
	return requestHandler.Handle(c)
}

// 通用请求处理
func (cm *CrudManager) getCrudInstance(tableName string) (*Crud, bool) {
	cm.mu.RLock()
	instance, exists := cm.tables[tableName]
	cm.mu.RUnlock()

	if !exists {
		return nil, false
	}
	return instance, true
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
