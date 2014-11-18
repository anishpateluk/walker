package helpers

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/iParadigms/walker"
)

// LoadTestConfig loads the given test config yaml file. The given path is
// assumed to be relative to the `walker/helpers/` directory, the location of this
// file. This will panic if it cannot read the requested config file. If you
// expect an error or are testing walker.ReadConfigFile, use `GetTestFileDir()`
// instead.
func LoadTestConfig(filename string) {
	testdir := GetTestFileDir()
	err := walker.ReadConfigFile(path.Join(testdir, filename))
	if err != nil {
		panic(err.Error())
	}
}

// GetTestFileDir returns the directory where shared test files are stored, for
// example test config files. It will panic if it could not get the path from
// the runtime.
func GetTestFileDir() string {
	_, p, _, ok := runtime.Caller(0)
	if !ok {
		panic("Failed to get location of test source file")
	}
	return path.Dir(p)
}

// FakeDial makes connections to localhost, no matter what addr was given.
func FakeDial(network, addr string) (net.Conn, error) {
	_, port, _ := net.SplitHostPort(addr)
	return net.Dial(network, net.JoinHostPort("localhost", port))
}

// GetFakeTransport gets a http.RoundTripper that uses FakeDial
func GetFakeTransport() http.RoundTripper {
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		Dial:                FakeDial,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

//
// http.Transport that tracks which requests where canceled.
//
type CancelTrackingTransport struct {
	http.Transport
	Canceled map[string]int
}

func (self *CancelTrackingTransport) CancelRequest(req *http.Request) {
	key := req.URL.String()
	count := 0
	if c, cok := self.Canceled[key]; cok {
		count = c
	}
	self.Canceled[key] = count + 1
	self.Transport.CancelRequest(req)
}

//
// wontConnectDial has a Dial routine that will never connect
//
type wontConnectDial struct {
	quit chan struct{}
}

func (self *wontConnectDial) Close() error {
	close(self.quit)
	return nil
}

func (self *wontConnectDial) Dial(network, addr string) (net.Conn, error) {
	<-self.quit
	return nil, fmt.Errorf("I'll never connect!!")
}

func GetWontConnectTransport() (*CancelTrackingTransport, io.Closer) {
	dialer := &wontConnectDial{make(chan struct{})}
	trans := &CancelTrackingTransport{
		Transport: http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			Dial:                dialer.Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		Canceled: make(map[string]int),
	}

	return trans, dialer
}

//
// Spoof an Addr interface. Used by stallingConn
//
type emptyAddr struct{}

func (self *emptyAddr) Network() string {
	return ""
}

func (self *emptyAddr) String() string {
	return ""

}

//
// stallingConn will stall during any read or write
//
type stallingConn struct {
	closed bool
	quit   chan struct{}
}

func (self *stallingConn) Read(b []byte) (int, error) {
	<-self.quit
	return 0, fmt.Errorf("Staling Read")
}

func (self *stallingConn) Write(b []byte) (int, error) {
	<-self.quit
	return 0, fmt.Errorf("Staling Write")
}

func (self *stallingConn) Close() error {
	if !self.closed {
		close(self.quit)
	}
	self.closed = true
	return nil
}

func (self *stallingConn) LocalAddr() net.Addr {
	return &emptyAddr{}
}

func (self *stallingConn) RemoteAddr() net.Addr {
	return &emptyAddr{}
}

func (self *stallingConn) SetDeadline(t time.Time) error {
	return nil
}

func (self *stallingConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (self *stallingConn) SetWriteDeadline(t time.Time) error {
	return nil
}

//
// stallCloser tracks a bundle of stallingConn's
//
type stallCloser struct {
	stalls map[*stallingConn]bool
}

func (self *stallCloser) Close() error {
	for conn := range self.stalls {
		conn.Close()
	}
	return nil
}

func (self *stallCloser) newConn() *stallingConn {
	x := &stallingConn{quit: make(chan struct{})}
	self.stalls[x] = true
	return x
}

func (self *stallCloser) Dial(network, addr string) (net.Conn, error) {
	return self.newConn(), nil
}

func GetStallingReadTransport() (*CancelTrackingTransport, io.Closer) {
	dialer := &stallCloser{make(map[*stallingConn]bool)}
	trans := &CancelTrackingTransport{
		Transport: http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			Dial:                dialer.Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		Canceled: make(map[string]int),
	}
	return trans, dialer
}

// Parse is a helper to just get a walker.URL object from a string we know is a
// safe url (ParseURL requires us to deal with potential errors)
func Parse(ref string) *walker.URL {
	u, err := walker.ParseURL(ref)
	if err != nil {
		panic("Failed to parse walker.URL: " + ref)
	}
	return u
}

// UrlParse is similar to `parse` but gives a Go builtin URL type (not a walker
// URL)
func UrlParse(ref string) *url.URL {
	u := Parse(ref)
	return u.URL
}

func Response404() *http.Response {
	return &http.Response{
		Status:        "404",
		StatusCode:    404,
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        http.Header{"Content-Type": []string{"text/html"}},
		Body:          ioutil.NopCloser(strings.NewReader("")),
		ContentLength: -1,
	}
}

func Response307(link string) *http.Response {
	return &http.Response{
		Status:        "307",
		StatusCode:    307,
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        http.Header{"Location": []string{link}, "Content-Type": []string{"text/html"}},
		Body:          ioutil.NopCloser(strings.NewReader("")),
		ContentLength: -1,
	}
}

func Response200() *http.Response {
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Header:     http.Header{"Content-Type": []string{"text/html"}},
		Body: ioutil.NopCloser(strings.NewReader(
			`<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title>No Links</title>
</head>
<div id="menu">
</div>
</html>`)),
		ContentLength: -1,
	}
}

// MapRoundTrip maps input links --> http.Response. See TestRedirects for example.
type MapRoundTrip struct {
	Responses map[string]*http.Response
}

func (mrt *MapRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	res, resOk := mrt.Responses[req.URL.String()]
	if !resOk {
		return Response404(), nil
	}
	return res, nil
}

// This allows the MapRoundTrip to be canceled. Which is needed to prevent
// errant robots.txt GET's to break TestRedirects.
func (self *MapRoundTrip) CancelRequest(req *http.Request) {
}
