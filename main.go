package main

import (
	"errors"
	"log"
	"net"
	"strings"
)

type message struct {
	clientDomain string
	smtpCommands map[string]string
	atmHeaders   map[string]string
	body         string
	from         string
	date         string
	subject      string
	to           string
}

type connection struct {
	conn net.Conn
	id   int
	buf  []byte
}

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:25")
	if err != nil {
		panic(err)
	}
	defer l.Close()

	log.Println("Listening")

	id := 0
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		id += 1
		c := connection{conn, id, nil}
		go c.handle()
	}
}

// TODO

func (c *connection) handle() {
	defer c.conn.Close()
	log.Println("Connection accepted")

	err := c.writeLine("220")
	if err != nil {
		log.Panic(err)
	}

	log.Println("Awaiting EHLO")

	line, err := c.readLine()
	if err != nil {
		log.Panic(err)
	}

	if !strings.HasPrefix(line, "EHLO") {
		log.Panic(errors.New("Expected EHLO got: " + line))
	}

	msg := message{
		smtpCommands: map[string]string{},
		atmHeaders:   map[string]string{},
	}
	msg.clientDomain = line[len("EHLO "):]

	log.Println("Received EHLO")

	err = c.writeLine("250 ")
	if err != nil {
		log.Panic(err)
	}

	log.Println("Done EHLO")
	for line != "" {
		line, err = c.readLine()
		if err != nil {
			log.Panic(err)
		}

		pieces := strings.SplitN(line, ":", 2)
		smtpCommand := strings.ToUpper(pieces[0])

		// Special command without a value
		if smtpCommand == "DATA" {
			err = c.writeLine("354")
			if err != nil {
				log.Panic(err)
			}
			break
		}

		smtpValue := pieces[1]
		msg.smtpCommands[smtpCommand] = smtpValue

		log.Println("Got command: " + line)

		err = c.writeLine("250 OK")
		if err != nil {
			log.Panic(err)
		}
	}

	//         Done SMTP headers, reading ARPA text message headers
	log.Println("Done SMTP commands, reading ARPA text message headers")

	for {
		line, err = c.readMultiLine()
		if err != nil {
			log.Panic(err)
		}

		if strings.TrimSpace(line) == "" {
			break
		}

		pieces := strings.SplitN(line, ": ", 2)
		atmHeader := strings.ToUpper(pieces[0])
		atmValue := pieces[1]
		msg.atmHeaders[atmHeader] = atmValue

		if atmHeader == "SUBJECT" {
			msg.subject = atmValue
		}
		if atmHeader == "TO" {
			msg.to = atmValue
		}
		if atmHeader == "FROM" {
			msg.from = atmValue
		}
		if atmHeader == "DATE" {
			msg.date = atmValue
		}
	}

	log.Println("Done ARPA text message headers, reading body")

	msg.body, err = c.readToEndOfBody()
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Got body (%d bytes)", len(msg.body))

	err = c.writeLine("250 OK")
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Message:\n%s\n", msg.body)

	log.Println("Connection closed")
}

func (c *connection) writeLine(msg string) error {
	msg += "\r\n"
	for len(msg) > 0 {
		n, err := c.conn.Write([]byte(msg))
		if err != nil {
			return err
		}

		msg = msg[n:]
	}

	return nil
}

func (c *connection) readLine() (string, error) {
	for {
		b := make([]byte, 1024)
		n, err := c.conn.Read(b)
		if err != nil {
			return "", err
		}

		c.buf = append(c.buf, b[:n]...)
		for i, b := range c.buf {
			// If end of line
			if b == '\n' && i > 0 && c.buf[i-1] == '\r' {
				// i-1 because drop the CRLF, no one cares after this
				line := string(c.buf[:i-1])
				c.buf = c.buf[i+1:]
				return line, nil
			}
		}
	}
}

func (c *connection) readMultiLine() (string, error) {
	for {
		noMoreReads := false
		for i, b := range c.buf {
			if i > 1 &&
				b != ' ' &&
				b != '\t' &&
				c.buf[i-2] == '\r' &&
				c.buf[i-1] == '\n' {
				// i-2 because drop the CRLF, no one cares after this
				line := string(c.buf[:i-2])
				c.buf = c.buf[i:]
				return line, nil
			}

			noMoreReads = c.isBodyClose(i)
		}

		if !noMoreReads {
			b := make([]byte, 1024)
			n, err := c.conn.Read(b)
			if err != nil {
				return "", err
			}

			c.buf = append(c.buf, b[:n]...)

			// If this gets here more than once it's going to be an infinite loop
		}
	}
}

func (c *connection) isBodyClose(i int) bool {
	return i > 4 &&
		c.buf[i-4] == '\r' &&
		c.buf[i-3] == '\n' &&
		c.buf[i-2] == '.' &&
		c.buf[i-1] == '\r' &&
		c.buf[i-0] == '\n'
}

func (c *connection) readToEndOfBody() (string, error) {
	for {
		for i := range c.buf {
			if c.isBodyClose(i) {
				return string(c.buf[:i-4]), nil
			}
		}

		b := make([]byte, 1024)
		n, err := c.conn.Read(b)
		if err != nil {
			return "", err
		}

		c.buf = append(c.buf, b[:n]...)
	}
}
