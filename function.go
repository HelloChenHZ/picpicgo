package main

import (
	"strings"
	"time"
	"log"
	"os"
	"fmt"
)

func IsPic(url string) bool {
	exts := []string{"jpg", "png", "gif", "csv"}
	for _, ext := range exts {
		if strings.Contains(strings.ToLower(url), "."+ext) {
			return true
		}
	}

	return false
}

func trace(msg string) func(){
	start := time.Now()
	return func(){
		log.Printf("exit %s (%s)", msg, time.Since(start))
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}

	if os.IsNotExist(err) {
		return false
	}

	return true
}

func mkdirs(s string) {
	err := os.MkdirAll(s, 0777)
	if err != nil {
		fatal(err)
	} else {
		fmt.Println("Create Directory %v OK!", s)
	}
}

func fatal(e error) {
	if e != nil {
		log.Fatal(e)
	}
}
