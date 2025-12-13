package server

import (
	"bufio"
	"fmt"
	"log"
	"log/slog"
	"net"
	"sync"
	"time"

	p "gitlab.univ-nantes.fr/iutna.info2.r305/proj/internal/pkg/proto"
)

// connectiontime : moment où le serveur normal a commencé à écouter.
// Utilisé pour calculer l'uptime dans DebugServer.
var connectiontime time.Time

// WaitGroup pour attendre la fin des deux serveurs (normal + contrôle)
// lors d'un RunServer.
var serverWg sync.WaitGroup

// RunServer lance deux listeners concurrents : un server "normal" et un server "control".
// On utilise un WaitGroup pour attendre leur terminaison.
func RunServer(port *string, controlPort *string) {

	serverWg.Add(2)
	go runNormalServer(port)
	go runControlServer(controlPort)

	// Attendre que les deux serveurs se terminent (appelé après shutdown).
	serverWg.Wait()
	log.Println("Tous les serveurs sont arrêtés")
}

// Listener principal pour les clients "normaux".
func runNormalServer(port *string) {
	defer serverWg.Done()

	l, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		slog.Error(err.Error())
		return
	}
	// enregistrer le temps de début pour l'uptime
	connectiontime = time.Now()

	// On ferme le listener à la sortie de la fonction.
	defer func() {
		err := l.Close()
		if err != nil {
			return
		}
		slog.Debug("Stopped listening on port " + *port)
	}()
	slog.Debug("Now listening on port " + *port)

	// Goroutine qui ferme le listener lorsque shutdownChan est fermé.
	// Cela permet à l'Accept() bloquant de sortir avec une erreur contrôlable.
	go func() {
		<-shutdownChan
		err := l.Close()
		if err != nil {
			return
		}
	}()

	for {
		c, err := l.Accept()
		if err != nil {
			// Si on est en cours d'arrêt, on termine proprement la boucle.
			if isServerShuttingDown() {
				slog.Info("Server normal terminé, arrêt des nouvelles connexions")
				return
			}
			// Erreur non liée au shutdown : on log et on continue.
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String() + " on port " + *port)
		go HandleClient(c)
	}
}

// Listener pour le port de contrôle (commande TERMINATE, HIDE/REVEAL, etc.).
func runControlServer(controlPort *string) {
	defer serverWg.Done()

	l, err := net.Listen("tcp", ":"+*controlPort)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	defer func() {
		err := l.Close()
		if err != nil {
			return
		}
		slog.Debug("Stopped listening on port " + *controlPort)
	}()
	slog.Debug("Now listening on port " + *controlPort)

	// Même mécanisme de fermeture via shutdownChan.
	go func() {
		<-shutdownChan
		err := l.Close()
		if err != nil {
			return
		}
	}()

	for {
		c, err := l.Accept()
		if err != nil {
			if isServerShuttingDown() {
				slog.Info("Server de contrôle terminé, arrêt des nouvelles connexions")
				return
			}
			slog.Error(err.Error())
			continue
		}
		slog.Info("Incoming connection from " + c.RemoteAddr().String())
		go HandleControlClient(c)
	}
}

// ClientLogOut : décrémente le compteur client et ferme la connexion.
// Appelée en defer à la sortie des handlers.
func ClientLogOut(conn net.Conn) {
	taille := decrementerClient()
	log.Println("nombre de client : ", taille)

	log.Println("adresse IP du client : ", conn.RemoteAddr().String(), " déconnecté le : ", time.Now())

	err := conn.Close()
	if err != nil {
		return
	}
}

// DebugServer : envoie des informations de debug au client.
// Utilise getCompteurClient/getCompteurOperations et connectiontime pour l'uptime.
// Ici p.Send_message est appelé avec nil pour le net.Conn car seule la writer est utilisée.
func DebugServer(writer *bufio.Writer) bool {
	msg := fmt.Sprintf("DebugInfo: clients=%d, operations=%d, uptime=%s",
		getCompteurClient(),
		getCompteurOperations(),
		time.Since(connectiontime).Truncate(time.Second).String())

	if err := p.Send_message(nil, writer, msg); err != nil {
		log.Println("Erreur lors de l'envoi du message de debug:", err)
		return false
	}
	return true
}
