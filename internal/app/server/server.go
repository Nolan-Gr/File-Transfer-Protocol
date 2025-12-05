package server

import (
	"bufio"
	"log"
	"log/slog"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

func RunServer(port *string) {
	l, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	defer func() {
		l.Close()
		slog.Debug("Stopped listening on port " + *port)
	}()
	slog.Debug("Now listening on port " + *port)

	for {
		c, err := l.Accept()
		if err != nil {
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String())
		// Handle each client concurrently
		go HandleClient(c)
	}
}

// Gère la communication avec un client unique
func HandleClient(conn net.Conn) {
	defer conn.Close()
	log.Println("Nouveau client connecté:", conn.RemoteAddr().String())

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Étape 1 : Le serveur envoie "hello"
	if err := p.Send_message(writer, "hello"); err != nil {
		log.Println("Erreur lors de l'envoi de 'hello':", err)
		return
	}

	for {
		// Attente de la réponse du client
		msg, err := p.Receive_message(reader)
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
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'start':", err)
				return
			}
		case "data":
			// Étape 5 & Répétition : Sur réception de "data", le serveur répond "ok"
			if err := p.Send_message(writer, "ok"); err != nil {
				log.Println("Erreur lors de l'envoi de 'ok' après 'data':", err)
				return
			}
		case "end":
			// Étape 7 : Sur réception de "end", le serveur répond "ok", puis le client se déconnecte.
			if err := p.Send_message(writer, "ok"); err != nil {
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
