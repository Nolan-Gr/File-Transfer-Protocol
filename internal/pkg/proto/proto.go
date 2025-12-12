package proto

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

// Timeout par défaut (peut être modifié via flag)
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
	defer conn.SetWriteDeadline(time.Time{})

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
	defer conn.SetReadDeadline(time.Time{})

	message, err := in.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("erreur lecture message: %w", err)
	}

	return message, nil
}
