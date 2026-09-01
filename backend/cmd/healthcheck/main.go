package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

const defaultHealthcheckURL = "http://127.0.0.1:8080/health/ready"

func main() {
	target := os.Getenv("HEALTHCHECK_URL")
	if target == "" {
		target = defaultHealthcheckURL
	}

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "unexpected health status: %d\n", response.StatusCode)
		os.Exit(1)
	}
}
