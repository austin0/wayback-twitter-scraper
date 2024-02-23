package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/gookit/color"
)

type Proxy struct {
	IP       string
	Port     string
	Username string
	Password string
}

var (
	transportOptions = tls_client.TransportOptions{
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: 1 << 26,
		DisableCompression:     true,
	}

	requestHeaders = http.Header{
		"accept":     {"*/*"},
		"user-agent": {"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0"},
		http.HeaderOrderKey: {
			"accept",
			"user-agent",
		},
	}
)

// GetProxyClient() returns a new HTTP client with a random proxy from the list
func GetProxyClient() tls_client.HttpClient {
	proxy := getProxy()

	customRedirect := func(req *http.Request, via []*http.Request) error {
		// On redirect replace the old request with a new one but with the location from the redirect to prevent TLS EOF errors
		// Errors come from https://github.com/bogdanfinn/fhttp/blob/master/transfer.go#L205 where either proxies and or concurrency push the redirect transfer above 200ms
		req.Header = via[0].Header
		req.Body = nil
		req.ContentLength = 0
		*req = *req.Clone(context.Background())
		return nil
	}

	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Firefox_120),
		tls_client.WithProxyUrl(proxy),
		tls_client.WithTransportOptions(&transportOptions),
		tls_client.WithDefaultHeaders(requestHeaders),
		tls_client.WithCustomRedirectFunc(customRedirect),
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		color.Magenta.Printf("Retrying - Error creating HTTP client: %+v\n", err)
		return GetProxyClient()
	}

	return client
}

func getProxy() string {
	var proxy string

	for len(Proxies) == 0 {
		color.Red.Println("No proxies available, waiting for one to become available")
		time.Sleep(5 * time.Second)
	}

	ProxyMutex.Lock()

	proxy = Proxies[len(Proxies)-1]
	Proxies = Proxies[:len(Proxies)-1]
	ProxiesActive = append(ProxiesActive, proxy)

	ProxyMutex.Unlock()

	return proxy
}

func rotateClientProxy(httpClient tls_client.HttpClient) {
	returnProxy(httpClient)

	err := httpClient.SetProxy(getProxy())
	if err != nil {
		color.Red.Printf("Error rotating proxy: %+v\n", err)
		return
	}
}

func returnProxy(httpClient tls_client.HttpClient) {
	ProxyMutex.Lock()

	activeProxy := httpClient.GetProxy()
	for i, proxy := range ProxiesActive {
		if proxy == activeProxy {
			ProxiesActive = append(ProxiesActive[:i], ProxiesActive[i+1:]...)
			Proxies = append(Proxies, proxy)
			break
		}
	}

	ProxyMutex.Unlock()
}

func LoadProxies() {
	fmt.Printf("\nLoading Proxies: %s/Proxies/Proxies.txt\n", HomeDirectory)

	proxyFile, err := os.Open(fmt.Sprintf("%s/proxies/proxies.txt", HomeDirectory))
	if err != nil {
		color.Red.Printf("Error opening proxy file: %+v\n", err)
		os.Exit(1)
	}

	defer proxyFile.Close()

	scanner := bufio.NewScanner(proxyFile)
	for scanner.Scan() {
		proxyParts := strings.Split(scanner.Text(), ":")
		proxyString := fmt.Sprintf("http://%s:%s@%s:%s", proxyParts[2], proxyParts[3], proxyParts[0], proxyParts[1])
		Proxies = append(Proxies, proxyString)
		if len(proxyParts) != 4 {
			color.Yellow.Println("Invalid proxy format:", scanner.Text())
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		color.Red.Println("Error reading proxy file:", err)
	}

	fmt.Printf("Loaded %d Proxies from file\n\n", len(Proxies))
}
