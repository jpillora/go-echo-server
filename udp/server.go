package udp

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"unicode"
)

//Start a udp server on port
func Start(port int) error {
	a := fmt.Sprintf("0.0.0.0:%d", port)
	addr, err := net.ResolveUDPAddr("udp", a)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	log.Printf("Listening for udp packets on %s...", a)
	defer conn.Close()
	defer log.Printf("Closed")
	buff := make([]byte, 9014)
	for {
		n, addr, err := conn.ReadFromUDP(buff)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("receive error: %s", err)
			continue
		}
		b := buff[:n]
		rs := []rune(string(b))
		for i, r := range rs {
			if unicode.IsPrint(r) {
				rs[i] = unicode.To(unicode.UpperCase, r)
			}
		}
		b = []byte(string(rs))
		n, err = conn.WriteTo(b, addr)
		if err != nil {
			log.Printf("send error: %s", err)
			continue
		}
		log.Printf("client %s: echo '%s' (%d bytes)", addr, strings.TrimSpace(string(b)), n)
	}
	return nil
}
