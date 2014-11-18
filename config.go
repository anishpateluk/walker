package walker

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"code.google.com/p/log4go"
)

// Config is the configuration instance the rest of walker should access for
// global configuration values. See WalkerConfig for available config members.
var Config WalkerConfig

// ConfigName is the path (can be relative or absolute) to the config file that
// should be read.
var ConfigName string = "walker.yaml"

func init() {
	err := readConfig()
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			log4go.Info("Did not find config file %v, continuing with defaults", ConfigName)
		} else {
			panic(err.Error())
		}
	}
}

// WalkerConfig defines the available global configuration parameters for
// walker. It reads values straight from the config file (walker.yaml by
// default). See sample-walker.yaml for explanations and default values.
type WalkerConfig struct {
	AddNewDomains           bool     `yaml:"add_new_domains"`
	AddedDomainsCacheSize   int      `yaml:"added_domains_cache_size"`
	MaxDNSCacheEntries      int      `yaml:"max_dns_cache_entries"`
	UserAgent               string   `yaml:"user_agent"`
	AcceptFormats           []string `yaml:"accept_formats"`
	AcceptProtocols         []string `yaml:"accept_protocols"`
	MaxHTTPContentSizeBytes int64    `yaml:"max_http_content_size_bytes"`
	IgnoreTags              []string `yaml:"ignore_tags"`
	//TODO: allow -1 as a no max value
	MaxLinksPerPage         int      `yaml:"max_links_per_page"`
	NumSimultaneousFetchers int      `yaml:"num_simultaneous_fetchers"`
	BlacklistPrivateIPs     bool     `yaml:"blacklist_private_ips"`
	HttpTimeout             string   `yaml:"http_timeout"`
	HonorMetaNoindex        bool     `yaml:"honor_meta_noindex"`
	HonorMetaNofollow       bool     `yaml:"honor_meta_nofollow"`
	ExcludeLinkPatterns     []string `yaml:"exclude_link_patterns"`
	IncludeLinkPatterns     []string `yaml:"include_link_patterns"`
	DefaultCrawlDelay       string   `yaml:"default_crawl_delay"`
	MaxCrawlDelay           string   `yaml:"max_crawl_delay"`
	PurgeSidList            []string `yaml:"purge_sid_list"`

	Dispatcher struct {
		MaxLinksPerSegment   int     `yaml:"num_links_per_segment"`
		RefreshPercentage    float64 `yaml:"refresh_percentage"`
		NumConcurrentDomains int     `yaml:"num_concurrent_domains"`
		MinLinkRefreshTime   string  `yaml:"min_link_refresh_time"`
	} `yaml:"dispatcher"`

	// TODO: consider these config items
	// allowed schemes (file://, https://, etc.)
	// allowed return content types (or file extensions)
	// http timeout
	// http max delays (how many attempts to give a webserver that's reporting 'busy')
	// http content size limit
	// ftp content limit
	// ftp timeout
	// regex matchers for hosts, paths, etc. to include or exclude
	// max crawl delay (exclude or notify of sites that try to set a massive crawl delay)
	// max simultaneous fetches/crawls/segments

	Cassandra struct {
		Hosts             []string `yaml:"hosts"`
		Keyspace          string   `yaml:"keyspace"`
		ReplicationFactor int      `yaml:"replication_factor"`
		Timeout           string   `yaml:"timeout"`
		CQLVersion        string   `yaml:"cql_version"`
		ProtoVersion      int      `yaml:"proto_version"`
		Port              int      `yaml:"port"`
		NumConns          int      `yaml:"num_conns"`
		NumStreams        int      `yaml:"num_streams"`
		DiscoverHosts     bool     `yaml:"discover_hosts"`
		MaxPreparedStmts  int      `yaml:"max_prepared_stmts"`

		//TODO: Currently only exposing values needed for testing; should expose more?
		//Consistency      Consistency
		//Compressor       Compressor
		//Authenticator    Authenticator
		//RetryPolicy      RetryPolicy
		//SocketKeepalive  time.Duration
		//ConnPoolType     NewPoolFunc
		//Discovery        DiscoveryConfig
	} `yaml:"cassandra"`

	Console struct {
		Port              int    `yaml:"port"`
		TemplateDirectory string `yaml:"template_directory"`
		PublicFolder      string `yaml:"public_folder"`
	} `yaml:"console"`
}

// SetDefaultConfig resets the Config object to default values, regardless of
// what was set by any configuration file.
func SetDefaultConfig() {
	// NOTE: go-yaml has a bug where it does not overwrite sequence values
	// (i.e. lists), it appends to them.
	// See https://github.com/go-yaml/yaml/issues/48
	// Until this is fixed, for any sequence value, in readConfig we have to
	// nil it and then fill in the default value if yaml.Unmarshal did not fill
	// anything in

	Config.AddNewDomains = false
	Config.AddedDomainsCacheSize = 20000
	Config.MaxDNSCacheEntries = 20000
	Config.UserAgent = "Walker (http://github.com/iParadigms/walker)"
	Config.AcceptFormats = []string{"text/html", "text/*;"} //NOTE you can add quality factors by doing "text/html; q=0.4"
	Config.AcceptProtocols = []string{"http", "https"}
	Config.MaxHTTPContentSizeBytes = 20 * 1024 * 1024 // 20MB
	Config.IgnoreTags = []string{"script", "img", "link"}
	Config.MaxLinksPerPage = 1000
	Config.NumSimultaneousFetchers = 10
	Config.BlacklistPrivateIPs = true
	Config.HttpTimeout = "30s"
	Config.HonorMetaNoindex = true
	Config.HonorMetaNofollow = false
	Config.ExcludeLinkPatterns = nil
	Config.IncludeLinkPatterns = nil
	Config.DefaultCrawlDelay = "1s"
	Config.MaxCrawlDelay = "5m"
	Config.PurgeSidList = nil

	Config.Dispatcher.MaxLinksPerSegment = 500
	Config.Dispatcher.RefreshPercentage = 25
	Config.Dispatcher.NumConcurrentDomains = 1
	Config.Dispatcher.MinLinkRefreshTime = "0s"

	Config.Cassandra.Hosts = []string{"localhost"}
	Config.Cassandra.Keyspace = "walker"
	Config.Cassandra.ReplicationFactor = 3
	Config.Cassandra.Timeout = "2s"
	Config.Cassandra.CQLVersion = "3.0.0"
	Config.Cassandra.ProtoVersion = 2
	Config.Cassandra.Port = 9042
	Config.Cassandra.NumConns = 2
	Config.Cassandra.NumStreams = 128
	Config.Cassandra.DiscoverHosts = false
	Config.Cassandra.MaxPreparedStmts = 1000

	Config.Console.Port = 3000
	Config.Console.TemplateDirectory = "console/templates"
	Config.Console.PublicFolder = "console/public"
}

// ReadConfigFile sets a new path to find the walker yaml config file and
// forces a reload of the config.
func ReadConfigFile(path string) error {
	ConfigName = path
	return readConfig()
}

// MustReadConfigFile calls ReadConfigFile and panics on error.
func MustReadConfigFile(path string) {
	err := ReadConfigFile(path)
	if err != nil {
		panic(err.Error())
	}
}

func assertConfigInvariants() error {
	var errs []string
	dis := &Config.Dispatcher
	if dis.RefreshPercentage < 0.0 || dis.RefreshPercentage > 100.0 {
		errs = append(errs, "Dispatcher.RefreshPercentage must be a floating point number b/w 0 and 100")
	}
	if dis.MaxLinksPerSegment < 1 {
		errs = append(errs, "Dispatcher.MaxLinksPerSegment must be greater than 0")
	}
	if dis.NumConcurrentDomains < 1 {
		errs = append(errs, "Dispatcher.NumConcurrentDomains must be greater than 0")
	}

	_, err := time.ParseDuration(Config.HttpTimeout)
	if err != nil {
		errs = append(errs, fmt.Sprintf("HttpTimeout failed to parse: %v", err))
	}

	_, err = time.ParseDuration(Config.Cassandra.Timeout)
	if err != nil {
		errs = append(errs, fmt.Sprintf("Cassandra.Timeout failed to parse: %v", err))
	}

	_, err = aggregateRegex(Config.ExcludeLinkPatterns, "exclude_link_patterns")
	if err != nil {
		errs = append(errs, err.Error())
	}

	_, err = aggregateRegex(Config.IncludeLinkPatterns, "include_link_patterns")
	if err != nil {
		errs = append(errs, err.Error())
	}

	_, err = time.ParseDuration(Config.Dispatcher.MinLinkRefreshTime)
	if err != nil {
		errs = append(errs, fmt.Sprintf("Dispatcher.MinLinkRefreshTime failed to parse: %v", err))
	}

	def, err := time.ParseDuration(Config.DefaultCrawlDelay)
	if err != nil {
		errs = append(errs, fmt.Sprintf("DefaultCrawlDelay failed to parse: %v", err))
	}

	max, err := time.ParseDuration(Config.MaxCrawlDelay)
	if err != nil {
		errs = append(errs, fmt.Sprintf("MaxCrawlDelay failed to parse: %v", err))
	}

	if def > max {
		errs = append(errs, "Consistency problem: MaxCrawlDelay > DefaultCrawlDealy")
	}

	if len(errs) > 0 {
		em := ""
		for _, err := range errs {
			log4go.Error("Config Error: %v", err)
			em += "\t"
			em += err
			em += "\n"
		}
		return fmt.Errorf("Config Error:\n%v\n", em)
	}

	return nil
}

// This function allows code to set up data structures that depend on the
// config. It is always called right after the config file is consumed. But
// it's also public so if you modify the config in a test, you may need to
// call this function. This function is idempotent; so you can call it as many
// times as you like.
func PostConfigHooks() {
	err := setupParseURL()
	if err != nil {
		panic(err)
	}
}

func readConfig() error {
	SetDefaultConfig()

	// See NOTE in SetDefaultConfig regarding sequence values
	Config.AcceptFormats = []string{}
	Config.AcceptProtocols = []string{}
	Config.IgnoreTags = []string{}
	Config.Cassandra.Hosts = []string{}
	Config.PurgeSidList = []string{}

	data, err := ioutil.ReadFile(ConfigName)
	if err != nil {
		return fmt.Errorf("Failed to read config file (%v): %v", ConfigName, err)
	}
	err = yaml.Unmarshal(data, &Config)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal yaml from config file (%v): %v", ConfigName, err)
	}

	// See NOTE in SetDefaultConfig regarding sequence values
	if len(Config.AcceptFormats) == 0 {
		Config.AcceptFormats = []string{"text/html", "text/*;"}
	}
	if len(Config.AcceptProtocols) == 0 {
		Config.AcceptProtocols = []string{"http", "https"}
	}
	if len(Config.IgnoreTags) == 0 {
		Config.IgnoreTags = []string{"script", "img", "link"}
	}
	if len(Config.Cassandra.Hosts) == 0 {
		Config.Cassandra.Hosts = []string{"localhost"}
	}
	if len(Config.PurgeSidList) == 0 {
		Config.PurgeSidList = []string{"jsessionid", "phpsessid", "aspsessionid"}
	}

	err = assertConfigInvariants()
	if err != nil {
		log4go.Info("Loaded config file %v", ConfigName)
	}

	PostConfigHooks()

	return err
}
