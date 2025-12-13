package server

import (
	"bufio"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// ParcourFolder : fonction récursive utilisée par tree pour construire l'arborescence.
// Retourne la liste sous forme de chaîne et le nombre total d'éléments trouvés.
// Remarque : gère les erreurs en les loggant, et continue sur sous-dossiers problématiques.
func ParcourFolder(fichiers []os.DirEntry, list string, size int) (string, int) {
	for _, fichier := range fichiers {
		fileInfo, err := fichier.Info()
		if err != nil {
			log.Println("Erreur lors de la lecture du fichier:", err)
			return err.Error(), 0
		}
		size = size + 1
		if fichier.Name()[0] != '.' {
			if fichier.IsDir() {
				log.Println(filepath.Join("Docs/", fichier.Name()))
				var newfichiers, err = os.ReadDir(filepath.Join("Docs/", fichier.Name()))
				if err != nil {
					log.Println("Erreur lecture sous-dossier:", err)
					continue
				}
				var liste, newsize = ParcourFolder(newfichiers, list, size)
				size = size + newsize
				list = list + " --" + fichier.Name() + " " + strconv.FormatInt(fileInfo.Size(), 10) + " -- sous-dossier: " + " [" + liste + "]"
			} else {
				list = list + " --" + fichier.Name() + " " + strconv.FormatInt(fileInfo.Size(), 10)
			}
		}
	}
	return list, size
}

// tree : construit et envoie l'arbre complet du dossier "Docs".
// Protocole similaire à LIST : Start -> attendre OK -> envoyer la liste complète.
func tree(conn net.Conn, writer *bufio.Writer, reader *bufio.Reader) bool {
	log.Println("tree func")

	var fichiers, err = os.ReadDir("Docs")
	if err != nil {
		log.Println("Erreur lecture dossier Docs:", err)
		return false
	}

	var list = ""
	var size = 0

	if err := p.Send_message(conn, writer, "Start"); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de 'Start' (tree):", err)
		}
		return false
	}

	log.Println(fichiers)
	data, err := p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la confirmation 'OK' (tree):", err)
		}
		return false
	}
	log.Println("data:", data)

	if strings.TrimSpace(data) == "OK" {
		var templist, tempsize = ParcourFolder(fichiers, list, size)
		log.Println("list : ", tempsize, templist)
		list = list + templist
		size = tempsize
		var newlist = "FileCnt : " + strconv.Itoa(size) + list
		log.Println(newlist)

		if err := p.Send_message(conn, writer, newlist); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de la liste finale (tree):", err)
			}
			return false
		}
	} else {
		log.Println("Protocole tree échoué : Attendu 'OK', reçu:", strings.TrimSpace(data))
		return false
	}

	return true
}
