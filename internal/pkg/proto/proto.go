package proto

import (
	"bufio"
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
	return nil
}

func Receive_message(in *bufio.Reader) (string, error) {
	message, err := in.ReadString('\n')
	if err != nil {
		return "", err
	}

	return message, nil
}
