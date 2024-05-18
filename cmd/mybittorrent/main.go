package main

import (
	// Uncomment this line to pass the first stage
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	util "github.com/bittorrent-client/utils"
	// bencode "github.com/jackpal/bencode-go" // Available if you need it!
)

const PEER_ID = "00112233445566778899"

type Torrent struct {
	trackerUrl    string
	contentLength int
	infoHash      string
	peiceLength   int
	peices        []string
	peers         []string
	conn          net.Conn
	peerId        string
}

func (torrent *Torrent) Read(decoded interface{}) {
	if mappedData, ok := decoded.(map[string]interface{}); ok {
		if tUrl, ok := mappedData["announce"].(string); ok {
			torrent.trackerUrl = tUrl
		}
		if info, ok := mappedData["info"].(map[string]interface{}); ok {
			if cl, ok := info["length"].(int); ok {
				torrent.contentLength = cl
			}
			encoded, err := util.EncodeBencode(info)
			if err != nil {
				fmt.Println(err)
				return
			}
			hasher := sha1.New()
			hasher.Write([]byte(encoded))
			torrent.infoHash = string(hasher.Sum(nil))
			if pl, ok := info["piece length"].(int); ok {
				torrent.peiceLength = pl
			}
			if piecesRaw, ok := info["pieces"].(string); ok {
				piecesCount := len(piecesRaw) / 20
				for i := 0; i < piecesCount; i++ {
					torrent.peices = append(torrent.peices, hex.EncodeToString([]byte(piecesRaw[i*20:i*20+20])))
				}
			}
		}
	}
}
func (torrent *Torrent) FindPeers() {
	queryParams := url.Values{}
	queryParams.Add("info_hash", string(torrent.infoHash))
	queryParams.Add("peer_id", PEER_ID)
	queryParams.Add("port", "6881")
	queryParams.Add("uploaded", "0")
	queryParams.Add("downloaded", "0")
	queryParams.Add("left", strconv.Itoa(torrent.contentLength))
	queryParams.Add("compact", "1")
	u, err := url.Parse(torrent.trackerUrl)
	if err != nil {
		fmt.Println("Error parsing URL:", err)
		return
	}
	u.RawQuery = queryParams.Encode()
	resp, err := http.Get(u.String())
	if err != nil {
		fmt.Println("Error making GET request:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Println("Error: Non-OK HTTP status:", resp.StatusCode)
		return
	}
	// Read and parse the response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}
	response, _, _ := util.DecodeBencode(string(bodyBytes), 0)
	if responseData, ok := response.(map[string]interface{}); ok {
		if peers, ok := responseData["peers"].(string); ok {
			bytes, err := hex.DecodeString(fmt.Sprintf("%x", peers))
			if err != nil {
				fmt.Println("Error decoding hex string:", err)
				return
			}
			// Iterate through each peer (6 bytes per peer)
			for i := 0; i < len(bytes); i += 6 {
				if i+6 <= len(bytes) {
					ip := net.IPv4(bytes[i], bytes[i+1], bytes[i+2], bytes[i+3])
					port := binary.BigEndian.Uint16(bytes[i+4 : i+6])
					torrent.peers = append(torrent.peers, fmt.Sprintf("%s:%d", ip, port))
				}
			}
		}
	}
}
func TorrrentDecodeFile(filepath string) interface{} {
	content, err := os.ReadFile(filepath)
	if err != nil {
		log.Fatal(err)
	}
	decoded, _, err := util.DecodeBencode(string(content), 0)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return decoded
}
func (torrent *Torrent) PeerSent(messageId int, request []byte) error {
	buf := bytes.NewBuffer([]byte{})
	binary.Write(buf, binary.BigEndian, int32(1))
	binary.Write(buf, binary.BigEndian, int8(messageId))
	if request != nil {
		buf.Write(request)
	}
	binary.BigEndian.PutUint32(buf.Bytes(), uint32(buf.Len()-4))
	_, err := torrent.conn.Write(buf.Bytes())
	if err != nil {
		fmt.Println("Failed to send peer message:", err)
		return err
	}
	return nil
}
func (torrent *Torrent) PeerReceive() (int, []byte, int32, error) {
	responseLen := make([]byte, 4)
	_, err := torrent.conn.Read(responseLen)
	// fmt.Printf("%x\n", responseLen)
	if err != nil {
		fmt.Println("Error reading peer length:", err)
		os.Exit(1)
		return 0, nil, 0, err
	}
	byteReader := bytes.NewReader(responseLen)
	var length int32
	binary.Read(byteReader, binary.BigEndian, &length)
	response := make([]byte, length)
	_, err = torrent.conn.Read(response)
	if err != nil {
		fmt.Println("Error reading peer data", err)
		os.Exit(1)
		return 0, nil, 0, err
	}
	code := int8(response[0])
	payload := response[1:length]
	return int(code), payload, length, err
}
func (torrent *Torrent) PeerMessage(messageId int, request []byte) (int, []byte, int32, error) {
	torrent.PeerSent(messageId, request)
	return torrent.PeerReceive()
}
func (torrent *Torrent) Connect() error {
	var err error
	torrent.conn, err = net.Dial("tcp", torrent.peers[0])
	return err
}
func (torrent *Torrent) DownloadPiece(peice_no int, outfilePath string) []byte {
	// torrent.Handshake()
	torrent.PeerReceive()
	torrent.PeerSent(INTERESTED, nil)
	torrent.PeerReceive()
	blockSize := 16 * 1024
	var pieceLength int
	if peice_no == len(torrent.peices)-1 {
		pieceLength = torrent.contentLength % torrent.peiceLength
	} else {
		pieceLength = torrent.peiceLength
	}
	pieceData := make([]byte, 0, pieceLength)
	for offset := 0; offset < pieceLength; offset += blockSize {
		blockSize := minInt(blockSize, pieceLength-offset)
		buf := bytes.NewBuffer([]byte{})
		binary.Write(buf, binary.BigEndian, int32(peice_no))
		binary.Write(buf, binary.BigEndian, int32(offset))
		binary.Write(buf, binary.BigEndian, int32(blockSize))
		payload := buf.Bytes()
		torrent.PeerSent(REQUEST, payload)
		response := make([]byte, blockSize+13)
		_, err := io.ReadFull(torrent.conn, response)
		if err != nil {
			fmt.Println("full", err)
		}
		respReader := bytes.NewReader(response)
		var messageLen, peiceIndex, begin int32
		var messageId int8
		binary.Read(respReader, binary.BigEndian, &messageLen)
		binary.Read(respReader, binary.BigEndian, &messageId)
		binary.Read(respReader, binary.BigEndian, &peiceIndex)
		binary.Read(respReader, binary.BigEndian, &begin)
		pieceData = append(pieceData, response[13:]...)
	}
	return pieceData
}
func (torrent *Torrent) Handshake() error {
	var err error
	message := make([]byte, 0)
	message = append(message, byte(19))
	message = append(message, []byte("BitTorrent protocol")...)
	message = append(message, []byte{0, 0, 0, 0, 0, 0, 0, 0}...)
	message = append(message, []byte(torrent.infoHash)...)
	message = append(message, []byte(PEER_ID)...)
	_, err = torrent.conn.Write(message)
	if err != nil {
		fmt.Println("Failed to send handshake:", err)
		return err
	}
	response := make([]byte, 68)
	_, err = torrent.conn.Read(response)
	if err != nil {
		fmt.Println("Error reading:", err)
		os.Exit(1)
		return err
	}
	torrent.peerId = string(response[48:])
	return nil
}
func (torrent *Torrent) Close() {
	torrent.conn.Close()
}

const (
	CHOKE          = 0
	UNCHOKE        = 1
	INTERESTED     = 2
	NOT_INTERESTED = 3
	HAVE           = 4
	BITFIELD       = 5
	REQUEST        = 6
	PIECE          = 7
	CANCEL         = 8
)

func minInt(a, b int) int {
	if a > b {
		return b
	}
	return a
}
func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	// fmt.Println("Logs from your program will appear here!")
	command := os.Args[1]
	switch command {
	case "decode":
		bencodedValue := os.Args[2]
		decoded, _, err := util.DecodeBencode(bencodedValue, 0)
		if err != nil {
			fmt.Println(err)
			return
		}
		jsonOutput, _ := json.Marshal(decoded)
		fmt.Println(string(jsonOutput))
	case "info":
		filepath := os.Args[2]
		decoded := TorrrentDecodeFile(filepath)
		var torrent Torrent
		torrent.Read(decoded)
		fmt.Println("Tracker URL:", torrent.trackerUrl)
		fmt.Println("Length:", torrent.contentLength)
		fmt.Printf("Info Hash: %x\n", torrent.infoHash)
		fmt.Printf("Piece Length: %d\n", torrent.peiceLength)
		fmt.Println("Piece Hashes:")
		for _, peice := range torrent.peices {
			fmt.Printf("%s\n", peice)
		}
	case "peers":
		filepath := os.Args[2]
		decoded := TorrrentDecodeFile(filepath)
		var torrent Torrent
		torrent.Read(decoded)
		torrent.FindPeers()
		for _, peer := range torrent.peers {
			fmt.Println(peer)
		}
	case "handshake":
		filepath := os.Args[2]
		// peerAddress := os.Args[3]
		decoded := TorrrentDecodeFile(filepath)
		var torrent Torrent
		torrent.Read(decoded)
		torrent.FindPeers()
		torrent.Connect()
		defer torrent.Close()
		torrent.Handshake()
		fmt.Printf("Peer ID: %x\n", torrent.peerId)
	case "download_piece":
		outfilePath := os.Args[3]
		torrentFile := os.Args[4]
		peice_no, _ := strconv.Atoi(os.Args[5])
		// peerAddress := os.Args[3]
		decoded := TorrrentDecodeFile(torrentFile)
		var torrent Torrent
		torrent.Read(decoded)
		torrent.FindPeers()
		torrent.Connect()

		torrent.Handshake()
		pieceData := torrent.DownloadPiece(peice_no, outfilePath)
		torrent.Close()
		dirPath := filepath.Dir(outfilePath)
		os.MkdirAll(dirPath, 0755)
		err := os.WriteFile(outfilePath, pieceData, 0644)
		if err != nil {
			log.Fatalf("Failed to write to file: %v", err)
		}
		fmt.Printf("Piece 0 downloaded to %s\n", outfilePath)

	case "download":
		outfilePath := os.Args[3]
		torrentFile := os.Args[4]
		decoded := TorrrentDecodeFile(torrentFile)
		var torrent Torrent
		torrent.Read(decoded)
		torrent.FindPeers()
		content := make([]byte, 0)
		for i := 0; i < len(torrent.peices); i++ {
			torrent.Connect()
			torrent.Handshake()
			pieceData := torrent.DownloadPiece(i, outfilePath)
			content = append(content, pieceData...)
			torrent.Close()
		}

		fmt.Printf("Downloaded %s to %s\n", torrentFile, outfilePath)
		dirPath := filepath.Dir(outfilePath)
		os.MkdirAll(dirPath, 0755)
		err := os.WriteFile(outfilePath, content, 0644)
		if err != nil {
			log.Fatalf("Failed to write to file: %v", err)
		}
		fmt.Printf("Downloaded %s to %s\n", torrentFile, outfilePath)
	default:
		fmt.Println("Unknown command: " + command)
		os.Exit(1)
	}

}
