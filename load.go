package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/schollz/progressbar"
)

type durationMetrics struct {
	DnsLookup        int
	TCPConn          int
	TLSHandshake     int
	ServerProcessing int
	ContentTransfer  int
	StatusCode       int
	URL              string
}

func (c *cassowary) runLoadTest(cmdOutputChan chan<- durationMetrics, workerChan chan string) {
	for _ = range workerChan {
		tt := newTransport(c.client.Transport)
		c.client.Transport = tt

		request, err := http.NewRequest("GET", c.baseURL, nil)
		if err != nil {
			panic(err)
		}

		trace := &httptrace.ClientTrace{
			DNSStart:             tt.DNSStart,
			DNSDone:              tt.DNSDone,
			ConnectStart:         tt.ConnectStart,
			ConnectDone:          tt.ConnectDone,
			GotConn:              tt.GotConn,
			GotFirstResponseByte: tt.GotFirstResponseByte,
		}

		request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
		resp, err := c.client.Do(request)
		if err != nil {
			panic(err)
		}

		if resp != nil {
			_, err = io.Copy(ioutil.Discard, resp.Body)
			if err != nil {
				fmt.Println("Failed to read HTTP response body", err)
			}
			resp.Body.Close()
			c.bar.Add(1)
		}

		// Body fully read here
		tt.current.end = time.Now()
		for _, trace := range tt.traces {
			if trace.tls {
				fmt.Println("Connect: ", trace.connectDone.Sub(trace.dnsDone).Seconds())
				fmt.Println("tls: ", trace.gotConn.Sub(trace.dnsDone).Seconds())
			} else {
				fmt.Println("connect: ", trace.gotConn.Sub(trace.dnsDone).Seconds())
			}

		}

	}
}

func (c *cassowary) coordinate() error {

	color := color.New(color.FgCyan).Add(color.Underline)
	color.Printf("\nStarting Load Test with %d concurrent users\n\n", c.concurrencyLevel)

	var urlSuffixes []string
	var err error

	c.client = &http.Client{
		Timeout: time.Second * 5,
		Transport: &http.Transport{
			MaxIdleConns:        300,
			MaxIdleConnsPerHost: 300,
			MaxConnsPerHost:     300,
			DisableCompression:  false,
		},
	}

	c.bar = progressbar.New(c.requests)

	if c.fileMode {
		urlSuffixes, err = readFile(c.inputFile)
		if err != nil {
			return err
		}
		c.requests = len(urlSuffixes)
	}

	var wg sync.WaitGroup
	channel := make(chan durationMetrics, c.requests)
	workerChan := make(chan string)

	wg.Add(c.concurrencyLevel)
	start := time.Now()

	for i := 0; i < c.concurrencyLevel; i++ {
		go func() {
			c.runLoadTest(channel, workerChan)
			wg.Done()
		}()
	}

	if c.fileMode {
		for _, line := range urlSuffixes {
			workerChan <- line
		}
	}

	close(workerChan)
	wg.Wait()
	close(channel)

	end := time.Since(start)
	fmt.Println(end)

	return nil
}
