// Command loadtest opens a set of WebSocket players and measures snapshot
// throughput and ping RTT against a running server.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type collector struct {
	snapshots atomic.Int64
	shots     atomic.Int64
	errors    atomic.Int64
	mu        sync.Mutex
	rtts      []time.Duration
}

func (c *collector) addRTT(d time.Duration) {
	c.mu.Lock()
	c.rtts = append(c.rtts, d)
	c.mu.Unlock()
}

func main() {
	url := flag.String("url", "ws://127.0.0.1:8080/ws", "websocket endpoint")
	clients := flag.Int("clients", 8, "concurrent players (room caps at 8)")
	seconds := flag.Int("seconds", 10, "test duration in seconds")
	rate := flag.Int("rate", 30, "input messages per second per client")
	flag.Parse()

	col := &collector{}
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < *clients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := run(*url, idx, time.Duration(*seconds)*time.Second, *rate, col); err != nil {
				col.errors.Add(1)
				fmt.Fprintf(os.Stderr, "client %d: %v\n", idx, err)
			}
		}(i)
		time.Sleep(25 * time.Millisecond)
	}
	wg.Wait()

	report(col, *clients, time.Since(start))
}

func run(url string, idx int, duration time.Duration, rate int, col *collector) error {
	cookie, err := guestCookie(url)
	if err != nil {
		return err
	}
	header := http.Header{}
	header.Set("Cookie", cookie)

	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "join",
		"name": fmt.Sprintf("load-%02d", idx),
	}); err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &env) != nil {
				continue
			}
			switch env.Type {
			case "snapshot":
				col.snapshots.Add(1)
			case "shot":
				col.shots.Add(1)
			case "pong":
				var pong struct {
					T float64 `json:"t"`
				}
				if json.Unmarshal(data, &pong) == nil && pong.T > 0 {
					col.addRTT(time.Duration(time.Now().UnixNano() - int64(pong.T)))
				}
			}
		}
	}()

	seq := 0
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	deadline := time.Now().Add(duration)
	lastPing := time.Now()

	for now := range ticker.C {
		if now.After(deadline) {
			break
		}
		seq++

		_ = conn.WriteJSON(map[string]any{
			"type":    "input",
			"seq":     seq,
			"dt":      1.0 / float64(rate),
			"forward": 1,
			"right":   rand.Intn(3) - 1,
			"sprint":  true,
			"jump":    false,
			"yaw":     rand.Float64()*6.28 - 3.14,
			"pitch":   0,
			"rtt":     0,
		})

		if rand.Intn(6) == 0 {
			_ = conn.WriteJSON(map[string]any{
				"type":  "shoot",
				"yaw":   rand.Float64()*6.28 - 3.14,
				"pitch": 0,
			})
		}

		if time.Since(lastPing) > time.Second {
			lastPing = time.Now()
			_ = conn.WriteJSON(map[string]any{
				"type": "ping",
				"t":    float64(time.Now().UnixNano()),
			})
		}
	}

	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))

	select {
	case <-done:
	case <-time.After(time.Second):
	}
	return nil
}

func report(col *collector, clients int, elapsed time.Duration) {
	snaps := col.snapshots.Load()
	shots := col.shots.Load()
	errs := col.errors.Load()

	col.mu.Lock()
	rtts := append([]time.Duration(nil), col.rtts...)
	col.mu.Unlock()
	sort.Slice(rtts, func(i, j int) bool { return rtts[i] < rtts[j] })

	percentile := func(p float64) time.Duration {
		if len(rtts) == 0 {
			return 0
		}
		idx := int(float64(len(rtts)-1) * p)
		return rtts[idx]
	}

	perClient := float64(snaps) / elapsed.Seconds() / float64(clients)
	fmt.Printf("clients=%d elapsed=%.1fs snapshots=%d (%.1f/s per client, expect ~20) shots=%d errors=%d\n",
		clients, elapsed.Seconds(), snaps, perClient, shots, errs)
	fmt.Printf("rtt samples=%d p50=%s p95=%s\n", len(rtts), percentile(0.5), percentile(0.95))

	if errs > 0 {
		os.Exit(1)
	}
}

func guestCookie(wsURL string) (string, error) {
	base := strings.Replace(wsURL, "ws://", "http://", 1)
	base = strings.Replace(base, "wss://", "https://", 1)
	base = strings.TrimSuffix(base, "/ws") + "/api/auth"

	resp, err := http.Post(base, "application/json", strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth status %d", resp.StatusCode)
	}
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return "", fmt.Errorf("auth returned no cookie")
	}
	return cookies[0].String(), nil
}
