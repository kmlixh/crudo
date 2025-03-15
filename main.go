package crudo

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gopkg.in/yaml.v3"
)

func main() {
	// Load YAML config
	yamlData, err := os.ReadFile("consul_config.yml")
	if err != nil {
		log.Fatalf("Failed to read YAML config: %v", err)
	}

	// Parse YAML into ServiceConfig
	var config ServiceConfig
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		log.Fatalf("Failed to parse YAML: %v", err)
	}

	// Initialize CrudManager
	manager, err := NewCrudManager(&config)
	if err != nil {
		log.Fatalf("Failed to initialize CrudManager: %v", err)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"code":    code,
				"message": err.Error(),
				"data":    nil,
			})
		},
	})

	// Add middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New())

	// Register routes
	api := app.Group("/api")
	manager.RegisterRoutes(api)

	// Add health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   fmt.Sprintf("%v", time.Now()),
		})
	})

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s", port)
	log.Fatal(app.Listen(":" + port))
}
