/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"BrizyGBMigration/cmd"
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cmd.Execute(ctx)
}
