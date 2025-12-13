package client

import (
	"bufio"
	"errors"
	"log"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// ListClient demande la liste des fichiers et l'affiche.
// split : [ "LIST", "<dir>", "<posActuelle>" ]
func ListClient(conn net.Conn, split []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	if err := p.Send_message(conn, writer, "List "+split[1]); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande LIST:", err)
		}
		return false
	}

	// Attend la réponse du serveur
	var response, err = p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse LIST:", err)
		}
		return false
	}
	response = strings.TrimSpace(response)

	if response == "Start" {
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
				log.Println("Timeout lors de la lecture de la liste:", err)
			}
			return false
		}

		// Le serveur renvoie les éléments séparés par "--"
		var datas = strings.Split(data, "--")
		log.Println("\n=== Liste des fichiers disponibles ===")
		for _, item := range datas {
			if strings.TrimSpace(item) != "" {
				log.Println(strings.TrimSpace(item))
			}
		}
		log.Println("=====================================")
	}

	// Fin de l'opération LIST : on envoie "ok" pour clore l'échange
	if err := p.Send_message(conn, writer, "ok"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'ok' final LIST:", err)
		}
		return false
	}

	return true
}
