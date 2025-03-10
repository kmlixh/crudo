# CRUDO - A Flexible CRUD Framework for Go

CRUDO is a powerful and flexible CRUD (Create, Read, Update, Delete, Operations) framework for Go, built on top of the Fiber web framework and GOM database library. It provides a simple yet powerful way to create RESTful APIs with minimal boilerplate code.

## Features

- Automatic CRUD endpoint generation
- Field mapping between API and database
- Flexible query operations
- Pagination support
- Custom field selection for list and detail views
- Automatic type conversion
- Table information retrieval

## Installation

```bash
go get github.com/lixinghua5540/crudo
```

## Quick Start

```go
package main

import (
    "github.com/gofiber/fiber/v2"
    "github.com/kmlixh/crudo"
    "github.com/kmlixh/gom/v4"
)

func main() {
    // Initialize database connection
    db, err := gom.Open("postgres", "host=localhost user=postgres password=password dbname=mydb port=5432 sslmode=disable")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // Create field mapping
    transferMap := map[string]string{
        "apiField1": "db_field1",
        "apiField2": "db_field2",
    }

    // Initialize CRUD instance
    crud, err := crudo.NewCrud(
        "/data",           // API prefix
        "my_table",       // Table name
        db,               // Database instance
        transferMap,      // Field mapping
        nil,             // Fields to show in list view
        nil,             // Fields to show in detail view
    )
    if err != nil {
        panic(err)
    }

    // Setup Fiber app
    app := fiber.New()
    crud.RegisterRoutes(app.Group("/api"))
    app.Listen(":8080")
}
```

## API Endpoints

The framework automatically generates the following endpoints:

### 1. Create/Update Record (POST /api/data/save)

```bash
# Create new record
curl -X POST http://localhost:8080/api/data/save \
  -H "Content-Type: application/json" \
  -d '{"apiField1": "value1", "apiField2": 100}'

# Update existing record
curl -X POST http://localhost:8080/api/data/save \
  -H "Content-Type: application/json" \
  -d '{"id": 1, "apiField1": "updated_value"}'
```

### 2. Get Single Record (GET /api/data/get)

```bash
# Get record by ID
curl http://localhost:8080/api/data/get?id=1

# Get record with conditions
curl http://localhost:8080/api/data/get?apiField1_eq=value1
```

### 3. List Records (GET /api/data/list)

```bash
# Basic pagination
curl http://localhost:8080/api/data/list?page=1&pageSize=10

# With sorting
curl http://localhost:8080/api/data/list?orderBy=apiField1&orderByDesc=apiField2

# With filters
curl http://localhost:8080/api/data/list?apiField1_like=value%&apiField2_gt=50
```

### 4. Delete Record (DELETE /api/data/delete)

```bash
# Delete by ID
curl -X DELETE http://localhost:8080/api/data/delete?id=1
```

### 5. Get Table Information (GET /api/data/table)

```bash
# Get table structure
curl http://localhost:8080/api/data/table
```

## Query Operations

The framework supports various query operations:

| Operation | URL Parameter Format | Example |
|-----------|---------------------|---------|
| Equal | field_eq | ?name_eq=John |
| Not Equal | field_ne | ?age_ne=25 |
| Greater Than | field_gt | ?age_gt=18 |
| Greater Equal | field_ge | ?age_ge=21 |
| Less Than | field_lt | ?price_lt=100 |
| Less Equal | field_le | ?price_le=50 |
| In | field_in | ?status_in=active,pending |
| Not In | field_notIn | ?status_notIn=deleted,archived |
| Is Null | field_isNull | ?deletedAt_isNull=true |
| Is Not Null | field_isNotNull | ?email_isNotNull=true |
| Between | field_between | ?age_between=18,30 |
| Not Between | field_notBetween | ?price_notBetween=100,200 |
| Like | field_like | ?name_like=%John% |
| Not Like | field_notLike | ?name_notLike=%test% |

## Field Mapping

Field mapping allows you to use different field names in your API compared to your database:

```go
transferMap := map[string]string{
    "userName": "user_name",     // API field : Database field
    "userAge": "age",
    "userEmail": "email_address"
}
```

## Custom Field Selection

You can specify which fields to include in list and detail views:

```go
crud, err := crudo.NewCrud(
    "/data",
    "users",
    db,
    transferMap,
    []string{"id", "user_name", "age"},     // Fields for list view
    []string{"id", "user_name", "age", "email_address", "created_at"},  // Fields for detail view
)
```

## Response Format

All endpoints return responses in the following format:

```json
{
    "code": 200,           // Status code (200 for success, 500 for error)
    "message": "success",  // Status message
    "data": {             // Response data
        // ... response data here
    }
}
```

For paginated responses:

```json
{
    "code": 200,
    "message": "success",
    "data": {
        "Page": 1,
        "PageSize": 10,
        "Total": 100,
        "List": [
            // ... array of records
        ]
    }
}
```

## Error Handling

The framework automatically handles common errors and returns appropriate error messages:

```json
{
    "code": 500,
    "message": "record not found",
    "data": {}
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details. 