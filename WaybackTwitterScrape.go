package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/corona10/goimghdr"
	"github.com/gookit/color"
)

var (
	pageUnprocessed   []string
	pageProcessed     []string
	imageUnprocessed  []string
	imageProcessed    []string
	localImageCache   = make(map[string]bool)
	totalDownloads    int
	waybackResultsURL string
	homeDirectory     string
	usernameLocation  string
	mediaDir          string
	profileDir        string
	twitterUsername   string
	waybackPrefix     = "https://web.archive.org/web/20200126021126if_/"
	mediaRegex        = regexp.MustCompile(`https://pbs.twimg.com/media/[A-Za-z0-9_.\-]+.jpg`)
	profileRegex      = regexp.MustCompile(`https://pbs.twimg.com/profile_images/[0-9]+/[A-Za-z0-9_.\-]+.jpg`)
	filenameRegex     = regexp.MustCompile(`[A-Za-z0-9_.]+.jpg`)
	proxies           []Proxy
	popMutex          sync.Mutex
	mapMutex          sync.Mutex
)

type Proxy struct {
	IP       string
	Port     string
	Username string
	Password string
}

func main() {
	firstRun := true
	for invalidUsername(firstRun) {
		fmt.Print("Enter Twitter username: ")
		fmt.Scanln(&twitterUsername)
		firstRun = false
	}

	homeDirectory = getPWD()                                                   // ./wayback-twitter-scraper/
	usernameLocation = filepath.Join(homeDirectory, "images", twitterUsername) // ./wayback-twitter-scraper/images/0xf6i
	mediaDir = filepath.Join(usernameLocation, "media")                        // ./wayback-twitter-scraper/images/0xf6i/media
	profileDir = filepath.Join(usernameLocation, "profile")                    // ./wayback-twitter-scraper/images/0xf6i/profile
	createDirectories()

	fmt.Printf("\nLoading proxies: %s/proxies/proxies.txt\n", homeDirectory)
	loadProxies(homeDirectory, &proxies)
	fmt.Printf("Loaded %d proxies from file\n\n", len(proxies))

	createLocalImageCache()

	color.Cyan.Printf("Fetching list of Wayback Machine cached pages for profile: %s\n", twitterUsername)
	fetchWaybackPages()

	if len(pageUnprocessed) == 0 {
		color.Red.Printf("Found %d cached Wayback Machine pages - exiting\n", len(pageUnprocessed))
		os.Exit(0)
	} else {
		color.Cyan.Printf("Found %d cached Wayback Machine pages\n", len(pageUnprocessed))
	}

	color.LightGreen.Printf("\n=== Visiting the cached pages and checking for images\n")
	parseImages()
	color.Green.Printf("\nFound %d cached images for: %s\n", len(imageUnprocessed), twitterUsername)

	imageUnprocessed = removeAlreadySavedImages(imageUnprocessed, localImageCache) // Remove previously downloaded images from the unprocessedImages list

	color.LightGreen.Println("Downloading indentified images from Wayback Machine Cache\n")
	downloadImage()
	color.Green.Printf("\nSaved %d images for username: %s\n", totalDownloads, twitterUsername)

	color.Gray.Printf("\nPurging any corrupted images in %s\n", usernameLocation)
	purgeCorrupted(twitterUsername)

	color.Magenta.Printf("\nReport created: %s/%s-report.txt\n", usernameLocation, getCurrentDate())
	createReport(usernameLocation)
}

func invalidUsername(firstRun bool) bool {
	if firstRun {
		return true
	}

	if twitterUsername == "" {
		fmt.Println(`"" - is not a valid username`)
		return true
	}

	valid := regexp.MustCompile(`^[a-zA-Z0-9_]{1,15}$`).MatchString(twitterUsername)
	if !valid {
		fmt.Println("Username can only contain alphanumeric characters and underscores (1-15 characters)")
		return true
	}

	return false
}

func getPWD() string {
	pwd, err := os.Getwd()
	if err != nil {
		color.Red.Println("Error getting PWD:", err)
		return ""
	}

	return pwd
}

func loadProxies(homeDirectory string, proxies *[]Proxy) {
	proxyFile, err := os.Open(fmt.Sprintf("%s/proxies/proxies.txt", homeDirectory))
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

		*proxies = append(*proxies, Proxy{
			IP:       proxyParts[0],
			Port:     proxyParts[1],
			Username: proxyParts[2],
			Password: proxyParts[3],
		})
	}

	if err := scanner.Err(); err != nil {
		color.Red.Println("Error reading proxy file:", err)
	}
}

func createDirectories() {
	if err := os.MkdirAll(usernameLocation, os.ModePerm); err != nil {
		color.Red.Printf("Unable to create necessary directory %s: %s\n", usernameLocation, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(mediaDir, os.ModePerm); err != nil {
		color.Red.Printf("Unable to create necessary directory %s: %s\n", mediaDir, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(profileDir, os.ModePerm); err != nil {
		color.Red.Printf("Unable to create necessary directory %s: %s\n", profileDir, err)
		os.Exit(1)
	}
}

func createLocalImageCache() {
	for _, directoryPath := range []string{mediaDir, profileDir} {
		paths, err := filepath.Glob(fmt.Sprintf("%s/*", directoryPath))
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		for _, path := range paths {
			localImageCache[filepath.Base(path)] = true
		}
	}
	if len(localImageCache) > 0 {
		color.HiMagenta.Printf("Discovered %d locally stored files - express filtering enabled\n", len(localImageCache))
	}
}

func fetchWaybackPages() {
	waybackResultsURL = fmt.Sprintf("https://web.archive.org/web/timemap/json?url=https://twitter.com/%s&matchType=prefix", twitterUsername)

	resp, err := http.Get(waybackResultsURL)
	if err != nil {
		color.Red.Println("Error fetching Wayback Machine results:", err)
		return
	}
	defer resp.Body.Close()

	var waybackResults [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&waybackResults); err != nil {
		color.Red.Println("Error decoding Wayback Machine results:", err)
		return
	}

	for _, result := range waybackResults {
		pageURL, _ := result[2].(string)
		if strings.Contains(pageURL, `http`) {
			pageUnprocessed = append(pageUnprocessed, pageURL)
		}
	}
}

func parseImages() {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Limit to 5 concurrent goroutines

	for len(pageUnprocessed) > 0 {
		wg.Add(1)
		var pageURL string

		pageUnprocessed, pageURL = pop(pageUnprocessed)

		go func(pageURL string) { // Pass pageURL as an argument
			defer wg.Done()

			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			combinedURL := waybackPrefix + pageURL
			fmt.Printf("%s - Visiting %s to parse images\n", getPageProgress(), pageURL)

			htmlContent, err := parseImagesWithRetry(combinedURL)
			switch err {
			case nil:
				pageProcessed = append(pageProcessed, pageURL)
				color.Green.Printf("%s - Successfully parsed %s\n", getPageProgress(), pageURL)
			default:
				fmt.Printf("Error parsing images from %s - %s", combinedURL, err)
				pageUnprocessed = append(pageUnprocessed, pageURL)
				return
			}

			mediaURLs := mediaRegex.FindAllString(htmlContent, -1)
			profileURLs := addSizeSpread(profileRegex.FindAllString(htmlContent, -1))
			imageURLs := append(mediaURLs, profileURLs...)
			imageUnprocessed = slicesRemoveDuplicates(append(imageUnprocessed, imageURLs...))
		}(pageURL)
	}
	wg.Wait()
}

func parseImagesWithRetry(combinedURL string) (string, error) {
	var errCatcher error
	retry := 5
	for i := 0; i < retry; i++ {
		resp, err := getProxyClient().Get(combinedURL)
		if err != nil {
			color.Red.Println("Error fetching page content:", err)
			errCatcher = err
			time.Sleep(2 * time.Second) // Wait before retrying
			errCatcher = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			color.Red.Printf("Error: HTTP request failed with status code %d\n", resp.StatusCode)
			errCatcher = err
			time.Sleep(2 * time.Second) // Wait before retrying
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			color.Red.Printf("Error reading response body content for %s: %s", combinedURL, err)
			errCatcher = err
			time.Sleep(2 * time.Second) // Wait before retrying
			continue
		}

		htmlContent := string(body)
		return htmlContent, nil
	}
	return "", errCatcher
}

func downloadImageWithRetry(imageURL string, downloadPath string) error {
	var errCatcher error
	retry := 5
	for i := 0; i < retry; i++ {

		resp, err := getProxyClient().Get(imageURL)
		if err != nil {
			color.Red.Printf("Retrying - Error fetching image: %s\n", err)
			errCatcher = err
			time.Sleep(2 * time.Second)
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode == 404 {
			return fmt.Errorf("404 Resource Missing")
		}

		if resp.StatusCode != http.StatusOK {
			color.Red.Printf("Retrying - Error fetching image: HTTP status %d\n", resp.StatusCode)
			time.Sleep(2 * time.Second)
			errCatcher = err
			continue
		}

		file, err := os.Create(downloadPath)
		if err != nil {
			color.Red.Printf("Retrying - Error creating file: %s\n", err)
			time.Sleep(2 * time.Second)
			errCatcher = err
			continue
		}
		defer file.Close()

		_, err = io.Copy(file, resp.Body)
		if err != nil {
			color.Red.Printf("Retrying - Error saving image: %s\n", err)
			time.Sleep(2 * time.Second)
			errCatcher = err
			continue
		}
	}
	color.Red.Printf("Aborting - Error downloading image after %d retries: %s\n", retry, imageURL)
	return errCatcher
}

func downloadImage() error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Limit to 5 concurrent goroutines

	for len(imageUnprocessed) > 0 {
		wg.Add(1)
		var imageURL string
		var imageType string

		imageUnprocessed, imageURL = pop(imageUnprocessed)

		if strings.Contains(imageURL, "media") {
			imageType = "media"
		} else {
			imageType = "profile"
		}

		go func(imageURL string, imageType string) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			imageName := filenameRegex.FindString(imageURL)
			combinedURL := waybackPrefix + imageURL
			downloadPath := fmt.Sprintf("%s/%s/%s", usernameLocation, imageType, imageName)

			color.Yellow.Printf("%s - Fetching %s image %s\n", getImageProgress(), imageType, imageURL)

			err := downloadImageWithRetry(combinedURL, downloadPath)
			switch err {
			case nil:
				totalDownloads += 1
				imageProcessed = append(imageProcessed, imageURL)
				color.Green.Printf("%s - Saved %s\n", getImageProgress(), imageURL)
			case errors.New("404 Resource Missing"):
				color.Red.Printf("404 Resource Missing - %s - Aborting thread\n", imageURL)
			default:
				color.Red.Printf("Error downloading image from %s - %s\n", combinedURL, err)
				imageUnprocessed = append(imageUnprocessed, imageURL)
				return
			}
		}(imageURL, imageType)
	}
	wg.Wait()

	return nil
}

func purgeCorrupted(twitterUsername string) {
	images, err := filepath.Glob(fmt.Sprintf("%s/images/%s/*.jpg", homeDirectory, twitterUsername))
	if err != nil {
		color.Red.Println("Error listing image files:", err)
		return
	}

	purgedCounter := 0

	for _, image := range images {
		if _, err := os.Stat(image); err != nil {
			color.Red.Printf("Error checking image file: %s\n", err)
			continue
		}
		if _, err := os.Stat(image); err == nil {
			filetype, _ := goimghdr.What(image)
			if filetype != "jpeg" {
				if err := os.Remove(image); err != nil {
					color.Red.Printf("Error removing corrupted image: %s\n", err)
					continue
				}
				color.Green.Printf("Removed corrupted image: %s\n", image)
				purgedCounter += 1
			}
		}
	}

	color.Green.Println(`No corrupted images found - success!`)
}

func createReport(usernameLocation string) {
	header := fmt.Sprintf(`=== Wayback Report - %s - %s`, twitterUsername, getCurrentDate())
	totalProcessed := fmt.Sprintf("Pages Parsed: %d | Images Proccesed: %d | Downloaded Images: %d", len(pageProcessed), len(imageProcessed), totalDownloads)
	pageString := ""
	for _, link := range pageProcessed {
		pageString += fmt.Sprintf("%s\n", link)
	}
	imageString := ""
	for _, link := range imageProcessed {
		imageString += fmt.Sprintf("%s\n", link)
	}

	report := fmt.Sprintf("%s\n%s\n%s\n%s\n", header, totalProcessed, pageString, imageString)

	reportFile, err := os.Create(fmt.Sprintf("%s/%s-report.txt", usernameLocation, getCurrentDate()))
	if err != nil {
		color.Red.Println("Error creating report file:", err)
		return
	}
	defer reportFile.Close()

	_, err = reportFile.WriteString(report)
	if err != nil {
		color.Red.Println("Error writing to report file:", err)
		return
	}
}

func getProxyClient() *http.Client {
	randomProxy := proxies[rand.Intn(len(proxies))]
	proxyString := fmt.Sprintf("http://%s:%s@%s:%s", randomProxy.Username, randomProxy.Password, randomProxy.IP, randomProxy.Port)

	proxyURL, err := url.Parse(proxyString)
	if err != nil {
		color.Red.Sprintf(`Error: Unable to parse proxy - %s`, proxyString)
		return nil
	}

	proxyClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	return proxyClient
}

func addSizeSpread(profileURLs []string) []string {
	for _, profileURL := range profileURLs {
		baseProfile := truncateString(profileURL, 65)
		profileURLs = append(profileURLs, fmt.Sprintf(`%s.jpg`, baseProfile))
		profileURLs = append(profileURLs, fmt.Sprintf(`%s_400x400.jpg`, baseProfile))
		profileURLs = append(profileURLs, fmt.Sprintf(`%s_normal.jpg`, baseProfile))
		profileURLs = append(profileURLs, fmt.Sprintf(`%s_bigger.jpg`, baseProfile))
	}
	return profileURLs
}

func getPageProgress() string {
	return fmt.Sprintf("[%d / %d]", len(pageProcessed), (len(pageUnprocessed) + len(pageProcessed)))
}

func getImageProgress() string {
	return fmt.Sprintf("[%d / %d]", len(imageProcessed), (len(imageUnprocessed) + len(imageProcessed)))
}

func pop(slice []string) ([]string, string) {
	popMutex.Lock()
	defer popMutex.Unlock()

	if len(slice) == 0 {
		return slice, ""
	}
	popped := slice[len(slice)-1]
	slice = slice[:len(slice)-1]
	return slice, popped
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func slicesRemoveDuplicates(linkArray []string) []string {
	uniqueLinks := make(map[string]bool)
	var uniqueLinksSlice []string

	mapMutex.Lock()
	defer mapMutex.Unlock()

	for _, link := range linkArray {
		uniqueLinks[link] = true
	}
	for link := range uniqueLinks {
		uniqueLinksSlice = append(uniqueLinksSlice, link)
	}

	return uniqueLinksSlice
}

// Removes the objects stored in localImageCache from the unprocessedImageSlice
// Reduced redundant work by doing the comparison once as opposied to on every thread
func removeAlreadySavedImages(unprocessedImageSlice []string, localImageCache map[string]bool) []string {
	var result []string

	mapMutex.Lock()
	defer mapMutex.Unlock()

	for _, link := range unprocessedImageSlice {
		if !localImageCache[link] {
			result = append(result, link)
		}
	}

	return result
}

func getCurrentDate() string {
	currentTime := time.Now()
	dateString := currentTime.Format("2006-01-02")
	return dateString
}
