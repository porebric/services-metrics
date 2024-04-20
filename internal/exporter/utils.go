package exporter

import "time"

func nsToS(ns uint64) float64 {
	return float64(ns) / float64(time.Second)
}
