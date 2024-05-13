package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	http "github.com/bogdanfinn/fhttp"
	"github.com/corona10/goimghdr"
	"github.com/gookit/color"
)

func main() {
	log.SetOutput(os.Stdout)
	DrawTitle()                    // Draw the title of the program
	inputUsername(TwitterUsername) // Prompt user for Twitter username
	CreateDirectories()            // Create necessary directories for storing images
	LoadProxies()                  // Load proxies from the proxies.txt file
	CreateStoredImageMap()         // Create an in-memory map of stored images
	fetchWaybackPages()            // Fetch Wayback Machine cached pages
	parseImages()                  // Parse images from the cached pages
	RemoveCommonItems()            // Remove previously downloaded images from the unprocessedImages list
	downloadImages()               // Download images from the Wayback Machine cache
	purgeCorrupted()               // Purge any corrupted images
	createReport()                 // Create a report of the downloaded images
}

func inputUsername(defaultUser string) {
	if defaultUser != "" {
		TwitterUsername = defaultUser
		WaybackResultsURL = fmt.Sprintf("https://web.archive.org/web/timemap/json?url=twitter.com/%s&matchType=prefix", TwitterUsername)
		return
	}

	fmt.Print("\nEnter a Twitter username: ")
	fmt.Scanln(&TwitterUsername)

	for InvalidUsernameCheck() {
		fmt.Print("Enter a Twitter username: ")
		fmt.Scanln(&TwitterUsername)
	}

	WaybackResultsURL = fmt.Sprintf("https://web.archive.org/web/timemap/json?url=twitter.com/%s&matchType=prefix", TwitterUsername)
}

func fetchWaybackPages() {
	color.Cyan.Printf("Fetching list of Wayback Machine cached pages for profile: %s\n", TwitterUsername)

	var waybackResults [][]interface{}
	var req *http.Request
	var resp *http.Response
	var err error

	httpClient := GetProxyClient()
	defer returnProxy(httpClient)

	req, err = http.NewRequest(http.MethodGet, WaybackResultsURL, http.NoBody)
	if err != nil {
		log.Println(err)
		return
	}

	for i := 0; i < 5; i++ {
		resp, err = httpClient.Do(req)
		if err != nil {
			color.Red.Printf("Retrying - Error fetching Wayback Machine results: %+v\n", err)
			rotateClientProxy(httpClient)
			continue
		}
		defer resp.Body.Close()

		if err := json.NewDecoder(resp.Body).Decode(&waybackResults); err != nil {
			color.Red.Printf("Error decoding Wayback Machine results: %+v\n", err)
			rotateClientProxy(httpClient)
			continue
		}
		break
	}

	for _, result := range waybackResults {
		pageURL := result[2].(string)
		if strings.Contains(pageURL, `http`) {
			PageUnprocessed = append(PageUnprocessed, pageURL)
		}
	}

	TotalPages = len(PageUnprocessed)

	if TotalPages == 0 {
		color.Red.Printf("Found %d cached Wayback Machine pages - exiting\n", TotalPages)
		os.Exit(0)
	} else {

		color.Cyan.Printf("Found %d cached Wayback Machine pages\n", TotalPages)
	}
}

func parseImages() {
	color.LightGreen.Printf("\n=== Visiting the cached pages and checking for images\n")

	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxThreads) // Limit to "MaxThreads" concurrent goroutines

	for len(PageUnprocessed) > 0 {
		wg.Add(1)
		var pageURL string

		PageMutex.Lock()
		PageUnprocessed, pageURL = Pop(PageUnprocessed)
		PageMutex.Unlock()

		go func(pageURL string) { // Pass pageURL as an argument
			defer wg.Done()

			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			combinedURL := WaybackPrefix + pageURL
			color.Gray.Printf("%s - Visiting %s to parse images\n", GetPageProgress(), pageURL)

			htmlContent, err := parseImagesWithRetry(combinedURL)
			switch err {
			case nil:
				PageMutex.Lock()
				PageProcessed = append(PageProcessed, pageURL)
				PageMutex.Unlock()
				color.Green.Printf("%s - Successfully parsed %s\n", GetPageProgress(), pageURL)
			case ErrPageMissingContent:
				color.FgDarkGray.Printf("Skipping %s - not a valid page\n", pageURL)
				return
			case ErrPageRetries:
				fallthrough
			default:
				color.Red.Printf("Error parsing images from %s - %s\n", combinedURL, err)
				PageMutex.Lock()
				PageUnprocessed = append(PageUnprocessed, pageURL)
				PageMutex.Unlock()
				return
			}

			for _, resource := range Resources {
				var resourceURLs []string
				switch resource {
				case "media":
					resourceURLs = MediaRegex.FindAllString(htmlContent, -1)
				case "profile":
					resourceURLs = ProfileRegex.FindAllString(htmlContent, -1)
				}
				ImageMutex.Lock()
				ImageUnprocessed = RemoveDuplicates(append(ImageUnprocessed, resourceURLs...))
				ImageMutex.Unlock()
			}
		}(pageURL)
	}
	wg.Wait()

	TotalImages = len(ImageUnprocessed)
	color.Green.Printf("\nFound %d cached images for: %s\n", TotalImages, TwitterUsername)
}

func parseImagesWithRetry(combinedURL string) (string, error) {
	var req *http.Request
	var resp *http.Response
	var err error

	httpClient := GetProxyClient()
	defer returnProxy(httpClient)

	for i := 0; i < RetryAttempts; i++ {
		req, err = http.NewRequest(http.MethodGet, combinedURL, http.NoBody)
		if err != nil {
			color.Red.Printf("Error building parse request: %+v\n", err)
			rotateClientProxy(httpClient)
			continue
		}

		resp, err = httpClient.Do(req)
		if err != nil {
			color.Red.Printf("Error fetching page content: %+v\n", err)
			rotateClientProxy(httpClient)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == 404 {
			return "404 - Not a valid page", ErrPageMissingContent
		}

		if resp.StatusCode != http.StatusOK {
			color.Red.Printf("Error: HTTP request failed with status code %d\n", resp.StatusCode)
			rotateClientProxy(httpClient)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			color.Red.Printf("Error reading response body content for %s: %s\n", combinedURL, err)
			rotateClientProxy(httpClient)
			continue
		}

		htmlContent := string(body)
		return htmlContent, nil
	}
	return "", ErrPageRetries
}

func downloadImageWithRetry(imageURL string, downloadPath string) error {
	var req *http.Request
	var resp *http.Response
	var err error

	httpClient := GetProxyClient()
	defer returnProxy(httpClient)

	for i := 0; i < RetryAttempts; i++ {
		req, err = http.NewRequest(http.MethodGet, imageURL, http.NoBody)
		if err != nil {
			color.Red.Printf("Retrying - Error building image download request: %s\n", err)
			continue
		}
		resp, err = httpClient.Do(req)
		if err != nil {
			color.Red.Printf("Retrying - Error fetching image: %+v\n", err)
			rotateClientProxy(httpClient)
			continue
		}

		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			color.Red.Printf("Retrying - Error reading image: %s\n", err)
			rotateClientProxy(httpClient)
			continue
		}

		if resp.StatusCode == 404 && resp.Header.Get("Content-Type") == "text/html" {
			return ErrPageMissingContent
		}

		if resp.StatusCode != http.StatusOK {
			color.Red.Printf("Retrying - Error fetching image: HTTP status %d\n", resp.StatusCode)
			rotateClientProxy(httpClient)
			continue
		}

		file, err := os.Create(downloadPath)
		if err != nil {
			color.Red.Printf("Retrying - Error creating file: %s\n", err)
			rotateClientProxy(httpClient)
			continue
		}
		defer file.Close()

		_, err = io.Copy(file, bytes.NewReader(bodyBytes))
		if err != nil {
			color.Red.Printf("Retrying - Error saving image: %s\n", err)
			rotateClientProxy(httpClient)
			continue
		}
		return nil
	}
	color.Red.Printf("Aborting - Error downloading image after %d retries: %s\n", RetryAttempts, imageURL)
	return ErrImageRetries
}

func downloadImages() {
	color.LightGreen.Println("Downloading identified images from Wayback Machine Cache\n")

	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxThreads) // Limit to "MaxThreads" concurrent goroutines

	for len(ImageUnprocessed) > 0 {
		wg.Add(1)
		var imageURL string
		var imageType string

		ImageMutex.Lock()
		ImageUnprocessed, imageURL = Pop(ImageUnprocessed)
		ImageMutex.Unlock()

		if strings.Contains(imageURL, "media") {
			imageType = "media"
		} else {
			imageType = "profile"
		}

		go func(imageURL string, imageType string) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			imageName := FilenameRegex.FindString(imageURL)
			combinedURL := WaybackPrefix + imageURL
			downloadPath := fmt.Sprintf("%s/%s/%s", UsernameLocation, imageType, imageName)

			err := downloadImageWithRetry(combinedURL, downloadPath)
			switch err {
			case nil:
				ImageMutex.Lock()
				TotalDownloads += 1
				ImageProcessed = append(ImageProcessed, imageURL)
				ImageMutex.Unlock()
				color.Green.Printf("%s - Saved %s\n", GetImageProgress(), imageURL)
				return
			case ErrPageMissingContent:
				ImageMutex.Lock()
				ImageProcessed = append(ImageProcessed, imageURL)
				ImageMutex.Unlock()
				color.FgDarkGray.Printf("Skipping %s - not a valid image file\n", imageURL)
				return
			default:
				color.Red.Printf("Error downloading image from %s - %s\n", combinedURL, err.Error())
				ImageMutex.Lock()
				ImageUnprocessed = append(ImageUnprocessed, imageURL)
				ImageMutex.Unlock()
				return
			}
		}(imageURL, imageType)
	}
	wg.Wait()

	color.Green.Printf("\nSaved %d images for: %s\n", TotalDownloads, TwitterUsername)
}
func purgeCorrupted() {
	color.Gray.Printf("Purging any corrupted images in %s\n", UsernameLocation)

	images, err := filepath.Glob(fmt.Sprintf("%s/images/%s/*.jpg", HomeDirectory, TwitterUsername))
	if err != nil {
		color.Red.Printf("Error listing image files: %+v\n", err)
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
				color.Yellow.Printf("Removed corrupted image: %s\n", image)
				purgedCounter += 1
			}
		}
	}

	if purgedCounter == 0 {
		color.Green.Printf("No corrupted images found - success!\n")
	} else {
		color.Green.Printf("Purged %d corrupted images\n", purgedCounter)
	}
}

func createReport() {
	header := fmt.Sprintf(`=== Wayback Report - %s - %s`, TwitterUsername, GetCurrentDate())
	totalProcessed := fmt.Sprintf("Pages Parsed: %d | Images Proccesed: %d | Downloaded Images: %d", TotalPages, TotalImages, TotalDownloads)
	pageString := ""
	for _, link := range PageProcessed {
		pageString += fmt.Sprintf("%s\n", link)
	}
	imageString := ""
	for _, link := range ImageProcessed {
		imageString += fmt.Sprintf("%s\n", link)
	}

	report := fmt.Sprintf("%s\n%s\n%s\n%s\n", header, totalProcessed, pageString, imageString)

	reportFile, err := os.Create(fmt.Sprintf("%s/%s-report.txt", UsernameLocation, GetCurrentDate()))
	if err != nil {
		color.Red.Printf("Error creating report file: %+v\n", err)
		return
	}
	defer reportFile.Close()

	_, err = reportFile.WriteString(report)
	if err != nil {
		color.Red.Printf("Error writing to report file: %+v\n", err)
		return
	}

	color.Magenta.Printf("Report created: %s/%s-report.txt\n", UsernameLocation, GetCurrentDate())
}
