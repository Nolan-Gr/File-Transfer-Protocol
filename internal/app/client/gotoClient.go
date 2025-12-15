package client

import (
	"bufio"
	"errors"
	"log"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// GOTOClient demande au serveur de changer de dossier et met à jour la position locale.
// Retourne :
// - Le nom du nouveau chemin (si Start ou back réussi)
// - "NO!" (si navigation impossible)
// - "" (si erreur réseau critique)
// split : [ "GOTO", "<target>", "<posActuelle>" ]
func GOTOClient(conn net.Conn, posActuelle string, split []string, writer *bufio.Writer, reader *bufio.Reader) string {
	if err := p.Send_message(conn, writer, "GOTO "+split[1]+" "+split[2]); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de l'envoi de la commande GOTO:", err)
		}
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return "" // ERREUR RÉSEAU CRITIQUE
	}

	var response, err = p.Receive_message(conn, reader)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			log.Println("Timeout lors de la réception de la réponse GOTO:", err)
		}
		log.Println("Erreur lors de la réception de la réponse:", err)
		return "" // ERREUR RÉSEAU CRITIQUE
	}
	response = strings.TrimSpace(response)
	log.Println(response)

	// Interprétation des réponses serveur :
	if response == "Start" {
		// Succès de descente. Le nouveau chemin est posActuelle/split[1]
		return split[1]
	} else if response == "back" {
		// Succès de remontée. Calculer et retourner le nouveau chemin
		var index = ParcourPath(split[2])
		if index == -1 {
			log.Println("Déjà à la racine, impossible de remonter")
			return split[2] // on reste au même endroit
		}
		return split[2][0:index]
	} else { // Inclut NO! et toute autre réponse inattendue
		log.Println("Navigation impossible ou réponse inattendue:", response)
		return "NO!" // ÉCHEC DE NAVIGATION NON CRITIQUE
	}
}

// ParcourPath retourne l'index de la dernière barre '/' dans split[2].
// Utilisé pour remonter d'un niveau dans le chemin local.
// Remarque : si il n'y a pas de '/', la fonction plantera (index out of range).
func ParcourPath(split string) int {
	var posTab []int
	for i, pos := range split {
		if pos == '/' {
			// on enregistre les positions où il y a une barre
			posTab = append(posTab, i)
		}
	}
	// On renvoie la position de la dernière barre trouvée
	if len(posTab) == 0 {
		return -1
	}
	log.Println(posTab)
	return posTab[len(posTab)-1]
}
