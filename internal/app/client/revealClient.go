package client

import (
	"bufio"
	"errors"
	"log"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// RevealClient demande au serveur de révéler un fichier caché (port de contrôle).
// split : [ "REVEAL", "<filename>", "<position>" ]
func RevealClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	command := "REVEAL " + split[1] + " " + split[2]
	if err := p.Send_message(conn, writer, command); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande REVEAL:", err)
		}
		return false
	}

	// Attendre la réponse du serveur
	response, err := p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse REVEAL:", err)
		}
		return false
	}
	response = strings.TrimSpace(response)

	if response == "FileUnknown" {
		log.Println("Fichier introuvable (ou pas caché) sur le serveur")
	} else if response == "OK" {
		log.Printf("Fichier '%s' révélé avec succès\n", split[1])
	} else {
		log.Println("Réponse inattendue du serveur:", response)
	}

	return true
}
