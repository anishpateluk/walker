package walker

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/stretchr/testify/mock"
)

// MockDatastore implements walker's Datastore interface for testing.
type MockDatastore struct {
	mock.Mock
}

func (ds *MockDatastore) StoreParsedURL(u *URL, fr *FetchResults) {
	ds.Mock.Called(u, fr)
}

func (ds *MockDatastore) StoreURLFetchResults(fr *FetchResults) {
	ds.Mock.Called(fr)
}

// ClaimNewHost implements walker.Datastore interface
func (ds *MockDatastore) ClaimNewHost() string {
	args := ds.Mock.Called()
	return args.String(0)
}

// UnclaimHost implements walker.Datastore interface
func (ds *MockDatastore) UnclaimHost(host string) {
	ds.Mock.Called(host)
}

// UnclaimAll implements method on cassandra.Datastore
func (ds *MockDatastore) UnclaimAll() error {
	args := ds.Mock.Called()
	return args.Error(0)
}

func (ds *MockDatastore) LinksForHost(domain string) <-chan *URL {
	args := ds.Mock.Called(domain)
	urls := args.Get(0).([]*URL)
	ch := make(chan *URL, len(urls))
	for _, u := range urls {
		ch <- u
	}
	close(ch)
	return ch
}

// KeepAlive implements walker.Datastore interface
func (ds *MockDatastore) KeepAlive() error {
	ds.Mock.Called()
	return nil
}

func (ds *MockDatastore) Close() {
	ds.Mock.Called()
}

// MockHandler implements the walker.Handler interface
type MockHandler struct {
	mock.Mock
}

func (h *MockHandler) HandleResponse(fr *FetchResults) {
	// Copy response body so that the fetcher code can reuse readBuffer
	var buffer bytes.Buffer
	_, err := buffer.ReadFrom(fr.Response.Body)
	if err != nil {
		panic(err)
	}
	fr.Response.Body = ioutil.NopCloser(bytes.NewReader(buffer.Bytes()))
	h.Mock.Called(fr)
}

// MockDispatcher implements the walker.Dispatcher interface
type MockDispatcher struct {
	mock.Mock
}

// StartDispatcher implements the walker.Dispatcher interface
func (d *MockDispatcher) StartDispatcher() error {
	args := d.Mock.Called()
	return args.Error(0)
}

// StopDispatcher implements the walker.Dispatcher interface
func (d *MockDispatcher) StopDispatcher() error {
	args := d.Mock.Called()
	return args.Error(0)
}

// MockResponse is the source object used to build fake responses in
// MockHTTPHandler.
type MockResponse struct {
	// Status defaults to 200
	Status int

	// Status defaults to "GET"
	Method string

	// Body defaults to nil (no response body)
	Body string

	// Headers of response
	Headers http.Header

	//ContentType defaults to "text/html"
	ContentType string

	// How long is the content
	ContentLength int
}

// MockHTTPHandler implements http.Handler to serve mock requests.
//
// It is not a mere mock.Mock object because using `.Return()` to return
// *http.Response objects is hard to do, and this provides conveniences in our
// tests.
//
// It should be instantiated with `NewMockRemoteServer()`
type MockHTTPHandler struct {
	// returns keeps track of mock calls and what to respond with. The top
	// level map is by method, i.e. returns["GET"]["http://test.com/"] => an
	// expected response
	returns map[string]map[string]*MockResponse

	// headers stores the headers sent to the Mock server indexed (as for
	// returns) by the pair (method, url)
	headers map[string]map[string][]http.Header
}

// NewMockHTTPHandler creates a new MockHTTPHandler
func NewMockHTTPHandler() *MockHTTPHandler {
	s := new(MockHTTPHandler)
	s.returns = map[string]map[string]*MockResponse{
		"DELETE":  map[string]*MockResponse{},
		"GET":     map[string]*MockResponse{},
		"HEAD":    map[string]*MockResponse{},
		"OPTIONS": map[string]*MockResponse{},
		"POST":    map[string]*MockResponse{},
		"PUT":     map[string]*MockResponse{},
		"TRACE":   map[string]*MockResponse{},
	}
	s.headers = map[string]map[string][]http.Header{
		"DELETE":  map[string][]http.Header{},
		"GET":     map[string][]http.Header{},
		"HEAD":    map[string][]http.Header{},
		"OPTIONS": map[string][]http.Header{},
		"POST":    map[string][]http.Header{},
		"PUT":     map[string][]http.Header{},
		"TRACE":   map[string][]http.Header{},
	}
	return s
}

// SetResponse sets a mock response for the server to return when it sees an
// incoming request matching the given link. The link should have a scheme and
// host (ex. "http://test.com/stuff"). Empty fields on MockResponse will be
// filled in with default values (see MockResponse)
func (s *MockHTTPHandler) SetResponse(link string, r *MockResponse) {
	if r.Method == "" {
		r.Method = "GET"
	}
	m := s.returns[r.Method]
	m[link] = r
}

// storeHeader stores header information
func (s *MockHTTPHandler) storeHeader(method string, link string, inHeaders http.Header) error {
	// first copy the input headers
	headers := http.Header{}
	for key, list := range inHeaders {
		var nlist []string
		nlist = append(nlist, list...)
		headers[key] = nlist
	}

	// now put them in the right place
	m, mok := s.headers[method]
	if !mok {
		return fmt.Errorf("Failed to find method %v in headers", method)
	}

	m[link] = append(m[link], headers)
	return nil
}

// ServeHTTP implements http.Handler interface
func (s *MockHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		r.URL.Scheme = "http"
	} else {
		r.URL.Scheme = "https"
	}
	r.URL.Host = r.Host

	m, ok := s.returns[r.Method]
	if !ok {
		panic(fmt.Sprintf("Got an http method we didn't expect: %v", r.Method))
	}
	link := r.URL.String()

	s.storeHeader(r.Method, link, r.Header)

	res, ok := m[link]
	if !ok {
		// No particular response requested, just return 200 OK return
		return
	}

	if res.Status == 0 {
		res.Status = 200
	}
	if res.ContentType == "" {
		res.ContentType = "text/html"
	}

	w.Header().Set("Content-Type", res.ContentType)
	if res.ContentLength != 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", res.ContentLength))
	}

	w.WriteHeader(res.Status)

	_, err := w.Write([]byte(res.Body))
	if err != nil {
		panic(fmt.Sprintf("Failed to write response for page %v, err: %v", r.URL, err))
	}
}

// MockRemoteServer wraps MockHTTPHandler to start a fake server for the user.
// Use `NewMockRemoteServer()`
type MockRemoteServer struct {
	*MockHTTPHandler
	listener net.Listener
}

// NewMockRemoteServer starts a server listening on port 80. It wraps
// MockHTTPHandler so mock return values can be set. Stop should be called at
// the end of the test to stop the server.
func NewMockRemoteServer() (*MockRemoteServer, error) {
	rs := new(MockRemoteServer)
	rs.MockHTTPHandler = NewMockHTTPHandler()
	var err error
	rs.listener, err = net.Listen("tcp", ":80")
	if err != nil {
		return nil, fmt.Errorf("Failed to listen on port 80, you probably do "+
			"not have sufficient privileges to run this test (source error: %v", err)
	}
	go http.Serve(rs.listener, rs)
	return rs, nil
}

// Headers allows user to inspect the headers included in the request object
// sent to MockRemoteServer. The triple (method, url, depth) selects which
// header to return. Here:
//   (a) method is the http method (GET, POST, etc.)
//   (b) url is the full url of the page that received the request.
//   (c) depth is an integer specifying which (of possibly many) headers for the
//   given (method, url) pair to return. Use depth=-1 to get the latest
//   header.
func (rs *MockRemoteServer) Headers(method string, url string, depth int) (http.Header, error) {
	m, mok := rs.MockHTTPHandler.headers[method]
	if !mok {
		return nil, fmt.Errorf("Failed to find method %q", method)
	}
	head, headok := m[url]
	if !headok {
		return nil, fmt.Errorf("Failed to find link %q", url)
	}

	if depth >= len(head) {
		return nil, fmt.Errorf("Depth (%d) was >= length of headers %d", depth, len(head))
	}

	if depth < 0 {
		return head[len(head)-1], nil
	}

	return head[depth], nil
}

// Requested returns true if the url was requested, and false otherwise.
func (rs *MockRemoteServer) Requested(method string, url string) bool {
	m, mok := rs.MockHTTPHandler.headers[method]
	if !mok {
		return false
	}

	head, headok := m[url]
	if !headok {
		return false
	}

	if len(head) == 0 {
		return false
	}

	return true
}

// Stop will stop the faux-server.
func (rs *MockRemoteServer) Stop() {
	rs.listener.Close()
}
