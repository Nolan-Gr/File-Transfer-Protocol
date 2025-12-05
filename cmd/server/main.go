package main

import (
	"flag"
	"log"
	"log/slog"
	"net"

	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/app/server"
)

func parseArgs() (port *string) {

	logLevel := flag.Bool("d", false, "enable debug log level")
	port = flag.String("p", "3333", "server port (default: 3333)")

	flag.Parse()

	if *logLevel {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Set logging level to debug")
	}

	return
}

func main() {
	port := parseArgs()
	server.RunServer(port)

	// Créer un écouteur TCP sur le port spécifié
	listener, err := net.Listen("tcp", ":"+"8080")
	if err != nil {
		log.Fatal("Erreur à l'écoute:", err)
	}
	defer listener.Close()

	log.Println("Serveur démarré et écoute sur le port", "8080")

	// Boucle infinie pour accepter les connexions
	for {
		// Accepter une nouvelle connexion
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Erreur lors de l'acceptation de la connexion:", err)
			continue // Continuer à écouter
		}

		// Gérer le client dans une nouvelle goroutine
		// Le serveur ne se termine pas, mais se met en attente du client suivant
		go server.HandleClient(conn)
	}
}
