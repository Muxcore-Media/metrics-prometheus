package main

import (
	"log/slog"
	"os"

	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"

	"github.com/Muxcore-Media/metrics-prometheus/internal"
	"github.com/Muxcore-Media/metrics-prometheus/internal/grpctls"
)

func main() {
	mod := internal.NewModule(internal.Config{})
	if err := modulesdk.Run(modulesdk.Config{
		Module:   mod,
		Insecure: grpctls.InsecureAllowed(),
	}); err != nil {
		slog.Error("module exited", "error", err)
		os.Exit(1)
	}
}
