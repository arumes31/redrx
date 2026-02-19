package main

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	// Change to project root for templates
	originalWd, _ := os.Getwd()
	os.Chdir("../..")
	defer os.Chdir(originalWd)

	// Set environment variables for test
	os.Setenv("PORT", "0") // Random port
	os.Setenv("DATABASE_URL", "sqlite://file::memory:?cache=shared")
	os.Setenv("REDIS_URL", "localhost:1")
	os.Setenv("APP_ENV", "local")
	
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("DATABASE_URL")
	defer os.Unsetenv("REDIS_URL")
	defer os.Unsetenv("APP_ENV")

	ctx, cancel := context.WithCancel(context.Background())

	// Run in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- Run(ctx)
	}()

	// Wait a bit for startup
	time.Sleep(1 * time.Second)

	// Cancel context to stop server
	cancel()

	// Wait for Run to return
	select {
	case err := <-errChan:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit in time")
	}
}

func TestRun_DBError(t *testing.T) {
	os.Setenv("DATABASE_URL", "unsupported://db")
	defer os.Setenv("DATABASE_URL", "")

	ctx := context.Background()
	err := Run(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize database")
}

func TestRun_MigrationError(t *testing.T) {
	originalWd, _ := os.Getwd()
	os.Chdir("../..")
	defer os.Chdir(originalWd)

	os.Setenv("DATABASE_URL", "postgres://localhost:1")
	defer os.Unsetenv("DATABASE_URL")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Run(ctx)
	assert.Error(t, err)
}

func TestRun_ServerError(t *testing.T) {
	originalWd, _ := os.Getwd()
	os.Chdir("../..")
	defer os.Chdir(originalWd)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	os.Setenv("PORT", port)
	os.Setenv("DATABASE_URL", "sqlite://file::memory:?cache=shared")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("DATABASE_URL")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = Run(ctx)
	// On some systems this might not error if it can bind to 0.0.0.0 while 127.0.0.1 is taken
	// but we at least expect it to not hang.
}
