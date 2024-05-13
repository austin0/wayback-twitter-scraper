package main

import (
	"fmt"
	"regexp"
	"sync"
)

var (
	// Error variables
	ErrPageMissingContent = fmt.Errorf("404 - Page not found")
	ErrPageRetries        = fmt.Errorf("error fetching page content after %d retries", RetryAttempts)
	ErrImageRetries       = fmt.Errorf("failed save image after %d retries", RetryAttempts)

	// Resource variables
	Resources = []string{"media", "profile"}

	// Proxy variables
	Proxies       []string
	ProxiesActive []string
	ProxyMutex    sync.Mutex
	UseProxies    bool

	// Page variables
	PageUnprocessed []string
	PageProcessed   []string
	TotalPages      = 0
	PageMutex       sync.Mutex

	// Image variables
	ImageUnprocessed []string
	ImageProcessed   []string
	StoredImageMap   = make(map[string]bool)
	TotalImages      = 0
	TotalDownloads   = 0
	ImageMutex       sync.Mutex

	// URL variables
	WaybackResultsURL string
	WaybackPrefix     = "https://web.archive.org/web/20200126021126if_/"

	// Directory variables
	HomeDirectory    = GetPWD()
	UsernameLocation string
	MediaDir         string
	ProfileDir       string

	// Twitter variables
	TwitterUsername string

	// Regular expressions
	MediaRegex    = regexp.MustCompile(`https://pbs.twimg.com/media/[A-Za-z0-9_.\-]+.jpg`)
	ProfileRegex  = regexp.MustCompile(`https://pbs.twimg.com/profile_images/[0-9]+/[A-Za-z0-9_.\-]+.jpg`)
	FilenameRegex = regexp.MustCompile(`[A-Za-z0-9_.\-]+.jpg`)

	// Other variables
	MaxThreads    = 50
	RetryAttempts = 5
)
