package crudo

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kmlixh/gom/v4"
	"github.com/kmlixh/gom/v4/define"
	_ "github.com/kmlixh/gom/v4/factory/mysql"
	_ "github.com/kmlixh/gom/v4/factory/postgres"
)

// config.go
type DatabaseConfig struct {
	Name     string     `yaml:"name"`
	Host     string     `yaml:"host"`
	Port     int        `yaml:"port"`
	User     string     `yaml:"user"`
	Password string     `yaml:"password"`
	Database string     `yaml:"database"`
	Driver   string     `yaml:"driver"`
	DSN      string     `yaml:"dsn"`     // 可选，如果提供则直接使用，否则从其他字段构建
	Options  *DBOptions `yaml:"options"` // 数据库选项
}

type TableConfig struct {
	Name           string            `yaml:"name"`
	Database       string            `yaml:"database"`
	Table          string            `yaml:"table"`
	PathPrefix     string            `yaml:"path_prefix"`
	TransferMap    map[string]string `yaml:"field_map"`
	FieldOfList    []string          `yaml:"list_fields"`
	FieldOfDetail  []string          `yaml:"detail_fields"`
	HandlerFilters []string          `yaml:"handler_filters"`
}

// DBOptions 定义数据库初始化选项
type DBOptions struct {
	MaxOpenConns    int   `yaml:"max_open_conns"`     // 最大打开连接数
	MaxIdleConns    int   `yaml:"max_idle_conns"`     // 最大空闲连接数
	ConnMaxLifetime int64 `yaml:"conn_max_lifetime"`  // 连接最大生命周期（秒）
	ConnMaxIdleTime int64 `yaml:"conn_max_idle_time"` // 空闲连接最大生命周期（秒）
	Debug           bool  `yaml:"debug"`              // 是否开启调试模式
}

type ServiceConfig struct {
	Databases []DatabaseConfig `yaml:"databases"`
	Tables    []TableConfig    `yaml:"tables"`
}

// Basic type definitions to fix compilation errors

// crud_manager.go
type CrudManager struct {
	config *ServiceConfig
	dbs    map[string]*gom.DB
	routes map[string]ICrud // key is full path for routing
	mu     sync.RWMutex
}

func NewCrudManager(config *ServiceConfig) (*CrudManager, error) {
	cm := &CrudManager{
		config: config,
		dbs:    make(map[string]*gom.DB),
		routes: make(map[string]ICrud),
	}
	return cm, nil
}

func (cm *CrudManager) init() error {
	fmt.Println("Initializing CrudManager...")

	// 初始化数据库连接
	for _, dbConf := range cm.config.Databases {
		fmt.Printf("Connecting to database %s (%s)...\n", dbConf.Name, dbConf.Driver)

		// 如果没有提供 DSN，则构建它
		dsn := dbConf.DSN
		if dsn == "" {
			switch dbConf.Driver {
			case "mysql":
				dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
					dbConf.User, dbConf.Password, dbConf.Host, dbConf.Port, dbConf.Database)
			case "postgres":
				dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
					dbConf.Host, dbConf.Port, dbConf.User, dbConf.Password, dbConf.Database)
			default:
				return fmt.Errorf("unsupported database driver: %s", dbConf.Driver)
			}
		}

		// 创建数据库选项，使用默认值
		dbOptions := &define.DBOptions{
			Debug:           false, // 默认不开启调试
			MaxOpenConns:    0,     // 默认不限制
			MaxIdleConns:    0,     // 默认不限制
			ConnMaxLifetime: 0,     // 默认不限制
			ConnMaxIdleTime: 0,     // 默认不限制
		}

		// 如果配置了选项，则使用配置的值
		if dbConf.Options != nil {
			dbOptions = &define.DBOptions{
				MaxOpenConns:    dbConf.Options.MaxOpenConns,
				MaxIdleConns:    dbConf.Options.MaxIdleConns,
				ConnMaxLifetime: time.Duration(dbConf.Options.ConnMaxLifetime) * time.Second,
				ConnMaxIdleTime: time.Duration(dbConf.Options.ConnMaxIdleTime) * time.Second,
				Debug:           dbConf.Options.Debug,
			}
		}

		db, err := gom.Open(dbConf.Driver, dsn, dbOptions)
		if err != nil {
			return fmt.Errorf("failed to connect to database %s: %v", dbConf.Name, err)
		}
		fmt.Printf("Successfully connected to database %s\n", dbConf.Name)
		cm.dbs[dbConf.Name] = db
	}

	// 初始化表配置
	for _, tblConf := range cm.config.Tables {
		fmt.Printf("Initializing table %s...\n", tblConf.Name)
		db, ok := cm.dbs[tblConf.Database]
		if !ok {
			return fmt.Errorf("database not found for table %s: %s", tblConf.Name, tblConf.Database)
		}

		fmt.Printf("Creating CRUD instance for table %s...\n", tblConf.Name)
		crud, err := NewCrud(
			tblConf.Name,
			tblConf.Table,
			db,
			tblConf.TransferMap,
			tblConf.FieldOfList,
			tblConf.FieldOfDetail,
			tblConf.HandlerFilters,
		)
		if err != nil {
			return fmt.Errorf("failed to create crud for %s: %v", tblConf.Name, err)
		}

		cm.routes[tblConf.PathPrefix] = crud
		fmt.Printf("Registered CRUD instance for table %s\n", tblConf.Name)
	}

	fmt.Println("CrudManager initialization completed.")
	return nil
}

// RegisterRoutes 注册统一路由
func (cm *CrudManager) RegisterRoutes(r fiber.Router) {
	// 注册所有路由
	r.All("/*", cm.handle)
}

func (cm *CrudManager) handle(c *fiber.Ctx) error {
	path := c.Params("*")

	// 找到匹配的 Crud 实例
	cm.mu.RLock()
	var matchedCrud ICrud
	var matchedPrefix string
	for prefix, crud := range cm.routes {
		// 确保前缀格式一致
		tempPrefix := strings.TrimPrefix(prefix, "/")

		// 只有当 tempPrefix 不为空且 path 以 tempPrefix 开头时才匹配
		if tempPrefix != "" && strings.HasPrefix(path, tempPrefix) {
			matchedCrud = crud
			matchedPrefix = tempPrefix
			break
		}
	}
	cm.mu.RUnlock()

	if matchedCrud == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "path not configured"})
	}

	// 获取操作部分
	operation := strings.TrimPrefix(path, matchedPrefix)
	operation = strings.TrimPrefix(operation, "/")

	// 获取对应的处理器
	handler, exists := matchedCrud.GetHandler(operation)
	if !exists || handler == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "operation not configured"})
	}

	if c.Method() != handler.Method {
		return c.Status(http.StatusMethodNotAllowed).JSON(fiber.Map{"error": "method not allowed"})
	}

	if handler.PreHandle != nil {
		if err := handler.PreHandle(c); err != nil {
			return err
		}
	}

	return handler.Handle(c)
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
	cm.routes = make(map[string]ICrud)
	return cm.init()
}

func (cm *CrudManager) RegisterCrud(crud ICrud) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	c, ok := crud.(*Crud)
	if !ok {
		return fmt.Errorf("invalid crud instance type")
	}

	if _, exists := cm.routes[c.Table]; exists {
		return fmt.Errorf("crud instance already registered for table: %s", c.Table)
	}

	cm.routes[c.Table] = c
	return nil
}

// 辅助方法：根据数据库名查找第一个匹配的表配置
func (cm *CrudManager) findTableConfigByDB(dbName string) (*TableConfig, bool) {
	for _, tblConf := range cm.config.Tables {
		if tblConf.Database == dbName {
			return &tblConf, true
		}
	}
	return nil, false
}

// GetCrud returns the Crud instance for the given table name
func (cm *CrudManager) GetCrud(tableName string) *Crud {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.routes[tableName].(*Crud)
}
