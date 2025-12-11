package main

import (
	"flag"
	"log/slog"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/app/client"
)

func parseArgs() (remote string) {
	dFlag := flag.Bool("d", false, "enable debug log level")
	aFlag := flag.String("a", "127.0.0.1", "server address (default: 127.0.0.1)")
	pFlag := flag.String("p", "3333", "server port (default: 3333)")
	flag.Parse()

	if *dFlag {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Set logging level to debug")
		slog.Debug("type 'MESSAGES' for history of messages")
	}
	port := *pFlag

	if *pFlag == "control" {
		port = "3334"
	}

	remote = *aFlag + ":" + port
	return
}

func main() {
	remote := parseArgs()
	client.Run(remote)
}
