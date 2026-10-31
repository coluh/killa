package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"killa/server/internal/account"
	"killa/server/internal/game"
	"killa/server/internal/store"
	"killa/server/internal/transport"
)

func main() {
	addr := flag.String("addr", envOr("KILLA_ADDR", ":8080"), "listen address")
	staticDir := flag.String("static", envOr("KILLA_STATIC", ""), "directory with the built client")
	origins := flag.String("origins", envOr("KILLA_ALLOWED_ORIGINS", ""), "extra allowed origins, comma separated")
	matchSeconds := flag.Int("match-seconds", 600, "match duration in seconds")
	scoreLimit := flag.Int("score-limit", 40, "kills required to win")
	bots := flag.Bool("bots", true, "fill empty slots with bots")
	maxRooms := flag.Int("rooms", 16, "maximum concurrent rooms")
	mapName := flag.String("map", "reference", "map: reference (imported blockout) or classic")
	dbPath := flag.String("db", envOr("KILLA_DB", "./killa.db"), "sqlite database path")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	world := game.NewWarehouseReference()
	if *mapName == "classic" {
		world = game.NewWarehouse()
	}
	manager := game.NewManager(ctx, world, *maxRooms)
	manager.SetRules(time.Duration(*matchSeconds)*time.Second, *scoreLimit)
	manager.SetAutoBots(*bots)

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("killa: open database: %v", err)
	}
	defer st.Close()

	accounts := account.New(st)
	matches := make(chan game.MatchResult, 64)
	go recordMatches(matches, st)

	manager.SetMatchRecorder(func(result game.MatchResult) {
		select {
		case matches <- result:
		default:
			log.Printf("killa: match write queue full, dropping %s", result.ID)
		}
	})

	ws := transport.NewServer(manager, splitOrigins(*origins))
	ws.SetSessionResolver(accounts)

	started := time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		status := manager.Status()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rooms":         status.Rooms,
			"humans":        status.Humans,
			"players":       status.Players,
			"uptimeSeconds": int(time.Since(started).Seconds()),
		})
	})
	accounts.Register(mux)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("/ws", ws.HandleWS)
	mux.Handle("/", staticHandler(*staticDir))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("killa: listening on %s (static=%q, rooms=%d, map=%s, db=%s)", *addr, *staticDir, *maxRooms, *mapName, *dbPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// staticHandler serves the built client and falls back to index.html so that
// extension-less routes keep working.
func staticHandler(dir string) http.Handler {
	if dir == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "static dir not configured: use -static", http.StatusNotFound)
		})
	}

	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || filepath.Ext(r.URL.Path) == "" {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}

func recordMatches(ch <-chan game.MatchResult, st *store.Store) {
	for result := range ch {
		in := store.MatchInput{
			ID:        result.ID,
			RoomID:    result.RoomID,
			StartedAt: result.StartedAt,
			EndedAt:   result.EndedAt,
			Score:     result.Score,
			Winner:    result.Winner,
		}
		for _, p := range result.Players {
			in.Players = append(in.Players, store.MatchPlayer{
				ProfileID: p.ProfileID,
				Team:      p.Team,
				Kills:     p.Kills,
				Deaths:    p.Deaths,
				Assists:   p.Assists,
				Damage:    p.Damage,
				Headshots: p.Headshots,
			})
		}
		if err := st.RecordMatch(in); err != nil {
			log.Printf("killa: record match %s: %v", result.ID, err)
		}
	}
}
