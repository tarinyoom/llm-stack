package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

type pullEvent struct {
	Status    string `json:"status"`
	Error     string `json:"error"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
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
	if raw == "" {
		return nil, errors.New("REQUIRED_MODELS must not be empty")
	}
	fields := strings.Fields(strings.ReplaceAll(raw, ",", " "))
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("REQUIRED_MODELS parsed to zero entries")
	}
	return out, nil
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

func pingOllama(ctx context.Context, base string, timeout time.Duration) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tags", nil)
	resp, err := httpClient(timeout).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama /api/tags status %d", resp.StatusCode)
	}
	return nil
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

func pullModel(ctx context.Context, base string, timeout time.Duration, model string) error {
	body := strings.NewReader(`{"name":"` + model + `"}`)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/pull", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient(timeout).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("pull %s failed: status %d: %s", model, resp.StatusCode, string(b))
	}
	dec := json.NewDecoder(bufio.NewReader(resp.Body))
	for {
		var ev pullEvent
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("pull %s: decode: %w", model, err)
		}
		if ev.Error != "" {
			return fmt.Errorf("pull %s: %s", model, ev.Error)
		}
		if ev.Status != "" {
			log.Printf("pull %s: %s", model, ev.Status)
		}
		if ev.Status == "success" {
			return nil
		}
	}
	return fmt.Errorf("pull %s: stream ended without success", model)
}

func ensureAll(ctx context.Context, base string, timeout time.Duration, required []string) error {
	have, err := listModels(ctx, base, timeout)
	if err != nil {
		return err
	}
	var missing []string
	for _, need := range required {
		if _, ok := have[need]; !ok {
			missing = append(missing, need)
		}
	}
	for _, m := range missing {
		log.Printf("pulling missing model: %s", m)
		if err := pullModel(ctx, base, timeout, m); err != nil {
			return err
		}
	}
	return nil
}

func waitUntilReady(ctx context.Context, base string, reqTimeout, startupLimit time.Duration, required []string) error {
	deadline := time.Now().Add(startupLimit)
	var last error
	for {
		if time.Now().After(deadline) {
			if last == nil {
				last = errors.New("startup timeout")
			}
			return last
		}
		if err := pingOllama(ctx, base, reqTimeout); err != nil {
			last = fmt.Errorf("ollama not responding: %w", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err := ensureAll(ctx, base, reqTimeout, required); err != nil {
			last = fmt.Errorf("ensure models: %w", err)
			time.Sleep(2 * time.Second)
			continue
		}
		return nil
	}
}

func main() {
	ollama, err := requireEnv("OLLAMA_BASE_URL")
	if err != nil {
		log.Fatal(err)
	}
	rawModels, err := requireEnv("REQUIRED_MODELS")
	if err != nil {
		log.Fatal(err)
	}
	required, err := parseModels(rawModels)
	if err != nil {
		log.Fatal(err)
	}
	reqTimeoutStr, err := requireEnv("REQUEST_TIMEOUT")
	if err != nil {
		log.Fatal(err)
	}
	startupTimeoutStr, err := requireEnv("STARTUP_TIMEOUT")
	if err != nil {
		log.Fatal(err)
	}
	reqTimeout, err := mustParseDuration(reqTimeoutStr, "REQUEST_TIMEOUT")
	if err != nil {
		log.Fatal(err)
	}
	startupLimit, err := mustParseDuration(startupTimeoutStr, "STARTUP_TIMEOUT")
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := waitUntilReady(ctx, ollama, reqTimeout, startupLimit, required); err != nil {
		log.Fatalf("startup failed: %v", err)
	}
	log.Printf("all required models present")
	<-ctx.Done()
}

