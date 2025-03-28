package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	baseURL = "http://20.244.56.144/test"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Post struct {
	ID      int    `json:"id"`
	UserID  int    `json:"userid"`
	Content string `json:"content"`
}

type Comment struct {
	ID      int    `json:"id"`
	PostID  int    `json:"postid"`
	Content string `json:"content"`
}

type UserPostCount struct {
	UserID string
	Name   string
	Count  int
}

type Cache struct {
	sync.RWMutex
	users          map[string]string
	userPostCounts map[string]int
	posts          []Post
	postComments   map[int]int
	lastUpdated    time.Time
	updateInterval time.Duration
}

var cache = &Cache{
	users:          make(map[string]string),
	userPostCounts: make(map[string]int),
	posts:          make([]Post, 0),
	postComments:   make(map[int]int),
	updateInterval: 30 * time.Second,
}

func (c *Cache) updateData() {
	c.Lock()
	defer c.Unlock()

	if time.Since(c.lastUpdated) < c.updateInterval {
		return
	}

	// Fetch users
	users, err := fetchUsers()
	if err != nil {
		log.Printf("Error fetching users: %v", err)
		return
	}

	newUserPostCounts := make(map[string]int)
	newPosts := make([]Post, 0)
	newPostComments := make(map[int]int)

	// Update users and fetch their posts
	for userID, userName := range users {
		c.users[userID] = userName
		posts, err := fetchUserPosts(userID)
		if err != nil {
			log.Printf("Error fetching posts for user %s: %v", userID, err)
			continue
		}

		newUserPostCounts[userID] = len(posts)
		newPosts = append(newPosts, posts...)

		// Fetch comments for each post
		for _, post := range posts {
			comments, err := fetchPostComments(post.ID)
			if err != nil {
				log.Printf("Error fetching comments for post %d: %v", post.ID, err)
				continue
			}
			newPostComments[post.ID] = len(comments)
		}
	}

	c.userPostCounts = newUserPostCounts
	c.posts = newPosts
	c.postComments = newPostComments
	c.lastUpdated = time.Now()
}

func fetchUsers() (map[string]string, error) {
	resp, err := http.Get(fmt.Sprintf("%s/users", baseURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result["users"], nil
}

func fetchUserPosts(userID string) ([]Post, error) {
	resp, err := http.Get(fmt.Sprintf("%s/users/%s/posts", baseURL, userID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string][]Post
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result["posts"], nil
}

func fetchPostComments(postID int) ([]Comment, error) {
	resp, err := http.Get(fmt.Sprintf("%s/posts/%d/comments", baseURL, postID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string][]Comment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result["comments"], nil
}

func getTopUsers(c *gin.Context) {
	cache.updateData()

	cache.RLock()
	defer cache.RUnlock()

	var users []UserPostCount
	for userID, count := range cache.userPostCounts {
		users = append(users, UserPostCount{
			UserID: userID,
			Name:   cache.users[userID],
			Count:  count,
		})
	}

	sort.Slice(users, func(i, j int) bool {
		return users[i].Count > users[j].Count
	})

	if len(users) > 5 {
		users = users[:5]
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func getPosts(c *gin.Context) {
	postType := c.Query("type")
	if postType != "latest" && postType != "popular" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid post type. Use 'latest' or 'popular'"})
		return
	}

	cache.updateData()

	cache.RLock()
	defer cache.RUnlock()

	switch postType {
	case "popular":
		var maxComments int
		var popularPosts []Post

		// Find max comments
		for _, count := range cache.postComments {
			if count > maxComments {
				maxComments = count
			}
		}

		// Find all posts with max comments
		for _, post := range cache.posts {
			if cache.postComments[post.ID] == maxComments {
				popularPosts = append(popularPosts, post)
			}
		}

		c.JSON(http.StatusOK, gin.H{"posts": popularPosts})

	case "latest":
		posts := make([]Post, len(cache.posts))
		copy(posts, cache.posts)

		sort.Slice(posts, func(i, j int) bool {
			return posts[i].ID > posts[j].ID
		})

		if len(posts) > 5 {
			posts = posts[:5]
		}

		c.JSON(http.StatusOK, gin.H{"posts": posts})
	}
}

func main() {
	r := gin.Default()

	r.GET("/users", getTopUsers)
	r.GET("/posts", getPosts)

	// Start background cache update
	go func() {
		for {
			cache.updateData()
			time.Sleep(cache.updateInterval)
		}
	}()

	log.Fatal(r.Run(":8080"))
}
