package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"

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

var transportOptions = tls_client.TransportOptions{
	MaxIdleConnsPerHost: -1,
	DisableKeepAlives:   true,
	MaxConnsPerHost:     0,
}

// GetProxyClient() returns a new HTTP client with a random proxy from the list
func GetProxyClient() tls_client.HttpClient {

	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_120),
		tls_client.WithProxyUrl(getProxy()),
		tls_client.WithTransportOptions(&transportOptions),
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		log.Println(err)
	}

	return client
}

func getProxy() string {
	proxy := Proxies[rand.Intn(len(Proxies))]
	proxyString := fmt.Sprintf("http://%s:%s@%s:%s", proxy.Username, proxy.Password, proxy.IP, proxy.Port)
	return proxyString
}

func rotateClientProxy(httpClient tls_client.HttpClient) {
	err := httpClient.SetProxy(getProxy())
	if err != nil {
		log.Println(err)
		return
	}
}

func LoadProxies() {
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

		Proxies = append(Proxies, Proxy{
			IP:       proxyParts[0],
			Port:     proxyParts[1],
			Username: proxyParts[2],
			Password: proxyParts[3],
		})
	}

	if err := scanner.Err(); err != nil {
		color.Red.Println("Error reading proxy file:", err)
	}

	fmt.Printf("Loaded %d Proxies from file\n\n", len(Proxies))
}
