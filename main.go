package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"database/sql"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// Use an environment variable for the secret key in production
var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

var db *sql.DB

func main() {
	// Load .env file from the current directory
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found")
	}

	// Build connection string from environment variables
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_SSLMODE"),
	)

	log.Println("Step 1: Starting SBS application...")
	db, err = sql.Open("postgres", connStr) // Use the dynamically built string
	if err != nil {
		log.Fatal("DB CONNECTION FAILED:", err)
	}
	log.Println("Step 2: Database connection opened (not yet verified).")

	// Ping the database to verify the connection is alive.
	err = db.Ping()
	if err != nil {
		log.Fatal("DB PING FAILED:", err)
	}
	log.Println("Step 3: Database ping successful.")

	defer db.Close()

	// Fallback for local development if JWT_SECRET is not set
	if len(jwtSecret) == 0 {
		log.Println("Step 4: JWT_SECRET not set, using default.")
		jwtSecret = []byte("a_very_secret_key_for_dev")
	}

	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	log.Println("Step 5: Gin engine created and CORS configured.")

	// Public routes
	r.POST("/register", register)
	r.POST("/login", login)
	r.GET("/leaderboard", leaderboard)

	// Authenticated routes
	auth := r.Group("/")
	auth.Use(authMiddleware())
	{
		auth.GET("/profile", profile)
		auth.POST("/betslip", postBetslip)
		// New endpoint for posting comments
		auth.POST("/betslips/:id/comments", postComment)
	}
	log.Println("Step 6: All routes configured.")

	fmt.Println("SBS LIVE → http://localhost:8080")
	r.Run(":8080")
}

// authMiddleware validates the JWT token
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Attach user ID to the context for use in subsequent handlers
			c.Set("user_id", claims["user_id"])
		} else {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			return
		}

		c.Next()
	}
}

// REGISTER
func register(c *gin.Context) {
	var u struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	_, err = db.Exec(`INSERT INTO users(username,email,password_hash,role) VALUES($1,$2,$3,'punter') ON CONFLICT (username) DO NOTHING`, u.Username, u.Email, string(hashedPassword))
	if err != nil {
		// This could be a unique constraint violation on email, etc.
		c.JSON(http.StatusConflict, gin.H{"error": "Username or email already exists"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Welcome " + u.Username + "!"})
}

// LOGIN
func login(c *gin.Context) {
	var u struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var id int
	var hashedPassword string
	err := db.QueryRow("SELECT id, password_hash FROM users WHERE username=$1", u.Username).Scan(&id, &hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(u.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": id,
		"exp":     time.Now().Add(time.Hour * 72).Unix(), // Token expires in 3 days
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenString})
}

func postBetslip(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	var b struct {
		Platform string          `json:"platform"`
		Games    json.RawMessage `json:"games"`
	}
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// New betslips should always be 'pending' and should not affect user stats yet.
	// The platform should come from the request.
	var betslipID int
	err := db.QueryRow("INSERT INTO betslips(user_id, platform, games, status) VALUES($1, $2, $3, 'pending') RETURNING id", userID, b.Platform, b.Games).Scan(&betslipID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return a meaningful success message
	c.JSON(http.StatusCreated, gin.H{"message": "Betslip posted successfully!", "betslip_id": betslipID})
}

func profile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	var w, l int
	err := db.QueryRow("SELECT total_wins, total_losses FROM users WHERE id=$1", userID).Scan(&w, &l)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"wins": w, "losses": l})
}

func postComment(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	betslipID := c.Param("id")

	var comment struct {
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&comment); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if comment.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Comment content cannot be empty"})
		return
	}

	_, err := db.Exec(
		"INSERT INTO comments (user_id, betslip_id, content) VALUES ($1, $2, $3)",
		userID, betslipID, comment.Content,
	)

	if err != nil {
		// A foreign key constraint violation might happen if the betslip_id is invalid
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code.Name() == "foreign_key_violation" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Betslip not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to post comment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Comment posted successfully"})
}

func leaderboard(c *gin.Context) {
	punterType := c.Query("type")
	query := "SELECT username, total_wins, total_losses, win_rate FROM users"
	args := []interface{}{}

	if punterType == "ai" {
		query += " WHERE role = 'ai_punter'"
	} else if punterType == "human" {
		query += " WHERE role != 'ai_punter'"
	}

	query += " ORDER BY win_rate DESC LIMIT 10"

	rows, err := db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var lb []gin.H
	for rows.Next() {
		var u string
		var w, l int
		var wr float64
		rows.Scan(&u, &w, &l, &wr)
		lb = append(lb, gin.H{"user": u, "wins": w, "losses": l, "rate": wr})
	}
	c.JSON(200, lb)
}
