package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/docker/docker/client"
	"github.com/porebric/logger"
	"github.com/porebric/services-metrics/internal/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const envPrefix = "SERVICE_"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx = logger.ToContext(ctx, logger.New(
		logger.InfoLevel,
		logger.WithPlainText(),
	))

	var services []string
	for _, env := range os.Environ() {
		name, value, _ := strings.Cut(env, "=")
		if strings.HasPrefix(name, envPrefix) {
			services = append(services, value)
		}
	}

	addr := ":9338"
	if os.Getenv("ADDR") != "" {
		addr = os.Getenv("ADDR")
	}

	docker, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		logger.Fatal(ctx, "cannot create docker client", "error", err)
	}

	all, err := strconv.ParseBool(os.Getenv("ALL_SERVICES"))
	if err == nil {
		all = false
	}

	if !all && len(services) == 0 {
		// если сервисов нет, то берём все, даже если флаг false
		all = true
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		exporter.New(
			docker,
			services,
			all,
			logger.FromContext(ctx),
		),
	)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})

	http.Handle("/metrics", handler)

	http.Handle("/", http.RedirectHandler("/metrics", http.StatusMovedPermanently))

	logger.Info(ctx, "listening", "addr", addr)

	go func() {
		if err = http.ListenAndServe(addr, nil); err != nil {
			logger.Error(ctx, err, "listening")
			stop()
		}
	}()

	<-ctx.Done()
}
