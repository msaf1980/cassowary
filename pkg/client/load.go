package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar"
)

type durationMetrics struct {
	Method           string
	URL              string
	DNSLookup        float64
	TCPConn          float64
	TLSHandshake     float64
	ServerProcessing float64
	ContentTransfer  float64
	StatusCode       string
	Failed           bool
	TotalDuration    float64
	BodySize         int
	ResponseSize     int64
}

func (c *Cassowary) runLoadTest(outPutChan chan<- durationMetrics, workerChan chan *Query) {
	for u := range workerChan {

		var request *http.Request
		var err error

		switch c.HTTPMethod {
		case "GET":
			request, err = http.NewRequest(u.Method, u.URL, nil)
			if err != nil {
				log.Fatalf("%v", err)
			}
		default:
			if len(u.Data) > 0 {
				request, err = http.NewRequest(u.Method, u.URL, bytes.NewBuffer(u.Data))
			} else {
				request, err = http.NewRequest(u.Method, u.URL, nil)
			}
			if err != nil {
				log.Fatalf("%v", err)
			}
			//request.Header.Set("Content-Type", "application/json")
			if len(u.Data) > 0 {
				request.Header.Set("Content-Type", u.DataType)
			}
		}

		for _, h := range u.RequestHeaders {
			request.Header.Add(h[0], h[1])
		}
		if !c.FileMode && len(c.RequestHeader) == 2 {
			request.Header.Add(c.RequestHeader[0], c.RequestHeader[1])
		}

		var t0, t1, t2, t3, t4, t5, t6 time.Time

		trace := &httptrace.ClientTrace{
			DNSStart: func(_ httptrace.DNSStartInfo) { t0 = time.Now() },
			DNSDone:  func(_ httptrace.DNSDoneInfo) { t1 = time.Now() },
			ConnectStart: func(_, _ string) {
				if t1.IsZero() {
					// connecting directly to IP
					t1 = time.Now()
				}
			},
			ConnectDone: func(net, addr string, err error) {
				if err != nil {
					log.Fatalf("unable to connect to host %v: %v", addr, err)
				}
				t2 = time.Now()

			},
			GotConn:              func(_ httptrace.GotConnInfo) { t3 = time.Now() },
			GotFirstResponseByte: func() { t4 = time.Now() },
			TLSHandshakeStart:    func() { t5 = time.Now() },
			TLSHandshakeDone:     func(_ tls.ConnectionState, _ error) { t6 = time.Now() },
		}

		request = request.WithContext(httptrace.WithClientTrace(context.Background(), trace))
		resp, err := c.Client.Do(request)
		if err != nil {
			log.Fatalf("%v", err)
		}
		var respSize int64
		if resp != nil {
			respSize, err = io.Copy(ioutil.Discard, resp.Body)
			if err != nil {
				fmt.Println("Failed to read HTTP response body", err)
			}
			resp.Body.Close()
		}

		if !c.DisableTerminalOutput {
			c.Bar.Add(1)
		}

		// Body fully read here
		t7 := time.Now()
		if t0.IsZero() {
			// we skipped DNS
			t0 = t1
		}

		failed := false
		var statusCode string
		if u.Validator == nil {
			if err != nil {
				statusCode = err.Error()
			} else {
				if resp.StatusCode > 226 {
					failed = true
				}
				statusCode = strconv.Itoa(resp.StatusCode)
			}
		} else {
			failed, statusCode = u.Validator(resp.StatusCode, respSize, nil, err)
		}

		out := durationMetrics{
			Method:           u.Method,
			URL:              u.URL,
			DNSLookup:        float64(t1.Sub(t0) / time.Millisecond), // dns lookup
			TCPConn:          float64(t3.Sub(t1) / time.Millisecond), // tcp connection
			ServerProcessing: float64(t4.Sub(t3) / time.Millisecond), // server processing
			ContentTransfer:  float64(t7.Sub(t4) / time.Millisecond), // content transfer
			StatusCode:       statusCode,
			BodySize:         len(u.Data),
			ResponseSize:     respSize,
			Failed:           failed,
		}

		if c.IsTLS {
			out.TCPConn = float64(t2.Sub(t1) / time.Millisecond)
			out.TLSHandshake = float64(t6.Sub(t5) / time.Millisecond) // tls handshake
		} else {
			out.TCPConn = float64(t3.Sub(t1) / time.Millisecond)
		}

		out.TotalDuration = out.DNSLookup + out.TCPConn + out.ServerProcessing + out.ContentTransfer

		outPutChan <- out
	}
}

// Coordinate bootstraps the load test based on values in Cassowary struct
func (c *Cassowary) Coordinate() (ResultMetrics, error) {
	var dnsDur []float64
	var tcpDur []float64
	var tlsDur []float64
	var serverDur []float64
	var transferDur []float64
	var totalDur []float64
	var bodySize []float64
	var respSize []float64

	if c.FileMode {
		if (len(c.URLPaths) > 0 && c.URLIterator != nil) || (len(c.URLPaths) == 0 && c.URLIterator == nil) {
			return ResultMetrics{}, fmt.Errorf("use URLPaths or URLIterator in FileMode")
		}
		if len(c.URLPaths) > 0 {
			if c.Requests > len(c.URLPaths) {
				c.URLPaths = generateSuffixes(c.URLPaths, c.Requests)
			}
			c.Requests = len(c.URLPaths)
		}
	}

	tls, err := isTLS(c.BaseURL)
	if err != nil {
		return ResultMetrics{}, err
	}
	c.IsTLS = tls

	c.Client = &http.Client{
		Timeout: time.Second * time.Duration(c.Timeout),
		Transport: &http.Transport{
			TLSClientConfig:     c.TLSConfig,
			MaxIdleConnsPerHost: 10000,
			DisableCompression:  false,
			DisableKeepAlives:   c.DisableKeepAlive,
		},
	}

	c.Bar = progressbar.New(c.Requests)

	if !c.DisableTerminalOutput {
		col := color.New(color.FgCyan).Add(color.Underline)
		col.Printf("\nStarting Load Test with %d requests using %d concurrent users\n\n", c.Requests, c.ConcurrencyLevel)
	}

	var wg sync.WaitGroup
	channel := make(chan durationMetrics, c.Requests)
	workerChan := make(chan *Query, c.ConcurrencyLevel)

	wg.Add(c.ConcurrencyLevel)
	start := time.Now()

	for i := 0; i < c.ConcurrencyLevel; i++ {
		go func() {
			c.runLoadTest(channel, workerChan)
			wg.Done()
		}()
	}

	if c.Duration > 0 {
		durationMS := c.Duration * 1000
		nextTick := durationMS / c.Requests
		ticker := time.NewTicker(time.Duration(nextTick) * time.Millisecond)
		if nextTick == 0 {
			log.Fatalf("The combination of %v requests and %v(s) duration is invalid. Try raising the duration to a greater value", c.Requests, c.Duration)
		}
		done := make(chan bool)
		iter := 0

		go func() {
			for {
				select {
				case <-done:
					return
				case _ = <-ticker.C:
					if c.FileMode {
						if c.URLIterator == nil {
							workerChan <- &Query{Method: "GET", URL: c.BaseURL + c.URLPaths[iter]}
							iter++
						} else {
							workerChan <- c.URLIterator.Next()
						}
					} else {
						if c.HTTPMethod == "GET" || len(c.Data) == 0 {
							workerChan <- &Query{Method: c.HTTPMethod, URL: c.BaseURL}
						} else {
							workerChan <- &Query{Method: c.HTTPMethod, URL: c.BaseURL, DataType: "application/json", Data: c.Data}
						}
					}
				}
			}
		}()

		time.Sleep(time.Duration(durationMS) * time.Millisecond)
		ticker.Stop()
		done <- true
	} else if c.FileMode {
		if c.URLIterator == nil {
			for _, line := range c.URLPaths {
				workerChan <- &Query{Method: "GET", URL: c.BaseURL + line}
			}
		} else {
			for i := 0; i < c.Requests; i++ {
				workerChan <- c.URLIterator.Next()
			}
		}
	} else {
		for i := 0; i < c.Requests; i++ {
			if c.HTTPMethod == "GET" || len(c.Data) == 0 {
				workerChan <- &Query{Method: c.HTTPMethod, URL: c.BaseURL}
			} else {
				workerChan <- &Query{Method: c.HTTPMethod, URL: c.BaseURL, DataType: "application/json", Data: c.Data}
			}
		}
	}

	close(workerChan)
	wg.Wait()
	close(channel)

	end := time.Since(start)
	if !c.DisableTerminalOutput {
		fmt.Println(end)
	}

	failedR := 0
	successMap := make(map[string]int)
	failedMap := make(map[string]int)

	for item := range channel {
		if item.Failed {
			// Failed Requests
			failedR++
			failedMap[item.StatusCode]++
		} else {
			successMap[item.StatusCode]++
			bodySize = append(bodySize, float64(item.BodySize))
			respSize = append(respSize, float64(item.ResponseSize))
		}
		if item.DNSLookup != 0 {
			dnsDur = append(dnsDur, item.DNSLookup)
		}
		if item.TCPConn < 1000 {
			tcpDur = append(tcpDur, item.TCPConn)
		}
		if c.IsTLS {
			tlsDur = append(tlsDur, item.TLSHandshake)
		}
		serverDur = append(serverDur, item.ServerProcessing)
		transferDur = append(transferDur, item.ContentTransfer)
		totalDur = append(totalDur, item.TotalDuration)
	}

	// DNS
	dnsMedian := calcMedian(dnsDur)

	// TCP
	tcpMean := calcMean(tcpDur)
	tcpMedian := calcMedian(tcpDur)
	tcp95 := calc95Percentile(tcpDur)

	// Server Processing
	serverMean := calcMean(serverDur)
	serverMedian := calcMedian(serverDur)
	server95 := calc95Percentile(serverDur)

	// Content Transfer
	transferMean := calcMean(transferDur)
	transferMedian := calcMedian(transferDur)
	transfer95 := calc95Percentile(transferDur)

	bodyMean := calcMean(bodySize)
	bodyMedian := calcMedian(bodySize)
	body95 := calc95Percentile(bodySize)

	respMean := calcMean(respSize)
	respMedian := calcMedian(respSize)
	resp95 := calc95Percentile(respSize)

	// Request per second
	reqS := requestsPerSecond(c.Requests, end)

	outPut := ResultMetrics{
		BaseURL:           c.BaseURL,
		FailedRequests:    failedR,
		RespSuccess:       successMap,
		RespFailed:        failedMap,
		RequestsPerSecond: reqS,
		TotalRequests:     c.Requests,
		DNSMedian:         dnsMedian,
		TCPStats: tcpStats{
			TCPMean:   tcpMean,
			TCPMedian: tcpMedian,
			TCP95p:    tcp95,
		},
		ProcessingStats: serverProcessingStats{
			ServerProcessingMean:   serverMean,
			ServerProcessingMedian: serverMedian,
			ServerProcessing95p:    server95,
		},
		ContentStats: contentTransfer{
			ContentTransferMean:   transferMean,
			ContentTransferMedian: transferMedian,
			ContentTransfer95p:    transfer95,
		},
		BodySize: contentSize{
			Mean:   bodyMean,
			Median: bodyMedian,
			P95:    body95,
		},
		RespSize: contentSize{
			Mean:   respMean,
			Median: respMedian,
			P95:    resp95,
		},
	}

	// output histogram
	if c.Histogram {
		err := c.PlotHistogram(totalDur)
		if err != nil {
		}
	}

	// output boxplot
	if c.Boxplot {
		err := c.PlotBoxplot(totalDur)
		if err != nil {
		}
	}
	return outPut, nil
}
