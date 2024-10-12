package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/porebric/logger"
	"github.com/prometheus/client_golang/prometheus"
)

type exporter struct {
	docker   *client.Client
	services []string
	all      bool
	ctx      context.Context
}

func New(ctx context.Context, docker *client.Client, services []string, all bool) *exporter {
	e := &exporter{docker: docker, ctx: ctx}
	if !all {
		containers, err := e.docker.ContainerList(context.Background(), container.ListOptions{All: true})
		if err != nil {
			logger.Fatal(ctx, "cannot list containers", "error", err)
			return nil
		}

		for _, service := range services {
			find := false
			for _, c := range containers {
				if service == containerName(c) {
					find = true
				}
			}

			if !find {
				logger.Fatal(ctx, "invalid service in configs", "service", service)
			}
		}
	}
	return &exporter{docker: docker, ctx: ctx, services: services, all: all}
}

func (e *exporter) Describe(_ chan<- *prometheus.Desc) {

}

func (e *exporter) Collect(ch chan<- prometheus.Metric) {
	ctx := e.ctx

	containers, err := e.docker.ContainerList(
		context.Background(),
		container.ListOptions{All: true},
	)
	if err != nil {
		logger.Fatal(ctx, "cannot list containers", "error", err)
		return
	}

	var wg sync.WaitGroup
	totalMemoryCounter := new(TotalMemoryCounter)
	for _, c := range containers {
		c := c
		if !e.all && !slices.Contains(e.services, containerName(c)) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.collectContainer(&c, ch, totalMemoryCounter); err != nil {
				logger.Error(ctx, err, "cannot collect container", "container", containerName(c))
			}
		}()
	}
	wg.Wait()

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("total_memory_usage_bytes", "", []string{}, nil),
		prometheus.GaugeValue,
		float64(totalMemoryCounter.Get()),
	)

	totalMemoryCounter.Clear()
}

func (e *exporter) collectContainer(c *types.Container, ch chan<- prometheus.Metric, totalMemoryCounter *TotalMemoryCounter) error {
	labelsNames := []string{
		"name", "state",
	}

	labelsValues := []string{
		containerName(*c),
		c.State,
	}

	if c.State != "running" {
		return nil
	}

	var stats types.StatsJSON
	statsReader, err := e.docker.ContainerStatsOneShot(context.TODO(), c.ID)
	if err != nil {
		return fmt.Errorf("cannot get stats: %v", err)
	}
	defer statsReader.Body.Close()
	err = json.NewDecoder(statsReader.Body).Decode(&stats)
	if err != nil {
		return fmt.Errorf("cannot decode stats: %v", err)
	}

	containerInfo, err := e.docker.ContainerInspect(context.TODO(), c.ID)
	if err != nil {
		return fmt.Errorf("cannot inspect container: %v", err)
	}

	if containerInfo.SizeRw != nil {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("disk_usage_bytes", "", labelsNames, nil),
			prometheus.GaugeValue,
			float64(*containerInfo.SizeRw),
			labelsValues...,
		)
	} else {
		logger.Error(e.ctx, nil, "SizeRw is nil")
	}

	if containerInfo.SizeRootFs != nil {
		// Полный размер файловой системы
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc("disk_size_rootfs_bytes", "", labelsNames, nil),
			prometheus.GaugeValue,
			float64(*containerInfo.SizeRootFs),
			labelsValues...,
		)
	} else {
		logger.Error(e.ctx, nil, "SizeRootFs is nil")
	}

	// CPU
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("cpu_seconds_total", "", labelsNames, nil),
		prometheus.CounterValue,
		nsToS(stats.CPUStats.CPUUsage.TotalUsage),
		labelsValues...,
	)

	// Memory
	memoryBytes := stats.MemoryStats.Usage
	cacheKey := "total_inactive_file"
	if _, isCgroupV1 := stats.MemoryStats.Stats["total_inactive_file"]; !isCgroupV1 {
		cacheKey = "inactive_file"
	}
	if cacheBytes, ok := stats.MemoryStats.Stats[cacheKey]; ok {
		if memoryBytes < cacheBytes {
			memoryBytes = 0
		} else {
			memoryBytes -= cacheBytes
		}
	}

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("memory_usage_bytes", "", labelsNames, nil),
		prometheus.GaugeValue,
		float64(memoryBytes),
		labelsValues...,
	)

	totalMemoryCounter.Add(memoryBytes)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("memory_limit_bytes", "", labelsNames, nil),
		prometheus.GaugeValue,
		float64(stats.MemoryStats.Limit),
		labelsValues...,
	)

	// Network
	var rxBytes, txBytes uint64
	for _, network := range stats.Networks {
		rxBytes += network.RxBytes
		txBytes += network.TxBytes
	}

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("network_rx_bytes_total", "", labelsNames, nil),
		prometheus.CounterValue,
		float64(rxBytes),
		labelsValues...,
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc("network_tx_bytes_total", "", labelsNames, nil),
		prometheus.CounterValue,
		float64(txBytes),
		labelsValues...,
	)

	// Block I/O
	var readBytes, writeBytes uint64
	for _, blkioStat := range stats.BlkioStats.IoServiceBytesRecursive {
		switch blkioStat.Op {
		case "read":
			readBytes += blkioStat.Value
		case "write":
			writeBytes += blkioStat.Value
		}
	}

	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
		"blkio_read_bytes_total", "",
		labelsNames, nil),
		prometheus.CounterValue,
		float64(readBytes),
		labelsValues...)

	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
		"blkio_write_bytes_total", "",
		labelsNames, nil),
		prometheus.CounterValue,
		float64(writeBytes),
		labelsValues...)

	// PIDs
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
		"pids", "",
		labelsNames, nil),
		prometheus.GaugeValue,
		float64(stats.PidsStats.Current),
		labelsValues...)

	return nil
}
