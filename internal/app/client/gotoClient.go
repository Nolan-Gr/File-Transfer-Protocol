package client

import (
	"bufio"
	"log"
	"net"
	"strings"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// GOTOClient demande au serveur de changer de dossier et met à jour la position locale.
// split : [ "GOTO", "<target>", "<posActuelle>" ]
func GOTOClient(conn net.Conn, posActuelle string, split []string, writer *bufio.Writer, reader *bufio.Reader) string {
	if err := p.Send_message(conn, writer, "GOTO "+split[1]+" "+split[2]); err != nil {
		log.Println("Erreur lors de l'envoi de la commande:", err)
		return ""
	}

	var response, err = p.Receive_message(conn, reader)
	if err != nil {
		log.Println("Erreur lors de la réception de la réponse:", err)
		return ""
	}
	response = strings.TrimSpace(response)

	// Interprétation des réponses serveur :
	// "Start" -> on peut aller dans le dossier demandé
	// "NO!" -> on est déjà dans ce dossier
	// "back" -> il faut remonter d'un niveau ; on calcule la nouvelle position à partir du chemin
	if response == "Start" {
		posActuelle = split[1]
	} else if response == "NO!" {
		log.Println("vous êtes déjà dans ce fichier")
	} else if response == "back" {
		// on cherche la dernière barre '/' dans le chemin et on garde la partie précédente
		var index = ParcourPath(split)
		// split[2][0:index] garde la partie du chemin avant la dernière barre
		posActuelle = split[2][0:index]
	}
	return posActuelle
}

// ParcourPath retourne l'index de la dernière barre '/' dans split[2].
// Utilisé pour remonter d'un niveau dans le chemin local.
// Remarque : si il n'y a pas de '/', la fonction plantera (index out of range).
func ParcourPath(split []string) int {
	var posTab []int
	for i, pos := range split[2] {
		if pos == '/' {
			// on enregistre les positions où il y a une barre
			posTab = append(posTab, i)
		}
	}
	// On renvoie la position de la dernière barre trouvée
	return posTab[len(posTab)-1]
}
