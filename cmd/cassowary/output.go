package main

import (
	"fmt"

	"github.com/fatih/color"
)

const (
	summaryTable = `` + "\n\n" +
		` TCP Connect.....................: Avg/mean=%sms ` + "\t" + `Median=%sms` + "\t" + `p(95)=%sms` + "\n" +
		` Server Processing...............: Avg/mean=%sms ` + "\t" + `Median=%sms` + "\t" + `p(95)=%sms` + "\n" +
		` Content Transfer................: Avg/mean=%sms ` + "\t" + `Median=%sms` + "\t" + `p(95)=%sms` + "\n" +
		` Body Size.......................: Avg/mean=%s bytes ` + "\t" + `Median=%s bytes` + "\t" + `p(95)=%s bytes` + "\n" +
		` Response Size...................: Avg/mean=%s bytes ` + "\t" + `Median=%s bytes` + "\t" + `p(95)=%s bytes` + "\n" +
		`` + "\n" +
		`Summary: ` + "\n" +
		` Total Req.......................: %s` + "\n" +
		` Failed Req......................: %s` + "\n" +
		` DNS Lookup......................: %sms` + "\n" +
		` Req/s...........................: %s` + "\n\n"
)

func printf(format string, a ...interface{}) {
	fmt.Fprintf(color.Output, format, a...)
}
