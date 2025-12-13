package client

import (
	"bufio"
	"errors"
	"log"
	"net"
	"os"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// Getclient gère la commande GET : demander un fichier, recevoir son contenu et sauvegarder localement
// splitGET : [ "GET", "<filename>", "<position>" ]
func Getclient(conn net.Conn, splitGET []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	if err := p.Send_message(conn, writer, "GET"+" "+splitGET[1]+" "+splitGET[2]); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande GET:", err)
		}
		return false
	}

	// Attend la réponse du serveur
	var response, err = p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse GET:", err)
		}
		return false
	}
	response = strings.TrimSpace(response)
	log.Println(response)

	// fichier introuvable
	if response == "FileUnknown" {
		log.Println("Fichier introuvable sur le serveur")

		// Envoie "OK" pour confirmer la réception de FileUnknown
		if err := p.Send_message(conn, writer, "OK"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'OK':", err)
			}
			return false
		}

	} else if response == "Start" {
		// Le serveur envoie ensuite le contenu du fichier
		data, err := p.Receive_message(conn, reader)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de la lecture du fichier:", err)
			}
			return false
		}

		// Sauvegarde le fichier localement avec le même nom
		err = os.WriteFile(splitGET[1], []byte(data), 0770)
		if err != nil {
			log.Println("Erreur lors de la sauvegarde du fichier:", err)
			return false
		}

		log.Printf("Fichier '%s' reçu et sauvegardé (%d octets)\n", splitGET[1], len(data))
		log.Printf("Contenu du fichier '%s':\n%s\n", splitGET[1], data)

		// Envoie "OK" pour confirmer la bonne réception
		if err := p.Send_message(conn, writer, "OK"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'OK':", err)
			}
			return false
		}
	} else {
		// Toute autre réponse est imprévue
		log.Println("Réponse inattendue du serveur:", response)
	}

	return true
}
