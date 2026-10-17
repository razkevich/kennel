// Package web serves the Gather-style map.
package web

import (
	"context"

	"github.com/razkevich/kennel/internal/app"
)

func Serve(ctx context.Context, a *app.App, addr string, open bool) error {
	<-ctx.Done()
	return nil
}
