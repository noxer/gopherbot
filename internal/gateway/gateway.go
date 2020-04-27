// Package gateway provides the gopher gateway server
package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis"
	"github.com/rs/zerolog"
	"github.com/theckman/gopher2/config"
	"github.com/theckman/gopher2/workqueue"
)

type server struct {
	l *zerolog.Logger
	q *workqueue.Q
}

// RunServer starts the gateway HTTP server.
func RunServer(cfg config.C) error {
	// set up signal catching
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM, syscall.SIGINT)

	// set up zerolog
	zerolog.TimestampFieldName = "timestamp"
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	// set up logging
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	logger.Level(zerolog.DebugLevel)

	// get redis config ready
	redisOpts := &redis.Options{
		Network:      "tcp",
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  11 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolSize:     20,
		MinIdleConns: 5,
		PoolTimeout:  5 * time.Second,
	}

	// quick Redis test code
	// XXX(theckman): REMOVE ME!
	c := redis.NewClient(redisOpts)
	defer func() { _ = c.Close() }()
	key := fmt.Sprintf("heartbeat:%s:%s", cfg.Heroku.AppName, cfg.Heroku.DynoID)
	res := c.Set(key, time.Now().Unix(), 0)
	if err := res.Err(); err != nil {
		logger.Error().Err(err).Msg("failed to set Redis key")
	}

	// set up the workqueue
	q, err := workqueue.New(workqueue.Config{
		ConsumerName:      cfg.Heroku.DynoID,
		ConsumerGroup:     cfg.Heroku.AppName,
		VisibilityTimeout: 10 * time.Second,
		RedisOptions:      redisOpts,
		Logger:            &logger,
	})
	if err != nil {
		return fmt.Errorf("failed to build workqueue: %w", err)
	}

	// set up the server
	srv := server{
		l: &logger,
		q: q,
	}

	// set up the router
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleNotFound)
	mux.HandleFunc("/_ruok", srv.handleRUOK)
	mux.HandleFunc("/slack/event", srv.handleSlackEvent)

	socketAddr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	logger.Info().Str("addr", socketAddr).Msg("binding to TCP socket")

	// set up the network socket
	listener, err := net.Listen("tcp", socketAddr)
	if err != nil {
		return fmt.Errorf("failed to open HTTP socket: %w", err)
	}

	defer func() { _ = listener.Close() }()

	// set up the HTTP server
	httpSrvr := &http.Server{
		Handler:     mux,
		ReadTimeout: 20 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	serveStop, serverShutdown := make(chan struct{}), make(chan struct{})
	var serveErr, shutdownErr error

	// HTTP server parent goroutine
	go func() {
		defer close(serveStop)
		serveErr = httpSrvr.Serve(listener)
	}()

	// signal handling / graceful shutdown goroutine
	go func() {
		defer close(serverShutdown)
		sig := <-signalCh

		logger.Info().Str("signal", sig.String()).Msg("shutting HTTP server down gracefully")

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		if shutdownErr = httpSrvr.Shutdown(ctx); shutdownErr != nil {
			logger.Error().Err(shutdownErr).Msg("failed to gracefully shut down HTTP server")
		}
	}()

	// wait for it to die
	<-serverShutdown
	<-serveStop

	// log errors for informational purposes
	logger.Info().Interface("serve_err", serveErr).Interface("shutdown_err", shutdownErr).Msg("server shut down")

	return nil
}
