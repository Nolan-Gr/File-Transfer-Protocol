package server

import (
	"bufio"
	"errors"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// ListServer : envoie la liste des fichiers non cachés dans le dossier demandé.
// Protocole : envoie "Start", attend "OK" du client, puis envoie "FileCnt : N --name size ..."
func ListServer(conn net.Conn, commHideReveal []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	var fichiers, err = os.ReadDir(commHideReveal[1])
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return false
	}

	var list = ""
	var size = 0

	if err := p.Send_message(conn, writer, "Start"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'Start' LIST:", err)
		}
		return false
	}

	log.Println(fichiers)
	data, err := p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la confirmation LIST:", err)
		}
		return false
	}
	log.Println("data:", data)

	if strings.TrimSpace(data) == "OK" {
		for _, fichier := range fichiers {
			// Ignorer les fichiers cachés (commençant par '.')
			if fichier.Name()[0] != '.' {
				fileInfo, err := fichier.Info()
				if err != nil {
					log.Println("Erreur lors de la lecture du fichier:", err)
					return false
				}
				list = list + " --" + fichier.Name() + " " + strconv.FormatInt(fileInfo.Size(), 10)
				size = size + 1
			}
		}
	}

	var newlist = "FileCnt : " + strconv.Itoa(size) + list
	log.Println(newlist)
	if err := p.Send_message(conn, writer, newlist); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la liste:", err)
		}
		return false
	}

	return true
}
