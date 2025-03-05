package crudo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// TokenClaims 定义token的声明结构
type TokenClaims struct {
	Subject   string `json:"sub"`
	ExpiresAt int64  `json:"exp"`
}

// TokenStore 定义token存储接口
type TokenStore interface {
	SaveToken(token string, userId string, userType string, expireAt time.Time) error
	GetToken(token string) (string, string, error)
	DeleteToken(token string) error
	GetTokensOfUser(userId string, userType string) []string
	GenerateToken() string
}

// RedisTokenStore 实现基于Redis的token存储
type RedisTokenStore struct {
	client *redis.Client
}

func NewRedisTokenStore(client *redis.Client) TokenStore {
	return &RedisTokenStore{client: client}
}

func (s *RedisTokenStore) GenerateToken() string {
	return uuid.New().String()
}

func (s *RedisTokenStore) SaveToken(token string, userId string, userType string, expireAt time.Time) error {
	data := map[string]string{
		"userId":   userId,
		"userType": userType,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.client.Set(context.Background(), token, string(jsonData), time.Until(expireAt)).Err()
}

func (s *RedisTokenStore) GetToken(token string) (string, string, error) {
	jsonData, err := s.client.Get(context.Background(), token).Result()
	if err != nil {
		return "", "", err
	}
	var data map[string]string
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return "", "", err
	}
	return data["userId"], data["userType"], nil
}

func (s *RedisTokenStore) DeleteToken(token string) error {
	return s.client.Del(context.Background(), token).Err()
}

func (s *RedisTokenStore) GetTokensOfUser(userId string, userType string) []string {
	pattern := fmt.Sprintf("*%s*%s*", userId, userType)
	tokens, _ := s.client.Keys(context.Background(), pattern).Result()
	return tokens
}

// TokenMiddleware 创建一个基于token的中间件
func TokenMiddleware(tokenKey string, tokenExpire time.Duration, tokenSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(tokenKey)
		if token == "" {
			RenderJson(c, 401, "token is empty", nil)
			c.Abort()
			return
		}
		claims, err := ParseToken(token, tokenSecret)
		if err != nil {
			RenderJson(c, 401, err.Error(), nil)
			c.Abort()
			return
		}
		if claims.ExpiresAt < time.Now().Unix() {
			RenderJson(c, 401, "token expired", nil)
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

// TokenMiddlewareWithRedis 创建一个基于Redis的token中间件
func TokenMiddlewareWithRedis(tokenKey string, tokenExpire time.Duration, tokenSecret string, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(tokenKey)
		if token == "" {
			RenderJson(c, 401, "token is empty", nil)
			c.Abort()
			return
		}
		claims, err := ParseToken(token, tokenSecret)
		if err != nil {
			RenderJson(c, 401, err.Error(), nil)
			c.Abort()
			return
		}
		if claims.ExpiresAt < time.Now().Unix() {
			RenderJson(c, 401, "token expired", nil)
			c.Abort()
			return
		}
		// 从redis中获取token
		val, err := redisClient.Get(c, token).Result()
		if err != nil {
			RenderJson(c, 401, "token not found", nil)
			c.Abort()
			return
		}
		if val != claims.Subject {
			RenderJson(c, 401, "token invalid", nil)
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

// ParseToken 解析token
func ParseToken(token string, secret string) (*TokenClaims, error) {
	// 这里需要实现具体的token解析逻辑
	// 为了测试，我们先返回一个简单的实现
	if token == "" {
		return nil, errors.New("token is empty")
	}
	return &TokenClaims{
		Subject:   "test",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, nil
}

var store TokenStore

func SetStore(tokenStore TokenStore) {
	store = tokenStore
}

func GenTokenForUser(userId string, userType string, expire time.Duration) (string, error) {
	token := store.GenerateToken()
	expireAt := time.Now().Add(expire)
	err := store.SaveToken(token, userId, userType, expireAt)
	return token, err
}

func CheckToken(token string) bool {
	_, _, err := store.GetToken(token)
	return err == nil
}

func CheckTokenGin(c *gin.Context) {
	token := c.GetHeader("token")
	if token == "" {
		RenderJson(c, 401, "unauthorized", nil)
		c.Abort()
		return
	}
	userId, _, err := store.GetToken(token)
	if err != nil || userId == "" {
		RenderJson(c, 401, "unauthorized", nil)
		c.Abort()
		return
	}
	c.Set("userId", userId)
	c.Next()
}

func GetTokensOfUser(userId string, userType string) []string {
	return store.GetTokensOfUser(userId, userType)
}
