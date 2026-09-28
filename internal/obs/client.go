package obs

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var ErrNotConnected = errors.New("OBS não está conectado")

type Settings struct {
	Host     string
	Port     int
	Password string
}

type Status struct {
	Connected    bool   `json:"connected"`
	CurrentScene string `json:"currentScene"`
}

type Scene struct {
	Name string `json:"name"`
}

type requestResponse struct {
	RequestType   string `json:"requestType"`
	RequestID     string `json:"requestId"`
	RequestStatus struct {
		Result  bool   `json:"result"`
		Code    int    `json:"code"`
		Comment string `json:"comment"`
	} `json:"requestStatus"`
	ResponseData json.RawMessage `json:"responseData"`
}

type envelope struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

type Client struct {
	mu       sync.RWMutex
	conn     *websocket.Conn
	pending  map[string]chan requestResponse
	sequence atomic.Uint64
	closed   chan struct{}
}

func NewClient() *Client {
	return &Client{pending: make(map[string]chan requestResponse)}
}

func (c *Client) Connect(ctx context.Context, settings Settings) error {
	c.Disconnect()
	address := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", settings.Host, settings.Port), Path: "/"}
	dialer := websocket.Dialer{HandshakeTimeout: 4 * time.Second}
	conn, _, err := dialer.DialContext(ctx, address.String(), nil)
	if err != nil {
		return fmt.Errorf("conectar ao OBS: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		return err
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("ler saudação do OBS: %w", err)
	}
	var hello struct {
		Op int `json:"op"`
		D  struct {
			RPCVersion     int `json:"rpcVersion"`
			Authentication *struct {
				Challenge string `json:"challenge"`
				Salt      string `json:"salt"`
			} `json:"authentication"`
		} `json:"d"`
	}
	if err := json.Unmarshal(payload, &hello); err != nil || hello.Op != 0 {
		conn.Close()
		return errors.New("resposta de saudação inválida do OBS")
	}
	identify := map[string]any{"rpcVersion": 1}
	if hello.D.Authentication != nil {
		identify["authentication"] = authentication(settings.Password, hello.D.Authentication.Salt, hello.D.Authentication.Challenge)
	}
	if err := conn.WriteJSON(map[string]any{"op": 1, "d": identify}); err != nil {
		conn.Close()
		return fmt.Errorf("autenticar no OBS: %w", err)
	}
	_, payload, err = conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("confirmar autenticação do OBS: %w", err)
	}
	var identified envelope
	if err := json.Unmarshal(payload, &identified); err != nil || identified.Op != 2 {
		conn.Close()
		return errors.New("OBS recusou a autenticação")
	}
	_ = conn.SetReadDeadline(time.Time{})
	c.mu.Lock()
	c.conn = conn
	c.pending = make(map[string]chan requestResponse)
	c.closed = make(chan struct{})
	c.mu.Unlock()
	go c.readLoop(conn)
	return nil
}

func (c *Client) Disconnect() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	if c.closed != nil {
		select {
		case <-c.closed:
		default:
			close(c.closed)
		}
	}
	for id, channel := range c.pending {
		close(channel)
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (c *Client) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var data struct {
		CurrentProgramSceneName string `json:"currentProgramSceneName"`
	}
	if err := c.request(ctx, "GetCurrentProgramScene", nil, &data); err != nil {
		return Status{Connected: c.Connected()}, err
	}
	return Status{Connected: true, CurrentScene: data.CurrentProgramSceneName}, nil
}

func (c *Client) Scenes(ctx context.Context) ([]Scene, error) {
	var data struct {
		Scenes []struct {
			SceneName string `json:"sceneName"`
		} `json:"scenes"`
	}
	if err := c.request(ctx, "GetSceneList", nil, &data); err != nil {
		return nil, err
	}
	scenes := make([]Scene, 0, len(data.Scenes))
	for _, item := range data.Scenes {
		scenes = append(scenes, Scene{Name: item.SceneName})
	}
	return scenes, nil
}

func (c *Client) SetScene(ctx context.Context, name string) error {
	if name == "" {
		return errors.New("o nome da cena é obrigatório")
	}
	return c.request(ctx, "SetCurrentProgramScene", map[string]any{"sceneName": name}, nil)
}

func (c *Client) request(ctx context.Context, requestType string, data any, output any) error {
	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return ErrNotConnected
	}
	id := fmt.Sprintf("request-%d", c.sequence.Add(1))
	responseChannel := make(chan requestResponse, 1)
	c.pending[id] = responseChannel
	payload := map[string]any{"requestType": requestType, "requestId": id}
	if data != nil {
		payload["requestData"] = data
	}
	err := c.conn.WriteJSON(map[string]any{"op": 6, "d": payload})
	if err != nil {
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("enviar comando ao OBS: %w", err)
	}
	c.mu.Unlock()

	select {
	case response, ok := <-responseChannel:
		if !ok {
			return ErrNotConnected
		}
		if !response.RequestStatus.Result {
			return fmt.Errorf("OBS recusou %s (%d): %s", requestType, response.RequestStatus.Code, response.RequestStatus.Comment)
		}
		if output != nil && len(response.ResponseData) > 0 {
			if err := json.Unmarshal(response.ResponseData, output); err != nil {
				return fmt.Errorf("decodificar resposta do OBS: %w", err)
			}
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

func (c *Client) readLoop(conn *websocket.Conn) {
	defer c.disconnectIfCurrent(conn)
	for {
		var message envelope
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		if message.Op != 7 {
			continue
		}
		var response requestResponse
		if err := json.Unmarshal(message.D, &response); err != nil {
			continue
		}
		c.mu.Lock()
		channel := c.pending[response.RequestID]
		delete(c.pending, response.RequestID)
		c.mu.Unlock()
		if channel != nil {
			channel <- response
			close(channel)
		}
	}
}

func (c *Client) disconnectIfCurrent(conn *websocket.Conn) {
	c.mu.Lock()
	if c.conn != conn {
		c.mu.Unlock()
		return
	}
	c.conn = nil
	if c.closed != nil {
		select {
		case <-c.closed:
		default:
			close(c.closed)
		}
	}
	for id, channel := range c.pending {
		close(channel)
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

func authentication(password, salt, challenge string) string {
	secretHash := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(secretHash[:])
	authHash := sha256.Sum256([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(authHash[:])
}
