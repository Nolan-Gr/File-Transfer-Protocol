package client

import (
	"bufio"
	"errors"
	"log"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// treeClient demande l'arbre (liste) d'un dossier et l'affiche.
// Similaire à ListClient mais affiche aussi le contexte ("vous êtes à la racine", etc.).
func treeClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	if err := p.Send_message(conn, writer, "tree "+split[1]); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande TREE:", err)
		}
		return false
	}

	// Attend la réponse du serveur
	var response, err = p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse TREE:", err)
		}
		return false
	}
	response = strings.TrimSpace(response)

	if response == "Start" {
		log.Println("entered start")
		// Le serveur va envoyer la liste ; on confirme par "OK"
		if err := p.Send_message(conn, writer, "OK"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'OK':", err)
			}
			return false
		}

		data, err := p.Receive_message(conn, reader)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de la lecture de la liste TREE:", err)
			}
			return false
		}
		log.Println(data)

		var datas = strings.Split(data, "--")
		if split[1] == "Docs" {
			// racine
			log.Println("vous êtes à la racine")
		} else {
			log.Println("vous êtes dans", split[1])
		}
		log.Println("\n=== Liste des fichiers disponibles ===")
		for _, item := range datas {
			if strings.TrimSpace(item) != "" {
				log.Println(strings.TrimSpace(item))
			}
		}
		log.Println("=====================================")
	}

	// Fin de l'opération TREE : envoi de l'acquittement final "ok"
	if err := p.Send_message(conn, writer, "ok"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'ok' final TREE:", err)
		}
		return false
	}

	return true
}
