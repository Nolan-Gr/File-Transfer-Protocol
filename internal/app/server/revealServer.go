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

// REVEAL : retire le prefixe '.' pour rendre visible le fichier.
func REVEAL(conn net.Conn, commHideReveal []string, writer *bufio.Writer) bool {
	var fichiers, err = os.ReadDir(commHideReveal[2]) // recupere le repertoire
	if err != nil {
		log.Println("Erreur lecture dossier :", err)
		return false
	}
	var found = false

	for _, fichier := range fichiers { // trouver le fichier dans le repertoire
		if commHideReveal[1] == fichier.Name() { // fichier trouvé
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var oldPath = filepath.Join(commHideReveal[2], fichier.Name())
			var newPath = filepath.Join(commHideReveal[2], strings.TrimPrefix(fichier.Name(), ".")) // enleve le "." en tête du fichier

			err := os.Rename(oldPath, newPath)
			if err != nil {
				log.Println("Ne peut pas rename le fichier :", err)
				return false
			}
			log.Println("Le fichier a bien été REVEAL")

			if err := p.Send_message(conn, writer, "OK"); err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'OK' REVEAL:", err)
				}
				return false
			}
			break
		}
	}

	// gestion du fileUnknown
	if !found {
		log.Println("Fichier non trouvé:", commHideReveal[1])
		if err := p.Send_message(conn, writer, "FileUnknown"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'FileUnknown' REVEAL:", err)
			}
			return false
		}
	}

	return true
}
