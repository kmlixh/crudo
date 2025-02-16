package main

import (
	"github.com/gin-gonic/gin"
	"github.com/kmlixh/crudo"
	"github.com/kmlixh/gom/v4"
	"github.com/kmlixh/gom/v4/define"
)

func main() {
	db, err := gom.Open("mysql", "user:password@tcp(127.0.0.1:3306)/dbname", &define.DBOptions{
		Debug: true,
	})
	if err != nil {
		panic("failed to connect database")
	}

	r := gin.Default()
	crud, err := crudo.NewCrud("users", db)
	if err != nil {
		panic(err)
	}

	crud.RegisterRoutes(r.Group("/api"))
	r.Run(":8080")
}
