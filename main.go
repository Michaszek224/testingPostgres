package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

type Planet struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var db *sql.DB

func main() {
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	dbname := os.Getenv("DB_NAME")

	psqlLoginString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbname)

	var err error
	db, err = sql.Open("postgres", psqlLoginString)
	if err != nil {
		log.Fatalf("Error opening db: %v", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatalf("Could not connect to db: %v", err)

	}
	err = setupDB()
	if err != nil {
		log.Fatalf("error sertting up database: %v", err)
	}
	r := gin.Default()
	r.GET("/", getPlanets)
	r.GET("/:id", getPlanetById)
	r.DELETE("/:id", deletePlanet)
	r.PUT("/:id", updatePlanet)
	r.POST("/", addPlanet)
	r.Run()
}

func setupDB() error {
	createTableSql := `
	CREATE TABLE IF NOT EXISTS planets (
		id SERIAL PRIMARY KEY,
		name TEXT NOT NULL
	);`
	_, err := db.Exec(createTableSql)
	if err != nil {
		return fmt.Errorf("error creating table:%v", err)
	}
	log.Println("table created")
	return nil
}

func getPlanets(ctx *gin.Context) {
	rows, err := db.Query("SELECT id, name FROM planets")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error message": "error selecting planets",
		})
		log.Printf("error selecting planets: %v", err)
		return
	}
	defer rows.Close()

	planets := []Planet{}
	for rows.Next() {
		var p Planet
		err := rows.Scan(&p.ID, &p.Name)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"error message": "error scanning planets",
			})
			log.Printf("error scanning planets: %v", err)
			return
		}
		planets = append(planets, p)

	}
	ctx.JSON(http.StatusOK, planets)
}

func getPlanetById(ctx *gin.Context) {
	id := ctx.Param("id")
	row := db.QueryRow("SELECT id, name FROM planets WHERE id=$1", id)

	var getPlanet Planet
	err := row.Scan(&getPlanet.ID, &getPlanet.Name)
	if err != nil {
		if err == sql.ErrNoRows {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "planet not found",
			})
		} else {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"error message": "error scanning planet by id",
			})
		}
		return
	}
	ctx.JSON(http.StatusOK, getPlanet)
}

func deletePlanet(ctx *gin.Context) {
	id := ctx.Param("id")

	result, err := db.Exec("DELETE FROM planets WHERE id=$1", id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "error deleting planet",
		})
		log.Printf("error deleting planet: %v", err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "error getting rows affected",
		})
		log.Printf("error getting rows affected: %v", err)
		return
	}

	if rowsAffected == 0 {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "Planet not found",
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Planeted deleted",
	})
}

func updatePlanet(ctx *gin.Context) {

}

func addPlanet(ctx *gin.Context) {
	var newPlanet Planet

	err := ctx.BindJSON(&newPlanet)
	if err != nil {
		ctx.JSON(http.StatusOK, gin.H{
			"error": "Invalid request bvody",
		})
		return
	}

	var id int
	err = db.QueryRow("INSERT INTO planets (name) VALUES ($1) RETURNING id", newPlanet.Name).Scan(&id)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error message": "error adding a planet",
		})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"message":     "planet added succesfully",
		"inserted_id": id,
		"name":        newPlanet.Name,
	})
}
