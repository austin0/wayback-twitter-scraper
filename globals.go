package main

import (
	"regexp"

	http "github.com/bogdanfinn/fhttp"
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
	MaxThreads        = 300
	RequestHeaders    = http.Header{
		"authority":                 {"web.archive.org"},
		"accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		"accept-language":           {"en-GB,en;q=0.9,en-US;q=0.8,ja;q=0.7"},
		"cache-control":             {"max-age=0"},
		"cookie":                    {"donation-identifier=c30fe575612bf4b8ccd3c44c9617110d; abtest-identifier=b0be6c99d3b4fcc0bddd14a95a2016b9; view-search=tiles; showdetails-search=; PHPSESSID=hd68tsd3ellj2f21fiol6na6v5"},
		"dnt":                       {"1"},
		"sec-ch-ua":                 {`"Not A(Brand";v="99", "Google Chrome";v="121", "Chromium";v="121"`},
		"sec-ch-ua-mobile":          {"?0"},
		"sec-ch-ua-platform":        {`"Windows"`},
		"sec-fetch-dest":            {"document"},
		"sec-fetch-mode":            {"navigate"},
		"sec-fetch-site":            {"none"},
		"sec-fetch-user":            {"?1"},
		"sec-gpc":                   {"1"},
		"upgrade-insecure-requests": {"1"},
		"user-agent":                {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"},
		http.HeaderOrderKey: {
			"cache-control",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"upgrade-insecure-requests",
			"user-agent",
			"accept",
			"sec-fetch-site",
			"sec-fetch-mode",
			"sec-fetch-user",
			"sec-fetch-dest",
			"accept-encoding",
			"accept-language",
			"cookie",
			"dnt",
			"sec-gpc",
		},
	}
)
