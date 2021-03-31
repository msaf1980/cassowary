package client

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestLoadCoordinate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cass := Cassowary{
		BaseURL:               srv.URL,
		ConcurrencyLevel:      1,
		Requests:              10,
		DisableTerminalOutput: true,
	}

	metrics, err := cass.Coordinate()
	if err != nil {
		t.Error(err)
	}

	if metrics.BaseURL != srv.URL {
		t.Errorf("Wanted %s but got %s", srv.URL, metrics.BaseURL)
	}

	if metrics.TotalRequests != 10 {
		t.Errorf("Wanted %d but got %d", 1, metrics.TotalRequests)
	}

	if metrics.FailedRequests != 0 {
		t.Errorf("Wanted %d but got %d", 0, metrics.FailedRequests)
	}
}

func TestLoadCoordinateURLPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cass := Cassowary{
		BaseURL:               srv.URL,
		ConcurrencyLevel:      1,
		Requests:              30,
		FileMode:              true,
		URLPaths:              []string{"/get_user", "/get_accounts", "/get_orders"},
		DisableTerminalOutput: true,
	}
	metrics, err := cass.Coordinate()
	if err != nil {
		t.Error(err)
	}
	if metrics.BaseURL != srv.URL {
		t.Errorf("Wanted %s but got %s", srv.URL, metrics.BaseURL)
	}

	if metrics.TotalRequests != 30 {
		t.Errorf("Wanted %d but got %d", 1, metrics.TotalRequests)
	}

	if metrics.FailedRequests != 0 {
		t.Errorf("Wanted %d but got %d", 0, metrics.FailedRequests)
	}

	if len(cass.URLPaths) != 30 {
		t.Errorf("Wanted %d but got %d", 30, len(cass.URLPaths))
	}
}

type URLIterator struct {
	baseURL string
	pos     uint64
	data    []string
}

func (it *URLIterator) Next() *Query {
	for {
		pos := atomic.AddUint64(&it.pos, 1)
		if pos > uint64(len(it.data)) {
			if !atomic.CompareAndSwapUint64(&it.pos, pos, 0) {
				// retry
				continue
			}
			pos = 0
		} else {
			pos--
		}
		//return &Query{Method: "GET", URL: it.baseURL + it.data[pos]}
		return &Query{Method: "POST", URL: it.baseURL + it.data[pos], DataType: "application/json", Data: []byte("{ \"test\": \"POST\" }")}
	}
}

func NewURLIterator(baseURL string, data []string) *URLIterator {
	if len(data) == 0 {
		return nil
	}
	return &URLIterator{baseURL: baseURL, data: data, pos: 0}
}

func TestLoadCoordinateURLIterator(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	it := NewURLIterator(srv.URL, []string{"/test1", "/test2", "/test3"})

	cass := Cassowary{
		ConcurrencyLevel:      1,
		Requests:              32,
		FileMode:              true,
		URLIterator:           it,
		DisableTerminalOutput: true,
	}
	metrics, err := cass.Coordinate()
	if err != nil {
		t.Error(err)
	}

	if metrics.TotalRequests != 32 {
		t.Errorf("Wanted %d but got %d", 32, metrics.TotalRequests)
	}

	if metrics.FailedRequests != 0 {
		t.Errorf("Wanted %d but got %d", 0, metrics.FailedRequests)
	}
}

func TestCoordinateTLSConfig(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))

	pemCerts, err := ioutil.ReadFile("testdata/ca.pem")
	if err != nil {
		t.Fatal("Invalid ca.pem path")
	}

	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(pemCerts) {
		t.Fatal("Failed to read CA from PEM")
	}

	cert, err := tls.LoadX509KeyPair("testdata/server.pem", "testdata/server-key.pem")
	if err != nil {
		t.Fatal("Invalid server.pem/server-key.pem path")
	}

	srv.TLS = &tls.Config{
		ClientCAs:    ca,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
	}
	srv.StartTLS()

	cert, err = tls.LoadX509KeyPair("testdata/client.pem", "testdata/client-key.pem")
	if err != nil {
		t.Fatal("Invalid client.pem/client-key.pem path")
	}
	clientTLSConfig := &tls.Config{
		RootCAs:      ca,
		Certificates: []tls.Certificate{cert},
	}

	cass := Cassowary{
		BaseURL:               srv.URL,
		ConcurrencyLevel:      1,
		Requests:              10,
		TLSConfig:             clientTLSConfig,
		DisableTerminalOutput: true,
	}

	metrics, err := cass.Coordinate()
	if err != nil {
		t.Error(err)
	}

	if metrics.BaseURL != srv.URL {
		t.Errorf("Wanted %s but got %s", srv.URL, metrics.BaseURL)
	}

	if metrics.TotalRequests != 10 {
		t.Errorf("Wanted %d but got %d", 1, metrics.TotalRequests)
	}

	if metrics.FailedRequests != 0 {
		t.Errorf("Wanted %d but got %d", 0, metrics.FailedRequests)
	}
}
