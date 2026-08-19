package semanticrouter

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type ModelSelectorTCPServer struct {
	listener net.Listener
	service  *ModelSelectionService
	config   IntegrationConfig
	closed   chan struct{}
	wg       sync.WaitGroup
}

func NewModelSelectorTCPServer(service *ModelSelectionService, config IntegrationConfig) (*ModelSelectorTCPServer, error) {
	if service == nil {
		return nil, fmt.Errorf("selection service is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &ModelSelectorTCPServer{service: service, config: config, closed: make(chan struct{})}, nil
}
func (s *ModelSelectorTCPServer) Start() error {
	if s.listener != nil {
		return fmt.Errorf("server already started")
	}
	listener, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return err
	}
	s.listener = listener
	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}
func (s *ModelSelectorTCPServer) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}
func (s *ModelSelectorTCPServer) Close() error {
	select {
	case <-s.closed:
		return nil
	default:
		close(s.closed)
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.wg.Wait()
	return nil
}
func (s *ModelSelectorTCPServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go func() { defer s.wg.Done(); s.handleConn(conn) }()
	}
}
func (s *ModelSelectorTCPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.config.RequestTimeout))
	request, err := readIntegrationFrame(conn, s.config.MaxFrameBytes)
	if err != nil {
		_ = writeIntegrationFrame(conn, &ModelSelectionResponse{ProtocolVersion: ModelSelectorProtocolVersion, DryRun: true, ShadowOnly: true, Error: err.Error()}, s.config.MaxFrameBytes)
		return
	}
	_ = writeIntegrationFrame(conn, s.service.Select(context.Background(), request), s.config.MaxFrameBytes)
}

func readIntegrationFrame(r io.Reader, maxBytes int) (*ModelSelectionRequest, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("read frame header: %w", err)
	}
	length := int(binary.BigEndian.Uint32(header[:]))
	if length <= 0 || length > maxBytes {
		return nil, fmt.Errorf("invalid frame length %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read frame body: %w", err)
	}
	var request ModelSelectionRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}
	return &request, nil
}
func writeIntegrationFrame(w io.Writer, value interface{}, maxBytes int) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(body) == 0 || len(body) > maxBytes {
		return fmt.Errorf("response frame exceeds limit")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))
	if _, err = w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}
