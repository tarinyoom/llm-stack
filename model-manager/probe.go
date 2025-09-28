package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func requireEnv(key string) (string, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return "", fmt.Errorf("required env %s not set", key)
	}
	return v, nil
}

func parseModels(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	fields := strings.Fields(strings.ReplaceAll(raw, ",", " "))
	if len(fields) == 0 {
		return nil, fmt.Errorf("REQUIRED_MODELS must not be empty")
	}
	return fields, nil
}

func mustParseDuration(val, key string) (time.Duration, error) {
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("%s invalid duration %q: %w", key, val, err)
	}
	return d, nil
}

func httpClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: timeout,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

func listModels(ctx context.Context, base string, timeout time.Duration) (map[string]struct{}, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tags", nil)
	resp, err := httpClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var tr tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	have := make(map[string]struct{}, len(tr.Models))
	for _, m := range tr.Models {
		have[m.Name] = struct{}{}
	}
	return have, nil
}

func main() {
	ollama, err := requireEnv("OLLAMA_BASE_URL")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	rawModels, err := requireEnv("REQUIRED_MODELS")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	required, err := parseModels(rawModels)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	reqTimeoutStr, err := requireEnv("REQUEST_TIMEOUT")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	reqTimeout, err := mustParseDuration(reqTimeoutStr, "REQUEST_TIMEOUT")
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
	defer cancel()
	have, err := listModels(ctx, ollama, reqTimeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	for _, need := range required {
		if _, ok := have[need]; !ok {
			fmt.Fprintf(os.Stderr, "missing model: %s\n", need)
			os.Exit(1)
		}
	}
}

