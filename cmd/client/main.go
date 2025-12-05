package main

import (
	"flag"
	"log"
	"log/slog"
	"net"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/app/client"
)

func parseArgs() (remote string) {
	dFlag := flag.Bool("d", false, "enable debug log level")
	aFlag := flag.String("a", "127.0.0.1", "server address (default: 127.0.0.1)")
	pFlag := flag.String("p", "3333", "server port (default: 3333)")
	flag.Parse()

	if *dFlag {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	remote = *aFlag + ":" + *pFlag
	return
}

func main() {
	// Tenter de se connecter au serveur
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		log.Fatal("Erreur de connexion au serveur:", err)
	}

	client.RunClient(conn)
}
