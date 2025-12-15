package server

import (
	"bufio"
	"log"
	"net"
	"os"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// GOTO : navigue vers un dossier donné
func GOTO(commGoto []string, conn net.Conn, writer *bufio.Writer) {
	if commGoto[1] == ".." {
		if err := p.Send_message(conn, writer, "back"); err != nil {
			log.Println("Erreur lors de l'envoi de 'Start':", err)
			return
		}
	} else if commGoto[2] != commGoto[1] {
		var fichiers, err = os.ReadDir(commGoto[2])
		if err != nil {
			log.Println(err)
		}
		for _, fichier := range fichiers {
			if fichier.Name() == commGoto[1] && fichier.IsDir() {
				if err := p.Send_message(conn, writer, "Start"); err != nil {
					log.Println("Erreur lors de l'envoi de 'Start':", err)
					return
				}
			}
		}
	} else {
		if err := p.Send_message(conn, writer, "NO!"); err != nil {
			log.Println("Erreur lors de l'envoi de 'Start':", err)
			return
		}
	}
}
