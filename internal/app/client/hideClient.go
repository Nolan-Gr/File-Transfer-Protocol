package client

import (
	"bufio"
	"errors"
	"log"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// HideClient demande au serveur de cacher un fichier (commande disponible sur le port de contrôle)
// split : [ "HIDE", "<filename>", "<position>" ]
func HideClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	command := "HIDE " + split[1] + " " + split[2]
	if err := p.Send_message(conn, writer, command); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande HIDE:", err)
		}
		return false
	}

	// Attendre la réponse du serveur
	response, err := p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse HIDE:", err)
		}
		return false
	}
	response = strings.TrimSpace(response)

	if response == "FileUnknown" {
		log.Println("Fichier introuvable sur le serveur")
	} else if response == "OK" {
		log.Printf("Fichier '%s' caché avec succès\n", split[1])
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}

	return true
}
