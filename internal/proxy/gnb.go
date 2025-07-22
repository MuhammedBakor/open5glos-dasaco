package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/free5gc/ngap/ngapType"
	"github.com/ishidawataru/sctp"
	"github.com/sirupsen/logrus"
)

type AmfRan struct {
	Conn net.Conn
	Name string
	Log  *logrus.Entry
}

func handleGNBConnection(ran *AmfRan) {
	defer func() {
		ran.Conn.Close()
		log.Printf("[INFO] gNB connection closed: %s", ran.Name)
	}()

	buf := make([]byte, 4096)
	for {
		ran.Conn.SetReadDeadline(time.Now().Add(300 * time.Second)) // 5 minute timeout
		n, err := ran.Conn.Read(buf)
		if err != nil {
			log.Printf("[ERROR] gNB read error: %v", err)
			return
		}

		log.Printf("[INFO] Received %d bytes from gNB %s", n, ran.Name)

		// Decode NGAP message
		ngapMsg, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[ERROR] NGAP decode error from gNB: %v", err)
			continue
		}

		// Handle message or forward to AMF
		handleGNBMessage(ran, ngapMsg, buf[:n])
	}

}

func handleGNBMessage(ran *AmfRan, ngapMsg *ngapType.NGAPPDU, rawMsg []byte) {
	switch ngapMsg.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := ngapMsg.InitiatingMessage
		if initiatingMessage == nil {
			return
		}

		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			// Handle NG Setup locally and respond
			handleNGSetupFromGNB(ran, initiatingMessage)
		default:
			// Forward other messages to AMF
			forwardToAMF(rawMsg)
		}
	default:
		// Forward other message types to AMF
		forwardToAMF(rawMsg)
	}
}

func handleNGSetupFromGNB(ran *AmfRan, initiatingMessage *ngapType.InitiatingMessage) {
	ran.Log.Infoln("[INFO] Handling NGSetupRequest from gNB...")

	// Build and send response
	pkt, err := BuildNGSetupResponse()
	if err != nil {
		ran.Log.Errorf("[ERROR] Build NGSetupResponse failed: %v", err)
		return
	}

	SendToRan(ran, pkt)
	ran.Log.Infoln("[INFO] Sent NGSetup Response to gNB")
}

func forwardToAMF(message []byte) {
	// Implementation for forwarding messages to AMF
}