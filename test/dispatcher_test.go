// +build cassandra

package test

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	// "code.google.com/p/log4go"
	"github.com/gocql/gocql"
	"github.com/iParadigms/walker"
)

type DispatcherTest struct {
	Tag                  string
	ExistingDomainInfos  []ExistingDomainInfo
	ExistingLinks        []ExistingLink
	ExpectedSegmentLinks []walker.URL
}

type ExistingDomainInfo struct {
	Dom        string
	ClaimTok   gocql.UUID
	Priority   int
	Dispatched bool
}

type ExistingLink struct {
	URL    walker.URL
	Status int // -1 indicates this is a parsed link, not yet fetched
	GetNow bool
}

var DispatcherTests = []DispatcherTest{
	// This first test is complicated, so I describe it in this comment. Below
	// you'll see we set
	//   Config.Dispatcher.MaxLinksPerSegment = 9
	//   Config.Dispatcher.RefreshPercentage = 33
	//
	// Below you see 3 GetNow links which will for sure be in segments.  That
	// means there are 6 additional links to push to segments. Of those 33%
	// should be refresh links: or 2 ( = 6 * 0.33) already crawled links. And
	// 4 (= 6-2) links should be not-yet-crawled links. And that is the
	// composition of the first tests expected.
	DispatcherTest{
		Tag: "BasicTest",

		ExistingDomainInfos: []ExistingDomainInfo{
			{"test.com", gocql.UUID{}, 0, false},
		},

		ExistingLinks: []ExistingLink{
			{URL: walker.URL{URL: urlParse("http://test.com/page1.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/page2.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/page404.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/page500.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},

			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled1.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled2.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled3.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled4.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled5.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},

			{URL: walker.URL{URL: urlParse("http://test.com/page1.html"),
				LastCrawled: time.Now().AddDate(0, 0, -4)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/page2.html"),
				LastCrawled: time.Now().AddDate(0, 0, -3)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/page404.html"),
				LastCrawled: time.Now().AddDate(0, 0, -2)}, Status: http.StatusNotFound},
			{URL: walker.URL{URL: urlParse("http://test.com/page500.html"),
				LastCrawled: time.Now().AddDate(0, 0, -1)}, Status: http.StatusInternalServerError},

			{URL: walker.URL{URL: urlParse("http://test.com/getnow1.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1, GetNow: true},
			{URL: walker.URL{URL: urlParse("http://test.com/getnow2.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1, GetNow: true},
			{URL: walker.URL{URL: urlParse("http://test.com/getnow3.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1, GetNow: true},
		},

		ExpectedSegmentLinks: []walker.URL{
			// The two oldest already crawled links
			{URL: urlParse("http://test.com/page1.html"),
				LastCrawled: time.Now().AddDate(0, 0, -4)},
			{URL: urlParse("http://test.com/page2.html"),
				LastCrawled: time.Now().AddDate(0, 0, -3)},

			// 4 uncrawled links
			{URL: urlParse("http://test.com/notcrawled1.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled2.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled3.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled4.html"),
				LastCrawled: walker.NotYetCrawled},

			// all of the getnow links
			{URL: urlParse("http://test.com/getnow1.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/getnow2.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/getnow3.html"),
				LastCrawled: walker.NotYetCrawled},
		},
	},

	// Similar to above test, but now there are no getnows, so you
	// should have 6 not-yet-crawled, and 3 already crawled
	DispatcherTest{
		Tag: "AllCrawledCorrectOrder",

		ExistingDomainInfos: []ExistingDomainInfo{
			{"test.com", gocql.UUID{}, 0, false},
		},

		ExistingLinks: []ExistingLink{
			{URL: walker.URL{URL: urlParse("http://test.com/l.html"),
				LastCrawled: time.Now().AddDate(0, -2, -4)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/m.html"),
				LastCrawled: time.Now().AddDate(0, -3, -1)}, Status: http.StatusOK},

			{URL: walker.URL{URL: urlParse("http://test.com/a.html"),
				LastCrawled: time.Now().AddDate(0, 0, -1)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/b.html"),
				LastCrawled: time.Now().AddDate(0, 0, -2)}, Status: http.StatusOK},

			{URL: walker.URL{URL: urlParse("http://test.com/c.html"),
				LastCrawled: time.Now().AddDate(0, 0, -3)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/d.html"),
				LastCrawled: time.Now().AddDate(0, 0, -4)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/e.html"),
				LastCrawled: time.Now().AddDate(0, -1, -1)}, Status: http.StatusOK},

			{URL: walker.URL{URL: urlParse("http://test.com/f.html"),
				LastCrawled: time.Now().AddDate(0, -1, -2)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/g.html"),
				LastCrawled: time.Now().AddDate(0, -1, -3)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/h.html"),
				LastCrawled: time.Now().AddDate(0, -1, -4)}, Status: http.StatusOK},

			{URL: walker.URL{URL: urlParse("http://test.com/i.html"),
				LastCrawled: time.Now().AddDate(0, -2, -1)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/j.html"),
				LastCrawled: time.Now().AddDate(0, -2, -2)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/k.html"),
				LastCrawled: time.Now().AddDate(0, -2, -3)}, Status: http.StatusOK},

			// These two links cover up the previous two l and m.html links.
			{URL: walker.URL{URL: urlParse("http://test.com/l.html"),
				LastCrawled: time.Now()}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/m.html"),
				LastCrawled: time.Now()}, Status: http.StatusOK},
		},

		ExpectedSegmentLinks: []walker.URL{
			// 9 Oldest links
			{URL: urlParse("http://test.com/k.html"),
				LastCrawled: time.Now().AddDate(0, -2, -3)},
			{URL: urlParse("http://test.com/j.html"),
				LastCrawled: time.Now().AddDate(0, -2, -2)},
			{URL: urlParse("http://test.com/i.html"),
				LastCrawled: time.Now().AddDate(0, -2, -1)},

			{URL: urlParse("http://test.com/h.html"),
				LastCrawled: time.Now().AddDate(0, -1, -4)},
			{URL: urlParse("http://test.com/g.html"),
				LastCrawled: time.Now().AddDate(0, -1, -3)},
			{URL: urlParse("http://test.com/f.html"),
				LastCrawled: time.Now().AddDate(0, -1, -2)},

			{URL: urlParse("http://test.com/e.html"),
				LastCrawled: time.Now().AddDate(0, -1, -1)},
			{URL: urlParse("http://test.com/d.html"),
				LastCrawled: time.Now().AddDate(0, 0, -4)},
			{URL: urlParse("http://test.com/c.html"),
				LastCrawled: time.Now().AddDate(0, 0, -3)},
		},
	},

	DispatcherTest{
		Tag: "NoGetNow",

		ExistingDomainInfos: []ExistingDomainInfo{
			{"test.com", gocql.UUID{}, 0, false},
		},

		ExistingLinks: []ExistingLink{
			{URL: walker.URL{URL: urlParse("http://test.com/page1.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/page2.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/page404.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/page500.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},

			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled1.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled2.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled3.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled4.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled5.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled6.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled7.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled8.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled9.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},

			{URL: walker.URL{URL: urlParse("http://test.com/page1.html"),
				LastCrawled: time.Now().AddDate(0, 0, -4)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/page2.html"),
				LastCrawled: time.Now().AddDate(0, 0, -3)}, Status: http.StatusOK},
			{URL: walker.URL{URL: urlParse("http://test.com/page404.html"),
				LastCrawled: time.Now().AddDate(0, 0, -2)}, Status: http.StatusNotFound},
			{URL: walker.URL{URL: urlParse("http://test.com/page500.html"),
				LastCrawled: time.Now().AddDate(0, 0, -1)}, Status: http.StatusInternalServerError},
		},

		ExpectedSegmentLinks: []walker.URL{
			// 3 crawled links
			{URL: urlParse("http://test.com/page1.html"),
				LastCrawled: time.Now().AddDate(0, 0, -4)},
			{URL: urlParse("http://test.com/page2.html"),
				LastCrawled: time.Now().AddDate(0, 0, -3)},
			{URL: urlParse("http://test.com/page404.html"),
				LastCrawled: time.Now().AddDate(0, 0, -2)},

			// 6 uncrawled links
			{URL: urlParse("http://test.com/notcrawled1.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled2.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled3.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled4.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled5.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled6.html"),
				LastCrawled: walker.NotYetCrawled},
		},
	},

	DispatcherTest{
		Tag: "OnlyUncrawled",

		ExistingDomainInfos: []ExistingDomainInfo{
			{"test.com", gocql.UUID{}, 0, false},
		},

		ExistingLinks: []ExistingLink{
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled1.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled2.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled3.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled4.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled5.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled6.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled7.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled8.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
			{URL: walker.URL{URL: urlParse("http://test.com/notcrawled9.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
		},

		ExpectedSegmentLinks: []walker.URL{
			{URL: urlParse("http://test.com/notcrawled1.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled2.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled3.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled4.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled5.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled6.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled7.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled8.html"),
				LastCrawled: walker.NotYetCrawled},
			{URL: urlParse("http://test.com/notcrawled9.html"),
				LastCrawled: walker.NotYetCrawled},
		},
	},

	DispatcherTest{ // Verifies that we work with query parameters properly
		Tag: "QueryParmsOK",
		ExistingDomainInfos: []ExistingDomainInfo{
			{"test.com", gocql.UUID{}, 0, false},
		},
		ExistingLinks: []ExistingLink{
			{URL: walker.URL{URL: urlParse("http://test.com/page1.html?p=v"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
		},
		ExpectedSegmentLinks: []walker.URL{
			{URL: urlParse("http://test.com/page1.html?p=v"),
				LastCrawled: walker.NotYetCrawled},
		},
	},

	DispatcherTest{ // Verifies that we don't generate an already-dispatched domain
		Tag: "NoAlreadyDispatched",
		ExistingDomainInfos: []ExistingDomainInfo{
			{"test.com", gocql.UUID{}, 0, true},
		},
		ExistingLinks: []ExistingLink{
			{URL: walker.URL{URL: urlParse("http://test.com/page1.html"),
				LastCrawled: walker.NotYetCrawled}, Status: -1},
		},
		ExpectedSegmentLinks: []walker.URL{},
	},
}

func TestDispatcherBasic(t *testing.T) {
	// These config settings MUST be here. The results of the test
	// change if these are changed.
	walker.Config.Dispatcher.MaxLinksPerSegment = 9
	walker.Config.Dispatcher.RefreshPercentage = 33

	var q *gocql.Query

	for _, dt := range DispatcherTests {
		db := getDB(t) // runs between tests to reset the db

		for _, edi := range dt.ExistingDomainInfos {
			q = db.Query(`INSERT INTO domain_info (dom, claim_tok, priority, dispatched)
							VALUES (?, ?, ?, ?)`,
				edi.Dom, edi.ClaimTok, edi.Priority, edi.Dispatched)
			if err := q.Exec(); err != nil {
				t.Fatalf("Failed to insert test domain info: %v\nQuery: %v", err, q)
			}
		}

		for _, el := range dt.ExistingLinks {
			dom, subdom, _ := el.URL.TLDPlusOneAndSubdomain()
			if el.Status == -1 {
				q = db.Query(`INSERT INTO links (dom, subdom, path, proto, time, getnow)
								VALUES (?, ?, ?, ?, ?, ?)`,
					dom,
					subdom,
					el.URL.RequestURI(),
					el.URL.Scheme,
					el.URL.LastCrawled,
					el.GetNow)
			} else {
				q = db.Query(`INSERT INTO links (dom, subdom, path, proto, time, stat, getnow)
								VALUES (?, ?, ?, ?, ?, ?, ?)`,
					dom,
					subdom,
					el.URL.RequestURI(),
					el.URL.Scheme,
					el.URL.LastCrawled,
					el.Status,
					el.GetNow)
			}
			if err := q.Exec(); err != nil {
				t.Fatalf("Failed to insert test links: %v\nQuery: %v", err, q)
			}
		}

		d := &walker.CassandraDispatcher{}
		go d.StartDispatcher()
		time.Sleep(time.Second)
		d.StopDispatcher()

		expectedResults := map[url.URL]bool{}
		for _, esl := range dt.ExpectedSegmentLinks {
			expectedResults[*esl.URL] = true
		}

		results := map[url.URL]bool{}
		iter := db.Query(`SELECT dom, subdom, path, proto
							FROM segments WHERE dom = 'test.com'`).Iter()
		var linkdomain, subdomain, path, protocol string
		for iter.Scan(&linkdomain, &subdomain, &path, &protocol) {
			u, _ := walker.CreateURL(linkdomain, subdomain, path, protocol, walker.NotYetCrawled)
			results[*u.URL] = true
		}
		if !reflect.DeepEqual(results, expectedResults) {
			t.Errorf("For tag %q expected results in segments: %v\nBut got: %v",
				dt.Tag, expectedResults, results)
		}

		for _, edi := range dt.ExistingDomainInfos {
			q = db.Query(`SELECT dispatched FROM domain_info WHERE dom = ?`, edi.Dom)
			var dispatched bool
			if err := q.Scan(&dispatched); err != nil {
				t.Fatalf("For tag %q failed to insert find domain info: %v\nQuery: %v", dt.Tag, err, q)
			}
			if !dispatched {
				t.Errorf("For tag %q `dispatched` flag not set on domain: %v", dt.Tag, edi.Dom)
			}
		}
	}
}

func TestDispatcherDispatchedFalseIfNoLinks(t *testing.T) {
	db := getDB(t)
	q := db.Query(`INSERT INTO domain_info (dom, claim_tok, priority, dispatched)
					VALUES (?, ?, ?, ?)`, "test.com", gocql.UUID{}, 0, false)
	if err := q.Exec(); err != nil {
		t.Fatalf("Failed to insert test domain info: %v\nQuery: %v", err, q)
	}

	d := &walker.CassandraDispatcher{}
	go d.StartDispatcher()
	// Pete says this time used to be 10 millis, but I was observing spurious nil channel
	// panics. Increased it to 100 to see if that would help.
	time.Sleep(time.Millisecond * 100)
	d.StopDispatcher()

	q = db.Query(`SELECT dispatched FROM domain_info WHERE dom = ?`, "test.com")
	var dispatched bool
	if err := q.Scan(&dispatched); err != nil {
		t.Fatalf("Failed to find domain info: %v\nQuery: %v", err, q)
	}
	if dispatched {
		t.Errorf("`dispatched` flag set to true when no links existed")
	}
}
