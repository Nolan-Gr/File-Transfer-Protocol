package proto

import (
	"bufio"
	"log"
	"strings"
)

func Send_message(out *bufio.Writer, message string) error {
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}

	_, err := out.WriteString(message)
	if err != nil {
		return err
	}
	err = out.Flush()
	if err != nil {
		return err
	}
	log.Println("Envoi du message :", message[:len(message)-1])
	return nil
}

func Receive_message(in *bufio.Reader) (string, error) {
	message, err := in.ReadString('\n')
	if err != nil {
		return "", err
	}
	log.Println("Réception du message :", message[:len(message)-1])

	return message, nil
}
