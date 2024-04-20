package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/porebric/logger"
	"github.com/prometheus/client_golang/prometheus"
)

type exporter struct {
	docker      *client.Client
	extraLabels map[string]*template.Template

	logEntity *logger.Logger
}

func New(docker *client.Client, extraLabels map[string]*template.Template, logEntity *logger.Logger) *exporter {
	return &exporter{docker: docker, extraLabels: extraLabels, logEntity: logEntity}
}

func (e *exporter) Describe(ch chan<- *prometheus.Desc) {
	labels := make([]string, len(e.extraLabels))
	for key, _ := range e.extraLabels {
		labels = append(labels, key)
	}
	ch <- prometheus.NewDesc("validate", "", labels, nil)
}

func (e *exporter) Collect(ch chan<- prometheus.Metric) {
	ctx := logger.ToContext(context.Background(), e.logEntity)
	containers, err := e.docker.ContainerList(
		context.Background(),
		container.ListOptions{All: true},
	)
	if err != nil {
		logger.Fatal(ctx, "cannot list containers", "error", err)
		return
	}

	var wg sync.WaitGroup
	for _, container := range containers {
		container := container
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.collectContainer(&container, ch); err != nil {
				logger.Fatal(ctx, "cannot collect container", "container", container.ID, "error", err)
			}
		}()
	}
	wg.Wait()
}

func (e *exporter) collectContainer(container *types.Container, ch chan<- prometheus.Metric) error {
	containerJson, err := e.docker.ContainerInspect(context.TODO(), container.ID)
	if err != nil {
		return err
	}

	labelsNames := []string{"name"}
	labelsValues := []string{strings.Trim(container.Names[0], "/")}
	for labelName, labelTemplate := range e.extraLabels {
		templateData := struct {
			Container     *types.Container
			ContainerJSON types.ContainerJSON
		}{
			container,
			containerJson,
		}
		var labelValue bytes.Buffer
		if err = labelTemplate.Execute(&labelValue, templateData); err != nil {
			return err
		}
		labelsNames = append(labelsNames, labelName)
		labelsValues = append(labelsValues, labelValue.String())
	}

	// Info
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
		"docker_container_info", "",
		labelsNames, nil),
		prometheus.GaugeValue,
		1,
		labelsValues...)

	if container.State != "running" {
		return nil
	}

	var stats types.StatsJSON
	statsReader, err := e.docker.ContainerStatsOneShot(context.TODO(), container.ID)
	if err != nil {
		return fmt.Errorf("cannot get stats: %v", err)
	}
	defer statsReader.Body.Close()
	err = json.NewDecoder(statsReader.Body).Decode(&stats)
	if err != nil {
		return fmt.Errorf("cannot decode stats: %v", err)
	}

	// CPU
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
		"docker_container_cpu_seconds_total", "",
		labelsNames, nil),
		prometheus.CounterValue,
		nsToS(stats.CPUStats.CPUUsage.TotalUsage),
		labelsValues...)

	// Memory
	{
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

		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
			"docker_container_memory_usage_bytes", "",
			labelsNames, nil),
			prometheus.GaugeValue,
			float64(memoryBytes),
			labelsValues...)

		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
			"docker_container_memory_limit_bytes", "",
			labelsNames, nil),
			prometheus.GaugeValue,
			float64(stats.MemoryStats.Limit),
			labelsValues...)
	}

	// Network
	{
		var rxBytes, txBytes uint64
		for _, network := range stats.Networks {
			rxBytes += network.RxBytes
			txBytes += network.TxBytes
		}

		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
			"docker_container_network_rx_bytes_total", "",
			labelsNames, nil),
			prometheus.CounterValue,
			float64(rxBytes),
			labelsValues...)

		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
			"docker_container_network_tx_bytes_total", "",
			labelsNames, nil),
			prometheus.CounterValue,
			float64(txBytes),
			labelsValues...)
	}

	// Block I/O
	{
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
			"docker_container_blkio_read_bytes_total", "",
			labelsNames, nil),
			prometheus.CounterValue,
			float64(readBytes),
			labelsValues...)

		ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
			"docker_container_blkio_write_bytes_total", "",
			labelsNames, nil),
			prometheus.CounterValue,
			float64(writeBytes),
			labelsValues...)
	}

	// PIDs
	ch <- prometheus.MustNewConstMetric(prometheus.NewDesc(
		"docker_container_pids", "",
		labelsNames, nil),
		prometheus.GaugeValue,
		float64(stats.PidsStats.Current),
		labelsValues...)

	return nil
}
