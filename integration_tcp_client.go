package semanticrouter

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

type ModelSelectorTCPClient struct {
	Address        string
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
	MaxFrameBytes  int
}

func NewModelSelectorTCPClient(config IntegrationConfig) *ModelSelectorTCPClient {
	return &ModelSelectorTCPClient{Address: config.ServiceAddress, ConnectTimeout: config.ConnectTimeout, RequestTimeout: config.RequestTimeout, MaxFrameBytes: config.MaxFrameBytes}
}
func (c *ModelSelectorTCPClient) Select(ctx context.Context, request *ModelSelectionRequest) (*ModelSelectionResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("client is nil")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := (&net.Dialer{Timeout: c.ConnectTimeout}).DialContext(ctx, "tcp", c.Address)
	if err != nil {
		return nil, fmt.Errorf("connect model selector: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(c.RequestTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if err := writeIntegrationFrame(conn, request, c.MaxFrameBytes); err != nil {
		return nil, fmt.Errorf("write selection request: %w", err)
	}
	response, err := readIntegrationResponse(conn, c.MaxFrameBytes)
	if err != nil {
		return nil, err
	}
	if !response.Success {
		return response, fmt.Errorf("model selector rejected request: %s", response.Error)
	}
	return response, nil
}
func readIntegrationResponse(conn net.Conn, maxBytes int) (*ModelSelectionResponse, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, fmt.Errorf("read response header: %w", err)
	}
	length := int(binary.BigEndian.Uint32(header[:]))
	if length <= 0 || length > maxBytes {
		return nil, fmt.Errorf("invalid response frame length %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	var response ModelSelectionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &response, nil
}
