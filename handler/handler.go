package handler

import (
	"fmt"
	"html/template"
	"path/filepath"
	"runtime"
	"time"

	task "github.com/gjbae1212/go-async-task"
	badge "github.com/gjbae1212/go-counter-badge/badge"
	websocket "github.com/gjbae1212/go-ws-broadcast"
	"github.com/gjbae1212/hit-counter/counter"
	"github.com/gjbae1212/hit-counter/env"
	"github.com/gjbae1212/hit-counter/internal"
	"github.com/gjbae1212/hit-counter/limiter"
	"github.com/go-redis/redis/v8"
	cache "github.com/patrickmn/go-cache"
)

var (
	iconsMap  map[string]badge.Icon
	iconsList []map[string]string
)

type Handler struct {
	Counter          counter.Counter
	Limiter          limiter.Limiter
	LocalCache       *cache.Cache
	AsyncTask        task.Keeper
	WebSocketBreaker websocket.Breaker
	IndexTemplate    *template.Template
	Badge            badge.Writer
	Icons            map[string]badge.Icon
	IconsList        []map[string]string
}

// NewHandler creates  handler object.
func NewHandler(redisAddr string) (*Handler, error) {
	if redisAddr == "" {
		return nil, fmt.Errorf("[err] NewHandler %w", internal.ErrorEmptyParams)
	}

	// create local cache
	localCache := cache.New(24*time.Hour, 10*time.Minute)

	// create redis-client.
	redisClient := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		Password:     "",
		DB:           0,
		MaxRetries:   1,
		MinIdleConns: runtime.NumCPU() * 3,
		PoolSize:     runtime.NumCPU() * 10,
	})

	// create counter interface.
	ctr, err := counter.NewCounter(counter.WithRedisClient(redisClient))
	if err != nil {
		return nil, fmt.Errorf("[err] NewHandler %w", err)
	}

	// create limiter interface.
	ltr, err := limiter.NewLimiter(limiter.WithRedisClient(redisClient))
	if err != nil {
		return nil, fmt.Errorf("[err] NewHandler %w", err)
	}

	// create async task
	asyncTask, err := task.NewAsyncTask(
		task.WithQueueSizeOption(1000),
		task.WithWorkerSizeOption(5),
		task.WithTimeoutOption(20*time.Second),
		task.WithErrorHandlerOption(func(err error) {
			internal.SentryError(err)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("[err] NewHandler %w", err)
	}

	// create websocket breaker
	breaker, err := websocket.NewBreaker(websocket.WithMaxReadLimit(1024),
		websocket.WithMaxMessagePoolLength(500),
		websocket.WithErrorHandlerOption(func(err error) {
			internal.SentryError(err)
		}))
	if err != nil {
		return nil, fmt.Errorf("[err] NewHandler %w", err)
	}

	// template
	indexName := "index.html"
	if env.GetPhase() == "local" {
		indexName = "local.html"
	}

	indexTemplate, err := template.ParseFiles(filepath.Join(internal.GetRoot(), "view", indexName))
	if err != nil {
		return nil, fmt.Errorf("[err] NewHandler %w", err)
	}

	// badge generator
	badgeWriter, err := badge.NewWriter()
	if err != nil {
		return nil, fmt.Errorf("[err] NewHandler %w", err)
	}

	return &Handler{
		LocalCache:       localCache,
		Counter:          ctr,
		Limiter:          ltr,
		AsyncTask:        asyncTask,
		WebSocketBreaker: breaker,
		IndexTemplate:    indexTemplate,
		Badge:            badgeWriter,
		Icons:            iconsMap,
		IconsList:        iconsList,
	}, nil
}

func init() {
	iconsMap = badge.GetIconsMap()
	iconsList = make([]map[string]string, 0, len(iconsMap))

	for k, _ := range iconsMap {
		j := make(map[string]string, 2)
		j["name"] = k
		j["url"] = fmt.Sprintf("/icon/%s", k)
		iconsList = append(iconsList, j)
	}
}
