package observability

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
)

func TestConfigFromEnvOTLPTLS(t *testing.T) {
	for value, want := range map[string]bool{
		"":      false, // unset keeps the plaintext exporter
		"true":  false,
		"FALSE": true,
		"false": true,
		"no":    false,
	} {
		t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", value)
		if got := ConfigFromEnv(StageLocal, "", nil).OTLPTLS; got != want {
			t.Fatalf("OTEL_EXPORTER_OTLP_INSECURE=%q: OTLPTLS = %v, want %v", value, got, want)
		}
	}
}

// firstExportBytes runs InitTracing against a raw TCP listener, exports one
// span and returns the first bytes the exporter wrote on the connection.
func firstExportBytes(t *testing.T, tlsOn bool) []byte {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	got := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			got <- nil
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 3)
		if _, err := io.ReadFull(conn, buf); err != nil {
			got <- nil
			return
		}
		got <- buf
	}()

	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	shutdown := InitTracing(Config{TracingEnabled: true, OTLPEndpoint: ln.Addr().String(), OTLPTLS: tlsOn}, nil)
	_, span := otel.Tracer("test").Start(context.Background(), "probe")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = shutdown(ctx) // the listener never answers; only the first bytes matter

	select {
	case b := <-got:
		return b
	case <-time.After(5 * time.Second):
		t.Fatal("exporter never connected")
		return nil
	}
}

// The exporter speaks TLS only when OTLPTLS is set: a TLS record starts with
// 0x16 0x03 (handshake, TLS major version); plaintext gRPC starts with the
// HTTP/2 client preface "PRI".
func TestInitTracingTransport(t *testing.T) {
	if b := firstExportBytes(t, true); len(b) != 3 || b[0] != 0x16 || b[1] != 0x03 {
		t.Fatalf("OTLPTLS=true: first bytes %q, want a TLS handshake record", b)
	}
	if b := firstExportBytes(t, false); string(b) != "PRI" {
		t.Fatalf("OTLPTLS=false: first bytes %q, want the HTTP/2 preface", b)
	}
}
