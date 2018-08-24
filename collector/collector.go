package collector

import (
	"strconv"
	"sync"
	"time"

	"github.com/ContaAzul/cachet_exporter/client"
	"github.com/andygrunwald/cachet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

const namespace = "cachet"

type metricConfig struct {
	Name   string            `yaml:"metric_name"`
	Labels map[string]string `yaml:"labels"`
}

// Config is the struct used to load the configurations from yaml file
type Config struct {
	Metrics []metricConfig `yaml:"metrics"`
}

type cachetCollector struct {
	mutex  sync.RWMutex
	client client.Client

	up             *prometheus.Desc
	scrapeDuration *prometheus.Desc
	incidentsTotal *prometheus.Desc
}

// NewCachetCollector returns a prometheus collector which exports
// metrics from a Cachet status page.
func NewCachetCollector(client client.Client) prometheus.Collector {
	return &cachetCollector{
		client: client,
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Cachet API is up and accepting requests",
			nil,
			nil,
		),
		scrapeDuration: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "scrape_duration_seconds"),
			"Time Cachet scrape took in seconds",
			nil,
			nil,
		),
		incidentsTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "incidents_total"),
			"Total of incidents by status",
			[]string{"status", "group_name", "component_name"},
			nil,
		),
	}
}

// Describe describes all the metrics exported by the Cachet exporter.
// It implements prometheus.Collector.
func (c *cachetCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.scrapeDuration
	ch <- c.incidentsTotal
}

// Collect fetches the metrics data from the Cachet application and
// delivers them as Prometheus metrics. It implements prometheus.Collector.
func (c *cachetCollector) Collect(ch chan<- prometheus.Metric) {
	// To protect metrics from concurrent collects.
	c.mutex.Lock()
	defer c.mutex.Unlock()

	start := time.Now()
	log.Info("Collecting metrics from Cachet")
	_, err := c.client.Ping()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		log.With("error", err).Error("failed to scrape Cachet")
		return
	}

	groups, err := c.client.GetAllComponentGroups()

	if err != nil {
		log.With("error", err).Error("failed to scrape Group Components")
	}

	incidents := map[int][]cachet.Incident{
		1: getIncidentsByStatus(c, 1),
		2: getIncidentsByStatus(c, 2),
		3: getIncidentsByStatus(c, 3),
	}
	for _, group := range groups {
		createIncidentsTotalMetric(c, group, incidents, ch)
	}

	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, time.Since(start).Seconds())
}

func getIncidentsByStatus(c *cachetCollector, status int) []cachet.Incident {
	incidents, err := c.client.GetAllIncidentsByStatus(status)
	if err != nil {
		log.With("error", err).Error("failed to scrape Group Components")
	}
	return incidents
}

func createIncidentsTotalMetric(c *cachetCollector, group cachet.ComponentGroup, incidents map[int][]cachet.Incident, ch chan<- prometheus.Metric) {
	for _, component := range group.EnabledComponents {
		createIncidentsTotalMetricByComponent(c, group, component, incidents, ch)
	}
}

func createIncidentsTotalMetricByComponent(c *cachetCollector, group cachet.ComponentGroup, component cachet.Component, incidents map[int][]cachet.Incident, ch chan<- prometheus.Metric) {
	for status, allIncidents := range incidents {
		componentIncidents := make([]cachet.Incident, 0)
		for _, incident := range allIncidents {
			if incident.ComponentID == component.ID && status == incident.Status {
				componentIncidents = append(componentIncidents, incident)
			}
		}
		ch <- prometheus.MustNewConstMetric(c.incidentsTotal, prometheus.GaugeValue, float64(len(componentIncidents)), strconv.Itoa(status), group.Name, component.Name)
	}

}
