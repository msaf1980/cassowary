package main

import (
	"fmt"

	"github.com/fatih/color"
)

const (
	summaryTable = "\n" +
		" Elapsed(ms)............: Min=%s\tMax=%s\tAvg=%s\tMedian=%s\tp(95)=%s\tp(99)=%s\n" +
		" TCP Connect(ms)........: Min=%s\tMax=%s\tAvg=%s\tMedian=%s\tp(95)=%s\tp(99)=%s\n" +
		" Server Processing(ms)..: Min=%s\tMax=%s\tAvg=%s\tMedian=%s\tp(95)=%s\tp(99)=%s\n" +
		" Content Transfer.......: Min=%s\tMax=%s\tAvg=%s\tMedian=%s\tp(95)=%s\tp(99)=%s\n" +
		" Body Size(bytes).......: Min=%s\tMax=%s\tAvg=%s\tMedian=%s\tp(95)=%s\tp(99)=%s\n" +
		" Response Size(bytes)...: Min=%s\tMax=%s\tAvg=%s\tMedian=%s\tp(95)=%s\tp(99)=%s\n" +
		"\n" +
		"Summary:\n" +
		" Total Req.......................: %s\n" +
		" Failed Req......................: %s\n" +
		" DNS Lookup......................: %sms\n" +
		" Req/s...........................: %s\n\n"
)

func printf(format string, a ...interface{}) {
	fmt.Fprintf(color.Output, format, a...)
}
