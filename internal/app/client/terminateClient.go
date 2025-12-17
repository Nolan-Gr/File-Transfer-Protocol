package client

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// TerminateClient envoie la commande TERMINATE au serveur de contrôle et attend la progression
// La boucle lit tous les messages jusqu'à ce que le serveur dit qu'il s'arrête
func TerminateClient(conn net.Conn, writer *bufio.Writer, reader *bufio.Reader) bool {
	if err := p.Send_message(conn, writer, "Terminate"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande TERMINATE:", err)
		}
		return false
	}

	log.Println("Commande TERMINATE envoyée, attente de la réponse du serveur...")

	for {
		rep, err := p.Receive_message(conn, reader)
		if err != nil {
			// La connexion peut être fermée après le message final
			if err == io.EOF {
				log.Println("Serveur déconnecté")
				return true
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de la réception de la réponse TERMINATE:", err)
			}
			return false
		}

		rep = strings.TrimSpace(rep)

		// On affiche les différents messages d'avancement ou le message final
		if rep == "Terminaison finie, le serveur s'éteint" {
			log.Println(rep)
			log.Println("Le serveur s'est arrêté avec succès")
			return true
		} else if strings.Contains(rep, "opération") {
			// message d'état intermédiaire
			log.Println(rep)
		} else {
			log.Println(rep)
		}
	}
}
