package walker

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"code.google.com/p/log4go"

	"github.com/dropbox/godropbox/container/lrucache"
	"github.com/gocql/gocql"
)

// Datastore defines the interface for an object to be used as walker's datastore.
//
// Note that this is for link and metadata storage required to make walker
// function properly. It has nothing to do with storing fetched content (see
// `Handler` for that).
type Datastore interface {
	// ClaimNewHost returns a hostname that is now claimed for this crawler to
	// crawl. A segment of links for this host is assumed to be available.
	// Returns the domain of the segment it claimed, or "" if there are none
	// available.
	ClaimNewHost() string

	// UnclaimHost indicates that all links from `LinksForHost` have been
	// processed, so other work may be done with this host. For example the
	// dispatcher will be free analyze the links and generate a new segment.
	UnclaimHost(host string)

	// LinksForHost returns a channel that will feed URLs for a given host.
	LinksForHost(host string) <-chan *URL

	// StoreURLFetchResults takes the return data/metadata from a fetch and
	// stores the visit. Fetchers will call this once for each link in the
	// segment being crawled.
	StoreURLFetchResults(fr *FetchResults)

	// StoreParsedURL stores a URL parsed out of a page (i.e. a URL we may not
	// have crawled yet). `u` is the URL to store. `fr` is the FetchResults
	// object for the fetch from which we got the URL, for any context the
	// datastore may want. A datastore implementation should handle `fr` being
	// nil, so links can be seeded without a fetch having occurred.
	//
	// URLs passed to StoreParsedURL should be absolute.
	//
	// This layer should handle efficiently deduplicating
	// links (i.e. a fetcher should be safe feeding the same URL many times.
	StoreParsedURL(u *URL, fr *FetchResults)
}

// CassandraDatastore is the primary Datastore implementation, using Apache
// Cassandra as a highly scalable backend.
type CassandraDatastore struct {
	cf *gocql.ClusterConfig
	db *gocql.Session

	// A group of domains that this datastore has already claimed, ready to
	// pass to a fetcher
	domains []string
	mu      sync.Mutex

	// A cache for domains we've already verified exist in domain_info
	addedDomains *lrucache.LRUCache
}

func GetCassandraConfig() *gocql.ClusterConfig {
	config := gocql.NewCluster(Config.Cassandra.Hosts...)
	config.Keyspace = Config.Cassandra.Keyspace
	return config
}

func NewCassandraDatastore() (*CassandraDatastore, error) {
	ds := &CassandraDatastore{
		cf: GetCassandraConfig(),
	}
	var err error
	ds.db, err = ds.cf.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("Failed to create cassandra datastore: %v", err)
	}
	ds.addedDomains = lrucache.New(Config.AddedDomainsCacheSize)
	return ds, nil
}

func (ds *CassandraDatastore) ClaimNewHost() string {

	// Get our range of priority values, sort high to low and select starting
	// with the highest priority
	//priorities := []int{}
	//var p int
	//priority_iter := ds.db.Query(`SELECT DISTINCT priority FROM domains_to_crawl`).Iter()
	//defer priority_iter.Close()
	//for priority_iter.Scan(&p) {
	//	priorities = append(priorities, p)
	//}
	//sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

	ds.mu.Lock()
	defer ds.mu.Unlock()

	if len(ds.domains) == 0 {
		start := time.Now()
		var domain string
		//TODO: when using priorities: `WHERE priority = ?`
		domain_iter := ds.db.Query(`SELECT dom FROM domain_info
									WHERE claim_tok = 00000000-0000-0000-0000-000000000000
									LIMIT 50`).Iter()
		defer domain_iter.Close()
		for domain_iter.Scan(&domain) {
			//TODO: use lightweight transaction to allow more crawlers
			//TODO: use a per-crawler uuid
			log4go.Debug("ClaimNewHost selected new domain in %v", time.Since(start))
			start = time.Now()
			crawluuid, _ := gocql.RandomUUID()
			err := ds.db.Query(`UPDATE domain_info SET claim_tok = ?, claim_time = ?
								WHERE dom = ?`,
				crawluuid, time.Now(), domain).Exec()
			if err != nil {
				log4go.Error("Failed to claim segment %v: %v", domain, err)
			} else {
				log4go.Debug("Claimed segment %v with token %v in %v", domain, crawluuid, time.Since(start))
				ds.domains = append(ds.domains, domain)
			}
		}
	}

	if len(ds.domains) == 0 {
		return ""
	}

	// Pop the last element and return it
	lastIndex := len(ds.domains) - 1
	domain := ds.domains[lastIndex]
	ds.domains = ds.domains[:lastIndex]
	return domain
}

func (ds *CassandraDatastore) UnclaimHost(host string) {
	err := ds.db.Query(`DELETE FROM segments WHERE dom = ?`, host).Exec()
	if err != nil {
		log4go.Error("Failed deleting segment links for %v: %v", host, err)
	}

	err = ds.db.Query(`UPDATE domain_info SET dispatched = false,
							claim_tok = 00000000-0000-0000-0000-000000000000
						WHERE dom = ?`, host).Exec()
	if err != nil {
		log4go.Error("Failed deleting %v from domains_to_crawl: %v", host, err)
	}
}

func (ds *CassandraDatastore) LinksForHost(domain string) <-chan *URL {
	links, err := ds.getSegmentLinks(domain)
	if err != nil {
		log4go.Error("Failed to grab segment for %v: %v", domain, err)
		return nil
	}
	log4go.Info("Returning %v links to crawl domain %v", len(links), domain)

	linkchan := make(chan *URL, len(links))
	for _, l := range links {
		linkchan <- l
	}
	close(linkchan)
	return linkchan
}

// dbfield is a little struct for updating a dynamic list of columns in the
// database
type dbfield struct {
	name  string
	value interface{}
}

func (ds *CassandraDatastore) StoreURLFetchResults(fr *FetchResults) {
	inserts := []dbfield{
		dbfield{"dom", fr.URL.ToplevelDomainPlusOne()},
		dbfield{"subdom", fr.URL.Subdomain()},
		dbfield{"path", fr.URL.RequestURI()},
		dbfield{"proto", fr.URL.Scheme},
		dbfield{"time", fr.FetchTime},
	}

	if fr.FetchError != nil {
		inserts = append(inserts, dbfield{"err", fr.FetchError.Error()})
	}

	if fr.ExcludedByRobots {
		inserts = append(inserts, dbfield{"robot_ex", true})
	}

	if fr.Response != nil {
		inserts = append(inserts, dbfield{"stat", fr.Response.StatusCode})
	}

	//TODO: redirectURL, _ := fr.Res.Location()
	//TODO: fp
	//TODO: can we get RemoteAddr? fr.Res.Request.RemoteAddr may not be filled in
	//TODO: fr.Res.Header.Get("Content-Type"),

	// Put the values together and run the query
	names := []string{}
	values := []interface{}{}
	placeholders := []string{}
	for _, f := range inserts {
		names = append(names, f.name)
		values = append(values, f.value)
		placeholders = append(placeholders, "?")
	}
	err := ds.db.Query(
		fmt.Sprintf(`INSERT INTO links (%s) VALUES (%s)`,
			strings.Join(names, ", "), strings.Join(placeholders, ", ")),
		values...,
	).Exec()
	if err != nil {
		log4go.Error("Failed storing fetch results: %v", err)
	}
}

func (ds *CassandraDatastore) StoreParsedURL(u *URL, fr *FetchResults) {
	if u.Host == "" {
		log4go.Warn("Not handling link because there is no host: %v", *u)
		return
	}
	domain := u.ToplevelDomainPlusOne()
	if Config.AddNewDomains {
		ds.addDomainIfNew(domain)
	}
	err := ds.db.Query(`INSERT INTO links (dom, subdom, path, proto, time)
						VALUES (?, ?, ?, ?, ?)`,
		domain, u.Subdomain(), u.RequestURI(), u.Scheme, NotYetCrawled).Exec()
	if err != nil {
		log4go.Error("failed inserting parsed url (%v) to cassandra, %v", u, err)
	}
}

// addDomainIfNew expects a toplevel domain, no subdomain
func (ds *CassandraDatastore) addDomainIfNew(domain string) {
	_, ok := ds.addedDomains.Get(domain)
	if ok {
		return
	}
	var count int
	err := ds.db.Query(`SELECT COUNT(*) FROM domain_info WHERE dom = ?`, domain).Scan(&count)
	if err != nil {
		log4go.Error("Failed to check if %v is in domain_info: %v", domain, err)
		return // with error, assume we already have it and move on
	}
	if count == 0 {
		err := ds.db.Query(`INSERT INTO domain_info (dom) VALUES (?)`, domain).Exec()
		if err != nil {
			log4go.Error("Failed to add new domain %v: %v", domain, err)
		}
	}
	ds.addedDomains.Set(domain, nil)
}

func (ds *CassandraDatastore) getSegmentLinks(domain string) (links []*URL, err error) {
	q := ds.db.Query(`SELECT dom, subdom, path, proto, time
						FROM segments WHERE dom = ?`, domain)
	iter := q.Iter()
	defer func() { err = iter.Close() }()

	var dbdomain, subdomain, path, protocol string
	var crawl_time time.Time
	for iter.Scan(&dbdomain, &subdomain, &path, &protocol, &crawl_time) {
		u, e := CreateURL(dbdomain, subdomain, path, protocol, crawl_time)
		if e != nil {
			log4go.Error("Error adding link (%v) to crawl: %v", u, e)
		} else {
			log4go.Debug("Adding link: %v", u)
			links = append(links, u)
		}
	}
	return
}

// CreateCassandraSchema creates the walker schema in the configured Cassandra
// database. It requires that the keyspace not already exist (so as to losing
// non-test data), with the exception of the walker_test schema, which it will
// drop automatically.
func CreateCassandraSchema() error {
	config := GetCassandraConfig()
	config.Keyspace = ""
	db, err := config.CreateSession()
	if err != nil {
		return fmt.Errorf("Could not connect to create cassandra schema: %v", err)
	}

	if Config.Cassandra.Keyspace == "walker_test" {
		err := db.Query("DROP KEYSPACE IF EXISTS walker_test").Exec()
		if err != nil {
			return fmt.Errorf("Failed to drop walker_test keyspace: %v", err)
		}
	}

	schema, err := GetCassandraSchema()
	if err != nil {
		return err
	}

	for _, q := range strings.Split(schema, ";") {
		err = db.Query(q).Exec()
		if err != nil {
			return fmt.Errorf("Failed to create schema: %v\nStatement:\n%v", err, q)
		}
	}
	return nil
}

func GetCassandraSchema() (string, error) {
	t, err := template.New("schema").Parse(schemaTemplate)
	if err != nil {
		return "", fmt.Errorf("Failure parsing the CQL schema template: %v", err)
	}
	var b bytes.Buffer
	t.Execute(&b, Config.Cassandra)
	return b.String(), nil
}

const schemaTemplate string = `-- The schema file for walker
--
-- This file gets generated from a Go template so the keyspace and replication
-- can be configured (particularly for testing purposes)
CREATE KEYSPACE {{.Keyspace}}
WITH REPLICATION = { 'class': 'SimpleStrategy', 'replication_factor': {{.ReplicationFactor}} };

-- links stores all links we have parsed out of pages and crawled.
--
-- Links found in a page (or inserted with other means) that have not been
-- crawled yet have 'time' set to the epoch (Jan 1 1970). Because 'time' is
-- part of the primary key, Cassandra will deduplicate identical parsed links.
--
-- Every time a link is crawled the results are inserted here. Note that the
-- initial link (with time=epoch) is not overwritten. Rather, for every link,
-- this table contains one row for the initial insert and one for each fetch
-- thereafter. We can effectively see our crawl history for every single link.
CREATE TABLE {{.Keyspace}}.links (
	-- top-level domain plus one component, ex. "google.com"
	dom text,

	-- subdomain, ex. "www" (does not include .)
	subdom text,

	-- path with query parameters, ex. "/index.html?a=b"
	path text,

	-- protocol "http"
	proto text,

	-- time we crawled this link (or epoch, meaning not-yet-fetched)
	time timestamp,

	-- status code of the fetch (null if we did not fetch)
	stat int,

	-- error text, describes the error if we could not fetch (otherwise null)
	err text,

	-- true if this link was excluded from the crawl due to robots.txt rules
	-- (null implies we were not excluded)
	robot_ex boolean,

	---- Items yet to be added to walker

	-- fingerprint, a hash of the page contents for identity comparison
	--fp bigint,

	-- structure fingerprint, a hash of the page structure only (defined as:
	-- html tags only, all contents and attributes stripped)
	--structfp bigint,

	-- ip address of the remote server
	--ip text,

	-- referer, maybe can be kept for parsed links
	--ref text,

	-- redirect_url if this link redirected somewhere else
	--redirect_url text,

	-- mime type, also known as Content-Type (ex. "text/html")
	--mime text,

	-- encoding of the text, ex. "utf8"
	--encoding text,

	PRIMARY KEY (dom, subdom, path, proto, time)
) WITH compaction = { 'class' : 'LeveledCompactionStrategy' };

-- segments contains groups of links that are ready to be crawled for a given domain.
-- Links belonging to the same domain are considered one segment.
CREATE TABLE {{.Keyspace}}.segments (
	dom text,
	subdom text,
	path text,
	proto text,

	-- time this link was last crawled, so that we can use if-modified-since headers
	time timestamp,

	PRIMARY KEY (dom, subdom, path, proto)
) WITH compaction = { 'class' : 'LeveledCompactionStrategy' };

CREATE TABLE {{.Keyspace}}.domain_info (
	dom text,

	-- an arbitrary number indicating priority level for crawling this domain.
	-- High priority domains will have segments generated more quickly when they
	-- are exhausted and will be claimed more quickly for crawling
	priority int,

	-- UUID of the crawler that claimed this domain for crawling. This is the
	-- zero UUID if unclaimed (it cannot be null because we index the column).
	claim_tok uuid,

	-- The time this domain was last claimed by a crawler. It remains set after
	-- a crawler unclaims this domain (i.e. if claim_tok is the zero UUID then
	-- claim_time simply means the last time a crawler claimed it, though we
	-- don't know which crawler). Storing claim time is also useful for
	-- unclaiming domains if a crawler is taking too long (implying that it was
	-- stopped abnormally)
	claim_time timestamp, -- define as last time crawled?

	-- true if this domain has had a segment generated and is ready for crawling
	dispatched boolean,

	---- Items yet to be added to walker

	-- true if this domain is excluded from the crawl (null implies not excluded)
	--excluded boolean,
	-- the reason this domain is excluded, null if not excluded
	--exclude_reason text,

	-- If not null, identifies another domain as a mirror of this one
	--mirr_for text,

	PRIMARY KEY (dom)
) WITH compaction = { 'class' : 'LeveledCompactionStrategy' };
CREATE INDEX ON {{.Keyspace}}.domain_info (claim_tok);
CREATE INDEX ON {{.Keyspace}}.domain_info (priority);
CREATE INDEX ON {{.Keyspace}}.domain_info (dispatched)`
