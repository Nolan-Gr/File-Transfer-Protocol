package proto

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// MessageTimeout=Timeout par défaut
var MessageTimeout = 20 * time.Second

// --- GESTION DE L'HISTORIQUE DES MESSAGES ---
var (
	// messageHistory est un canal
	messageHistory = make(chan string, 1000)
	// historyCache est la liste des messages qu'on prends pour l'affichage.
	historyCache = []string{"Historique des messages : \n"}
)

// on démarre une goroutine pour lire le canal et construire le cache.
// On ne loggue que si le mode debug est actif
func init() {
	go func() {
		for msg := range messageHistory {
			historyCache = append(historyCache, msg)
		}
	}()
}

// LogMessage ajoute un message formaté dans le canal d'historique.
func LogMessage(direction string, msg string) {
	select {
	case messageHistory <- fmt.Sprintf("%s message: %s \n", direction, strings.TrimSpace(msg)):
	default:
		log.Println("Avertissement: Le buffer d'historique de messages est plein.")
	}
}

// GetHistorique retourne l'historique des messages.
func GetHistorique() []string {
	return historyCache
}

// --- GESTION DEs ECHANGES DE MESSAGES ---
func Send_message(conn net.Conn, out *bufio.Writer, message string) error {
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}

	// Log message envoyé
	LogMessage("sent", message)

	// Définir une deadline pour l'opération d'écriture
	if err := conn.SetWriteDeadline(time.Now().Add(MessageTimeout)); err != nil {
		return fmt.Errorf("erreur définition deadline écriture: %w", err)
	}

	// Réinitialiser la deadline après l'opération
	defer func(conn net.Conn, t time.Time) {
		err := conn.SetWriteDeadline(t)
		if err != nil {
			log.Println("Erreur SetWriteDeadline defer:", err)
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

func Receive_message(conn net.Conn, in *bufio.Reader) (string, error) {
	// Définir une deadline pour l'opération de lecture
	if err := conn.SetReadDeadline(time.Now().Add(MessageTimeout)); err != nil {
		return "", fmt.Errorf("erreur définition deadline lecture: %w", err)
	}

	// Réinitialiser la deadline après l'opération
	defer func(conn net.Conn, t time.Time) {
		err := conn.SetReadDeadline(t)
		if err != nil {
			log.Println("Erreur SetReadDeadline defer:", err)
		}
	}(conn, time.Time{})

	message, err := in.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("erreur lecture message: %w", err)
	}

	// Log du message reçu
	LogMessage("received", message)

	return message, nil
}
