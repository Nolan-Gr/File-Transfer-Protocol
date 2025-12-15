package proto

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// MessageTimeout Timeout par défaut (peut être modifié via flag)
var MessageTimeout = 20 * time.Second

// Send_message envoie un message avec un timeout.
func Send_message(conn net.Conn, out *bufio.Writer, message string) error {
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}

	// Définir un deadline pour l'opération d'écriture
	if err := conn.SetWriteDeadline(time.Now().Add(MessageTimeout)); err != nil {
		return fmt.Errorf("erreur définition deadline écriture: %w", err)
	}

	// Réinitialiser le deadline après l'opération
	defer func(conn net.Conn, t time.Time) {
		err := conn.SetWriteDeadline(t)
		if err != nil {

		}
	}(conn, time.Time{})

	_, err := out.WriteString(message)
	if err != nil {
		return fmt.Errorf("erreur écriture message: %w", err)
	}

	err = out.Flush()
	if err != nil {
		return fmt.Errorf("erreur flush: %w", err)
	}

	return nil
}

// Receive_message lit un message avec un timeout.
func Receive_message(conn net.Conn, in *bufio.Reader) (string, error) {
	// Définir un deadline pour l'opération de lecture
	if err := conn.SetReadDeadline(time.Now().Add(MessageTimeout)); err != nil {
		return "", fmt.Errorf("erreur définition deadline lecture: %w", err)
	}

	// Réinitialiser le deadline après l'opération
	defer func(conn net.Conn, t time.Time) {
		err := conn.SetReadDeadline(t)
		if err != nil {

		}
	}(conn, time.Time{})

	message, err := in.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("erreur lecture message: %w", err)
	}

	return message, nil
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
