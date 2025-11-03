package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

type Planet struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var db *sql.DB
var rdb *redis.Client
var ctx = context.Background()

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

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		fmt.Printf("error redis addr")
		return
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})

	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Coul;d not connect to redis: %v", err)
	}
	log.Println("connected to redis")

	r := gin.Default()

	r.Use(rateLimiterMiddleware())

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

func getPlanets(c *gin.Context) {
	cacheKey := "planet:all"
	cachedData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		redisPlanets := []Planet{}
		if json.Unmarshal([]byte(cachedData), &redisPlanets) == nil {
			c.JSON(http.StatusOK, redisPlanets)
			return
		}
	}

	rows, err := db.Query("SELECT id, name FROM planets")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
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
			c.JSON(http.StatusBadRequest, gin.H{
				"error message": "error scanning planets",
			})
			log.Printf("error scanning planets: %v", err)
			return
		}
		planets = append(planets, p)

	}
	planetJson, err := json.Marshal(planets)
	if err == nil {
		rdb.Set(ctx, cacheKey, planetJson, 5*time.Minute)
	}

	c.JSON(http.StatusOK, planets)
}

func getPlanetById(c *gin.Context) {
	id := c.Param("id")

	cacheKey := fmt.Sprintf("planet:%s", id)
	cachedData, err := rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var planet Planet
		if json.Unmarshal([]byte(cachedData), &planet) == nil {
			c.JSON(http.StatusOK, planet)
			return
		}
	}

	row := db.QueryRow("SELECT id, name FROM planets WHERE id=$1", id)

	var getPlanet Planet
	err = row.Scan(&getPlanet.ID, &getPlanet.Name)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "planet not found",
			})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{
				"error message": "error scanning planet by id",
			})
		}
		return
	}

	planetJson, err := json.Marshal(getPlanet)
	if err == nil {
		rdb.Set(ctx, cacheKey, planetJson, 5*time.Minute)
	}

	c.JSON(http.StatusOK, getPlanet)
}

func deletePlanet(c *gin.Context) {
	id := c.Param("id")

	result, err := db.Exec("DELETE FROM planets WHERE id=$1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "error deleting planet",
		})
		log.Printf("error deleting planet: %v", err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "error getting rows affected",
		})
		log.Printf("error getting rows affected: %v", err)
		return
	}

	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Planet not found",
		})
		return
	}

	cacheKey := fmt.Sprintf("planet:%s", id)
	err = rdb.Del(ctx, cacheKey).Err()
	if err != nil {
		log.Printf("failed to invalidate cache for planet%s : %v", id, err)
	} else {
		log.Printf("Cache invalidated for planet:%s", id)
	}
	cacheKey = "planet:all"
	err = rdb.Del(ctx, cacheKey).Err()
	if err != nil {
		log.Printf("failed to invalidate cache for planets : %v", err)
	} else {
		log.Printf("Cache invalidated for planes")
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Planeted deleted",
	})
}

func updatePlanet(c *gin.Context) {
	id := c.Param("id")

	var updatedPlanet Planet
	err := c.BindJSON(&updatedPlanet)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	result, err := db.Exec("UPDATE planets SET name=$1 WHERE id=$2", updatedPlanet.Name, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating"})
		log.Printf("Error updating: %v", err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error affecrted rows"})
		return
	}

	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Planet not found"})
		return
	}
	cacheKey := fmt.Sprintf("planet:%s", id)
	err = rdb.Del(ctx, cacheKey).Err()
	if err != nil {
		log.Printf("failed to invalidate cache for planet%s : %v", id, err)
	} else {
		log.Printf("Cache invalidated for planet:%s", id)
	}
	cacheKey = "planet:all"
	err = rdb.Del(ctx, cacheKey).Err()
	if err != nil {
		log.Printf("failed to invalidate cache for planets : %v", err)
	} else {
		log.Printf("Cache invalidated for planes")
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Planet updated",
		"id":      id,
		"name":    updatedPlanet.Name,
	})
}

func addPlanet(c *gin.Context) {
	var newPlanet Planet

	err := c.BindJSON(&newPlanet)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request bvody",
		})
		return
	}

	var id int
	err = db.QueryRow("INSERT INTO planets (name) VALUES ($1) RETURNING id", newPlanet.Name).Scan(&id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error message": "error adding a planet",
		})
		return
	}
	cacheKey := "planet:all"
	err = rdb.Del(ctx, cacheKey).Err()
	if err != nil {
		log.Printf("failed to invalidate cache for planets : %v", err)
	} else {
		log.Printf("Cache invalidated for planes")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "planet added succesfully",
		"inserted_id": id,
		"name":        newPlanet.Name,
	})
}

func rateLimiterMiddleware() gin.HandlerFunc {
	limit := 20
	window := 1 * time.Minute

	return func(c *gin.Context) {
		key := fmt.Sprintf("rate_limit:%s", c.ClientIP())

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			log.Printf("Error redis limiter: %v", err)
			c.Next()
			return
		}

		if count == 1 {
			rdb.Expire(ctx, key, window)
		}

		if count > int64(limit) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests",
			})
			return
		}
		c.Next()
	}
}
