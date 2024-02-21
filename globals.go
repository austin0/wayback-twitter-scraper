package main

import (
	"regexp"
)

var (
	Proxies           []Proxy
	PageUnprocessed   []string
	PageProcessed     []string
	ImageUnprocessed  []string
	ImageProcessed    []string
	StoredImageMap        = make(map[string]bool)
	TotalDownloads    int = 0
	WaybackResultsURL string
	HomeDirectory     = GetPWD()
	UsernameLocation  string
	MediaDir          string
	ProfileDir        string
	TwitterUsername   string
	WaybackPrefix     = "https://web.archive.org/web/20200126021126if_/"
	MediaRegex        = regexp.MustCompile(`https://pbs.twimg.com/media/[A-Za-z0-9_.\-]+.jpg`)
	ProfileRegex      = regexp.MustCompile(`https://pbs.twimg.com/profile_images/[0-9]+/[A-Za-z0-9_.\-]+.jpg`)
	FilenameRegex     = regexp.MustCompile(`[A-Za-z0-9_.]+.jpg`)
	MaxThreads        = 20
)
