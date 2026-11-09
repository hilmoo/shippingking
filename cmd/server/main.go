package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/hilmoo/shippingking/internal/api"
	"github.com/hilmoo/shippingking/internal/wilayah"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz on the local server and exit 0/1 (for Docker HEALTHCHECK)")
	flag.Parse()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if *healthcheck {
		os.Exit(probe(port))
	}
	store, err := wilayah.Load()
	if err != nil {
		log.Fatalf("load data: %v", err)
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.NewHandler(store, os.Getenv("RAJAONGKIR_API_KEY")),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("shippingking listening on http://localhost:%s (%d subdistricts)", port, len(store.Subdistricts)-1)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func probe(port string) int {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
