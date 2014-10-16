// +build sudo

package test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/iParadigms/walker"
	"github.com/stretchr/testify/mock"
)

const html_body string = `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title>Test norobots site</title>
</head>

<div id="menu">
	<a href="/dir1/">Dir1</a>
	<a href="/dir2/">Dir2</a>
	<a id="other" href="http://other.com/" title="stuff">Other</a>
</div>
</html>`

const html_body_nolinks string = `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title>No Links</title>
</head>
<div id="menu">
</div>
</html>`

const html_test_links string = `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title>Test links page</title>
</head>

<div id="menu">
	<a href="relative-dir/">link</a>
	<a href="relative-page/page.html">link</a>
	<a href="/abs-relative-dir/">link</a>
	<a href="/abs-relative-page/page.html">link</a>
	<a href="https://other.org/abs-dir/">link</a>
	<a href="https://other.org/abs-page/page.html">link</a>
	<a href="javascript:doStuff();">link</a>
	<a href="ftp:ignoreme.zip;">link</a>
	<a href="ftP:ignoreme.zip;">link</a>
	<a href="hTTP:donot/ignore.html">link</a>
</div>
</html>`

func TestBasicFetchManagerRun(t *testing.T) {
	ds := &MockDatastore{}
	ds.On("ClaimNewHost").Return("norobots.com").Once()
	ds.On("LinksForHost", "norobots.com").Return([]*walker.URL{
		parse("http://norobots.com/page1.html"),
		parse("http://norobots.com/page2.html"),
		parse("http://norobots.com/page3.html"),
	})
	ds.On("UnclaimHost", "norobots.com").Return()

	ds.On("ClaimNewHost").Return("robotsdelay1.com").Once()
	ds.On("LinksForHost", "robotsdelay1.com").Return([]*walker.URL{
		parse("http://robotsdelay1.com/page4.html"),
		parse("http://robotsdelay1.com/page5.html"),
	})
	ds.On("UnclaimHost", "robotsdelay1.com").Return()

	ds.On("ClaimNewHost").Return("accept.com").Once()
	ds.On("LinksForHost", "accept.com").Return([]*walker.URL{
		parse("http://accept.com/accept_html.html"),
		parse("http://accept.com/accept_text.txt"),
		parse("http://accept.com/donthandle"),
	})
	ds.On("UnclaimHost", "accept.com").Return()

	ds.On("ClaimNewHost").Return("linktests.com").Once()
	ds.On("LinksForHost", "linktests.com").Return([]*walker.URL{
		parse("http://linktests.com/links/test.html"),
	})
	ds.On("UnclaimHost", "linktests.com").Return()

	// This last call will make ClaimNewHost return "" on each subsequent call,
	// which will put the fetcher to sleep.
	ds.On("ClaimNewHost").Return("")

	ds.On("StoreURLFetchResults", mock.AnythingOfType("*walker.FetchResults")).Return()
	ds.On("StoreParsedURL",
		mock.AnythingOfType("*walker.URL"),
		mock.AnythingOfType("*walker.FetchResults")).Return()

	h := &MockHandler{}
	h.On("HandleResponse", mock.Anything).Return()

	rs, err := NewMockRemoteServer()
	if err != nil {
		t.Fatal(err)
	}
	rs.SetResponse("http://norobots.com/robots.txt", &MockResponse{Status: 404})
	rs.SetResponse("http://norobots.com/page1.html", &MockResponse{
		Body: html_body,
	})
	rs.SetResponse("http://robotsdelay1.com/robots.txt", &MockResponse{
		Body: "User-agent: *\nCrawl-delay: 1\n",
	})

	walker.Config.AcceptFormats = []string{"text/html", "text/plain"}
	rs.SetResponse("http://accept.com/robots.txt", &MockResponse{Status: 404})
	rs.SetResponse("http://accept.com/accept_html.html", &MockResponse{
		ContentType: "text/html",
		Body:        html_body_nolinks,
	})
	rs.SetResponse("http://accept.com/accept_text.txt", &MockResponse{
		ContentType: "text/plain",
	})
	rs.SetResponse("http://accept.com/donthandle", &MockResponse{
		ContentType: "foo/bar",
	})
	rs.SetResponse("http://linktests.com/links/test.html", &MockResponse{
		Body: html_test_links,
	})
	manager := &walker.FetchManager{
		Datastore: ds,
		Handler:   h,
		Transport: GetFakeTransport(),
	}

	go manager.Start()
	time.Sleep(time.Second * 3)
	manager.Stop()

	rs.Stop()
	recvTextHtml := false
	recvTextPlain := false
	for _, call := range h.Calls {
		fr := call.Arguments.Get(0).(*walker.FetchResults)
		switch fr.URL.String() {
		case "http://norobots.com/page1.html":
			contents, _ := ioutil.ReadAll(fr.Response.Body)
			if string(contents) != html_body {
				t.Errorf("For %v, expected:\n%v\n\nBut got:\n%v\n",
					fr.URL, html_body, string(contents))
			}
		case "http://norobots.com/page2.html":
		case "http://norobots.com/page3.html":
		case "http://robotsdelay1.com/page4.html":
		case "http://robotsdelay1.com/page5.html":
		case "http://accept.com/accept_html.html":
			recvTextHtml = true
		case "http://accept.com/accept_text.txt":
			recvTextPlain = true
		case "http://linktests.com/links/test.html":
		default:
			t.Errorf("Got a Handler.HandleResponse call we didn't expect: %v", fr)
		}
	}
	if !recvTextHtml {
		t.Errorf("Failed to handle explicit Content-Type: text/html")
	}
	if !recvTextPlain {
		t.Errorf("Failed to handle Content-Type: text/plain")
	}

	// Link tests to ensure we resolve URLs to proper absolute forms
	for _, call := range ds.Calls {
		if call.Method == "StoreParsedURL" {
			u := call.Arguments.Get(0).(*walker.URL)
			fr := call.Arguments.Get(1).(*walker.FetchResults)
			if fr.URL.String() != "http://linktests.com/links/test.html" {
				continue
			}
			switch u.String() {
			case "http://linktests.com/links/relative-dir/":
			case "http://linktests.com/links/relative-page/page.html":
			case "http://linktests.com/abs-relative-dir/":
			case "http://linktests.com/abs-relative-page/page.html":
			case "https://other.org/abs-dir/":
			case "https://other.org/abs-page/page.html":
			case "http:donot/ignore.html":
			default:
				t.Errorf("StoreParsedURL call we didn't expect: %v", u)
			}
		}
	}

	ds.AssertExpectations(t)
	h.AssertExpectations(t)
}

func TestFetcherBlacklistsPrivateIPs(t *testing.T) {
	orig := walker.Config.BlacklistPrivateIPs
	defer func() { walker.Config.BlacklistPrivateIPs = orig }()
	walker.Config.BlacklistPrivateIPs = true

	ds := &MockDatastore{}
	ds.On("ClaimNewHost").Return("private.com").Once()
	ds.On("UnclaimHost", "private.com").Return()
	ds.On("ClaimNewHost").Return("")

	h := &MockHandler{}

	rs, err := NewMockRemoteServer()
	if err != nil {
		t.Fatal(err)
	}

	manager := &walker.FetchManager{
		Datastore: ds,
		Handler:   h,
		Transport: GetFakeTransport(),
	}

	go manager.Start()
	time.Sleep(time.Second * 1)
	manager.Stop()
	rs.Stop()

	if len(h.Calls) != 0 {
		t.Error("Did not expect any handler calls due to host resolving to private IP")
	}

	ds.AssertExpectations(t)
	h.AssertExpectations(t)
	ds.AssertNotCalled(t, "LinksForHost", "private.com")
}

func TestFetcherCreatesTransport(t *testing.T) {
	orig := walker.Config.BlacklistPrivateIPs
	defer func() { walker.Config.BlacklistPrivateIPs = orig }()
	walker.Config.BlacklistPrivateIPs = false

	ds := &MockDatastore{}
	ds.On("ClaimNewHost").Return("localhost.localdomain").Once()
	ds.On("LinksForHost", "localhost.localdomain").Return([]*walker.URL{
		parse("http://localhost.localdomain/"),
	})
	ds.On("StoreURLFetchResults", mock.AnythingOfType("*walker.FetchResults")).Return()
	ds.On("UnclaimHost", "localhost.localdomain").Return()
	ds.On("ClaimNewHost").Return("")

	h := &MockHandler{}
	h.On("HandleResponse", mock.Anything).Return()

	rs, err := NewMockRemoteServer()
	if err != nil {
		t.Fatal(err)
	}

	manager := &walker.FetchManager{
		Datastore: ds,
		Handler:   h,
	}

	go manager.Start()
	time.Sleep(time.Second * 1)
	manager.Stop()
	rs.Stop()

	if manager.Transport == nil {
		t.Fatalf("Expected Transport to get set")
	}
	_, ok := manager.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Expected Transport to get set to a *http.Transport")
	}

	// It would be great to check that the DNS cache actually got used here,
	// but with the current design there seems to be no way to check it

	ds.AssertExpectations(t)
	h.AssertExpectations(t)
}

func TestRedirects(t *testing.T) {
	link := func(index int) string {
		return fmt.Sprintf("http://sub.dom.com/page%d.html", index)
	}

	roundTriper := mapRoundTrip{
		responses: map[string]*http.Response{
			link(1): response307(link(2)),
			link(2): response307(link(3)),
			link(3): response200(),
		},
	}

	ds := &MockDatastore{}
	ds.On("ClaimNewHost").Return("dom.com").Once()
	ds.On("LinksForHost", "dom.com").Return([]*walker.URL{
		parse(link(1)),
	})
	ds.On("StoreURLFetchResults", mock.AnythingOfType("*walker.FetchResults")).Return()
	ds.On("UnclaimHost", "dom.com").Return()
	ds.On("ClaimNewHost").Return("")

	h := &MockHandler{}
	h.On("HandleResponse", mock.Anything).Return()

	manager := &walker.FetchManager{
		Datastore: ds,
		Handler:   h,
		Transport: &roundTriper,
	}

	go manager.Start()
	time.Sleep(time.Second * 2)
	manager.Stop()
	if len(h.Calls) < 1 {
		t.Fatalf("Expected to find calls made to handler, but didn't")
	}
	fr := h.Calls[0].Arguments.Get(0).(*walker.FetchResults)

	if fr.URL.String() != link(1) {
		t.Errorf("URL mismatch, got %q, expected %q", fr.URL.String(), link(1))
	}
	if len(fr.RedirectedFrom) != 2 {
		t.Errorf("RedirectedFrom length mismatch, got %d, expected %d", len(fr.RedirectedFrom), 2)
	}
	if fr.RedirectedFrom[0].String() != link(2) {
		t.Errorf("RedirectedFrom[0] mismatch, got %q, expected %q", fr.RedirectedFrom[0].String(), link(2))
	}
	if fr.RedirectedFrom[1].String() != link(3) {
		t.Errorf("RedirectedFrom[0] mismatch, got %q, expected %q", fr.RedirectedFrom[1].String(), link(3))
	}

	ds.AssertExpectations(t)
	h.AssertExpectations(t)
}

func TestHrefWithSpace(t *testing.T) {

	testPage := "http://t.com/page1.html"
	const html_with_href_space = `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title>Test links page</title>
</head>

<div id="menu">
	<a href=" relative-dir/">link</a>
	<a href=" relative-page/page.html">link</a>
	<a href=" /abs-relative-dir/">link</a>
	<a href=" /abs-relative-page/page.html">link</a>
	<a href=" https://other.org/abs-dir/">link</a>
	<a href=" https://other.org/abs-page/page.html">link</a>
</div>
</html>`

	ds := &MockDatastore{}
	ds.On("ClaimNewHost").Return("t.com").Once()
	ds.On("LinksForHost", "t.com").Return([]*walker.URL{
		parse(testPage),
	})
	ds.On("UnclaimHost", "t.com").Return()
	ds.On("ClaimNewHost").Return("")

	ds.On("StoreURLFetchResults", mock.AnythingOfType("*walker.FetchResults")).Return()
	ds.On("StoreParsedURL",
		mock.AnythingOfType("*walker.URL"),
		mock.AnythingOfType("*walker.FetchResults")).Return()

	h := &MockHandler{}
	h.On("HandleResponse", mock.Anything).Return()

	rs, err := NewMockRemoteServer()
	if err != nil {
		t.Fatal(err)
	}
	rs.SetResponse(testPage, &MockResponse{
		ContentType: "text/html",
		Body:        html_with_href_space,
	})

	manager := &walker.FetchManager{
		Datastore: ds,
		Handler:   h,
		Transport: GetFakeTransport(),
	}

	go manager.Start()
	time.Sleep(time.Second * 2)
	manager.Stop()

	rs.Stop()

	foundTCom := false
	for _, call := range h.Calls {
		fr := call.Arguments.Get(0).(*walker.FetchResults)
		if fr.URL.String() == testPage {
			foundTCom = true
			break
		}
	}
	if !foundTCom {
		t.Fatalf("Failed to find pushed link 'http://t.com/page1.html'")
	}

	expected := map[string]bool{
		"http://t.com/relative-dir/":               true,
		"http://t.com/relative-page/page.html":     true,
		"http://t.com/abs-relative-dir/":           true,
		"http://t.com/abs-relative-page/page.html": true,
		"https://other.org/abs-dir/":               true,
		"https://other.org/abs-page/page.html":     true,
	}

	for _, call := range ds.Calls {
		if call.Method == "StoreParsedURL" {
			u := call.Arguments.Get(0).(*walker.URL)
			fr := call.Arguments.Get(1).(*walker.FetchResults)
			if fr.URL.String() == testPage {
				if expected[u.String()] {
					delete(expected, u.String())
				} else {
					t.Errorf("StoreParsedURL mismatch found unexpected link %q", u.String())
				}
			}
		}
	}

	for link, _ := range expected {
		t.Errorf("StoreParsedURL didn't find link %q", link)
	}

	ds.AssertExpectations(t)
	h.AssertExpectations(t)
}
