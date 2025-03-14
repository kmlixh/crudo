# CRUDO - 一个灵活的 Go 语言 CRUD 框架

CRUDO 是一个基于 Fiber web 框架和 GOM 数据库库构建的强大而灵活的 CRUD（创建、读取、更新、删除、操作）框架。它提供了一种简单而强大的方式来创建 RESTful API，并且几乎不需要编写样板代码。

## 主要特性

- 自动生成 CRUD 接口端点
- API 和数据库之间的字段映射
- 灵活的查询操作
- 分页支持
- 列表和详情视图的自定义字段选择
- 自动类型转换
- 表信息检索
- 多数据库和多表管理
- 处理器过滤（可选择性启用特定操作）
- 线程安全的配置管理

## 安装

```bash
go get github.com/kmlixh/crudo@v1.0.0
```

## 快速开始

### 基本用法

```go
package main

import (
    "github.com/gofiber/fiber/v2"
    "github.com/kmlixh/crudo"
    "github.com/kmlixh/gom/v4"
)

func main() {
    // 初始化数据库连接
    db, err := gom.Open("postgres", "host=localhost user=postgres password=password dbname=mydb port=5432 sslmode=disable")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // 创建字段映射
    transferMap := map[string]string{
        "apiField1": "db_field1",
        "apiField2": "db_field2",
    }

    // 初始化 CRUD 实例
    crud, err := crudo.NewCrud(
        "/data",           // API 前缀
        "my_table",       // 表名
        db,               // 数据库实例
        transferMap,      // 字段映射
        nil,             // 列表视图显示的字段
        nil,             // 详情视图显示的字段
        nil,             // 处理器过滤（nil 表示启用所有处理器）
    )
    if err != nil {
        panic(err)
    }

    // 设置 Fiber 应用
    app := fiber.New()
    crud.RegisterRoutes(app.Group("/api"))
    app.Listen(":8080")
}
```

### 使用 CrudManager 管理多数据库和多表

```go
package main

import (
    "github.com/gofiber/fiber/v2"
    "github.com/kmlixh/crudo"
)

func main() {
    // 创建配置
    config := &crudo.ServiceConfig{
        Databases: []crudo.DatabaseConfig{
            {
                Name:     "db1",
                Host:     "localhost",
                Port:     5432,
                User:     "postgres",
                Password: "password",
                Database: "mydb",
            },
            {
                Name:     "db2",
                Host:     "localhost",
                Port:     5432,
                User:     "postgres",
                Password: "password",
                Database: "otherdb",
            },
        },
        Tables: []crudo.TableConfig{
            {
                Name:       "users",
                Database:   "db1",
                PathPrefix: "/users",
                FieldMap:   map[string]string{"userName": "user_name"},
                ListFields: []string{"id", "user_name"},
                DetailFields: []string{"id", "user_name", "email", "created_at"},
                HandlerFilters: []string{"save", "get", "list"}, // 只启用这些操作
            },
            {
                Name:       "products",
                Database:   "db2",
                PathPrefix: "/products",
                FieldMap:   map[string]string{"productName": "name"},
                // 不指定 HandlerFilters 则启用所有操作
            },
        },
    }

    // 初始化 CrudManager
    manager, err := crudo.NewCrudManager(config)
    if err != nil {
        panic(err)
    }

    // 设置 Fiber 应用
    app := fiber.New()
    
    // 注册统一路由
    manager.RegisterRoutes(app.Group("/api"))
    
    app.Listen(":8080")
}
```

## API 端点

框架自动生成以下端点：

### 1. 创建/更新记录 (POST /api/data/save)
- 支持创建新记录和更新现有记录
- 通过 JSON 格式提交数据
- 创建示例: `POST /api/data/save` 带 JSON 数据 `{"apiField1": "value", "apiField2": 123}`
- 更新示例: `POST /api/data/save` 带 JSON 数据 `{"id": 1, "apiField1": "new value"}`

**请求示例（创建新记录）：**
```bash
curl -X POST http://localhost:8080/api/data/save \
  -H "Content-Type: application/json" \
  -d '{"userName": "张三", "userAge": 28, "userEmail": "zhangsan@example.com"}'
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 1,
    "userName": "张三",
    "userAge": 28,
    "userEmail": "zhangsan@example.com"
  }
}
```

**请求示例（更新记录）：**
```bash
curl -X POST http://localhost:8080/api/data/save \
  -H "Content-Type: application/json" \
  -d '{"id": 1, "userName": "张三", "userAge": 29}'
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 1,
    "userName": "张三",
    "userAge": 29,
    "userEmail": "zhangsan@example.com"
  }
}
```

### 2. 获取单条记录 (GET /api/data/get)
- 支持通过 ID 获取记录: `GET /api/data/get?id=1`
- 支持条件查询: `GET /api/data/get?name_eq=John&age_gt=18`

**请求示例（通过 ID 获取）：**
```bash
curl -X GET http://localhost:8080/api/data/get?id=1
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 1,
    "userName": "张三",
    "userAge": 29,
    "userEmail": "zhangsan@example.com"
  }
}
```

**请求示例（条件查询）：**
```bash
curl -X GET "http://localhost:8080/api/data/get?userName_eq=张三&userAge_gt=25"
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 1,
    "userName": "张三",
    "userAge": 29,
    "userEmail": "zhangsan@example.com"
  }
}
```

### 3. 获取记录列表 (GET /api/data/list)
- 支持分页: `GET /api/data/list?page=1&pageSize=10`
- 支持排序: `GET /api/data/list?orderBy=name&orderByDesc=age`
- 支持多种过滤条件: `GET /api/data/list?status_in=active,pending&age_between=18,30`

**请求示例（基本分页）：**
```bash
curl -X GET "http://localhost:8080/api/data/list?page=1&pageSize=10"
```

**请求示例（带排序和过滤）：**
```bash
curl -X GET "http://localhost:8080/api/data/list?page=1&pageSize=10&orderBy=userName&userAge_between=20,30"
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "Page": 1,
    "PageSize": 10,
    "Total": 25,
    "List": [
      {
        "id": 1,
        "userName": "张三",
        "userAge": 29
      },
      {
        "id": 2,
        "userName": "李四",
        "userAge": 25
      },
      // ... 更多记录
    ]
  }
}
```

### 4. 删除记录 (DELETE /api/data/delete)
- 支持通过 ID 删除记录: `DELETE /api/data/delete?id=1`
- 支持条件删除: `DELETE /api/data/delete?status_eq=inactive`

**请求示例（通过 ID 删除）：**
```bash
curl -X DELETE http://localhost:8080/api/data/delete?id=1
```

**请求示例（条件删除）：**
```bash
curl -X DELETE "http://localhost:8080/api/data/delete?userAge_lt=18"
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "RowsAffected": 1
  }
}
```

### 5. 获取表信息 (GET /api/data/table)
- 获取数据表结构信息: `GET /api/data/table`

**请求示例：**
```bash
curl -X GET http://localhost:8080/api/data/table
```

**响应示例：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "TableName": "users",
    "Comment": "用户表",
    "PrimaryKey": ["id"],
    "PrimaryKeyAuto": ["id"],
    "Columns": [
      {
        "Name": "id",
        "Type": "int64",
        "Comment": "主键ID",
        "IsKey": true,
        "IsAuto": true
      },
      {
        "Name": "user_name",
        "Type": "string",
        "Comment": "用户名",
        "IsKey": false,
        "IsAuto": false
      },
      {
        "Name": "age",
        "Type": "int32",
        "Comment": "年龄",
        "IsKey": false,
        "IsAuto": false
      },
      {
        "Name": "email",
        "Type": "string",
        "Comment": "邮箱",
        "IsKey": false,
        "IsAuto": false
      }
    ]
  }
}
```

## 使用 CrudManager 的统一路由

当使用 CrudManager 时，所有的 CRUD 操作都通过 `/:table/:operation` 格式的 URL 进行访问：

- **URL 格式**: `/{table}/{operation}`
- **示例**:
  - GET `/api/users/list` - 获取用户列表
  - POST `/api/users/save` - 保存用户信息
  - GET `/api/products/get?id=1` - 获取产品详情
  - DELETE `/api/products/delete?id=1` - 删除产品

**请求示例（获取用户列表）：**
```bash
curl -X GET "http://localhost:8080/api/users/list?page=1&pageSize=10"
```

**请求示例（保存产品信息）：**
```bash
curl -X POST http://localhost:8080/api/products/save \
  -H "Content-Type: application/json" \
  -d '{"productName": "智能手机", "productPrice": 3999.99}'
```

## 查询操作

框架支持多种查询操作：

| 操作 | URL 参数格式 | 示例 |
|-----------|---------------------|---------|
| 等于 | field_eq | ?name_eq=John |
| 不等于 | field_ne | ?age_ne=25 |
| 大于 | field_gt | ?age_gt=18 |
| 大于等于 | field_ge | ?age_ge=21 |
| 小于 | field_lt | ?price_lt=100 |
| 小于等于 | field_le | ?price_le=50 |
| 在列表中 | field_in | ?status_in=active,pending |
| 不在列表中 | field_notIn | ?status_notIn=deleted,archived |
| 为空 | field_isNull | ?deletedAt_isNull=true |
| 不为空 | field_isNotNull | ?email_isNotNull=true |
| 介于 | field_between | ?age_between=18,30 |
| 不介于 | field_notBetween | ?price_notBetween=100,200 |
| 模糊匹配 | field_like | ?name_like=%John% |
| 不匹配 | field_notLike | ?name_notLike=%test% |

**复杂查询示例：**
```bash
# 查询年龄在20-30之间且状态为活跃或待定的用户
curl -X GET "http://localhost:8080/api/users/list?userAge_between=20,30&status_in=active,pending&orderBy=userName"
```

**组合多个条件：**
```bash
# 查询名称包含"智能"且价格大于1000的产品
curl -X GET "http://localhost:8080/api/products/list?productName_like=%智能%&productPrice_gt=1000"
```

## 字段映射

可以为 API 和数据库使用不同的字段名：

```go
transferMap := map[string]string{
    "userName": "user_name",     // API 字段 : 数据库字段
    "userAge": "age",
    "userEmail": "email_address"
}
```

字段映射的工作原理：
1. 当客户端发送请求时，API 字段名会自动转换为数据库字段名
2. 当服务器返回响应时，数据库字段名会自动转换为 API 字段名
3. 未映射的字段将保持原样

**映射前的数据库记录：**
```json
{
  "id": 1,
  "user_name": "张三",
  "age": 29,
  "email_address": "zhangsan@example.com"
}
```

**映射后的 API 响应：**
```json
{
  "id": 1,
  "userName": "张三",
  "userAge": 29,
  "userEmail": "zhangsan@example.com"
}
```

## 自定义字段选择

可以指定列表和详情视图中显示的字段：

```go
crud, err := crudo.NewCrud(
    "/data",
    "users",
    db,
    transferMap,
    []string{"id", "user_name", "age"},     // 列表视图字段
    []string{"id", "user_name", "age", "email_address", "created_at"},  // 详情视图字段
    nil,  // 处理器过滤
)
```

字段选择的作用：
1. `FieldOfList`: 控制 list 接口返回的字段，适用于列表页面，通常只返回必要的字段
2. `FieldOfDetail`: 控制 get 接口返回的字段，适用于详情页面，通常返回更完整的信息
3. 如果不指定（传入 nil），则返回表中的所有字段

**列表视图响应示例（只包含指定字段）：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "Page": 1,
    "PageSize": 10,
    "Total": 25,
    "List": [
      {
        "id": 1,
        "userName": "张三",
        "userAge": 29
      },
      // ... 更多记录
    ]
  }
}
```

**详情视图响应示例（包含更多字段）：**
```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 1,
    "userName": "张三",
    "userAge": 29,
    "userEmail": "zhangsan@example.com",
    "created_at": "2023-01-01T12:00:00Z"
  }
}
```

## 处理器过滤

可以选择性地启用特定的 CRUD 操作：

```go
// 只启用保存和获取操作
crud, err := crudo.NewCrud(
    "/data",
    "users",
    db,
    transferMap,
    nil,
    nil,
    []string{"save", "get"},  // 只启用 save 和 get 处理器
)
```

处理器过滤的应用场景：
1. 创建只读的 API 端点（只启用 get 和 list）
2. 创建只写的 API 端点（只启用 save）
3. 根据权限控制可用的操作
4. 优化性能，只初始化需要的处理器

**尝试访问未启用的操作示例：**
```bash
# 如果只启用了 save 和 get 操作，尝试访问 list 操作
curl -X GET "http://localhost:8080/api/data/list"
```

**响应示例（操作不存在）：**
```json
{
  "code": 500,
  "message": "operation not configured",
  "data": {}
}
```

## 响应格式

所有端点返回统一的响应格式：

```json
{
    "code": 200,           // 状态码（200 表示成功，500 表示错误）
    "message": "success",  // 状态信息
    "data": {             // 响应数据
        // ... 具体数据
    }
}
```

分页响应格式：

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "Page": 1,
        "PageSize": 10,
        "Total": 100,
        "List": [
            // ... 记录数组
        ]
    }
}
```

## 错误处理

框架自动处理常见错误并返回适当的错误信息：

```json
{
    "code": 500,
    "message": "record not found",
    "data": {}
}
```

常见错误类型：
1. 数据库连接错误
2. 记录未找到
3. 参数验证错误
4. 操作不存在错误
5. 请求方法错误

**错误示例（记录未找到）：**
```bash
curl -X GET "http://localhost:8080/api/data/get?id=999"
```

**响应：**
```json
{
  "code": 500,
  "message": "record not found",
  "data": {}
}
```

**错误示例（请求方法错误）：**
```bash
# 使用 GET 方法调用需要 POST 的接口
curl -X GET "http://localhost:8080/api/data/save"
```

**响应：**
```json
{
  "code": 500,
  "message": "method not allowed",
  "data": {}
}
```

## CrudManager 高级功能

### 配置结构

```yaml
# 数据库配置
databases:
  - name: "db1"           # 数据库名称
    host: "localhost"     # 数据库主机
    port: 5432           # 端口
    user: "postgres"     # 用户名
    password: "password" # 密码
    database: "mydb"     # 数据库名

# 表配置
tables:
  - name: "users"        # 表名
    database: "db1"      # 对应的数据库名
    path_prefix: "/api"  # API路径前缀
    field_map:          # 字段映射
      userName: "user_name"
      userAge: "age"
    list_fields:        # 列表视图字段
      - "id"
      - "user_name"
    detail_fields:      # 详情视图字段
      - "id"
      - "user_name"
      - "age"
    handler_filters:    # 处理器过滤
      - "save"
      - "get"
      - "list"
```

### 动态配置更新

支持在运行时动态更新配置：

```go
// 更新配置
newConfig := &crudo.ServiceConfig{
    // 新的配置内容
}
err := manager.UpdateConfig(newConfig)
if err != nil {
    // 处理错误
}
```

更新配置时会自动：
- 关闭旧的数据库连接
- 建立新的数据库连接
- 重新初始化表配置
- 更新 CRUD 实例

### 线程安全

CrudManager 实现了完整的线程安全机制：
- 使用读写锁（sync.RWMutex）保护共享资源
- 支持并发访问数据库连接
- 支持并发处理多表操作
- 支持配置的安全更新

## 完整示例

### 基本 CRUD 操作

```go
package main

import (
    "log"

    "github.com/gofiber/fiber/v2"
    "github.com/kmlixh/crudo"
    "github.com/kmlixh/gom/v4"
    _ "github.com/kmlixh/gom/v4/factory/postgres" // 导入数据库驱动
)

func main() {
    // 初始化数据库连接
    db, err := gom.Open("postgres", "host=localhost user=postgres password=password dbname=mydb port=5432 sslmode=disable")
    if err != nil {
        log.Fatalf("数据库连接失败: %v", err)
    }
    defer db.Close()

    // 创建字段映射
    transferMap := map[string]string{
        "userName": "user_name",
        "userEmail": "email",
        "userAge": "age",
    }

    // 初始化 CRUD 实例
    crud, err := crudo.NewCrud(
        "/users",          // API 前缀
        "users",          // 表名
        db,               // 数据库实例
        transferMap,      // 字段映射
        []string{"id", "user_name", "age"}, // 列表视图字段
        nil,              // 详情视图字段（nil 表示所有字段）
        nil,              // 处理器过滤（nil 表示所有处理器）
    )
    if err != nil {
        log.Fatalf("CRUD 初始化失败: %v", err)
    }

    // 设置 Fiber 应用
    app := fiber.New()
    
    // 注册路由
    crud.RegisterRoutes(app.Group("/api"))
    
    // 添加一个首页
    app.Get("/", func(c *fiber.Ctx) error {
        return c.SendString("CRUDO API 服务已启动")
    })
    
    // 启动服务
    log.Println("服务启动在 http://localhost:8080")
    if err := app.Listen(":8080"); err != nil {
        log.Fatalf("服务启动失败: %v", err)
    }
}
```

### 使用 CrudManager 的完整示例

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/recover"
    "github.com/kmlixh/crudo"
    _ "github.com/kmlixh/gom/v4/factory/postgres" // 导入数据库驱动
)

func main() {
    // 创建配置
    config := &crudo.ServiceConfig{
        Databases: []crudo.DatabaseConfig{
            {
                Name:     "main_db",
                Host:     "localhost",
                Port:     5432,
                User:     "postgres",
                Password: "password",
                Database: "mydb",
            },
        },
        Tables: []crudo.TableConfig{
            {
                Name:       "users",
                Database:   "main_db",
                PathPrefix: "/users",
                FieldMap: map[string]string{
                    "userName": "user_name",
                    "userEmail": "email",
                },
                ListFields: []string{"id", "user_name"},
                DetailFields: []string{"id", "user_name", "email", "created_at"},
            },
            {
                Name:       "products",
                Database:   "main_db",
                PathPrefix: "/products",
                FieldMap: map[string]string{
                    "productName": "name",
                    "productPrice": "price",
                },
                HandlerFilters: []string{"get", "list"}, // 只读操作
            },
        },
    }

    // 初始化 CrudManager
    manager, err := crudo.NewCrudManager(config)
    if err != nil {
        log.Fatalf("CrudManager 初始化失败: %v", err)
    }

    // 设置 Fiber 应用
    app := fiber.New(fiber.Config{
        ErrorHandler: func(c *fiber.Ctx, err error) error {
            return c.Status(500).JSON(fiber.Map{
                "code":    500,
                "message": err.Error(),
                "data":    fiber.Map{},
            })
        },
    })
    
    // 添加中间件
    app.Use(recover.New())
    app.Use(logger.New())
    
    // 注册统一路由
    manager.RegisterRoutes(app.Group("/api"))
    
    // 添加一个首页
    app.Get("/", func(c *fiber.Ctx) error {
        return c.SendString("CRUDO API 服务已启动")
    })
    
    // 优雅关闭
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    
    go func() {
        <-c
        log.Println("正在关闭服务...")
        app.Shutdown()
    }()
    
    // 启动服务
    log.Println("服务启动在 http://localhost:8080")
    if err := app.Listen(":8080"); err != nil {
        log.Fatalf("服务启动失败: %v", err)
    }
}
```

## 贡献

欢迎提交 Pull Request 来贡献代码！

## 许可证

本项目采用 MIT 许可证 - 详见 LICENSE 文件 