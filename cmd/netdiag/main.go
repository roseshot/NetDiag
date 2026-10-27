package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/roseshot/NetDiag/internal/server"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	certFile := os.Getenv("NETDIAG_TLS_CERT_FILE")
	keyFile := os.Getenv("NETDIAG_TLS_KEY_FILE")
	if (certFile == "") != (keyFile == "") {
		log.Fatal("NETDIAG_TLS_CERT_FILE and NETDIAG_TLS_KEY_FILE must be configured together")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	handler, closeASN, err := server.NewWithContext(ctx)
	if err != nil {
		log.Fatalf("start network diagnostic listeners: %v", err)
	}
	defer closeASN()

	transport := "HTTP"
	if certFile != "" {
		transport = "HTTPS (direct Go TLS termination)"
	}
	log.Printf("NetDiag listener configuration: transport=%s address=:%s", transport, port)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ConnContext:       server.ConnContext,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if certFile != "" {
			log.Printf("NetDiag HTTPS listening on :%s", port)
			errCh <- srv.ListenAndServeTLS(certFile, keyFile)
			return
		}
		log.Printf("NetDiag HTTP listening on :%s", port)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}
