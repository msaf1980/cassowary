package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptrace"
	"os"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar"

	//"github.com/schollz/progressbar"

	"go.uber.org/ratelimit"
)

type durationMetrics struct {
	Started          time.Time
	Group            string
	Label            string
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
	ResponseType     string
}

func runLoadTest(c *Cassowary, outPutChan chan<- durationMetrics, g *QueryGroup) {
	for u := range g.workerChan {

		var request *http.Request
		var err error
		var contentType string

		switch u.Method {
		case "GET":
			request, err = http.NewRequest(u.Method, c.BaseURL+u.URL, nil)
			if err != nil {
				log.Fatalf("%v", err)
			}
		default:
			if len(u.Data) > 0 {
				request, err = http.NewRequest(u.Method, c.BaseURL+u.URL, bytes.NewBuffer(u.Data))
			} else {
				request, err = http.NewRequest(u.Method, c.BaseURL+u.URL, nil)
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
		if !g.FileMode && len(g.RequestHeader) == 2 {
			request.Header.Add(g.RequestHeader[0], g.RequestHeader[1])
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
		} else {
			contentType = resp.Header.Get("Content-Type")
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
			if c.Bar != nil {
				c.Bar.Add(1)
			}
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
			Started:   t7,
			Group:     g.Name,
			Label:     u.Name,
			Method:    u.Method,
			URL:       u.URL,
			DNSLookup: float64(t1.Sub(t0) / time.Millisecond), // dns lookup
			//TCPConn:          float64(t3.Sub(t1) / time.Millisecond), // tcp connection
			ServerProcessing: float64(t4.Sub(t3) / time.Millisecond), // server processing
			ContentTransfer:  float64(t7.Sub(t4) / time.Millisecond), // content transfer
			StatusCode:       statusCode,
			BodySize:         len(u.Data),
			ResponseSize:     respSize,
			Failed:           failed,
			ResponseType:     contentType,
		}

		if !t1.IsZero() {
			// new connection
			if c.IsTLS {
				out.TCPConn = float64(t2.Sub(t1) / time.Millisecond)
				out.TLSHandshake = float64(t6.Sub(t5) / time.Millisecond) // tls handshake
			} else {
				out.TCPConn = float64(t3.Sub(t1) / time.Millisecond)
			}
		}

		out.TotalDuration = out.DNSLookup + out.TCPConn + out.ServerProcessing + out.ContentTransfer

		outPutChan <- out

		g.l.Take()
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

	var requests int

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

	concurrency := 0
	for n := range c.Groups {
		g := &(c.Groups[n])

		if g.ConcurrencyLevel < 0 {
			g.ConcurrencyLevel = 0
		}
		concurrency += g.ConcurrencyLevel

		if g.loadTest == nil {
			g.loadTest = runLoadTest
		}

		if g.Requests > 0 && c.Duration > 0 && g.Delay == 0 {
			rateLimit := int(float64(g.Requests) / float64(c.Duration.Seconds()))
			if rateLimit <= 0 {
				log.Fatalf("The combination of %v requests and %v(s) duration is invalid. Try raising the duration to a greater value", g.Requests, c.Duration)
			}
			fmt.Printf("QueryGroup %s with %d rps during %v\n", g.Name, rateLimit, c.Duration)
			g.l = ratelimit.New(rateLimit)
		} else if g.Delay > 0 && c.Duration > 0 && g.Requests == 0 {
			fmt.Printf("QueryGroup %s with %v delay during %v\n", g.Name, g.Delay, c.Duration)
			g.l = NewSleepLimited(g.Delay)
		} else if c.Duration > 0 && g.Requests == 0 && g.Delay == 0 {
			fmt.Printf("QueryGroup %s during %d s\n", g.Name, c.Duration)
			g.l = ratelimit.NewUnlimited()
		} else if c.Duration == 0 && g.Requests > 0 {
			if g.Delay > 0 {
				fmt.Printf("QueryGroup %s with %d requests and %v delay\n", g.Name, g.Requests, g.Delay)
				g.l = NewSleepLimited(g.Delay)
			} else {
				fmt.Printf("QueryGroup %s with %d request\n", g.Name, g.Requests)
				g.l = ratelimit.NewUnlimited()
			}
		} else {
			log.Fatalf("The combination of %v requests, %v delay and %v(s) duration is invalid.", g.Requests, g.Delay, c.Duration)
		}

		if g.FileMode {
			if (len(g.URLPaths) > 0 && g.URLIterator != nil) || (len(g.URLPaths) == 0 && g.URLIterator == nil) {
				return ResultMetrics{}, fmt.Errorf("use URLPaths or URLIterator in FileMode for %s group", g.Name)
			}
			// if len(g.URLPaths) > 0 {
			// 	if g.Requests > len(g.URLPaths) {
			// 		g.URLPaths = generateSuffixes(g.URLPaths, g.Requests)
			// 	}
			// 	g.Requests = len(g.URLPaths)
			// }
		}

		if concurrency == 0 {
			log.Fatalf("Concurrency level need set for one group at least")
		}

		g.workerChan = make(chan *Query, g.ConcurrencyLevel)

		requests += g.Requests

		if !c.DisableTerminalOutput {
			col := color.New(color.FgCyan).Add(color.Underline)
			col.Printf("Starting Load Test in QueryGroup %s using %d concurrent users\n", g.Name, g.ConcurrencyLevel)
		}
	}

	if requests > 0 {
		c.Bar = progressbar.New(requests)
	}

	var fo *os.File
	if len(c.StatFile) > 0 {
		fo, err = os.Create(c.StatFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	channel := make(chan durationMetrics, requests)

	c.wgStart.Add(1)
	for n := range c.Groups {
		g := &(c.Groups[n])

		c.wgStart.Add(g.ConcurrencyLevel)
		c.wgStop.Add(g.ConcurrencyLevel)

		for i := 0; i < g.ConcurrencyLevel; i++ {
			go func() {
				c.wgStart.Done()
				c.wgStart.Wait()
				defer c.wgStop.Done()
				g.loadTest(c, channel, g)
			}()
		}

	}

	c.wgStart.Done()
	c.wgStart.Wait()

	start := time.Now()

	for n := range c.Groups {
		g := &(c.Groups[n])
		go func() {
			iter := 0
			reqs := g.Requests
			start := time.Now()
			for {
				if g.FileMode {
					if g.URLIterator == nil {
						g.workerChan <- &Query{Method: "GET", URL: g.URLPaths[iter]}
						if iter < len(g.URLPaths)-1 {
							iter++
						} else {
							iter = 0
						}
					} else {
						g.workerChan <- g.URLIterator.Next()
					}
				} else {
					if g.Method == "GET" || len(g.Data) == 0 {
						g.workerChan <- &Query{Method: g.Method}
					} else {
						g.workerChan <- &Query{Method: g.Method, DataType: "application/json", Data: g.Data}
					}
				}

				if g.Requests > 0 {
					if reqs <= 1 {
						break
					}
					reqs--
				} else if c.Duration > 0 {
					if time.Since(start) >= c.Duration {
						break
					}
				}
			}
			close(g.workerChan)
		}()
	}

	failedR := 0
	totalR := 0
	successMap := make(map[string]int)
	failedMap := make(map[string]int)

	go func() {
		var w *bufio.Writer
		var sb bytes.Buffer
		if fo != nil {
			defer fo.Close()
			w = bufio.NewWriter(fo)
			w.WriteString("timeStamp,elapsed,label,responseCode,responseMessage,threadName,dataType,success,failureMessage,bytes,sentBytes,grpThreads,allThreads,URL,Latency,IdleTime,Connect\n")
		}

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
			totalR++
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

			if w != nil {
				// timeStamp
				sb.WriteString(strconv.FormatInt(item.Started.UnixNano()/1000000, 10)) // ms
				sb.WriteRune(',')
				// elapsed
				sb.WriteString(strconv.FormatFloat(item.TotalDuration, 'f', 0, 64)) // ms
				sb.WriteRune(',')
				// label
				if len(item.Label) > 0 {
					sb.WriteString(item.Label)
				} else {
					sb.WriteString(item.Group)
				}
				sb.WriteRune(',')
				// responseCode
				sb.WriteString(item.StatusCode)
				sb.WriteRune(',')
				// responseMessage
				sb.WriteRune(',')
				// threadName
				sb.WriteString(item.Group)
				sb.WriteRune(',')
				// dataType
				sb.WriteString(item.ResponseType)
				sb.WriteRune(',')
				// success
				sb.WriteString(strconv.FormatBool(!item.Failed))
				sb.WriteRune(',')
				// failureMessage
				sb.WriteRune(',')
				// bytes
				sb.WriteString(strconv.FormatInt(item.ResponseSize, 10))
				sb.WriteRune(',')
				// sentBytes
				sb.WriteString(strconv.Itoa(item.BodySize))
				sb.WriteRune(',')
				// grpThreads
				sb.WriteString(strconv.Itoa(len(c.Groups)))
				sb.WriteRune(',')
				//allThreads
				sb.WriteString(strconv.Itoa(len(c.Groups)))
				sb.WriteRune(',')
				//URL
				sb.WriteString(item.URL)
				sb.WriteRune(',')
				// Latency (transfer)
				sb.WriteString(strconv.FormatFloat(item.ContentTransfer, 'f', 0, 64)) // ms
				sb.WriteRune(',')
				//IdleTime (server processing)
				sb.WriteString(strconv.FormatFloat(item.ServerProcessing, 'f', 0, 64)) // ms
				sb.WriteRune(',')
				// DNS resolve, connect and tls handshake
				sb.WriteString(strconv.FormatFloat(item.DNSLookup+item.TCPConn+item.TLSHandshake, 'f', 0, 64)) // ms
				sb.WriteRune('\n')

				if _, err := w.Write(sb.Bytes()); err != nil {
					panic(err)
				}
				sb.Reset()
			}
		}

		if w != nil {
			if err = w.Flush(); err != nil {
				panic(err)
			}
		}
	}()

	c.wgStop.Wait()
	time.Sleep(100 * time.Millisecond)

	close(channel)

	end := time.Since(start)
	if !c.DisableTerminalOutput {
		fmt.Println(end)
	}

	// DNS
	dnsMedian := calcMedian(dnsDur)

	// Elapsed (total)
	elapsedMin, elapsedMax, elapsedMean := calcStat(totalDur)
	elapsedMedian := calcMedian(totalDur)
	elapsed95 := calc95Percentile(totalDur)
	elapsed99 := calc99Percentile(totalDur)

	// TCP
	tcpMin, tcpMax, tcpMean := calcStat(tcpDur)
	tcpMedian := calcMedian(tcpDur)
	tcp95 := calc95Percentile(tcpDur)
	tcp99 := calc99Percentile(tcpDur)

	// Server Processing
	serverMin, serverMax, serverMean := calcStat(serverDur)
	serverMedian := calcMedian(serverDur)
	server95 := calc95Percentile(serverDur)
	server99 := calc99Percentile(serverDur)

	// Content Transfer
	transferMin, transferMax, transferMean := calcStat(transferDur)
	transferMedian := calcMedian(transferDur)
	transfer95 := calc95Percentile(transferDur)
	transfer99 := calc99Percentile(transferDur)

	bodyMin, bodyMax, bodyMean := calcStat(bodySize)
	bodyMedian := calcMedian(bodySize)
	body95 := calc95Percentile(bodySize)
	body99 := calc99Percentile(bodySize)

	respMin, respMax, respMean := calcStat(respSize)
	respMedian := calcMedian(respSize)
	resp95 := calc95Percentile(respSize)
	resp99 := calc99Percentile(respSize)

	// Request per second
	reqS := requestsPerSecond(totalR, end)

	outPut := ResultMetrics{
		BaseURL:           c.BaseURL,
		FailedRequests:    failedR,
		RespSuccess:       successMap,
		RespFailed:        failedMap,
		RequestsPerSecond: reqS,
		TotalRequests:     totalR,
		DNSMedian:         dnsMedian,
		ElapsedStats: stats{
			Min:    elapsedMin,
			Max:    elapsedMax,
			Mean:   elapsedMean,
			Median: elapsedMedian,
			P95:    elapsed95,
			P99:    elapsed99,
		},
		TCPStats: stats{
			Min:    tcpMin,
			Max:    tcpMax,
			Mean:   tcpMean,
			Median: tcpMedian,
			P95:    tcp95,
			P99:    tcp99,
		},
		ProcessingStats: stats{
			Min:    serverMin,
			Max:    serverMax,
			Mean:   serverMean,
			Median: serverMedian,
			P95:    server95,
			P99:    server99,
		},
		ContentStats: stats{
			Min:    transferMin,
			Max:    transferMax,
			Mean:   transferMean,
			Median: transferMedian,
			P95:    transfer95,
			P99:    transfer99,
		},
		BodySize: stats{
			Min:    bodyMin,
			Max:    bodyMax,
			Mean:   bodyMean,
			Median: bodyMedian,
			P95:    body95,
			P99:    body99,
		},
		RespSize: stats{
			Min:    respMin,
			Max:    respMax,
			Mean:   respMean,
			Median: respMedian,
			P95:    resp95,
			P99:    resp99,
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
