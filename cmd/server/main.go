package main

import (
	"flag"
	"log/slog"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/app/server"
)

func parseArgs() (port *string, controlPort *string) {

	logLevel := flag.Bool("d", false, "enable debug log level")
	port = flag.String("p", "3333", "server port (default: 3333)")
	controlPort = flag.String("cp", "3334", "Port de contrôle")

	flag.Parse()

	if *logLevel {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Set logging level to debug")
	}

	return
}

func main() {
	port, controlPort := parseArgs()
	server.RunServer(port, controlPort)
}
