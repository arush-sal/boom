// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	gourl "net/url"
	"os"
	"regexp"
	"runtime"
	"strings"

	"boom/boomer"
)

const (
	headerRegexp = `^([\w-]+):\s*(.+)`
	authRegexp   = `^(.+):([^\s].+)`
)

type headerSlice []string

func (h *headerSlice) String() string {
	return fmt.Sprintf("%s", *h)
}

func (h *headerSlice) Set(value string) error {
	*h = append(*h, value)
	return nil
}

var (
	headerslice headerSlice
	m           = flag.String("m", "GET", "")
	headers     = flag.String("h", "", "")
	body        = flag.String("d", "", "")
	bodyFile    = flag.String("D", "", "")
	respBody    = flag.String("M", "", "")
	accept      = flag.String("A", "", "")
	contentType = flag.String("T", "text/html", "")
	authHeader  = flag.String("a", "", "")

	output = flag.String("o", "", "")

	c = flag.Int("c", 50, "")
	n = flag.Int("n", 200, "")
	q = flag.Int("q", 0, "")
	t = flag.Int("t", 0, "")

	h2 = flag.Bool("h2", false, "")

	cpus = flag.Int("cpus", runtime.GOMAXPROCS(-1), "")

	disableCompression = flag.Bool("disable-compression", false, "")
	disableKeepAlives  = flag.Bool("disable-keepalive", false, "")
	proxyAddr          = flag.String("x", "", "")
)

var usage = `Usage: boom [options...] <url>

Options:
  -n  Number of requests to run.
  -c  Number of requests to run concurrently. Total number of requests cannot
      be smaller than the concurency level.
  -q  Rate limit, in seconds (QPS).
  -o  Output type. If none provided, a summary is printed.
      "csv" is the only supported alternative. Dumps the response
      metrics in comma-seperated values format.

  -m  HTTP method, one of GET, POST, PUT, DELETE, HEAD, OPTIONS.
  -H  Custom HTTP header. You can specify as many as needed by repeating the flag.
      for example, -H "Accept: text/html" -H "Content-Type: application/xml" .
  -t  Timeout in ms.
  -A  HTTP Accept header.
  -d  HTTP request body.
  -D  File containing the HTTP request body.
  -T  Content-type, defaults to "text/html".
  -a  Basic authentication, username:password.
  -x  HTTP Proxy address as host:port.
  -M  A file that should should match char-for-char the response body.

  -h2  Make HTTP/2 requests.

  -disable-compression  Disable compression.
  -disable-keepalive    Disable keep-alive, prevents re-use of TCP
                        connections between different HTTP requests.
  -cpus                 Number of used cpu cores.
                        (default for current machine is %d cores)
`

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, fmt.Sprintf(usage, runtime.NumCPU()))
	}

	flag.Var(&headerslice, "H", "")

	flag.Parse()
	if flag.NArg() < 1 {
		usageAndExit("")
	}

	runtime.GOMAXPROCS(*cpus)
	num := *n
	conc := *c
	q := *q

	if num <= 0 || conc <= 0 {
		usageAndExit("n and c cannot be smaller than 1.")
	}

	if num < conc {
		usageAndExit("n cannot be less than c")
	}

	url := flag.Args()[0]
	method := strings.ToUpper(*m)

	// set content-type
	header := make(http.Header)
	header.Set("Content-Type", *contentType)
	// set any other additional headers
	if *headers != "" {
		usageAndExit("flag '-h' is deprecated, please use '-H' instead.")
	}
	// set any other additional repeatable headers
	for _, h := range headerslice {
		match, err := parseInputWithRegexp(h, headerRegexp)
		if err != nil {
			usageAndExit(err.Error())
		}
		header.Set(match[1], match[2])
	}

	if *accept != "" {
		header.Set("Accept", *accept)
	}

	// set basic auth if set
	var username, password string
	if *authHeader != "" {
		match, err := parseInputWithRegexp(*authHeader, authRegexp)
		if err != nil {
			usageAndExit(err.Error())
		}
		username, password = match[1], match[2]
	}

	if *output != "csv" && *output != "" {
		usageAndExit("Invalid output type; only csv is supported.")
	}

	var proxyURL *gourl.URL
	if *proxyAddr != "" {
		var err error
		proxyURL, err = gourl.Parse(*proxyAddr)
		if err != nil {
			usageAndExit(err.Error())
		}
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		usageAndExit(err.Error())
	}
	req.Header = header
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	requestBody := ""
	if *body != "" && *bodyFile != "" {
		usageAndExit("Cannot set both body and bodyFile. Pick one.")
	} else if *body != "" {
		requestBody = *body
	} else if *bodyFile != "" {
		rb, err := ioutil.ReadFile(*bodyFile)
		if err != nil {
			usageAndExit("There was an error reading the body file")
		}
		requestBody = string(rb)
	}

	responseBody := ""
	testResp := false
	if *respBody != "" {
		testResp = true
		if rb, err := ioutil.ReadFile(*respBody); err != nil {
			usageAndExit("There was an error reading the response body file.")
		} else {
			responseBody = string(rb)
		}
	}

	(&boomer.Boomer{
		Request:            req,
		RequestBody:        requestBody,
		N:                  num,
		C:                  conc,
		QPS:                q,
		Timeout:            *t,
		DisableCompression: *disableCompression,
		DisableKeepAlives:  *disableKeepAlives,
		H2:                 *h2,
		ProxyAddr:          proxyURL,
		Output:             *output,
		ResponseBody:       responseBody,
		TestResponse:       testResp,
	}).Run()
}

func usageAndExit(msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, msg)
		fmt.Fprintf(os.Stderr, "\n\n")
	}
	flag.Usage()
	fmt.Fprintf(os.Stderr, "\n")
	os.Exit(1)
}

func parseInputWithRegexp(input, regx string) ([]string, error) {
	re := regexp.MustCompile(regx)
	matches := re.FindStringSubmatch(input)
	if len(matches) < 1 {
		return nil, fmt.Errorf("could not parse the provided input; input = %v", input)
	}
	return matches, nil
}
