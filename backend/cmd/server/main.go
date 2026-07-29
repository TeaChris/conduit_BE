package main

import (
	"context"
	"log"

	"github.com/conduit-platform/conduit/backend/internal/app"
)

func main() {
	ctx := context.Background()

	application, err := app.New(ctx)
	if err != nil {
		log.Fatalf("failed to initialize application: %v", err)
	}

	if err := application.Run(ctx); err != nil {
		log.Fatalf("application error: %v", err)
	}
}
