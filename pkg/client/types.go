package client

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"

	"github.com/schollz/progressbar"
	"go.uber.org/ratelimit"
)

type LoadTest func(c *Cassowary, outPutChan chan<- durationMetrics, g *QueryGroup)

// Validator(statusCode int, respSize int64, resp []byte, err error) (failed ool, statusCode string)
type Validator func(int, int64, []byte, error) (bool, string)

type Query struct {
	Method         string
	URL            string
	DataType       string
	Data           []byte // Body
	RequestHeaders [][2]string
	Validator      Validator // Custom Validator function
}

type QueryGroup struct {
	Name string

	ConcurrencyLevel int
	Delay            time.Duration
	Requests         int

	l ratelimit.Limiter

	FileMode    bool
	URLPaths    []string
	URLIterator Iterator

	Method        string
	Data          []byte
	RequestHeader []string

	loadTest LoadTest // Custom load test function

	workerChan chan *Query
}

// Cassowary is the main struct with bootstraps the load test
type Cassowary struct {
	IsTLS                 bool
	BaseURL               string
	ExportMetrics         bool
	ExportMetricsFile     string
	PromExport            bool
	Cloudwatch            bool
	Histogram             bool
	Boxplot               bool
	StatFile              string
	TLSConfig             *tls.Config
	PromURL               string
	DisableTerminalOutput bool
	DisableKeepAlive      bool
	Client                *http.Client
	Bar                   *progressbar.ProgressBar
	Timeout               int

	Duration time.Duration
	Groups   []QueryGroup

	wgStart sync.WaitGroup
	wgStop  sync.WaitGroup
}

// ResultMetrics are the aggregated metrics after the load test
type ResultMetrics struct {
	BaseURL           string                `json:"base_url"`
	TotalRequests     int                   `json:"total_requests"`
	FailedRequests    int                   `json:"failed_requests"`
	RespSuccess       map[string]int        `json:"responses_success"`
	RespFailed        map[string]int        `json:"responses_failed"`
	RequestsPerSecond float64               `json:"requests_per_second"`
	DNSMedian         float64               `json:"dns_median"`
	TCPStats          tcpStats              `json:"tcp_connect"`
	ProcessingStats   serverProcessingStats `json:"server_processing"`
	ContentStats      contentTransfer       `json:"content_transfer"`
	BodySize          contentSize           `json:"body_size"`
	RespSize          contentSize           `json:"resp_size"`
}

type tcpStats struct {
	TCPMean   float64 `json:"mean"`
	TCPMedian float64 `json:"median"`
	TCP95p    float64 `json:"95th_percentile"`
}

type serverProcessingStats struct {
	ServerProcessingMean   float64 `json:"mean"`
	ServerProcessingMedian float64 `json:"median"`
	ServerProcessing95p    float64 `json:"95th_percentile"`
}

type contentTransfer struct {
	ContentTransferMean   float64 `json:"mean"`
	ContentTransferMedian float64 `json:"median"`
	ContentTransfer95p    float64 `json:"95th_percentile"`
}

type contentSize struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	P95    float64 `json:"95th_percentile"`
}
