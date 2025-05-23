# Crudo - CRUD Operations Made Easy

Crudo is a Go library that provides automatic CRUD (Create, Read, Update, Delete) operations for database tables through a RESTful API. It supports multiple database types including PostgreSQL and MySQL.

## Features

- Automatic CRUD API generation for database tables
- Support for PostgreSQL and MySQL databases
- Configurable field mapping between API and database
- Customizable endpoints and operations
- Easy integration with Fiber web framework
- Batch operations support (bulk delete)

## Usage

### Configuration

Create a YAML configuration file (e.g., `consul_config.yml`) with your database and table configurations:

```yaml
# Database configurations
databases:
  - name: "mydb"
    driver: "postgres"  # or "mysql"
    host: "localhost"
    port: 5432
    user: "postgres"
    password: "password"
    database: "mydatabase"

# Table configurations
tables:
  - name: "users"
    database: "mydb"
    path_prefix: "/users"
    field_map:
      id: "id"
      username: "username"
      email: "email"
    list_fields:
      - "id"
      - "username"
      - "email"
    detail_fields:
      - "id"
      - "username"
      - "email"
      - "created_at"
    handler_filters:
      - "save"
      - "delete"
      - "get"
      - "list"
      - "table"
```

### Running the Server

```go
// main.go
package main

import (
    "log"
	"os"

    "github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v3"
)

func main() {
	// Load configuration
	yamlData, err := os.ReadFile("consul_config.yml")
    if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	var config ServiceConfig
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// Initialize CrudManager
	manager, err := NewCrudManager(&config)
    if err != nil {
		log.Fatalf("Failed to initialize CrudManager: %v", err)
    }

	// Create Fiber app
    app := fiber.New()
    
	// Register routes
	api := app.Group("/api")
	manager.RegisterRoutes(api)

	// Start server
	log.Fatal(app.Listen(":8080"))
}
```

## API Endpoints

For each configured table, the following endpoints are available:

- `GET /{path_prefix}/list` - List all records with pagination
- `GET /{path_prefix}/get` - Get a single record by ID or other conditions
- `POST /{path_prefix}/save` - Create or update a record
- `DELETE /{path_prefix}/delete` - Delete a record by ID or conditions
- `GET /{path_prefix}/table` - Get table metadata

## Batch Operations

### Batch Delete

You can delete multiple records at once by providing an array of IDs:

```http
DELETE /{path_prefix}/delete
Content-Type: application/json

{
    "ids": [1, 2, 3]
}
```

Response:

```json
{
    "code": 200,
    "data": {
        "deleted_count": 3,
        "ids": [1, 2, 3]
    },
    "msg": "ok"
}
```

### Query Parameters for Filtering

When performing operations like list or delete, you can use query parameters for filtering:

```
GET /{path_prefix}/list?name_eq=John&age_gt=20
```

The supported operations are:
- `_eq`: Equal
- `_ne`: Not Equal
- `_gt`: Greater Than
- `_ge`: Greater Than or Equal
- `_lt`: Less Than
- `_le`: Less Than or Equal
- `_in`: In a list of values
- `_like`: LIKE pattern match

## Running Tests

```bash
go test -v
```
