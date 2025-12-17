package server

import (
	"bufio"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// Getserver : implémentation de GET.
// - Parcourt le dossier fourni (commGet[2]) pour trouver le fichier commGet[1].
// - Envoie "Start", envoie le contenu si trouvé, puis attend la confirmation client.
func Getserver(conn net.Conn, commGet []string, writer *bufio.Writer, reader *bufio.Reader) bool {
	var fichiers, err = os.ReadDir(commGet[2])
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return false
	}
	var found = false

	for _, fichier := range fichiers {
		if commGet[1] == fichier.Name() && !strings.HasPrefix(fichier.Name(), ".") && !fichier.IsDir() {
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var path = filepath.Join(commGet[2], fichier.Name())

			if err := p.Send_message(conn, writer, "Start"); err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'Start':", err)
				}
				return false
			}

			var data, err = os.ReadFile(path)
			if err != nil {
				log.Println("Ne peut pas lire le contenu du fichier :", err)
				return false
			}

			err = p.Send_message(conn, writer, string(data))
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors du transfert du fichier:", err)
				}
				return false
			}
			break
		}
	}

	if !found {
		log.Println("Fichier non trouvé:", commGet[1])
		if err := p.Send_message(conn, writer, "FileUnknown"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'FileUnknown':", err)
			}
			return false
		}
	}

	// Attendre la confirmation du client après le transfert/erreur.
	var response, err2 = p.Receive_message(conn, reader)
	if err2 != nil {
		var netErr net.Error
		if errors.As(err2, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la confirmation GET:", err2)
		}
		return false
	}
	log.Println("Réponse du client:", response)
	return true
}
