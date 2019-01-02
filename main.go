package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	maxUDPPacketSize = 65536
	testMessageText  = `Тестовый payload, который содержит довольно много текста, но не слишком много, потому что на самом деле не такие уж и длинные сообщения с телефона обычно посылают`
	newMessagesText  = `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.`
)

var (
	isServer bool
	isClient bool

	serverHost    string
	serverIP      string
	serverPort    uint
	serverUDPPort uint

	udpFirstLatencyMs  float64
	httpFirstLatencyMs float64
	udpMaxLatencyMs    float64
	httpMaxLatencyMs   float64

	numMessages int
)

func init() {
	flag.BoolVar(&isServer, "server", false, "Run server")
	flag.BoolVar(&isClient, "client", false, "Run client")
	flag.StringVar(&serverHost, "host", "vbambuke.ru", "HostName of the server")
	flag.StringVar(&serverIP, "ip", "82.202.228.34", "IP of the server")
	flag.UintVar(&serverPort, "port", 17468, "Server port")
	flag.UintVar(&serverUDPPort, "udp-port", 17468, "UDP Server port")
	flag.IntVar(&numMessages, "num", 100, "How many messages to send")
}

func main() {
	flag.Parse()

	if isServer {
		startServer()
	} else if isClient {
		startClient()
	} else {
		log.Fatalf("Must run either as a server or as a client")
	}
}

func startServer() {
	go startHTTPServer()
	startUDPServer()
}

func startUDPServer() {
	buf := make([]byte, maxUDPPacketSize)

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%d", serverPort))
	if err != nil {
		log.Fatalf("Could not resolve udp addr: %s", err.Error())
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Could not listen udp addr: %s", err.Error())
	}
	defer conn.Close()

	log.Printf("Listening 0.0.0.0:%d (UDP)", serverUDPPort)

	for {
		n, uaddr, err := conn.ReadFromUDP(buf)
		if err == syscall.EINVAL {
			log.Fatalf("Could not read from UDP socket: %s", err.Error())
		} else if err != nil {
			log.Printf("Could not read from UDP: %s", err.Error())
			continue
		}

		req := string(buf[0:n])
		parts := strings.SplitN(req, " ", 2)

		curTs := time.Now().UnixNano()
		clientTs, _ := strconv.Atoi(parts[0])

		log.Printf("Received a message from %s: %s (length %d, lag is %.1f ms)", uaddr, parts[0], len(req), float64(curTs-int64(clientTs))/1e6)

		sendUDPResponse(conn, uaddr, curTs, clientTs)
	}
}

func sendUDPResponse(conn *net.UDPConn, uaddr *net.UDPAddr, curTs int64, clientTs int) {
	conn.WriteToUDP([]byte(fmt.Sprintf("%d %d", curTs, clientTs)), uaddr)
}

func startHTTPServer() {
	http.HandleFunc("/send", func(rw http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		clientTs, _ := strconv.Atoi(r.PostForm.Get("ts"))
		curTs := time.Now().UnixNano()

		log.Printf("HTTP: Received a message ts=%d lag is %.1f ms", curTs, float64(curTs-int64(clientTs))/1e6)

		rw.WriteHeader(200)
		rw.Write([]byte(fmt.Sprintf("%d", curTs)))
	})

	http.HandleFunc("/get", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
		ts := r.PostForm.Get("ts")
		log.Printf("HTTP: received /get with ts %s", ts)
		rw.Write([]byte(ts + "\n"))
		rw.Write([]byte(newMessagesText))
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", serverPort), nil))
}

func startClient() {
	udpCh := make(chan time.Duration, 1)
	httpCh := make(chan time.Duration, 1)

	go func() {
		udpCh <- sendMessagesUDP()
	}()

	go func() {
		httpCh <- sendMessagesHTTP()
	}()

	u := <-udpCh
	h := <-httpCh

	log.Printf("UDP: %s (first latency %1.f ms, max latency %.1f ms)", u, udpFirstLatencyMs, udpMaxLatencyMs)
	log.Printf("HTTP: %s (first latency %1.f ms, max latency %.1f ms)", h, httpFirstLatencyMs, httpMaxLatencyMs)
}

func sendMessagesHTTP() time.Duration {
	startTs := time.Now()

	cl := &http.Client{
		Timeout: 15 * time.Second,
	}

	for i := 0; i < numMessages; i++ {
		tryHTTPSend(cl)
	}

	return time.Since(startTs)
}

func tryHTTPSend(cl *http.Client) {
	requestTs := time.Now().UnixNano()

	for {
		res, err := cl.PostForm(
			fmt.Sprintf("http://%s:%d/send", serverHost, serverPort),
			url.Values{
				"ts":   {fmt.Sprintf("%d", requestTs)},
				"text": {testMessageText},
			},
		)

		if err == nil {
			body, err := ioutil.ReadAll(res.Body)
			res.Body.Close()

			if err != nil {
				log.Printf("HTTP: could not read body: %s", err.Error())
				continue
			}

			curTs := time.Now().UnixNano()
			serverTs, _ := strconv.ParseInt(string(body), 10, 64)

			totalLag := float64(curTs-requestTs) / 1e6

			if totalLag > httpMaxLatencyMs {
				httpMaxLatencyMs = totalLag
			}
			if httpFirstLatencyMs == 0 {
				httpFirstLatencyMs = totalLag
			}

			log.Printf("HTTP: read response from a server: total lag %.1f ms, server lag %.1f ms", totalLag, float64(serverTs-requestTs)/1e6)

			return
		}

		log.Printf("HTTP: could not send message: %s", err.Error())
	}
}

func sendMessagesUDP() time.Duration {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", serverIP, serverPort))
	if err != nil {
		log.Fatalf("Could not resolve udp addr: %s", err.Error())
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatalf("Could not dial UDP to server: %s", err.Error())
	}

	resCh := make(chan udpResult, 10)

	go readResp(conn, resCh)

	startTs := time.Now()

	for i := 0; i < numMessages; i++ {
		requestID := time.Now().UnixNano()
		tryUDPSend(conn, requestID, resCh)
	}

	return time.Since(startTs)
}

func tryUDPSend(conn *net.UDPConn, requestID int64, resCh chan udpResult) {
	for {
		_, err := conn.Write([]byte(fmt.Sprintf("%d %s", requestID, testMessageText)))
		if err != nil {
			log.Printf("UDP: Could not write to UDP: %s", err.Error())
		}

		if waitReply(requestID, time.After(time.Second), resCh) {
			return
		}
	}
}

func waitReply(requestID int64, timeout <-chan time.Time, resCh chan udpResult) (ok bool) {
	for {
		select {
		case res := <-resCh:
			if res.requestTs == requestID {
				curTs := time.Now().UnixNano()
				totalLag := float64(curTs-res.requestTs) / 1e6
				if totalLag > udpMaxLatencyMs {
					udpMaxLatencyMs = totalLag
				}
				if udpFirstLatencyMs == 0 {
					udpFirstLatencyMs = totalLag
				}

				log.Printf("UDP: read response from a server: total lag %.1f ms, server lag %.1f ms", totalLag, float64(res.serverTs-res.requestTs)/1e6)
				return true
			}
		case <-timeout:
			return false
		}
	}
}

type udpResult struct {
	serverTs  int64
	requestTs int64
}

func readResp(conn *net.UDPConn, resCh chan udpResult) {
	buf := make([]byte, maxUDPPacketSize)

	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("UDP: Could not read response from server: %s", err.Error())
			time.Sleep(time.Millisecond * 100)
		}

		_ = addr // TODO: you must check the source

		respStr := string(buf[0:n])
		parts := strings.SplitN(respStr, " ", 2)

		if len(parts) != 2 {
			log.Printf("UDP: Bad response: len(parts) = %d (expected 2)", len(parts))
			continue
		}

		var res udpResult

		res.serverTs, _ = strconv.ParseInt(parts[0], 10, 64)
		res.requestTs, _ = strconv.ParseInt(parts[1], 10, 64)

		resCh <- res
	}
}
