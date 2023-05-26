package main

/* 
  Warning.
  The following code is not safe. It has not been tested thoroughly.
  If you'd like to use it yourself, you are free to. However, i would recommend modifying it if you want to use it in production.
*/
import (
	"database/sql"
	"html/template"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

type Song struct {
	ID     int
	Title  string
	Album  string
	Rating int
}

type RateLimiter struct {
	rateLimitMap map[string]map[string]time.Time
	mu           sync.Mutex
}

func main() {
	rateLimiter := &RateLimiter{
		rateLimitMap: make(map[string]map[string]time.Time),
	}

	db, err := sql.Open("mysql", "username:pwd@tcp(ip:3306)/databasenm")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		rows, err := db.Query("SELECT * FROM songs ORDER BY Rating DESC")
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to retrieve songs")
			return
		}
		defer rows.Close()

		songs := []Song{}

		for rows.Next() {
			song := Song{}
			err := rows.Scan(&song.ID, &song.Title, &song.Album, &song.Rating)
			if err != nil {
				log.Println(err)
				continue
			}

			songs = append(songs, song)
		}

		if err := rows.Err(); err != nil {
			log.Println(err)
			c.String(http.StatusInternalServerError, "Failed to retrieve songs")
			return
		}

		data := struct {
			Songs []Song
		}{
			Songs: songs,
		}

		tmpl, err := template.ParseFiles("templates/index.html")
		if err != nil {
			log.Println(err)
			c.String(http.StatusInternalServerError, "Failed to load template")
			return
		}
		err = tmpl.Execute(c.Writer, data)
		if err != nil {
			log.Println(err)
			c.String(http.StatusInternalServerError, "Failed to execute template")
			return
		}
	})

	r.POST("/rate", func(c *gin.Context) {
		ip := getClientIP(c.Request)

		songID := c.PostForm("id")
		action := strings.TrimSpace(c.PostForm("action"))
		rateLimiter.mu.Lock()
		defer rateLimiter.mu.Unlock()

		if songRatings, ok := rateLimiter.rateLimitMap[ip]; ok {
			if _, ok := songRatings[songID]; ok {
				c.JSON(http.StatusConflict, gin.H{"message": "You have already rated this song in the last 24 hours"})
				return
			}
		}

		switch action {
		case "add":
			id, err := strconv.Atoi(songID)
			if err != nil {
				c.String(http.StatusBadRequest, "Invalid song ID")
				return
			}

			song, err := getSongByID(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve song"})
				return
			}

			song.Rating++

			err = updateSong(song)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to update song"})
				return
			}
			if _, ok := rateLimiter.rateLimitMap[ip]; !ok {
				rateLimiter.rateLimitMap[ip] = make(map[string]time.Time)
			}
			rateLimiter.rateLimitMap[ip][songID] = time.Now()
			c.JSON(http.StatusOK, gin.H{"message": "Rating updated successfully"})

		case "remove":
			id, err := strconv.Atoi(songID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Invalid song ID"})
				return
			}

			song, err := getSongByID(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to retrieve song"})
				return
			}
			song.Rating--

			err = updateSong(song)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Failed to update song"})
				return
			}
			if _, ok := rateLimiter.rateLimitMap[ip]; !ok {
				rateLimiter.rateLimitMap[ip] = make(map[string]time.Time)
			}
			rateLimiter.rateLimitMap[ip][songID] = time.Now()

			c.JSON(http.StatusOK, gin.H{"message": "Rating updated successfully"})
		default:
			c.JSON(http.StatusOK, gin.H{"message": "Invalid action"})
		}
	})

	r.Run(":8080")
}
func getSongByID(id int) (*Song, error) {
	db, e := sql.Open("mysql", "username:pwd@tcp(ip:3306)/databasenm")
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	query := "SELECT id, title, album, rating FROM songs WHERE id = ?"
	row := db.QueryRow(query, id)
	song := &Song{}
	err := row.Scan(&song.ID, &song.Title, &song.Album, &song.Rating)
	if err != nil {
		return nil, err
	}
	return song, nil
}
func updateSong(song *Song) error {
	db, e := sql.Open("mysql", "username:pwd@tcp(ip:3306)/databasenm")
	if e != nil {
		log.Fatal(e)
	}
	query := "UPDATE songs SET rating = ? WHERE id = ?"
	_, err := db.Exec(query, song.Rating, song.ID)
	if err != nil {
		return err
	}

	return nil
}
func getClientIP(req *http.Request) string {
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return ""
	}
	return ip
}
