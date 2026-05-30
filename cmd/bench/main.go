package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type result struct {
	status  int
	latency time.Duration
	err     error
}

func main() {
	var (
		mode        = flag.String("mode", "http", "http or ws")
		url         = flag.String("url", "http://127.0.0.1:19999/post/list", "target url")
		token       = flag.String("token", "", "jwt token")
		method      = flag.String("method", "GET", "http method")
		body        = flag.String("body", "", "http request body")
		contentType = flag.String("content-type", "application/json", "http content type")
		concurrency = flag.Int("c", 50, "concurrency")
		requests    = flag.Int("n", 1000, "total requests/messages")
		duration    = flag.Duration("duration", 0, "run duration, for example 30s; 0 means use -n")
		wsMessage   = flag.String("ws-message", `{"type":"ping"}`, "websocket message")
	)
	flag.Parse()

	if *mode != "http" && *mode != "ws" {
		log.Fatalf("invalid mode: %s", *mode)
	}

	start := time.Now()
	var results []result
	if *mode == "http" {
		results = runHTTP(*url, *method, *body, *contentType, *token, *concurrency, *requests, *duration)
	} else {
		results = runWS(*url, *token, *wsMessage, *concurrency, *requests, *duration)
	}
	printSummary(results, time.Since(start))
}

func runHTTP(url, method, body, contentType, token string, concurrency, requests int, duration time.Duration) []result {
	client := &http.Client{Timeout: 10 * time.Second}
	return run(concurrency, requests, duration, func() result {
		start := time.Now()
		req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
		if err != nil {
			return result{latency: time.Since(start), err: err}
		}
		if body != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			return result{latency: time.Since(start), err: err}
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return result{status: resp.StatusCode, latency: time.Since(start)}
	})
}

func runWS(url, token, message string, concurrency, requests int, duration time.Duration) []result {
	return run(concurrency, requests, duration, func() result {
		start := time.Now()
		header := http.Header{}
		if token != "" {
			header.Set("Authorization", "Bearer "+token)
		}
		dialer := websocket.Dialer{
			HandshakeTimeout: 5 * time.Second,
			TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		}
		conn, _, err := dialer.Dial(url, header)
		if err != nil {
			return result{latency: time.Since(start), err: err}
		}
		defer func() { _ = conn.Close() }()

		if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			return result{latency: time.Since(start), err: err}
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _, err = conn.ReadMessage()
		if err != nil {
			return result{latency: time.Since(start), err: err}
		}
		return result{status: http.StatusOK, latency: time.Since(start)}
	})
}

func run(concurrency, requests int, duration time.Duration, fn func() result) []result {
	if concurrency <= 0 {
		concurrency = 1
	}
	if requests <= 0 && duration <= 0 {
		requests = 1
	}

	ctx := context.Background()
	if duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}

	results := make([]result, 0, requests)
	resultsCh := make(chan result, concurrency*2)
	jobs := make(chan struct{}, concurrency*2)
	var sent atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				resultsCh <- fn()
			}
		}()
	}

	go func() {
		defer close(jobs)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if duration <= 0 && int(sent.Load()) >= requests {
				return
			}
			sent.Add(1)
			jobs <- struct{}{}
		}
	}()

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for res := range resultsCh {
		results = append(results, res)
	}
	return results
}

func printSummary(results []result, elapsed time.Duration) {
	total := len(results)
	if total == 0 {
		fmt.Println("no results")
		return
	}

	statuses := map[int]int{}
	errors := 0
	latencies := make([]time.Duration, 0, total)
	for _, res := range results {
		if res.err != nil {
			errors++
			continue
		}
		statuses[res.status]++
		latencies = append(latencies, res.latency)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	fmt.Printf("total=%d errors=%d elapsed=%s rps=%.2f\n", total, errors, elapsed.Round(time.Millisecond), float64(total)/elapsed.Seconds())
	fmt.Printf("statuses=%v\n", statuses)
	if len(latencies) == 0 {
		return
	}
	fmt.Printf("latency_min=%s p50=%s p95=%s p99=%s max=%s\n",
		latencies[0].Round(time.Millisecond),
		percentile(latencies, 0.50).Round(time.Millisecond),
		percentile(latencies, 0.95).Round(time.Millisecond),
		percentile(latencies, 0.99).Round(time.Millisecond),
		latencies[len(latencies)-1].Round(time.Millisecond),
	)
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * p)
	return values[index]
}
