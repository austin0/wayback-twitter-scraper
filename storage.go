package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gookit/color"
)

func CreateDirectories() {
	UsernameLocation = filepath.Join(HomeDirectory, "images", TwitterUsername) // ./wayback-twitter-scraper/images/0xf6i
	MediaDir = filepath.Join(UsernameLocation, "media")                        // ./wayback-twitter-scraper/images/0xf6i/media
	ProfileDir = filepath.Join(UsernameLocation, "profile")                    // ./wayback-twitter-scraper/images/0xf6i/profile

	if err := os.MkdirAll(UsernameLocation, os.ModePerm); err != nil {
		color.Red.Printf("Unable to create necessary directory %s: %s\n", UsernameLocation, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(MediaDir, os.ModePerm); err != nil {
		color.Red.Printf("Unable to create necessary directory %s: %s\n", MediaDir, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(ProfileDir, os.ModePerm); err != nil {
		color.Red.Printf("Unable to create necessary directory %s: %s\n", ProfileDir, err)
		os.Exit(1)
	}
}

func CreateStoredImageMap() {
	for _, directoryPath := range []string{MediaDir, ProfileDir} {
		paths, err := filepath.Glob(fmt.Sprintf("%s/*", directoryPath))
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		for _, path := range paths {
			StoredImageMap[filepath.Base(path)] = true
		}
	}
	if len(StoredImageMap) > 0 {
		color.HiMagenta.Printf("Discovered %d locally stored files - express filtering enabled\n", len(StoredImageMap))
	}
}
