package exporter

import (
	"strings"
	"time"

	"github.com/docker/docker/api/types"
)

func nsToS(ns uint64) float64 {
	return float64(ns) / float64(time.Second)
}

func containerName(container types.Container) string {
	return strings.Trim(container.Names[0], "/")
}
