package ptz

import (
	"context"
	"encoding/binary"
	"testing"
)

func TestBuildVISCAOverIPPacket(t *testing.T) {
	command := []byte{0x81, 0x01, 0x06, 0x01, 0x06, 0x06, 0x01, 0x03, 0xff}
	packet := buildPacket(command, 7)
	if packet[0] != 0x01 || packet[1] != 0x00 || binary.BigEndian.Uint16(packet[2:4]) != 9 {
		t.Fatalf("cabeçalho VISCA over IP inválido: %x", packet)
	}
	if binary.BigEndian.Uint32(packet[4:8]) != 7 {
		t.Fatalf("sequência inválida: %x", packet[4:8])
	}
	for index := range command {
		if packet[index+8] != command[index] {
			t.Fatalf("comando inesperado: %x", packet[8:])
		}
	}
}

func TestRejectsHostnameAndInvalidSpeed(t *testing.T) {
	client := NewClient()
	if err := client.Move(context.Background(), "camera.local", DefaultPort, "left", 6); err == nil {
		t.Fatal("esperava rejeitar hostname")
	}
	if err := client.Zoom(context.Background(), "127.0.0.1", DefaultPort, "in", 8); err == nil {
		t.Fatal("esperava rejeitar velocidade")
	}
}
