package server

import (
	"bufio"
	"log"
	"log/slog"
	"net"
	"strings"
	"time"
	"gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

func RunServer(port *string) {

	l, e := net.Listen("tcp", ":"+*port)
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		l.Close()
		slog.Debug("Stopped listening on port " + *port)
	}()
	slog.Debug("Now listening on port " + *port)

	c, e := l.Accept()
	if e != nil {
		slog.Error(e.Error())
		return
	}
	defer func() {
		c.Close()
		slog.Info("Connection closed")
	}()
	slog.Info("Incoming connection from " + c.RemoteAddr().String())

	time.Sleep(10 * time.Second)

	return
}

// Gère la communication avec un client unique
func handleClient(conn net.Conn) {
	defer conn.Close()
	log.Println("Nouveau client connecté:", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Étape 1 : Le serveur envoie "hello"
	if err := proto.send_message(writer, "hello"); err != nil {
		log.Println("Erreur lors de l'envoi de 'hello':", err)
		return
	}

	for {
		// Attente de la réponse du client
		msg, err := proto.receive_message(reader)
		if err != nil {
			// io.EOF ou autre erreur indique que le client s'est déconnecté
			log.Println("Client déconnecté ou erreur de lecture:", err)
			return
		}

		// Nettoyer le message (supprimer les espaces et le '\n')
		cleanedMsg := strings.TrimSpace(msg)

		switch cleanedMsg {
		case "start":
			// Étape 3 : Sur réception de "start", le serveur répond "ok"
			if err := send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
				return
			}
		case "data":
			// Étape 5 & Répétition : Sur réception de "data", le serveur répond "ok"
			if err := send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'data':", err)
				return
			}
		case "end":
			// Étape 7 : Sur réception de "end", le serveur répond "ok", puis le client se déconnecte.
			if err := send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'end':", err)
			}
			log.Println("Fin de la séquence de communication. Fermeture de la connexion.")
			// Le defer conn.Close() s'occupera de fermer la connexion
			return
		default:
			log.Println("Message inattendu du client:", cleanedMsg)
			return
		}
	}
}

func main() {
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
		go handleClient(conn)
	}
}
