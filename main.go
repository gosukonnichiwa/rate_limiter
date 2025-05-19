package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// Добавляем глобальную переменную для пула подключений к PostgreSQL
var dbPool *pgxpool.Pool

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"path", "status"},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal)
}

func connectPostgres(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func main() {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	// Подключаемся к Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: "redis:6379", // Используем имя контейнера вместо 127.0.0.1
	})

	_, err := redisClient.Ping(context.Background()).Result()
	if err != nil {
		log.Fatal("Failed to connect to Redis: ", err)
	}
	log.Println("Connected to Redis")

	// Подключаемся к PostgreSQL
	postgresConn := "postgres://limiter_user:password@host.docker.internal:5432/rate_limiter?sslmode=disable"
	dbPool, err = connectPostgres(context.Background(), postgresConn)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL: ", err)
	}
	defer dbPool.Close()
	log.Println("Connected to PostgreSQL")

	// Передаем dbPool в middleware
	r.Use(RateLimiterMiddleware(redisClient, dbPool, 5, time.Minute))

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Rate Limiter is working!"})
	})

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.Run(":8080")
}

// Добавляем dbPool в параметры middleware
func RateLimiterMiddleware(redisClient *redis.Client, dbPool *pgxpool.Pool, limit int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		key := "rate_limit:" + clientIP

		count, err := redisClient.Incr(c.Request.Context(), key).Result()
		if err != nil {
			c.AbortWithStatusJSON(500, gin.H{"error": "Internal Server Error"})
			return
		}

		if count == 1 {
			redisClient.Expire(c.Request.Context(), key, window)
		}

		// Логируем запрос ДО проверки лимита, чтобы учитывать все запросы
		_, err = dbPool.Exec(c.Request.Context(), `
			INSERT INTO request_logs (ip_address, path, status_code)
			VALUES ($1, $2, $3)`,
			clientIP,
			c.Request.URL.Path,
			c.Writer.Status(),
		)
		if err != nil {
			log.Printf("Failed to log request: %v", err)
		}

		if count > limit {
			c.AbortWithStatusJSON(429, gin.H{
				"error": "Too many requests",
				"limit": limit,
				"reset": window.Seconds(),
			})
			return
		}

		// В middleware увеличивайте счетчик:
		requestsTotal.WithLabelValues(c.Request.URL.Path, strconv.Itoa(c.Writer.Status())).Inc()

		c.Next()
	}
}
