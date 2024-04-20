package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"

	"github.com/docker/docker/client"
	"github.com/porebric/configs"
	"github.com/porebric/logger"
	"github.com/porebric/services-metrics/internal/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	configsKeysPath = "./config/configs_keys.yml"
	configsPath     = "./config/configs.yml"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx = logger.ToContext(ctx, logger.New(
		logger.InfoLevel,
		logger.WithPlainText(),
	))

	keysReader, err := os.Open(configsKeysPath)
	if err != nil {
		logger.Panic(ctx, "read config keys", "error", err)
	}

	confReader, err := os.Open(configsPath)
	if err != nil {
		logger.Fatal(ctx, "read config value", "error", err)
	}

	if err = configs.New().KeysReader(keysReader).YamlConfigs(confReader).Init(ctx); err != nil {
		logger.Fatal(ctx, "read config file", "error", err)
	}

	extraLabels := make(map[string]*template.Template)
	envPrefix := "LABEL_"
	for _, env := range os.Environ() {
		name, value, _ := strings.Cut(env, "=")
		if strings.HasPrefix(name, envPrefix) {
			label := strings.TrimPrefix(name, envPrefix)
			tmpl, err := template.New(label).Parse(value)
			if err != nil {
				logger.Fatal(ctx, fmt.Sprintf("invalid template for label %s: %v", label, err))
			}
			extraLabels[label] = tmpl
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

	registry := prometheus.NewRegistry()
	registry.MustRegister(exporter.New(docker, extraLabels, logger.FromContext(ctx)))

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
