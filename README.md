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

## 安装

```bash
go get github.com/lixinghua5540/crudo
```

## 快速开始

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

## API 端点

框架自动生成以下端点：

### 1. 创建/更新记录 (POST /api/data/save)
- 支持创建新记录和更新现有记录
- 通过 JSON 格式提交数据

### 2. 获取单条记录 (GET /api/data/get)
- 支持通过 ID 获取记录
- 支持条件查询

### 3. 获取记录列表 (GET /api/data/list)
- 支持分页
- 支持排序
- 支持多种过滤条件

### 4. 删除记录 (DELETE /api/data/delete)
- 支持通过 ID 删除记录

### 5. 获取表信息 (GET /api/data/table)
- 获取数据表结构信息

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

## 字段映射

可以为 API 和数据库使用不同的字段名：

```go
transferMap := map[string]string{
    "userName": "user_name",     // API 字段 : 数据库字段
    "userAge": "age",
    "userEmail": "email_address"
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
)
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

## 贡献

欢迎提交 Pull Request 来贡献代码！

## 许可证

本项目采用 MIT 许可证 - 详见 LICENSE 文件 