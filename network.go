package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gookit/color"
)

type Proxy struct {
	IP       string
	Port     string
	Username string
	Password string
}

type headerTransport struct {
	Base    http.RoundTripper
	Headers map[string]string
}

// GetProxyClient() returns a new HTTP client with a random proxy from the list
func GetProxyClient() *http.Client {
	randomProxy := Proxies[rand.Intn(len(Proxies))]
	proxyString := fmt.Sprintf("http://%s:%s@%s:%s", randomProxy.Username, randomProxy.Password, randomProxy.IP, randomProxy.Port)

	proxyURL, err := url.Parse(proxyString)
	if err != nil {
		color.Red.Sprintf(`Error: Unable to parse proxy - %s`, proxyString)
		return nil
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	// Create a new HTTP client with the custom transport
	proxyClient := &http.Client{
		Transport:     transport,
		CheckRedirect: redirectPolicyFunc,
		Timeout:       30 * time.Second,
	}

	// Add default headers to the client
	proxyClient.Transport = &headerTransport{
		Base:    proxyClient.Transport,
		Headers: getDefaultHeaders(),
	}

	return proxyClient
}

func getDefaultHeaders() map[string]string {
	defaultHeaders := map[string]string{
		"authority":                 "web.archive.org",
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"accept-language":           "en-GB,en;q=0.9,en-US;q=0.8,ja;q=0.7",
		"cache-control":             "max-age=0",
		"cookie":                    "donation-identifier=c30fe575612bf4b8ccd3c44c9617110d; abtest-identifier=b0be6c99d3b4fcc0bddd14a95a2016b9; view-search=tiles; showdetails-search=; PHPSESSID=hd68tsd3ellj2f21fiol6na6v5",
		"dnt":                       "1",
		"sec-ch-ua":                 `"Not A(Brand";v="99", "Google Chrome";v="121", "Chromium";v="121"`,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"Windows"`,
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "none",
		"sec-fetch-user":            "?1",
		"sec-gpc":                   "1",
		"upgrade-insecure-requests": "1",
		"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	}
	return defaultHeaders
}

// RoundTrip executes a single HTTP transaction and returns a Response for the provided Request.
func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add default headers to the request
	for key, value := range t.Headers {
		req.Header.Set(key, value)
	}
	// Forward the request to the underlying RoundTripper
	return t.Base.RoundTrip(req)
}

// redirectPolicyFunc is a custom function to handle redirects
func redirectPolicyFunc(req *http.Request, via []*http.Request) error {
	// Return nil to follow redirects
	return nil
}

func LoadProxies(HomeDirectory string, Proxies *[]Proxy) {
	fmt.Printf("\nLoading Proxies: %s/Proxies/Proxies.txt\n", HomeDirectory)

	proxyFile, err := os.Open(fmt.Sprintf("%s/proxies/proxies.txt", HomeDirectory))
	if err != nil {
		color.Red.Println("Error opening proxy file:", err)
		os.Exit(1)
	}

	defer proxyFile.Close()

	scanner := bufio.NewScanner(proxyFile)
	for scanner.Scan() {
		proxyParts := strings.Split(scanner.Text(), ":")
		if len(proxyParts) != 4 {
			color.Yellow.Println("Invalid proxy format:", scanner.Text())
			continue
		}

		*Proxies = append(*Proxies, Proxy{
			IP:       proxyParts[0],
			Port:     proxyParts[1],
			Username: proxyParts[2],
			Password: proxyParts[3],
		})
	}

	if err := scanner.Err(); err != nil {
		color.Red.Println("Error reading proxy file:", err)
	}

	fmt.Printf("Loaded %d Proxies from file\n\n", len(*Proxies))
}
