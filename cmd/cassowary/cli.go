package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/msaf1980/cassowary/pkg/client"
	"github.com/urfave/cli/v2"
)

var (
	version             = "dev"
	errConcurrencyLevel = errors.New("error: Concurrency level cannot be set to: 0")
	errRequestNo        = errors.New("error: No. of request cannot be set to: 0")
	errNotValidURL      = errors.New("error: Not a valid URL. Must have the following format: http{s}://{host}")
	errNotValidHeader   = errors.New("error: Not a valid header value. Did you forget : ?")
	errDurationValue    = errors.New("error: Duration cannot be set to 0 or negative")
)

func runLoadTest(c *client.Cassowary) error {
	metrics, metricsGroup, err := c.Coordinate()
	if err != nil {
		return err
	}

	for _, g := range c.Groups {
		if m, ok := metricsGroup[g.Name]; ok {
			client.OutPutResults(m)
		}
	}
	client.OutPutResults(metrics)
	
	if c.ExportMetrics {
		return client.OutPutJSON(c.ExportMetricsFile, metrics, metricsGroup)
	}

	if c.PromExport {
		err := c.PushPrometheusMetrics(metrics)
		if err != nil {
			return err
		}
	}

	if c.Cloudwatch {
		session, err := session.NewSession()
		if err != nil {
			return err
		}

		svc := cloudwatch.New(session)
		_, err = c.PutCloudwatchMetrics(svc, metrics)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateCLI(c *cli.Context) error {
	prometheusEnabled := false
	var header []string
	var httpMethod string
	var data []byte
	var duration time.Duration
	var urlSuffixes []string
	var statFile string
	fileMode := false

	if c.Int("concurrency") == 0 {
		return errConcurrencyLevel
	}

	if c.Int("requests") == 0 {
		return errRequestNo
	}

	if c.String("duration") != "" {
		var err error
		duration, err = time.ParseDuration(c.String("duration"))
		if err != nil {
			return err
		}
		if duration <= 0 {
			return errDurationValue
		}
	}

	if !client.IsValidURL(c.String("url")) {
		return errNotValidURL
	}

	if c.String("prompushgwurl") != "" {
		prometheusEnabled = true
	}

	if c.String("header") != "" {
		length := 0
		length, header = client.SplitHeader(c.String("header"))
		if length != 2 {
			return errNotValidHeader
		}
	}

	if c.String("file") != "" {
		var err error
		urlSuffixes, err = readLocalRemoteFile(c.String("file"))
		if err != nil {
			return nil
		}
		fileMode = true
	}

	statFile = c.String("stat-file")

	if c.String("postfile") != "" {
		httpMethod = "POST"
		fileData, err := readFile(c.String("postfile"))
		if err != nil {
			return err
		}
		data = fileData
	} else if c.String("putfile") != "" {
		httpMethod = "PUT"
		fileData, err := readFile(c.String("putfile"))
		if err != nil {
			return err
		}
		data = fileData
	} else if c.String("patchfile") != "" {
		httpMethod = "PATCH"
		fileData, err := readFile(c.String("patchfile"))
		if err != nil {
			return err
		}
		data = fileData
	} else {
		httpMethod = "GET"
	}

	tlsConfig := new(tls.Config)
	if c.String("ca") != "" {
		pemCerts, err := ioutil.ReadFile(c.String("ca"))
		if err != nil {
			return err
		}
		ca := x509.NewCertPool()
		if !ca.AppendCertsFromPEM(pemCerts) {
			return fmt.Errorf("failed to read CA from PEM")
		}
		tlsConfig.RootCAs = ca
	}

	if c.String("cert") != "" && c.String("key") != "" {
		cert, err := tls.LoadX509KeyPair(c.String("cert"), c.String("key"))
		if err != nil {
			return err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	cass := &client.Cassowary{
		BaseURL:           c.String("url"),
		PromExport:        prometheusEnabled,
		TLSConfig:         tlsConfig,
		PromURL:           c.String("prompushgwurl"),
		Cloudwatch:        c.Bool("cloudwatch"),
		Boxplot:           c.Bool("boxplot"),
		Histogram:         c.Bool("histogram"),
		ExportMetrics:     c.Bool("json-metrics"),
		ExportMetricsFile: c.String("json-metrics-file"),
		StatFile:          statFile,
		DisableKeepAlive:  c.Bool("disable-keep-alive"),
		Timeout:           c.Int("timeout"),
		Duration:          duration,
		Groups: []client.QueryGroup{
			{
				Name:             "default",
				Method:           httpMethod,
				URLPaths:         urlSuffixes,
				Data:             data,
				FileMode:         fileMode,
				ConcurrencyLevel: c.Int("concurrency"),
				Requests:         c.Int("requests"),
				RequestHeader:    header,
			},
		},
	}

	return runLoadTest(cass)
}

func runCLI(args []string) {
	app := cli.NewApp()
	app.Name = "cassowary - 學名"
	app.HelpName = "cassowary"
	app.UsageText = "cassowary [command] [command options] [arguments...]"
	app.EnableBashCompletion = true
	app.Usage = ""
	app.Version = version
	app.Commands = []*cli.Command{
		{
			Name:  "run",
			Usage: "start load-test",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "u",
					Aliases:  []string{"url"},
					Usage:    "the url (absoluteURI) to be used",
					Required: true,
				},
				&cli.IntFlag{
					Name:    "c",
					Aliases: []string{"concurrency"},
					Usage:   "number of concurrent users",
					Value:   1,
				},
				&cli.IntFlag{
					Name:    "n",
					Aliases: []string{"requests"},
					Usage:   "number of requests to perform",
					Value:   1,
				},
				&cli.StringFlag{
					Name:    "f",
					Aliases: []string{"file"},
					Usage:   "file-slurp mode: specify `FILE` path, local or www, containing the url suffixes",
				},
				&cli.StringFlag{
					Name:    "d",
					Aliases: []string{"duration"},
					Usage:   "set the duration of the load test (example: do 100 requests in a duration of 30s)",
				},
				&cli.IntFlag{
					Name:    "t",
					Aliases: []string{"timeout"},
					Usage:   "http client timeout",
					Value:   5,
				},
				&cli.StringFlag{
					Name:    "p",
					Aliases: []string{"prompushgwurl"},
					Usage:   "specify prometheus push gateway url to send metrics (optional)",
				},
				&cli.StringFlag{
					Name:    "H",
					Aliases: []string{"header"},
					Usage:   "add arbitrary header, eg. 'Host: www.example.com'",
				},
				&cli.BoolFlag{
					Name:    "C",
					Aliases: []string{"cloudwatch"},
					Usage:   "enable to send metrics to AWS Cloudwatch",
				},
				&cli.BoolFlag{
					Name:    "F",
					Aliases: []string{"json-metrics"},
					Usage:   "outputs metrics to a json file by setting flag to true",
				},
				&cli.BoolFlag{
					Name:    "b",
					Aliases: []string{"boxplot"},
					Usage:   "enable to generate a boxplot as png",
				},
				&cli.BoolFlag{
					Aliases: []string{"histogram"},
					Usage:   "enable to generate a histogram as png",
				},
				&cli.StringFlag{
					Name:  "postfile",
					Usage: "file containing data to POST (content type will default to application/json)",
				},
				&cli.StringFlag{
					Name:  "patchfile",
					Usage: "file containing data to PATCH (content type will default to application/json)",
				},
				&cli.StringFlag{
					Name:  "putfile",
					Usage: "file containing data to PUT (content type will default to application/json)",
				},
				&cli.StringFlag{
					Name:  "json-metrics-file",
					Usage: "outputs metrics to a custom json filepath, if json-metrics is set to true",
				},
				&cli.StringFlag{
					Name:  "stat-file",
					Usage: "output file for individual query statistics",
				},
				&cli.BoolFlag{
					Name:  "disable-keep-alive",
					Usage: "use this flag to disable http keep-alive",
				},
				&cli.StringFlag{
					Name:  "ca",
					Usage: "ca certificate to verify peer against",
				},
				&cli.StringFlag{
					Name:  "cert",
					Usage: "client authentication certificate",
				},
				&cli.StringFlag{
					Name:  "key",
					Usage: "client authentication key",
				},
			},
			Action: validateCLI,
		},
	}

	err := app.Run(args)
	if err != nil {
		fmt.Println(err)
		os.Exit(0)
	}
}
