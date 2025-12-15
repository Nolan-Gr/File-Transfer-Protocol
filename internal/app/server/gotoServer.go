package server

import (
	"bufio"
	"errors"
	"log"
	"net"
	"os"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// GOTO : navigue vers un dossier donné
// Retourne true si l'échange de protocole a réussi (y compris l'envoi de NO!), false si erreur réseau critique.
func GOTO(commGoto []string, conn net.Conn, writer *bufio.Writer) bool {
	target := commGoto[1]
	currentPath := commGoto[2]

	// 1. Remonter d'un niveau (..)
	if target == ".." {
		if err := p.Send_message(conn, writer, "back"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'back':", err)
			}
			log.Println("Erreur lors de l'envoi de 'back':", err)
			return false // Erreur réseau critique
		}
		return true
	}

	// 2. Descendre dans un sous-dossier (target)

	// Vérifier l'existence du dossier cible dans le chemin actuel
	fichiers, err := os.ReadDir(currentPath)
	if err != nil {
		log.Println("Erreur lecture dossier courant:", err)
		// Erreur locale : on informe le client via NO!
		if err := p.Send_message(conn, writer, "NO!"); err != nil {
			log.Println("Erreur lors de l'envoi de 'NO!' après échec lecture dossier:", err)
			return false // Erreur réseau critique
		}
		return true
	}

	var foundDir = false
	for _, fichier := range fichiers {
		if fichier.Name() == target && fichier.IsDir() {
			foundDir = true
			break
		}
	}

	if foundDir {
		// Dossier trouvé : envoi de "Start" (pour que le client mette à jour sa position)
		if err := p.Send_message(conn, writer, "Start"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'Start':", err)
			}
			log.Println("Erreur lors de l'envoi de 'Start':", err)
			return false // Erreur réseau critique
		}
	} else {
		// Dossier non trouvé ou est un fichier : envoi de "NO!"
		if err := p.Send_message(conn, writer, "NO!"); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("Timeout lors de l'envoi de 'NO!':", err)
			}
			log.Println("Erreur lors de l'envoi de 'NO!':", err)
			return false // Erreur réseau critique
		}
	}

	return true
}
