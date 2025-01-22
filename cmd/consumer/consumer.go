package main

import (
	"fmt"
	"github.com/viswals_task/controller"
	"github.com/viswals_task/core/services"
	"github.com/viswals_task/internal/logger"
	"github.com/viswals_task/pkg/database"
	"github.com/viswals_task/pkg/rabbitmq"
	"github.com/viswals_task/pkg/redis"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	DevEnvironment     = "dev"
	defaultRedisTTLStr = "60s"
	defaultBufferSize  = "50"
)

func main() {
	// initializing logger
	log, err := logger.Init(os.Stdout, strings.ToLower(os.Getenv("ENVIRONMENT")) == DevEnvironment)
	if err != nil {
		fmt.Printf("can't initialise logger throws error : %v", err)
		return
	}

	// initialize environment variables.
	var DbUrl string
	var InMemUrl string
	var QueueUrl string

	if url, ok := os.LookupEnv("POSTGRES_CONNECTION_STRING"); ok {
		DbUrl = url
	} else {
		log.Error("postgres connection string is not set using default url")
		return
	}

	if url, ok := os.LookupEnv("REDIS_CONNECTION_STRING"); ok {
		InMemUrl = url
	} else {
		log.Error("redis connection string is not set using default url")
		return
	}

	if url, ok := os.LookupEnv("RABBITMQ_CONNECTION_STRING"); ok {
		QueueUrl = url
	} else {
		log.Error("rabbitmq connection string is not set using default url")
		return
	}

	// initializing database service.
	dataStore, err := database.New(DbUrl)
	if err != nil {
		log.Error("can't initialise database throws error", zap.Error(err))
		return
	}

	doMigration := strings.ToLower(os.Getenv("MIGRATION")) == "true"

	if doMigration {
		dbName, ok := os.LookupEnv("DATABASE_NAME")
		if !ok {
			log.Error("database name is not set can't process migration")
			return
		}
		err := dataStore.Migrate(dbName)
		if err != nil {
			log.Error("can't migrate database throws error", zap.Error(err))
			return
		}
	}

	ttlstr, ok := os.LookupEnv("REDIS_TTL")
	if !ok {
		ttlstr = defaultRedisTTLStr
		log.Warn("redis TTL is not set using default values, you can specify ttl with REDIS_TTL flag", zap.String("ttl", ttlstr))
	}

	ttl, err := time.ParseDuration(ttlstr)
	if err != nil {
		log.Error("error fetching redis TTL throws error", zap.Error(err), zap.String("ttl", ttlstr))
		return
	}

	// initializing caching service.
	memStore, err := redis.New(InMemUrl, ttl)
	if err != nil {
		log.Error("can't initialise redis throws error", zap.Error(err))
		return
	}

	queueName, ok := os.LookupEnv("RABBITMQ_QUEUE_NAME")
	if !ok {
		log.Error("queue name is not set using environment variable 'QUEUE_NAME'")
		return
	}

	// initializing queuing service.
	queueService, err := rabbitmq.New(QueueUrl, queueName)
	if err != nil {
		log.Error("can't initialise rabbitmq throws error", zap.Error(err))
		return
	}

	consumer, err := services.NewConsumer(queueService, dataStore, memStore, log)
	if err != nil {
		log.Error("can't initialise database throws error", zap.Error(err))
		return
	}

	BufferSize, ok := os.LookupEnv("CHANNEL_SIZE")
	if !ok {
		log.Warn("Buffer size is not set using environment variable 'CHANNEL_SIZE', using default buffer size", zap.Any("buffer_size", defaultBufferSize))
		BufferSize = defaultBufferSize
	}

	bufferSize, err := strconv.Atoi(BufferSize)
	if err != nil {
		log.Error("error parsing buffer size throws error", zap.Error(err), zap.String("buffer_size", BufferSize))
		return
	}

	// create a separate go routine to handle upcoming data.
	wg := &sync.WaitGroup{}
	log.Info("starting consumer")
	go consumer.Consume(wg, bufferSize)

	// initialize user service.
	userService := services.NewUserService(dataStore, memStore, log)

	// initialize controller service.
	ctl := controller.New(userService, log)

	// initialize router
	registerRouter(ctl)

	// starts the http service.
	httpPort, ok := os.LookupEnv("HTTP_PORT")
	if !ok {
		log.Warn("http port is not set using environment variable 'HTTP_PORT', Using Default Port 5000")
		httpPort = "5000"
	}

	log.Info("starting http server on port " + httpPort)
	err = http.ListenAndServe(":"+httpPort, nil)
	if err != nil {
		log.Error("can't initialise database throws error", zap.Error(err))
		// DO not return as we have to wait for other go routines to complete successfully.
	}

	log.Info("stopping consumer")
	log.Info("stopping http server on port " + httpPort)

	wg.Wait()
}

func registerRouter(ctl *controller.Controller) {
	http.HandleFunc("GET /users", ctl.GetAllUsers)
	http.HandleFunc("GET /users/{id}", ctl.GetUser)
	http.HandleFunc("POST /users", ctl.CreateUser)
	http.HandleFunc("DELETE /users/{id}", ctl.DeleteUser)
	http.HandleFunc("GET /users/sse", ctl.GetAllUsersSSE)
	http.Handle("/", http.FileServer(http.Dir("./client")))
}
