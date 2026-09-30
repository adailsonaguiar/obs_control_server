package ptz

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const DefaultPort = 52381

type Client struct {
	sequence atomic.Uint32
}

func NewClient() *Client { return &Client{} }

func (c *Client) Move(ctx context.Context, host string, port int, direction string, speed int) error {
	pan, tilt, err := directionBytes(direction)
	if err != nil {
		return err
	}
	speedByte, err := speedByte(speed)
	if err != nil {
		return err
	}
	return c.send(ctx, host, port, []byte{0x81, 0x01, 0x06, 0x01, speedByte, speedByte, pan, tilt, 0xff})
}

func (c *Client) Zoom(ctx context.Context, host string, port int, direction string, speed int) error {
	var value byte
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "in":
		value = 0x20
	case "out":
		value = 0x30
	case "stop":
		value = 0x00
	default:
		return errors.New("direção de zoom inválida")
	}
	if value != 0 {
		if speed < 1 || speed > 7 {
			return errors.New("a velocidade do zoom deve estar entre 1 e 7")
		}
		value |= byte(speed)
	}
	return c.send(ctx, host, port, []byte{0x81, 0x01, 0x04, 0x07, value, 0xff})
}

func (c *Client) Preset(ctx context.Context, host string, port int, action string, number int) error {
	if number < 0 || number > 255 {
		return errors.New("o número do preset deve estar entre 0 e 255")
	}
	var operation byte
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "save":
		operation = 0x01
	case "recall":
		operation = 0x02
	default:
		return errors.New("ação de preset inválida")
	}
	return c.send(ctx, host, port, []byte{0x81, 0x01, 0x04, 0x3f, operation, byte(number), 0xff})
}

func (c *Client) send(ctx context.Context, host string, port int, command []byte) error {
	host = strings.TrimSpace(host)
	if net.ParseIP(host) == nil {
		return errors.New("informe um endereço IP válido para a câmera")
	}
	if port < 1 || port > 65535 {
		return errors.New("a porta da câmera deve estar entre 1 e 65535")
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 2 * time.Second}
	connection, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return fmt.Errorf("conectar à câmera PTZ: %w", err)
	}
	defer connection.Close()

	packet := buildPacket(command, c.sequence.Add(1))
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetWriteDeadline(deadline)
	} else {
		_ = connection.SetWriteDeadline(time.Now().Add(2 * time.Second))
	}
	if _, err := connection.Write(packet); err != nil {
		return fmt.Errorf("enviar comando VISCA: %w", err)
	}
	return nil
}

func buildPacket(command []byte, sequence uint32) []byte {
	packet := make([]byte, 8+len(command))
	packet[0], packet[1] = 0x01, 0x00
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(command)))
	binary.BigEndian.PutUint32(packet[4:8], sequence)
	copy(packet[8:], command)
	return packet
}

func directionBytes(direction string) (byte, byte, error) {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "up":
		return 0x03, 0x01, nil
	case "down":
		return 0x03, 0x02, nil
	case "left":
		return 0x01, 0x03, nil
	case "right":
		return 0x02, 0x03, nil
	case "up-left":
		return 0x01, 0x01, nil
	case "up-right":
		return 0x02, 0x01, nil
	case "down-left":
		return 0x01, 0x02, nil
	case "down-right":
		return 0x02, 0x02, nil
	case "stop":
		return 0x03, 0x03, nil
	default:
		return 0, 0, errors.New("direção PTZ inválida")
	}
}

func speedByte(speed int) (byte, error) {
	if speed < 1 || speed > 24 {
		return 0, errors.New("a velocidade de movimento deve estar entre 1 e 24")
	}
	return byte(speed), nil
}
