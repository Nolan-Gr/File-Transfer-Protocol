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

// HIDE : renomme le fichier en le préfixant par '.' pour le cacher.
// Envoie OK si succès, FileUnknown si fichier non trouvé.
func HIDE(conn net.Conn, commHideReveal []string, writer *bufio.Writer) bool {
	var fichiers, err = os.ReadDir(commHideReveal[2]) // recupere le repertoire
	if err != nil {
		log.Println("Erreur lecture dossier:", err)
		return false
	}
	var found = false

	for _, fichier := range fichiers { // trouver le fichier dans le repertoire
		if commHideReveal[1] == fichier.Name() && !strings.HasPrefix(fichier.Name(), ".") { // fichier trouvé
			found = true
			log.Println("Fichier trouvé:", fichier.Name())
			var oldPath = filepath.Join(commHideReveal[2], fichier.Name())
			var newPath = filepath.Join(commHideReveal[2], "."+fichier.Name()) // ajoute un "." devant le fichier

			err := os.Rename(oldPath, newPath)
			if err != nil {
				log.Println("Ne peut pas rename le fichier :", err)
				return false
			}
			log.Println("Le fichier a bien été HIDE")

			if err := p.Send_message(conn, writer, "OK"); err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Println("Timeout lors de l'envoi de 'OK' HIDE:", err)
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
				log.Println("Timeout lors de l'envoi de 'FileUnknown' HIDE:", err)
			}
			return false
		}
	}

	return true
}
